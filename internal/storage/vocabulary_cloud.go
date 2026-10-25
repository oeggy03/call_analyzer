package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oeggy03/call_analyzer/internal/domain"
)

// CloudVocabularyApplyResult counts local rows changed by a cloud vocabulary pull.
type CloudVocabularyApplyResult struct {
	Inserted int
	Updated  int
	Removed  int
}

// Changes is the number of local vocabulary rows a pull inserted, updated, or removed.
func (r CloudVocabularyApplyResult) Changes() int {
	return r.Inserted + r.Updated + r.Removed
}

type cloudApplyOutcome int

const (
	cloudUnchanged cloudApplyOutcome = iota
	cloudInserted
	cloudUpdated
)

// ApplyCloudVocabulary copies confirmed cloud rows into SQLite without writing
// outbox events. A newer local updated_at is left in place. A confirmed local
// row that is absent from the cloud is removed only when it has no pending
// vocabulary outbox row.
func (r *VocabularyRepository) ApplyCloudVocabulary(
	ctx context.Context,
	entries []domain.VocabularyEntry,
) (CloudVocabularyApplyResult, error) {
	var result CloudVocabularyApplyResult
	if r == nil || r.store == nil {
		return result, errors.New("vocabulary: storage is not configured")
	}
	kept := make(map[string]struct{}, len(entries))
	remoteIDs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Status != domain.VocabularyStatusConfirmed {
			continue
		}
		entry.ID = strings.TrimSpace(entry.ID)
		entry.Simplified = strings.TrimSpace(entry.Simplified)
		if entry.ID == "" || entry.Simplified == "" {
			continue
		}
		if strings.TrimSpace(entry.Traditional) == "" {
			entry.Traditional = entry.Simplified
		}
		remoteIDs[entry.ID] = struct{}{}
		outcome, localID, err := r.applyOneCloudEntry(ctx, entry)
		if err != nil {
			return result, err
		}
		if localID != "" {
			kept[localID] = struct{}{}
		}
		switch outcome {
		case cloudInserted:
			result.Inserted++
		case cloudUpdated:
			result.Updated++
		}
	}
	removed, err := r.removeConfirmedMissingFromCloud(ctx, remoteIDs, kept)
	if err != nil {
		return result, err
	}
	result.Removed = removed
	return result, nil
}

func (r *VocabularyRepository) applyOneCloudEntry(
	ctx context.Context,
	entry domain.VocabularyEntry,
) (cloudApplyOutcome, string, error) {
	outcome := cloudUnchanged
	localID := ""
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		existingID, existingUpdated, found, err := lookupVocabularyID(ctx, tx, `
			SELECT id, updated_at FROM vocabulary_entries WHERE id = ?`, entry.ID)
		if err != nil {
			return err
		}
		if found {
			localID = existingID
			if !cloudIsNewer(entry.UpdatedAt, existingUpdated) {
				return nil
			}
			if err := writeCloudEntry(ctx, tx, existingID, entry, true); err != nil {
				return err
			}
			if err := ackPendingVocabulary(ctx, tx, existingID); err != nil {
				return err
			}
			outcome = cloudUpdated
			return nil
		}

		keyID, keyUpdated, keyFound, err := lookupVocabularyID(ctx, tx, `
			SELECT id, updated_at FROM vocabulary_entries
			WHERE simplified = ? AND traditional = ? AND reading = ?`,
			entry.Simplified, entry.Traditional, entry.Reading)
		if err != nil {
			return err
		}
		if keyFound {
			localID = keyID
			if !cloudIsNewer(entry.UpdatedAt, keyUpdated) {
				return nil
			}
			if err := writeCloudEntry(ctx, tx, keyID, entry, true); err != nil {
				return err
			}
			if err := ackPendingVocabulary(ctx, tx, keyID); err != nil {
				return err
			}
			outcome = cloudUpdated
			return nil
		}

		if err := writeCloudEntry(ctx, tx, entry.ID, entry, false); err != nil {
			return err
		}
		localID = entry.ID
		outcome = cloudInserted
		return nil
	})
	if err != nil {
		return cloudUnchanged, "", err
	}
	return outcome, localID, nil
}

func (r *VocabularyRepository) removeConfirmedMissingFromCloud(
	ctx context.Context,
	remoteIDs map[string]struct{},
	kept map[string]struct{},
) (int, error) {
	removed := 0
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id FROM vocabulary_entries WHERE status = ?`,
			domain.VocabularyStatusConfirmed,
		)
		if err != nil {
			return fmt.Errorf("vocabulary: list confirmed ids: %w", err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return fmt.Errorf("vocabulary: scan confirmed id: %w", err)
			}
			ids = append(ids, id)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("vocabulary: close confirmed ids: %w", err)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("vocabulary: read confirmed ids: %w", err)
		}
		for _, id := range ids {
			if _, ok := kept[id]; ok {
				continue
			}
			if _, ok := remoteIDs[id]; ok {
				continue
			}
			pending, err := vocabularyOutboxPending(ctx, tx, id)
			if err != nil {
				return err
			}
			if pending {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM vocabulary_entries WHERE id = ? AND status = ?`,
				id, domain.VocabularyStatusConfirmed,
			); err != nil {
				return fmt.Errorf("vocabulary: remove cloud-missing entry %s: %w", id, err)
			}
			removed++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}

func lookupVocabularyID(
	ctx context.Context,
	tx *sql.Tx,
	query string,
	args ...any,
) (string, time.Time, bool, error) {
	var id string
	var updated int64
	err := tx.QueryRowContext(ctx, query, args...).Scan(&id, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("vocabulary: lookup cloud entry: %w", err)
	}
	return id, timeFromValue(updated), true, nil
}

func cloudIsNewer(remote, local time.Time) bool {
	return remote.UTC().UnixMilli() > local.UTC().UnixMilli()
}

func writeCloudEntry(
	ctx context.Context,
	tx *sql.Tx,
	localID string,
	entry domain.VocabularyEntry,
	update bool,
) error {
	provenance := string(entry.Provenance)
	if strings.TrimSpace(provenance) == "" {
		provenance = string(domain.ProvenanceImported)
	}
	mergedInto := any(nil)
	if id := strings.TrimSpace(entry.MergedIntoID); id != "" && id != localID {
		var exists string
		err := tx.QueryRowContext(ctx, `SELECT id FROM vocabulary_entries WHERE id = ?`, id).Scan(&exists)
		if err == nil {
			mergedInto = id
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("vocabulary: resolve merged target: %w", err)
		}
	}
	if update {
		if _, err := tx.ExecContext(ctx, `
			UPDATE vocabulary_entries
			SET simplified = ?, traditional = ?, reading = ?, marked_pinyin = ?,
				status = 'confirmed', provenance = ?, confidence = ?, teaching_cue = ?,
				manual = ?, priority = ?, merged_into_id = ?, created_at = ?, updated_at = ?
			WHERE id = ?`,
			entry.Simplified, entry.Traditional, entry.Reading, entry.MarkedPinyin,
			provenance, entry.Confidence, entry.TeachingCue, entry.Manual, entry.Priority,
			mergedInto, timeValue(entry.CreatedAt), timeValue(entry.UpdatedAt), localID,
		); err != nil {
			return fmt.Errorf("vocabulary: update cloud entry %s: %w", localID, err)
		}
	} else if _, err := tx.ExecContext(ctx, `
		INSERT INTO vocabulary_entries(
			id, simplified, traditional, reading, marked_pinyin, status,
			provenance, confidence, teaching_cue, manual, priority, merged_into_id,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'confirmed', ?, ?, ?, ?, ?, ?, ?, ?)`,
		localID, entry.Simplified, entry.Traditional, entry.Reading, entry.MarkedPinyin,
		provenance, entry.Confidence, entry.TeachingCue, entry.Manual, entry.Priority,
		mergedInto, timeValue(entry.CreatedAt), timeValue(entry.UpdatedAt),
	); err != nil {
		return fmt.Errorf("vocabulary: insert cloud entry %s: %w", localID, err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM senses WHERE entry_id = ?`, localID); err != nil {
		return fmt.Errorf("vocabulary: replace cloud senses: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM examples WHERE entry_id = ?`, localID); err != nil {
		return fmt.Errorf("vocabulary: replace cloud examples: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entry_tags WHERE entry_id = ?`, localID); err != nil {
		return fmt.Errorf("vocabulary: replace cloud tags: %w", err)
	}
	for index, sense := range entry.Senses {
		senseID := strings.TrimSpace(sense.ID)
		if senseID == "" {
			senseID = newID()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO senses(id, entry_id, gloss, part_of_speech, classifier, sort_order)
			VALUES (?, ?, ?, ?, ?, ?)`,
			senseID, localID, sense.Gloss, sense.PartOfSpeech, sense.Classifier, sense.SortOrder,
		); err != nil {
			return fmt.Errorf("vocabulary: insert cloud sense %d: %w", index+1, err)
		}
	}
	for index, example := range entry.Examples {
		exampleID := strings.TrimSpace(example.ID)
		if exampleID == "" {
			exampleID = newID()
		}
		provenance := string(example.Provenance)
		if strings.TrimSpace(provenance) == "" {
			provenance = string(domain.ProvenanceImported)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO examples(
				id, entry_id, simplified, traditional, reading, translation, provenance, generated
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			exampleID, localID, example.Simplified, example.Traditional, example.Reading,
			example.Translation, provenance, example.Generated,
		); err != nil {
			return fmt.Errorf("vocabulary: insert cloud example %d: %w", index+1, err)
		}
	}
	for _, tag := range entry.Tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		tagID := strings.TrimSpace(tag.ID)
		if tagID == "" {
			tagID = newID()
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tags(id, name) VALUES (?, ?) ON CONFLICT(name) DO NOTHING`,
			tagID, name,
		); err != nil {
			return fmt.Errorf("vocabulary: insert cloud tag %s: %w", name, err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, name).Scan(&tagID); err != nil {
			return fmt.Errorf("vocabulary: resolve cloud tag %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO entry_tags(entry_id, tag_id) VALUES (?, ?)`,
			localID, tagID,
		); err != nil {
			return fmt.Errorf("vocabulary: link cloud tag %s: %w", name, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO study_state(entry_id) VALUES (?)
		ON CONFLICT(entry_id) DO NOTHING`, localID); err != nil {
		return fmt.Errorf("vocabulary: initialize cloud study state: %w", err)
	}
	return nil
}

func vocabularyOutboxPending(ctx context.Context, tx *sql.Tx, entryID string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM outbox
		WHERE delivered_at IS NULL AND aggregate_type = 'vocabulary_entry' AND aggregate_id = ?`,
		entryID,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("vocabulary: check pending outbox: %w", err)
	}
	return count > 0, nil
}

func ackPendingVocabulary(ctx context.Context, tx *sql.Tx, entryID string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE outbox SET delivered_at = ?, last_error = ''
		WHERE delivered_at IS NULL AND aggregate_type = 'vocabulary_entry' AND aggregate_id = ?`,
		timeValue(time.Now().UTC()), entryID,
	); err != nil {
		return fmt.Errorf("vocabulary: acknowledge cloud outbox: %w", err)
	}
	return nil
}
