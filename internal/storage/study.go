package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oeggy03/call_analyzer/internal/domain"
)

type StudyRepository struct {
	store *Store
}

func (s *Store) Study() *StudyRepository {
	return &StudyRepository{store: s}
}

func (r *StudyRepository) GetState(ctx context.Context, entryID string) (domain.StudyState, error) {
	var state domain.StudyState
	var due, reviewed sql.NullInt64
	err := r.store.db.QueryRowContext(ctx, `
		SELECT entry_id, due_at, interval_days, ease, repetitions, lapses, last_reviewed_at
		FROM study_state WHERE entry_id = ?`, entryID,
	).Scan(
		&state.EntryID, &due, &state.IntervalDays, &state.Ease,
		&state.Repetitions, &state.Lapses, &reviewed,
	)
	if err != nil {
		return domain.StudyState{}, err
	}
	state.DueAt = scanNullableTime(due)
	state.LastReviewedAt = scanNullableTime(reviewed)
	return state, nil
}

func (r *StudyRepository) RecordReview(
	ctx context.Context,
	entryID string,
	rating int,
	reviewedAt time.Time,
	metadata string,
) (domain.ReviewEvent, domain.StudyState, error) {
	if entryID == "" {
		return domain.ReviewEvent{}, domain.StudyState{}, errors.New("study: entry id is required")
	}
	if rating < 0 || rating > 5 {
		return domain.ReviewEvent{}, domain.StudyState{}, errors.New("study: rating must be between 0 and 5")
	}
	if reviewedAt.IsZero() {
		reviewedAt = time.Now().UTC()
	}
	reviewedAt = reviewedAt.UTC()
	event := domain.ReviewEvent{
		ID:         newID(),
		EntryID:    entryID,
		Rating:     rating,
		ReviewedAt: reviewedAt,
		Metadata:   metadata,
	}
	var state domain.StudyState
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		var due, lastReviewed sql.NullInt64
		if err := tx.QueryRowContext(ctx, `
			SELECT entry_id, due_at, interval_days, ease, repetitions, lapses, last_reviewed_at
			FROM study_state WHERE entry_id = ?`, entryID,
		).Scan(
			&state.EntryID, &due, &state.IntervalDays, &state.Ease,
			&state.Repetitions, &state.Lapses, &lastReviewed,
		); err != nil {
			return err
		}
		state.DueAt = scanNullableTime(due)
		state.LastReviewedAt = scanNullableTime(lastReviewed)
		if state.Ease <= 0 {
			state.Ease = 2.5
		}
		if rating < 3 {
			state.Lapses++
			state.Repetitions = 0
			state.IntervalDays = 1
			state.Ease -= 0.2
			if state.Ease < 1.3 {
				state.Ease = 1.3
			}
		} else {
			if state.Repetitions == 0 {
				state.IntervalDays = 1
			} else if state.Repetitions == 1 {
				state.IntervalDays = 6
			} else {
				state.IntervalDays *= state.Ease
			}
			state.Repetitions++
			state.Ease += 0.1 - float64(5-rating)*0.08
			if state.Ease < 1.3 {
				state.Ease = 1.3
			}
		}
		dueAt := reviewedAt.Add(time.Duration(state.IntervalDays*24) * time.Hour)
		state.DueAt = &dueAt
		state.LastReviewedAt = &reviewedAt
		if _, err := tx.ExecContext(ctx, `
			UPDATE study_state
			SET due_at = ?, interval_days = ?, ease = ?, repetitions = ?,
				lapses = ?, last_reviewed_at = ?
			WHERE entry_id = ?`,
			timeValue(dueAt), state.IntervalDays, state.Ease, state.Repetitions,
			state.Lapses, timeValue(reviewedAt), entryID,
		); err != nil {
			return fmt.Errorf("study: update state: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO review_events(id, entry_id, rating, reviewed_at, metadata)
			VALUES (?, ?, ?, ?, ?)`,
			event.ID, event.EntryID, event.Rating, timeValue(event.ReviewedAt), event.Metadata,
		); err != nil {
			return fmt.Errorf("study: insert review: %w", err)
		}
		if err := enqueueOutboxTx(ctx, tx, domain.OutboxEvent{
			EventType:      "study.review.recorded",
			AggregateType:  "vocabulary_entry",
			AggregateID:    entryID,
			PayloadJSON:    fmt.Sprintf(`{"entryId":%q,"rating":%d}`, entryID, rating),
			IdempotencyKey: "study:review:" + event.ID,
			CreatedAt:      reviewedAt,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.ReviewEvent{}, domain.StudyState{}, err
	}
	return event, state, nil
}
