package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/vocabulary"
)

type VocabularyRepository struct {
	store *Store
}

func (s *Store) Vocabulary() *VocabularyRepository {
	return &VocabularyRepository{store: s}
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *VocabularyRepository) UpsertCandidate(ctx context.Context, candidate domain.VocabularyEntry) (domain.VocabularyEntry, error) {
	if strings.TrimSpace(candidate.Simplified) == "" {
		return domain.VocabularyEntry{}, errors.New("vocabulary: simplified form is required")
	}
	if strings.TrimSpace(candidate.Traditional) == "" {
		candidate.Traditional = candidate.Simplified
	}
	if strings.TrimSpace(candidate.Reading) == "" {
		return domain.VocabularyEntry{}, errors.New("vocabulary: reading is required")
	}
	numberedReading, markedPinyin, err := vocabulary.CanonicalPinyin(candidate.Reading)
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	candidate.Reading = numberedReading
	if candidate.MarkedPinyin == "" {
		candidate.MarkedPinyin = markedPinyin
	}
	candidate.ID = strings.TrimSpace(candidate.ID)
	candidate.Status = domain.VocabularyStatusCandidate
	if candidate.Provenance == "" {
		candidate.Provenance = domain.ProvenanceModel
	}
	now := time.Now().UTC()
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = now
	}
	candidate.UpdatedAt = now
	if candidate.ID == "" {
		candidate.ID = newID()
	}

	err = r.store.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO vocabulary_entries(
				id, simplified, traditional, reading, marked_pinyin, status,
				provenance, confidence, teaching_cue, manual, priority, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, 'candidate', ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(simplified, traditional, reading) DO UPDATE SET
				marked_pinyin = CASE
					WHEN excluded.marked_pinyin <> '' THEN excluded.marked_pinyin
					ELSE vocabulary_entries.marked_pinyin
				END,
				provenance = CASE
					WHEN vocabulary_entries.status = 'confirmed'
						THEN vocabulary_entries.provenance
					ELSE excluded.provenance
				END,
				confidence = MAX(vocabulary_entries.confidence, excluded.confidence),
				teaching_cue = MAX(vocabulary_entries.teaching_cue, excluded.teaching_cue),
				manual = MAX(vocabulary_entries.manual, excluded.manual),
				priority = CASE
					WHEN excluded.priority <> '' THEN excluded.priority
					ELSE vocabulary_entries.priority
				END,
				status = CASE
					WHEN vocabulary_entries.status IN ('confirmed', 'rejected', 'merged')
						THEN vocabulary_entries.status
					ELSE 'candidate'
				END,
				updated_at = excluded.updated_at`,
			candidate.ID, candidate.Simplified, candidate.Traditional, candidate.Reading,
			candidate.MarkedPinyin, candidate.Provenance, candidate.Confidence,
			candidate.TeachingCue, candidate.Manual, candidate.Priority,
			timeValue(candidate.CreatedAt), timeValue(candidate.UpdatedAt),
		)
		if err != nil {
			return fmt.Errorf("vocabulary: upsert candidate: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT id FROM vocabulary_entries
			WHERE simplified = ? AND traditional = ? AND reading = ?`,
			candidate.Simplified, candidate.Traditional, candidate.Reading,
		).Scan(&candidate.ID); err != nil {
			return fmt.Errorf("vocabulary: resolve candidate: %w", err)
		}

		for i := range candidate.Senses {
			sense := candidate.Senses[i]
			if sense.ID == "" {
				sense.ID = newID()
			}
			sense.EntryID = candidate.ID
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO senses(id, entry_id, gloss, part_of_speech, classifier, sort_order)
				SELECT ?, ?, ?, ?, ?, ?
				WHERE NOT EXISTS (
					SELECT 1 FROM senses
					WHERE entry_id = ? AND gloss = ? AND part_of_speech = ? AND classifier = ?
				)`,
				sense.ID, sense.EntryID, sense.Gloss, sense.PartOfSpeech, sense.Classifier, sense.SortOrder,
				sense.EntryID, sense.Gloss, sense.PartOfSpeech, sense.Classifier,
			); err != nil {
				return fmt.Errorf("vocabulary: insert sense: %w", err)
			}
		}
		for i := range candidate.Examples {
			example := candidate.Examples[i]
			if example.ID == "" {
				example.ID = newID()
			}
			example.EntryID = candidate.ID
			if example.Generated {
				example.Provenance = domain.ProvenanceModel
			}
			if example.Provenance == "" {
				example.Provenance = domain.ProvenanceUser
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO examples(
					id, entry_id, simplified, traditional, reading, translation,
					provenance, generated
				)
				SELECT ?, ?, ?, ?, ?, ?, ?, ?
				WHERE NOT EXISTS (
					SELECT 1 FROM examples
					WHERE entry_id = ? AND simplified = ? AND traditional = ?
						AND reading = ? AND translation = ?
				)`,
				example.ID, example.EntryID, example.Simplified, example.Traditional,
				example.Reading, example.Translation, example.Provenance, example.Generated,
				example.EntryID, example.Simplified, example.Traditional,
				example.Reading, example.Translation,
			); err != nil {
				return fmt.Errorf("vocabulary: insert example: %w", err)
			}
		}
		for _, evidence := range candidate.Evidence {
			if strings.TrimSpace(evidence.Text) == "" || evidence.LessonID == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO observations(
					id, lesson_id, segment_id, entry_id, evidence_text,
					start_ms, end_ms, confidence, created_at
				)
				SELECT ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?
				WHERE NOT EXISTS (
					SELECT 1 FROM observations
					WHERE lesson_id = ? AND entry_id = ? AND evidence_text = ?
						AND start_ms = ? AND end_ms = ?
				)`,
				newID(), evidence.LessonID, evidence.SegmentID, candidate.ID,
				evidence.Text, evidence.StartMS, evidence.EndMS, evidence.Confidence,
				timeValue(now),
				evidence.LessonID, candidate.ID, evidence.Text, evidence.StartMS, evidence.EndMS,
			); err != nil {
				return fmt.Errorf("vocabulary: insert evidence: %w", err)
			}
		}
		payload, err := json.Marshal(candidate)
		if err != nil {
			return fmt.Errorf("vocabulary: marshal candidate event: %w", err)
		}
		if err := enqueueOutboxTx(ctx, tx, domain.OutboxEvent{
			EventType:      "vocabulary.candidate.upserted",
			AggregateType:  "vocabulary_entry",
			AggregateID:    candidate.ID,
			PayloadJSON:    string(payload),
			IdempotencyKey: "vocabulary:candidate:" + candidate.ID + ":" + fmt.Sprint(timeValue(now)),
			CreatedAt:      now,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	return r.Get(ctx, candidate.ID)
}

// SaveManual upserts an explicitly reviewed vocabulary entry as confirmed.
// It is intentionally separate from UpsertCandidate: automatic extraction
// must never reopen a rejected entry, while an explicit manual save is allowed
// to restore that same Chinese-aware key.
func (r *VocabularyRepository) SaveManual(ctx context.Context, entry domain.VocabularyEntry) (domain.VocabularyEntry, error) {
	if strings.TrimSpace(entry.Simplified) == "" {
		return domain.VocabularyEntry{}, errors.New("vocabulary: simplified form is required")
	}
	if strings.TrimSpace(entry.Traditional) == "" {
		entry.Traditional = entry.Simplified
	}
	if strings.TrimSpace(entry.Reading) == "" {
		return domain.VocabularyEntry{}, errors.New("vocabulary: reading is required")
	}
	numberedReading, markedPinyin, err := vocabulary.CanonicalPinyin(entry.Reading)
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	entry.Reading = numberedReading
	entry.MarkedPinyin = markedPinyin
	if len(entry.Senses) == 0 || strings.TrimSpace(entry.Senses[0].Gloss) == "" {
		return domain.VocabularyEntry{}, errors.New("vocabulary: manual entry meaning is required")
	}
	if len(entry.Examples) == 0 {
		return domain.VocabularyEntry{}, errors.New("vocabulary: manual entry example is required")
	}
	for index := range entry.Examples {
		example := &entry.Examples[index]
		if strings.TrimSpace(example.Simplified) == "" ||
			strings.TrimSpace(example.Reading) == "" ||
			strings.TrimSpace(example.Translation) == "" {
			return domain.VocabularyEntry{}, fmt.Errorf(
				"vocabulary: manual example %d requires sentence, pinyin, and translation",
				index+1,
			)
		}
		example.Reading, _, err = vocabulary.CanonicalPinyin(example.Reading)
		if err != nil {
			return domain.VocabularyEntry{}, fmt.Errorf("vocabulary: example pinyin: %w", err)
		}
		if example.Generated {
			example.Provenance = domain.ProvenanceModel
		} else {
			example.Provenance = domain.ProvenanceUser
		}
	}

	now := time.Now().UTC()
	entry.Status = domain.VocabularyStatusConfirmed
	entry.Provenance = domain.ProvenanceUser
	entry.Manual = true
	entry.Priority = "manual"
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	if entry.ID == "" {
		entry.ID = newID()
	}

	err = r.store.withTx(ctx, func(tx *sql.Tx) error {
		var existingID string
		var existingStatus domain.VocabularyStatus
		lookupErr := tx.QueryRowContext(ctx, `
			SELECT id, status FROM vocabulary_entries
			WHERE simplified = ? AND traditional = ? AND reading = ?`,
			entry.Simplified, entry.Traditional, entry.Reading,
		).Scan(&existingID, &existingStatus)
		switch {
		case errors.Is(lookupErr, sql.ErrNoRows):
			_, err := tx.ExecContext(ctx, `
				INSERT INTO vocabulary_entries(
					id, simplified, traditional, reading, marked_pinyin, status,
					provenance, confidence, teaching_cue, manual, priority,
					created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, 'confirmed', ?, ?, ?, 1, 'manual', ?, ?)`,
				entry.ID, entry.Simplified, entry.Traditional, entry.Reading,
				entry.MarkedPinyin, entry.Provenance, entry.Confidence,
				entry.TeachingCue, timeValue(entry.CreatedAt), timeValue(entry.UpdatedAt),
			)
			if err != nil {
				return fmt.Errorf("vocabulary: save manual entry: %w", err)
			}
		case lookupErr != nil:
			return fmt.Errorf("vocabulary: resolve manual entry: %w", lookupErr)
		default:
			if existingStatus == domain.VocabularyStatusMerged {
				return fmt.Errorf("vocabulary: cannot restore merged entry %q", existingID)
			}
			entry.ID = existingID
			if existingStatus == domain.VocabularyStatusConfirmed {
				existing, err := scanVocabularyEntry(ctx, tx, existingID)
				if err != nil {
					return fmt.Errorf("vocabulary: load existing manual entry: %w", err)
				}
				if manualEntriesEqual(existing, entry) {
					if _, err := tx.ExecContext(ctx, `
						INSERT INTO study_state(entry_id) VALUES (?)
						ON CONFLICT(entry_id) DO NOTHING`, entry.ID); err != nil {
						return fmt.Errorf("vocabulary: initialize manual study state: %w", err)
					}
					return nil
				}
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE vocabulary_entries
				SET simplified = ?, traditional = ?, reading = ?, marked_pinyin = ?,
					status = 'confirmed', provenance = ?, confidence = ?,
					teaching_cue = ?, manual = 1, priority = 'manual',
					merged_into_id = NULL, updated_at = ?
				WHERE id = ?`,
				entry.Simplified, entry.Traditional, entry.Reading, entry.MarkedPinyin,
				entry.Provenance, entry.Confidence, entry.TeachingCue,
				timeValue(entry.UpdatedAt), entry.ID,
			); err != nil {
				return fmt.Errorf("vocabulary: restore manual entry: %w", err)
			}
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM senses WHERE entry_id = ?", entry.ID); err != nil {
			return fmt.Errorf("vocabulary: replace manual senses: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM examples WHERE entry_id = ?", entry.ID); err != nil {
			return fmt.Errorf("vocabulary: replace manual examples: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM entry_tags WHERE entry_id = ?", entry.ID); err != nil {
			return fmt.Errorf("vocabulary: replace manual tags: %w", err)
		}

		for index := range entry.Senses {
			sense := entry.Senses[index]
			if sense.ID == "" {
				sense.ID = newID()
			}
			sense.EntryID = entry.ID
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO senses(id, entry_id, gloss, part_of_speech, classifier, sort_order)
				VALUES (?, ?, ?, ?, ?, ?)`,
				sense.ID, sense.EntryID, sense.Gloss, sense.PartOfSpeech,
				sense.Classifier, sense.SortOrder,
			); err != nil {
				return fmt.Errorf("vocabulary: save manual sense: %w", err)
			}
		}
		for index := range entry.Examples {
			example := entry.Examples[index]
			if example.ID == "" {
				example.ID = newID()
			}
			example.EntryID = entry.ID
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO examples(
					id, entry_id, simplified, traditional, reading, translation,
					provenance, generated
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				example.ID, example.EntryID, example.Simplified, example.Traditional,
				example.Reading, example.Translation, example.Provenance, example.Generated,
			); err != nil {
				return fmt.Errorf("vocabulary: save manual example: %w", err)
			}
		}
		for _, tag := range entry.Tags {
			tagName := strings.TrimSpace(tag.Name)
			if tagName == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO tags(id, name) VALUES (?, ?) ON CONFLICT(name) DO NOTHING",
				newID(), tagName,
			); err != nil {
				return fmt.Errorf("vocabulary: save manual tag: %w", err)
			}
			var tagID string
			if err := tx.QueryRowContext(ctx, "SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID); err != nil {
				return fmt.Errorf("vocabulary: resolve manual tag: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT OR IGNORE INTO entry_tags(entry_id, tag_id) VALUES (?, ?)",
				entry.ID, tagID,
			); err != nil {
				return fmt.Errorf("vocabulary: link manual tag: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO study_state(entry_id) VALUES (?)
			ON CONFLICT(entry_id) DO NOTHING`, entry.ID); err != nil {
			return fmt.Errorf("vocabulary: initialize manual study state: %w", err)
		}

		saved, err := scanVocabularyEntry(ctx, tx, entry.ID)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(saved)
		if err != nil {
			return fmt.Errorf("vocabulary: marshal manual save event: %w", err)
		}
		return enqueueOutboxTx(ctx, tx, domain.OutboxEvent{
			EventType:      "vocabulary.manual.saved",
			AggregateType:  "vocabulary_entry",
			AggregateID:    entry.ID,
			PayloadJSON:    string(payload),
			IdempotencyKey: "vocabulary:manual:" + entry.ID + ":" + fmt.Sprint(timeValue(now)),
			CreatedAt:      now,
		})
	})
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	return r.Get(ctx, entry.ID)
}

func manualEntriesEqual(existing, desired domain.VocabularyEntry) bool {
	if existing.Status != domain.VocabularyStatusConfirmed ||
		existing.Simplified != desired.Simplified ||
		existing.Traditional != desired.Traditional ||
		existing.Reading != desired.Reading ||
		existing.MarkedPinyin != desired.MarkedPinyin ||
		existing.Provenance != domain.ProvenanceUser ||
		existing.Manual != desired.Manual ||
		existing.Priority != desired.Priority ||
		existing.Confidence != desired.Confidence ||
		existing.TeachingCue != desired.TeachingCue ||
		len(existing.Senses) != len(desired.Senses) ||
		len(existing.Examples) != len(desired.Examples) {
		return false
	}
	for index := range desired.Senses {
		got, want := existing.Senses[index], desired.Senses[index]
		if got.Gloss != want.Gloss ||
			got.PartOfSpeech != want.PartOfSpeech ||
			got.Classifier != want.Classifier ||
			got.SortOrder != want.SortOrder {
			return false
		}
	}
	for index := range desired.Examples {
		got, want := existing.Examples[index], desired.Examples[index]
		if got.Simplified != want.Simplified ||
			got.Traditional != want.Traditional ||
			got.Reading != want.Reading ||
			got.Translation != want.Translation ||
			got.Provenance != want.Provenance ||
			got.Generated != want.Generated {
			return false
		}
	}
	return manualTagNamesEqual(existing.Tags, desired.Tags)
}

func manualTagNamesEqual(existing, desired []domain.Tag) bool {
	counts := make(map[string]int, len(existing))
	for _, tag := range existing {
		counts[tag.Name]++
	}
	for _, tag := range desired {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		counts[name]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func (r *VocabularyRepository) Get(ctx context.Context, id string) (domain.VocabularyEntry, error) {
	var entry domain.VocabularyEntry
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		entry, err = scanVocabularyEntry(ctx, tx, id)
		return err
	})
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	return entry, nil
}

func (r *VocabularyRepository) ListCandidates(ctx context.Context, limit int) ([]domain.VocabularyEntry, error) {
	return r.listByStatus(ctx, domain.VocabularyStatusCandidate, limit)
}

func (r *VocabularyRepository) ListVocabulary(ctx context.Context, limit int) ([]domain.VocabularyEntry, error) {
	return r.listByStatus(ctx, domain.VocabularyStatusConfirmed, limit)
}

func (r *VocabularyRepository) ObservationStats(
	ctx context.Context,
	entryID string,
) (int, time.Time, error) {
	var count int
	var latest sql.NullInt64
	err := r.store.db.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(created_at)
		FROM observations WHERE entry_id = ?`, entryID,
	).Scan(&count, &latest)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("vocabulary: observation stats: %w", err)
	}
	if !latest.Valid {
		return count, time.Time{}, nil
	}
	return count, timeFromValue(latest.Int64), nil
}

func (r *VocabularyRepository) ListAll(ctx context.Context, limit int) ([]domain.VocabularyEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id FROM vocabulary_entries
		ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("vocabulary: list all: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("vocabulary: list all scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vocabulary: list all rows: %w", err)
	}
	entries := make([]domain.VocabularyEntry, 0, len(ids))
	for _, id := range ids {
		entry, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (r *VocabularyRepository) listByStatus(ctx context.Context, status domain.VocabularyStatus, limit int) ([]domain.VocabularyEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id FROM vocabulary_entries
		WHERE status = ?
		ORDER BY updated_at DESC LIMIT ?`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("vocabulary: list %s: %w", status, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("vocabulary: list %s scan: %w", status, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vocabulary: list %s rows: %w", status, err)
	}
	entries := make([]domain.VocabularyEntry, 0, len(ids))
	for _, id := range ids {
		entry, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (r *VocabularyRepository) Confirm(ctx context.Context, id string) (domain.VocabularyEntry, error) {
	return r.setStatus(ctx, id, domain.VocabularyStatusConfirmed, "vocabulary.candidate.confirmed")
}

func (r *VocabularyRepository) Reject(ctx context.Context, id string) (domain.VocabularyEntry, error) {
	return r.setStatus(ctx, id, domain.VocabularyStatusRejected, "vocabulary.candidate.rejected")
}

func (r *VocabularyRepository) setStatus(ctx context.Context, id string, status domain.VocabularyStatus, eventType string) (domain.VocabularyEntry, error) {
	if id == "" {
		return domain.VocabularyEntry{}, errors.New("vocabulary: id is required")
	}
	now := time.Now().UTC()
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		var current string
		if err := tx.QueryRowContext(ctx,
			"SELECT status FROM vocabulary_entries WHERE id = ?", id,
		).Scan(&current); err != nil {
			return err
		}
		if current == string(status) {
			return nil
		}
		if current != string(domain.VocabularyStatusCandidate) {
			return fmt.Errorf("vocabulary: cannot %s entry in status %q", status, current)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE vocabulary_entries SET status = ?, updated_at = ? WHERE id = ?`,
			status, timeValue(now), id,
		)
		if err != nil {
			return fmt.Errorf("vocabulary: set status: %w", err)
		}
		if count, _ := result.RowsAffected(); count == 0 {
			return sql.ErrNoRows
		}
		if status == domain.VocabularyStatusConfirmed {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO study_state(entry_id) VALUES (?)
				ON CONFLICT(entry_id) DO NOTHING`, id); err != nil {
				return fmt.Errorf("vocabulary: initialize study state: %w", err)
			}
		}
		entry, err := scanVocabularyEntry(ctx, tx, id)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("vocabulary: marshal status event: %w", err)
		}
		return enqueueOutboxTx(ctx, tx, domain.OutboxEvent{
			EventType:      eventType,
			AggregateType:  "vocabulary_entry",
			AggregateID:    id,
			PayloadJSON:    string(payload),
			IdempotencyKey: eventType + ":" + id + ":" + fmt.Sprint(timeValue(now)),
			CreatedAt:      now,
		})
	})
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	return r.Get(ctx, id)
}

func (r *VocabularyRepository) Edit(ctx context.Context, id string, edit domain.CandidateEdit) (domain.VocabularyEntry, error) {
	if id == "" {
		return domain.VocabularyEntry{}, errors.New("vocabulary: id is required")
	}
	updates := make([]string, 0, 6)
	args := make([]any, 0, 7)
	if edit.Simplified != nil {
		if strings.TrimSpace(*edit.Simplified) == "" {
			return domain.VocabularyEntry{}, errors.New("vocabulary: simplified form cannot be empty")
		}
		updates = append(updates, "simplified = ?")
		args = append(args, *edit.Simplified)
	}
	if edit.Traditional != nil {
		if strings.TrimSpace(*edit.Traditional) == "" {
			return domain.VocabularyEntry{}, errors.New("vocabulary: traditional form cannot be empty")
		}
		updates = append(updates, "traditional = ?")
		args = append(args, *edit.Traditional)
	}
	if edit.Reading != nil {
		if strings.TrimSpace(*edit.Reading) == "" {
			return domain.VocabularyEntry{}, errors.New("vocabulary: reading cannot be empty")
		}
		numbered, marked, err := vocabulary.CanonicalPinyin(*edit.Reading)
		if err != nil {
			return domain.VocabularyEntry{}, err
		}
		reading := numbered
		edit.Reading = &reading
		if edit.MarkedPinyin == nil {
			edit.MarkedPinyin = &marked
		}
		updates = append(updates, "reading = ?")
		args = append(args, *edit.Reading)
	}
	if edit.MarkedPinyin != nil {
		updates = append(updates, "marked_pinyin = ?")
		args = append(args, *edit.MarkedPinyin)
	}
	if edit.Confidence != nil {
		updates = append(updates, "confidence = ?")
		args = append(args, *edit.Confidence)
	}
	if edit.TeachingCue != nil {
		updates = append(updates, "teaching_cue = ?")
		args = append(args, *edit.TeachingCue)
	}
	hasPresentation := edit.Meaning != nil || edit.PartOfSpeech != nil || edit.Classifier != nil ||
		edit.Example != nil || edit.ExamplePinyin != nil || edit.ExampleTranslation != nil || edit.Tags != nil
	if len(updates) == 0 && !hasPresentation {
		return r.Get(ctx, id)
	}

	now := time.Now().UTC()
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		if len(updates) > 0 {
			updates = append(updates, "updated_at = ?")
			args = append(args, timeValue(now), id)
			result, err := tx.ExecContext(ctx,
				"UPDATE vocabulary_entries SET "+strings.Join(updates, ", ")+" WHERE id = ? AND status = 'candidate'",
				args...,
			)
			if err != nil {
				return fmt.Errorf("vocabulary: edit: %w", err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("vocabulary: edit rows: %w", err)
			}
			if count == 0 {
				return sql.ErrNoRows
			}
		} else {
			var status string
			if err := tx.QueryRowContext(ctx, "SELECT status FROM vocabulary_entries WHERE id = ?", id).Scan(&status); err != nil {
				return err
			}
			if status != string(domain.VocabularyStatusCandidate) {
				return sql.ErrNoRows
			}
		}
		if edit.Meaning != nil || edit.PartOfSpeech != nil || edit.Classifier != nil {
			var senseID, gloss, partOfSpeech, classifier string
			err := tx.QueryRowContext(ctx, `
				SELECT id, gloss, part_of_speech, classifier FROM senses
				WHERE entry_id = ? ORDER BY sort_order, id LIMIT 1`, id,
			).Scan(&senseID, &gloss, &partOfSpeech, &classifier)
			if errors.Is(err, sql.ErrNoRows) {
				if edit.Meaning != nil {
					gloss = *edit.Meaning
				}
				if edit.PartOfSpeech != nil {
					partOfSpeech = *edit.PartOfSpeech
				}
				if edit.Classifier != nil {
					classifier = *edit.Classifier
				}
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO senses(id, entry_id, gloss, part_of_speech, classifier, sort_order)
					VALUES (?, ?, ?, ?, ?, 0)`,
					newID(), id, gloss, partOfSpeech, classifier,
				); err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				if edit.Meaning != nil {
					gloss = *edit.Meaning
				}
				if edit.PartOfSpeech != nil {
					partOfSpeech = *edit.PartOfSpeech
				}
				if edit.Classifier != nil {
					classifier = *edit.Classifier
				}
				if _, err := tx.ExecContext(ctx, `
					UPDATE senses SET gloss = ?, part_of_speech = ?, classifier = ? WHERE id = ?`,
					gloss, partOfSpeech, classifier, senseID,
				); err != nil {
					return err
				}
			}
		}
		if edit.Example != nil || edit.ExamplePinyin != nil || edit.ExampleTranslation != nil {
			var exampleID, exampleText, examplePinyin, translation string
			err := tx.QueryRowContext(ctx, `
				SELECT id, simplified, reading, translation FROM examples
				WHERE entry_id = ? ORDER BY id LIMIT 1`, id,
			).Scan(&exampleID, &exampleText, &examplePinyin, &translation)
			if errors.Is(err, sql.ErrNoRows) {
				if edit.Example != nil {
					exampleText = *edit.Example
				}
				if edit.ExamplePinyin != nil {
					examplePinyin = *edit.ExamplePinyin
				}
				if edit.ExampleTranslation != nil {
					translation = *edit.ExampleTranslation
				}
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO examples(id, entry_id, simplified, reading, translation, provenance, generated)
					VALUES (?, ?, ?, ?, ?, ?, 0)`,
					newID(), id, exampleText, examplePinyin, translation, domain.ProvenanceUser,
				); err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				if edit.Example != nil {
					exampleText = *edit.Example
				}
				if edit.ExamplePinyin != nil {
					examplePinyin = *edit.ExamplePinyin
				}
				if edit.ExampleTranslation != nil {
					translation = *edit.ExampleTranslation
				}
				if _, err := tx.ExecContext(ctx, `
					UPDATE examples SET simplified = ?, reading = ?, translation = ?, provenance = ?, generated = 0
					WHERE id = ?`,
					exampleText, examplePinyin, translation, domain.ProvenanceUser, exampleID,
				); err != nil {
					return err
				}
			}
		}
		if edit.Tags != nil {
			if _, err := tx.ExecContext(ctx, "DELETE FROM entry_tags WHERE entry_id = ?", id); err != nil {
				return err
			}
			for _, name := range *edit.Tags {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				if _, err := tx.ExecContext(ctx,
					"INSERT INTO tags(id, name) VALUES (?, ?) ON CONFLICT(name) DO NOTHING",
					newID(), name,
				); err != nil {
					return err
				}
				var tagID string
				if err := tx.QueryRowContext(ctx, "SELECT id FROM tags WHERE name = ?", name).Scan(&tagID); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx,
					"INSERT OR IGNORE INTO entry_tags(entry_id, tag_id) VALUES (?, ?)",
					id, tagID,
				); err != nil {
					return err
				}
			}
		}
		entry, err := scanVocabularyEntry(ctx, tx, id)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("vocabulary: marshal edit event: %w", err)
		}
		return enqueueOutboxTx(ctx, tx, domain.OutboxEvent{
			EventType:      "vocabulary.candidate.edited",
			AggregateType:  "vocabulary_entry",
			AggregateID:    id,
			PayloadJSON:    string(payload),
			IdempotencyKey: "vocabulary:edit:" + id + ":" + fmt.Sprint(timeValue(now)),
			CreatedAt:      now,
		})
	})
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	return r.Get(ctx, id)
}

func (r *VocabularyRepository) Merge(ctx context.Context, sourceID, targetID string) (domain.VocabularyEntry, error) {
	if sourceID == "" || targetID == "" {
		return domain.VocabularyEntry{}, errors.New("vocabulary: source and target are required")
	}
	if sourceID == targetID {
		return domain.VocabularyEntry{}, errors.New("vocabulary: cannot merge an entry into itself")
	}
	now := time.Now().UTC()
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		var sourceStatus, mergedInto string
		if err := tx.QueryRowContext(ctx,
			"SELECT status, COALESCE(merged_into_id, '') FROM vocabulary_entries WHERE id = ?",
			sourceID,
		).Scan(&sourceStatus, &mergedInto); err != nil {
			return err
		}
		var targetStatus string
		if err := tx.QueryRowContext(ctx,
			"SELECT status FROM vocabulary_entries WHERE id = ?", targetID,
		).Scan(&targetStatus); err != nil {
			return err
		}
		if sourceStatus == string(domain.VocabularyStatusMerged) && mergedInto == targetID {
			return nil
		}
		if sourceStatus != string(domain.VocabularyStatusCandidate) {
			return fmt.Errorf("vocabulary: source must be a candidate, got %q", sourceStatus)
		}
		if targetStatus != string(domain.VocabularyStatusCandidate) && targetStatus != string(domain.VocabularyStatusConfirmed) {
			return fmt.Errorf("vocabulary: target must be a candidate or confirmed entry, got %q", targetStatus)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE observations SET entry_id = ? WHERE entry_id = ?", targetID, sourceID,
		); err != nil {
			return fmt.Errorf("vocabulary: move observations: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE senses SET entry_id = ? WHERE entry_id = ?", targetID, sourceID,
		); err != nil {
			return fmt.Errorf("vocabulary: move senses: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE examples SET entry_id = ? WHERE entry_id = ?", targetID, sourceID,
		); err != nil {
			return fmt.Errorf("vocabulary: move examples: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO entry_tags(entry_id, tag_id)
			SELECT ?, tag_id FROM entry_tags WHERE entry_id = ?`,
			targetID, sourceID,
		); err != nil {
			return fmt.Errorf("vocabulary: move tags: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM entry_tags WHERE entry_id = ?", sourceID,
		); err != nil {
			return fmt.Errorf("vocabulary: delete source tags: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE vocabulary_entries
			SET status = 'merged', merged_into_id = ?, updated_at = ?
			WHERE id = ?`,
			targetID, timeValue(now), sourceID,
		); err != nil {
			return fmt.Errorf("vocabulary: mark merged: %w", err)
		}
		payload, err := json.Marshal(map[string]string{
			"sourceId": sourceID,
			"targetId": targetID,
		})
		if err != nil {
			return err
		}
		return enqueueOutboxTx(ctx, tx, domain.OutboxEvent{
			EventType:      "vocabulary.candidate.merged",
			AggregateType:  "vocabulary_entry",
			AggregateID:    targetID,
			PayloadJSON:    string(payload),
			IdempotencyKey: "vocabulary:merge:" + sourceID + ":" + targetID,
			CreatedAt:      now,
		})
	})
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	return r.Get(ctx, targetID)
}

func (r *VocabularyRepository) SetTags(ctx context.Context, entryID string, names []string) ([]domain.Tag, error) {
	if entryID == "" {
		return nil, errors.New("vocabulary: entry id is required")
	}
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM entry_tags WHERE entry_id = ?", entryID); err != nil {
			return err
		}
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO tags(id, name) VALUES (?, ?) ON CONFLICT(name) DO NOTHING",
				newID(), name,
			); err != nil {
				return err
			}
			var tagID string
			if err := tx.QueryRowContext(ctx, "SELECT id FROM tags WHERE name = ?", name).Scan(&tagID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT OR IGNORE INTO entry_tags(entry_id, tag_id) VALUES (?, ?)",
				entryID, tagID,
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	entry, err := r.Get(ctx, entryID)
	if err != nil {
		return nil, err
	}
	return entry.Tags, nil
}

func scanVocabularyEntry(ctx context.Context, q queryer, id string) (domain.VocabularyEntry, error) {
	var entry domain.VocabularyEntry
	var merged sql.NullString
	var created, updated int64
	err := q.QueryRowContext(ctx, `
		SELECT id, simplified, traditional, reading, marked_pinyin, status,
			provenance, confidence, teaching_cue, manual, priority,
			merged_into_id, created_at, updated_at
		FROM vocabulary_entries WHERE id = ?`, id,
	).Scan(
		&entry.ID, &entry.Simplified, &entry.Traditional, &entry.Reading,
		&entry.MarkedPinyin, &entry.Status, &entry.Provenance, &entry.Confidence,
		&entry.TeachingCue, &entry.Manual, &entry.Priority, &merged, &created, &updated,
	)
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	if merged.Valid {
		entry.MergedIntoID = merged.String
	}
	entry.CreatedAt = timeFromValue(created)
	entry.UpdatedAt = timeFromValue(updated)

	senseRows, err := q.QueryContext(ctx, `
		SELECT id, entry_id, gloss, part_of_speech, classifier, sort_order
		FROM senses WHERE entry_id = ? ORDER BY sort_order, id`, id)
	if err != nil {
		return domain.VocabularyEntry{}, fmt.Errorf("vocabulary: load senses: %w", err)
	}
	for senseRows.Next() {
		var sense domain.Sense
		if err := senseRows.Scan(&sense.ID, &sense.EntryID, &sense.Gloss, &sense.PartOfSpeech, &sense.Classifier, &sense.SortOrder); err != nil {
			_ = senseRows.Close()
			return domain.VocabularyEntry{}, err
		}
		entry.Senses = append(entry.Senses, sense)
	}
	if err := senseRows.Err(); err != nil {
		_ = senseRows.Close()
		return domain.VocabularyEntry{}, err
	}
	_ = senseRows.Close()

	exampleRows, err := q.QueryContext(ctx, `
		SELECT id, entry_id, simplified, traditional, reading, translation, provenance, generated
		FROM examples WHERE entry_id = ? ORDER BY id`, id)
	if err != nil {
		return domain.VocabularyEntry{}, fmt.Errorf("vocabulary: load examples: %w", err)
	}
	for exampleRows.Next() {
		var example domain.Example
		if err := exampleRows.Scan(
			&example.ID, &example.EntryID, &example.Simplified, &example.Traditional,
			&example.Reading, &example.Translation, &example.Provenance, &example.Generated,
		); err != nil {
			_ = exampleRows.Close()
			return domain.VocabularyEntry{}, err
		}
		entry.Examples = append(entry.Examples, example)
	}
	if err := exampleRows.Err(); err != nil {
		_ = exampleRows.Close()
		return domain.VocabularyEntry{}, err
	}
	_ = exampleRows.Close()

	observationRows, err := q.QueryContext(ctx, `
		SELECT id, lesson_id, COALESCE(segment_id, ''), evidence_text,
			start_ms, end_ms, confidence
		FROM observations WHERE entry_id = ? ORDER BY created_at, id`, id)
	if err != nil {
		return domain.VocabularyEntry{}, fmt.Errorf("vocabulary: load evidence: %w", err)
	}
	for observationRows.Next() {
		var evidence domain.Evidence
		if err := observationRows.Scan(
			&evidence.ID, &evidence.LessonID, &evidence.SegmentID, &evidence.Text,
			&evidence.StartMS, &evidence.EndMS, &evidence.Confidence,
		); err != nil {
			_ = observationRows.Close()
			return domain.VocabularyEntry{}, err
		}
		entry.Evidence = append(entry.Evidence, evidence)
	}
	if err := observationRows.Err(); err != nil {
		_ = observationRows.Close()
		return domain.VocabularyEntry{}, err
	}
	_ = observationRows.Close()

	tagRows, err := q.QueryContext(ctx, `
		SELECT tags.id, tags.name
		FROM tags JOIN entry_tags ON entry_tags.tag_id = tags.id
		WHERE entry_tags.entry_id = ? ORDER BY tags.name`, id)
	if err != nil {
		return domain.VocabularyEntry{}, fmt.Errorf("vocabulary: load tags: %w", err)
	}
	for tagRows.Next() {
		var tag domain.Tag
		if err := tagRows.Scan(&tag.ID, &tag.Name); err != nil {
			_ = tagRows.Close()
			return domain.VocabularyEntry{}, err
		}
		entry.Tags = append(entry.Tags, tag)
	}
	if err := tagRows.Err(); err != nil {
		_ = tagRows.Close()
		return domain.VocabularyEntry{}, err
	}
	_ = tagRows.Close()
	return entry, nil
}
