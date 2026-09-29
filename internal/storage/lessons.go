package storage

import (
	"context"
	"database/sql"
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

func (r *LessonRepository) Start(ctx context.Context, title string, startedAt time.Time) (domain.Lesson, error) {
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	lesson := domain.Lesson{
		ID:        newID(),
		Title:     title,
		StartedAt: startedAt.UTC(),
		CreatedAt: time.Now().UTC(),
	}
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lessons(id, title, started_at, created_at)
			VALUES (?, ?, ?, ?)`,
			lesson.ID, lesson.Title, timeValue(lesson.StartedAt), timeValue(lesson.CreatedAt),
		)
		return err
	})
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("lessons: start: %w", err)
	}
	return lesson, nil
}

func (r *LessonRepository) End(ctx context.Context, id string, endedAt time.Time) (domain.Lesson, error) {
	if id == "" {
		return domain.Lesson{}, errors.New("lessons: id is required")
	}
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}
	var count int64
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE lessons SET ended_at = ? WHERE id = ?",
			timeValue(endedAt.UTC()), id,
		)
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
	var started, created int64
	var ended sql.NullInt64
	err := r.store.db.QueryRowContext(ctx, `
		SELECT id, title, started_at, ended_at, created_at
		FROM lessons WHERE id = ?`, id,
	).Scan(&lesson.ID, &lesson.Title, &started, &ended, &created)
	if err != nil {
		return domain.Lesson{}, err
	}
	lesson.StartedAt = timeFromValue(started)
	lesson.EndedAt = scanNullableTime(ended)
	lesson.CreatedAt = timeFromValue(created)
	return lesson, nil
}

func (r *LessonRepository) List(ctx context.Context, limit int) ([]domain.Lesson, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id, title, started_at, ended_at, created_at
		FROM lessons ORDER BY started_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("lessons: list: %w", err)
	}
	defer rows.Close()

	var lessons []domain.Lesson
	for rows.Next() {
		var lesson domain.Lesson
		var started, created int64
		var ended sql.NullInt64
		if err := rows.Scan(&lesson.ID, &lesson.Title, &started, &ended, &created); err != nil {
			return nil, fmt.Errorf("lessons: scan: %w", err)
		}
		lesson.StartedAt = timeFromValue(started)
		lesson.EndedAt = scanNullableTime(ended)
		lesson.CreatedAt = timeFromValue(created)
		lessons = append(lessons, lesson)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lessons: list rows: %w", err)
	}
	return lessons, nil
}

func (r *LessonRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("lessons: id is required")
	}
	var count int64
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "DELETE FROM lessons WHERE id = ?", id)
		if err != nil {
			return err
		}
		count, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("lessons: delete: %w", err)
	}
	if count == 0 {
		return sql.ErrNoRows
	}
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
