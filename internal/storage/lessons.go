package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oeggy03/call_analyzer/internal/domain"
)

type LessonRepository struct {
	store *Store
}

func (s *Store) Lessons() *LessonRepository {
	return &LessonRepository{store: s}
}

type LessonMetadata struct {
	ConsentRecorded bool
	Target          string
	STTModel        string
	AnalyzerModel   string
	RetentionPolicy string
}

func (r *LessonRepository) Start(ctx context.Context, title string, startedAt time.Time) (domain.Lesson, error) {
	return r.StartWithMetadata(ctx, title, startedAt, LessonMetadata{})
}

func (r *LessonRepository) StartWithMetadata(
	ctx context.Context,
	title string,
	startedAt time.Time,
	metadata LessonMetadata,
) (domain.Lesson, error) {
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	lesson := domain.Lesson{
		ID:              newID(),
		Title:           title,
		StartedAt:       startedAt.UTC(),
		CreatedAt:       time.Now().UTC(),
		ConsentRecorded: metadata.ConsentRecorded,
		Target:          metadata.Target,
		STTModel:        metadata.STTModel,
		AnalyzerModel:   metadata.AnalyzerModel,
		RetentionPolicy: metadata.RetentionPolicy,
	}
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lessons(
				id, title, started_at, created_at, consent_recorded, target,
				stt_model, analyzer_model, retention_policy, final_cost
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			lesson.ID, lesson.Title, timeValue(lesson.StartedAt), timeValue(lesson.CreatedAt),
			lesson.ConsentRecorded, lesson.Target, lesson.STTModel,
			lesson.AnalyzerModel, lesson.RetentionPolicy, lesson.FinalCost,
		)
		return err
	})
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("lessons: start: %w", err)
	}
	return lesson, nil
}

func (r *LessonRepository) End(ctx context.Context, id string, endedAt time.Time) (domain.Lesson, error) {
	return r.end(ctx, id, endedAt, nil)
}

func (r *LessonRepository) EndWithCost(
	ctx context.Context,
	id string,
	endedAt time.Time,
	finalCost float64,
) (domain.Lesson, error) {
	return r.end(ctx, id, endedAt, &finalCost)
}

func (r *LessonRepository) end(
	ctx context.Context,
	id string,
	endedAt time.Time,
	finalCost *float64,
) (domain.Lesson, error) {
	if id == "" {
		return domain.Lesson{}, errors.New("lessons: id is required")
	}
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}
	var count int64
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		query := "UPDATE lessons SET ended_at = ?"
		args := []any{timeValue(endedAt.UTC())}
		if finalCost != nil {
			query += ", final_cost = ?"
			args = append(args, *finalCost)
		}
		query += " WHERE id = ?"
		args = append(args, id)
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		count, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("lessons: end: %w", err)
	}
	if count == 0 {
		return domain.Lesson{}, sql.ErrNoRows
	}
	return r.Get(ctx, id)
}

func (r *LessonRepository) Get(ctx context.Context, id string) (domain.Lesson, error) {
	var lesson domain.Lesson
	err := scanLesson(r.store.db.QueryRowContext(ctx, `
		SELECT id, title, started_at, ended_at, created_at,
			consent_recorded, target, stt_model, analyzer_model,
			retention_policy, final_cost
		FROM lessons WHERE id = ?`, id), &lesson)
	if err != nil {
		return domain.Lesson{}, err
	}
	return lesson, nil
}

func (r *LessonRepository) List(ctx context.Context, limit int) ([]domain.Lesson, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id, title, started_at, ended_at, created_at,
			consent_recorded, target, stt_model, analyzer_model,
			retention_policy, final_cost
		FROM lessons ORDER BY started_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("lessons: list: %w", err)
	}
	defer rows.Close()

	var lessons []domain.Lesson
	for rows.Next() {
		var lesson domain.Lesson
		if err := scanLesson(rows, &lesson); err != nil {
			return nil, fmt.Errorf("lessons: scan: %w", err)
		}
		lessons = append(lessons, lesson)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lessons: list rows: %w", err)
	}
	return lessons, nil
}

func (r *LessonRepository) Latest(ctx context.Context) (domain.Lesson, error) {
	return r.latest(ctx, "")
}

func (r *LessonRepository) LatestComplete(ctx context.Context) (domain.Lesson, error) {
	return r.latest(ctx, " WHERE ended_at IS NOT NULL")
}

func (r *LessonRepository) latest(ctx context.Context, predicate string) (domain.Lesson, error) {
	var lesson domain.Lesson
	err := scanLesson(r.store.db.QueryRowContext(ctx, `
		SELECT id, title, started_at, ended_at, created_at,
			consent_recorded, target, stt_model, analyzer_model,
			retention_policy, final_cost
		FROM lessons`+predicate+`
		ORDER BY started_at DESC, created_at DESC LIMIT 1`), &lesson)
	if err != nil {
		return domain.Lesson{}, err
	}
	return lesson, nil
}

func (r *LessonRepository) EndOrphaned(ctx context.Context, endedAt time.Time) (int64, error) {
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}
	var count int64
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			`UPDATE lessons
			 SET ended_at = ?,
			     final_cost = COALESCE(
			         (SELECT SUM(cost) FROM request_usage WHERE session_id = lessons.id),
			         final_cost,
			         0
			     )
			 WHERE ended_at IS NULL`,
			timeValue(endedAt.UTC()),
		)
		if err != nil {
			return err
		}
		count, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("lessons: end orphaned: %w", err)
	}
	return count, nil
}

func (r *LessonRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("lessons: id is required")
	}
	var count int64
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			"SELECT DISTINCT entry_id FROM observations WHERE lesson_id = ?", id)
		if err != nil {
			return fmt.Errorf("lessons: list affected vocabulary: %w", err)
		}
		var affectedEntryIDs []string
		for rows.Next() {
			var entryID string
			if err := rows.Scan(&entryID); err != nil {
				_ = rows.Close()
				return err
			}
			affectedEntryIDs = append(affectedEntryIDs, entryID)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()

		// Pending outbox payloads include evidence text. Remove them before the
		// lesson is deleted so a local privacy deletion does not leave a second
		// copy of the transcript behind.
		for _, entryID := range affectedEntryIDs {
			if _, err := tx.ExecContext(ctx,
				"DELETE FROM outbox WHERE aggregate_type = 'vocabulary_entry' AND aggregate_id = ?",
				entryID,
			); err != nil {
				return fmt.Errorf("lessons: purge evidence outbox: %w", err)
			}
		}

		result, err := tx.ExecContext(ctx, "DELETE FROM lessons WHERE id = ?", id)
		if err != nil {
			return err
		}
		count, err = result.RowsAffected()
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		for _, entryID := range affectedEntryIDs {
			var status domain.VocabularyStatus
			err := tx.QueryRowContext(ctx,
				"SELECT status FROM vocabulary_entries WHERE id = ?", entryID,
			).Scan(&status)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			var remainingObservations int
			if err := tx.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM observations WHERE entry_id = ?", entryID,
			).Scan(&remainingObservations); err != nil {
				return err
			}
			if remainingObservations == 0 &&
				(status == domain.VocabularyStatusCandidate || status == domain.VocabularyStatusRejected) {
				if _, err := tx.ExecContext(ctx,
					"DELETE FROM vocabulary_entries WHERE id = ?", entryID,
				); err != nil {
					return err
				}
				payload, _ := json.Marshal(map[string]string{"id": entryID})
				if err := enqueueOutboxTx(ctx, tx, domain.OutboxEvent{
					EventType:      "vocabulary.entry.deleted",
					AggregateType:  "vocabulary_entry",
					AggregateID:    entryID,
					PayloadJSON:    string(payload),
					IdempotencyKey: "vocabulary:deleted:" + entryID + ":" + fmt.Sprint(timeValue(now)),
					CreatedAt:      now,
				}); err != nil {
					return err
				}
				continue
			}

			// Confirmed vocabulary survives lesson deletion as user-owned study
			// data. Queue a fresh evidence-free representation for future sync.
			entry, err := scanVocabularyEntry(ctx, tx, entryID)
			if err != nil {
				return err
			}
			entry.Evidence = nil
			payload, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			if err := enqueueOutboxTx(ctx, tx, domain.OutboxEvent{
				EventType:      "vocabulary.entry.evidence_redacted",
				AggregateType:  "vocabulary_entry",
				AggregateID:    entryID,
				PayloadJSON:    string(payload),
				IdempotencyKey: "vocabulary:redacted:" + entryID + ":" + fmt.Sprint(timeValue(now)),
				CreatedAt:      now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("lessons: delete: %w", err)
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type lessonScanner interface {
	Scan(dest ...any) error
}

func scanLesson(scanner lessonScanner, lesson *domain.Lesson) error {
	var started, created int64
	var ended sql.NullInt64
	var consent int
	if err := scanner.Scan(
		&lesson.ID,
		&lesson.Title,
		&started,
		&ended,
		&created,
		&consent,
		&lesson.Target,
		&lesson.STTModel,
		&lesson.AnalyzerModel,
		&lesson.RetentionPolicy,
		&lesson.FinalCost,
	); err != nil {
		return err
	}
	lesson.StartedAt = timeFromValue(started)
	lesson.EndedAt = scanNullableTime(ended)
	lesson.CreatedAt = timeFromValue(created)
	lesson.ConsentRecorded = consent != 0
	return nil
}

type TranscriptRepository struct {
	store *Store
}

func (s *Store) Transcripts() *TranscriptRepository {
	return &TranscriptRepository{store: s}
}

func (r *TranscriptRepository) Insert(ctx context.Context, segment domain.TranscriptSegment) (domain.TranscriptSegment, error) {
	if segment.ID == "" {
		segment.ID = newID()
	}
	if segment.CreatedAt.IsZero() {
		segment.CreatedAt = time.Now().UTC()
	}
	if segment.EndMS < segment.StartMS {
		return domain.TranscriptSegment{}, errors.New("transcripts: end must not precede start")
	}
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO transcript_segments(
				id, lesson_id, start_ms, end_ms, text, simplified_text,
				traditional_text, reading, source, confidence, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			segment.ID, segment.LessonID, segment.StartMS, segment.EndMS,
			segment.Text, segment.SimplifiedText, segment.TraditionalText,
			segment.Reading, segment.Source, segment.Confidence, timeValue(segment.CreatedAt),
		)
		return err
	})
	if err != nil {
		return domain.TranscriptSegment{}, fmt.Errorf("transcripts: insert: %w", err)
	}
	return segment, nil
}

func (r *TranscriptRepository) InsertBatch(ctx context.Context, segments []domain.TranscriptSegment) ([]domain.TranscriptSegment, error) {
	if len(segments) == 0 {
		return []domain.TranscriptSegment{}, nil
	}
	now := time.Now().UTC()
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		for i := range segments {
			if segments[i].ID == "" {
				segments[i].ID = newID()
			}
			if segments[i].CreatedAt.IsZero() {
				segments[i].CreatedAt = now
			}
			if segments[i].EndMS < segments[i].StartMS {
				return fmt.Errorf("transcripts: segment %s end precedes start", segments[i].ID)
			}
			_, err := tx.ExecContext(ctx, `
				INSERT INTO transcript_segments(
					id, lesson_id, start_ms, end_ms, text, simplified_text,
					traditional_text, reading, source, confidence, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				segments[i].ID, segments[i].LessonID, segments[i].StartMS,
				segments[i].EndMS, segments[i].Text, segments[i].SimplifiedText,
				segments[i].TraditionalText, segments[i].Reading, segments[i].Source,
				segments[i].Confidence, timeValue(segments[i].CreatedAt),
			)
			if err != nil {
				return fmt.Errorf("transcripts: insert batch: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return segments, nil
}

func (r *TranscriptRepository) Get(ctx context.Context, id string) (domain.TranscriptSegment, error) {
	var segment domain.TranscriptSegment
	var created int64
	err := r.store.db.QueryRowContext(ctx, `
		SELECT id, lesson_id, start_ms, end_ms, text, simplified_text,
			traditional_text, reading, source, confidence, created_at
		FROM transcript_segments WHERE id = ?`, id,
	).Scan(
		&segment.ID,
		&segment.LessonID,
		&segment.StartMS,
		&segment.EndMS,
		&segment.Text,
		&segment.SimplifiedText,
		&segment.TraditionalText,
		&segment.Reading,
		&segment.Source,
		&segment.Confidence,
		&created,
	)
	if err != nil {
		return domain.TranscriptSegment{}, err
	}
	segment.CreatedAt = timeFromValue(created)
	return segment, nil
}

func (r *TranscriptRepository) List(ctx context.Context, lessonID string) ([]domain.TranscriptSegment, error) {
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id, lesson_id, start_ms, end_ms, text, simplified_text,
			traditional_text, reading, source, confidence, created_at
		FROM transcript_segments
		WHERE lesson_id = ?
		ORDER BY start_ms, created_at`, lessonID,
	)
	if err != nil {
		return nil, fmt.Errorf("transcripts: list: %w", err)
	}
	defer rows.Close()

	var segments []domain.TranscriptSegment
	for rows.Next() {
		var segment domain.TranscriptSegment
		var created int64
		if err := rows.Scan(
			&segment.ID, &segment.LessonID, &segment.StartMS, &segment.EndMS,
			&segment.Text, &segment.SimplifiedText, &segment.TraditionalText,
			&segment.Reading, &segment.Source, &segment.Confidence, &created,
		); err != nil {
			return nil, fmt.Errorf("transcripts: scan: %w", err)
		}
		segment.CreatedAt = timeFromValue(created)
		segments = append(segments, segment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("transcripts: list rows: %w", err)
	}
	return segments, nil
}
