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

type OutboxRepository struct {
	store *Store
}

func (s *Store) Outbox() *OutboxRepository {
	return &OutboxRepository{store: s}
}

func (r *OutboxRepository) Enqueue(ctx context.Context, event domain.OutboxEvent) (domain.OutboxEvent, error) {
	if event.ID == "" {
		event.ID = newID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.IdempotencyKey == "" {
		event.IdempotencyKey = event.ID
	}
	if event.PayloadJSON == "" {
		event.PayloadJSON = "{}"
	}
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		return enqueueOutboxTx(ctx, tx, event)
	})
	if err != nil {
		return domain.OutboxEvent{}, err
	}
	return event, nil
}

func enqueueOutboxTx(ctx context.Context, tx *sql.Tx, event domain.OutboxEvent) error {
	if event.ID == "" {
		event.ID = newID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(event.IdempotencyKey) == "" {
		event.IdempotencyKey = event.ID
	}
	if event.PayloadJSON == "" {
		event.PayloadJSON = "{}"
	}
	_, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO outbox(
			id, event_type, aggregate_type, aggregate_id, payload_json,
			idempotency_key, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.EventType, event.AggregateType, event.AggregateID,
		event.PayloadJSON, event.IdempotencyKey, timeValue(event.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("outbox: enqueue: %w", err)
	}
	return nil
}

func (r *OutboxRepository) ListPending(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id, event_type, aggregate_type, aggregate_id, payload_json,
			idempotency_key, created_at, delivered_at, attempts, last_error
		FROM outbox
		WHERE delivered_at IS NULL
		ORDER BY created_at, id
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox: list pending: %w", err)
	}
	defer rows.Close()
	var events []domain.OutboxEvent
	for rows.Next() {
		event, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox: list rows: %w", err)
	}
	return events, nil
}

func scanOutbox(scanner interface{ Scan(...any) error }) (domain.OutboxEvent, error) {
	var event domain.OutboxEvent
	var created int64
	var delivered sql.NullInt64
	if err := scanner.Scan(
		&event.ID, &event.EventType, &event.AggregateType, &event.AggregateID,
		&event.PayloadJSON, &event.IdempotencyKey, &created, &delivered,
		&event.Attempts, &event.LastError,
	); err != nil {
		return domain.OutboxEvent{}, fmt.Errorf("outbox: scan: %w", err)
	}
	event.CreatedAt = timeFromValue(created)
	event.DeliveredAt = scanNullableTime(delivered)
	return event, nil
}

func (r *OutboxRepository) MarkDelivered(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("outbox: id is required")
	}
	var count int64
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE outbox SET delivered_at = ?, last_error = '' WHERE id = ?`,
			timeValue(time.Now().UTC()), id,
		)
		if err != nil {
			return err
		}
		count, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("outbox: mark delivered: %w", err)
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *OutboxRepository) MarkFailed(ctx context.Context, id string, failure error) error {
	if id == "" {
		return errors.New("outbox: id is required")
	}
	message := ""
	if failure != nil {
		message = failure.Error()
	}
	var count int64
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE outbox SET attempts = attempts + 1, last_error = ? WHERE id = ?`,
			message, id,
		)
		if err != nil {
			return err
		}
		count, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("outbox: mark failed: %w", err)
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type ObservationRepository struct {
	store *Store
}

func (s *Store) Observations() *ObservationRepository {
	return &ObservationRepository{store: s}
}

func (r *ObservationRepository) Insert(ctx context.Context, observation domain.Observation) (domain.Observation, error) {
	if observation.LessonID == "" || observation.EntryID == "" {
		return domain.Observation{}, errors.New("observations: lesson and entry are required")
	}
	if strings.TrimSpace(observation.EvidenceText) == "" {
		return domain.Observation{}, errors.New("observations: evidence text is required")
	}
	if observation.ID == "" {
		observation.ID = newID()
	}
	if observation.CreatedAt.IsZero() {
		observation.CreatedAt = time.Now().UTC()
	}
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO observations(
				id, lesson_id, segment_id, entry_id, evidence_text,
				start_ms, end_ms, confidence, created_at
			) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?)`,
			observation.ID, observation.LessonID, observation.SegmentID, observation.EntryID,
			observation.EvidenceText, observation.StartMS, observation.EndMS,
			observation.Confidence, timeValue(observation.CreatedAt),
		)
		return err
	})
	if err != nil {
		return domain.Observation{}, fmt.Errorf("observations: insert: %w", err)
	}
	return observation, nil
}

func (r *ObservationRepository) List(ctx context.Context, lessonID string) ([]domain.Observation, error) {
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id, lesson_id, COALESCE(segment_id, ''), entry_id, evidence_text,
			start_ms, end_ms, confidence, created_at
		FROM observations WHERE lesson_id = ?
		ORDER BY created_at, id`, lessonID)
	if err != nil {
		return nil, fmt.Errorf("observations: list: %w", err)
	}
	defer rows.Close()
	var observations []domain.Observation
	for rows.Next() {
		var observation domain.Observation
		var created int64
		if err := rows.Scan(
			&observation.ID, &observation.LessonID, &observation.SegmentID,
			&observation.EntryID, &observation.EvidenceText, &observation.StartMS,
			&observation.EndMS, &observation.Confidence, &created,
		); err != nil {
			return nil, fmt.Errorf("observations: scan: %w", err)
		}
		observation.CreatedAt = timeFromValue(created)
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("observations: rows: %w", err)
	}
	return observations, nil
}

type SettingsRepository struct {
	store *Store
}

func (s *Store) Settings() *SettingsRepository {
	return &SettingsRepository{store: s}
}

func (r *SettingsRepository) Get(ctx context.Context, key string) (domain.Setting, error) {
	var setting domain.Setting
	var updated int64
	err := r.store.db.QueryRowContext(ctx,
		"SELECT key, value_json, updated_at FROM settings WHERE key = ?", key,
	).Scan(&setting.Key, &setting.ValueJSON, &updated)
	if err != nil {
		return domain.Setting{}, err
	}
	setting.UpdatedAt = timeFromValue(updated)
	return setting, nil
}

func (r *SettingsRepository) Set(ctx context.Context, key, valueJSON string) (domain.Setting, error) {
	if strings.TrimSpace(key) == "" {
		return domain.Setting{}, errors.New("settings: key is required")
	}
	now := time.Now().UTC()
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO settings(key, value_json, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
			key, valueJSON, timeValue(now),
		)
		return err
	})
	if err != nil {
		return domain.Setting{}, fmt.Errorf("settings: set: %w", err)
	}
	return domain.Setting{Key: key, ValueJSON: valueJSON, UpdatedAt: now}, nil
}

func (r *SettingsRepository) List(ctx context.Context) ([]domain.Setting, error) {
	rows, err := r.store.db.QueryContext(ctx,
		"SELECT key, value_json, updated_at FROM settings ORDER BY key")
	if err != nil {
		return nil, fmt.Errorf("settings: list: %w", err)
	}
	defer rows.Close()
	var settings []domain.Setting
	for rows.Next() {
		var setting domain.Setting
		var updated int64
		if err := rows.Scan(&setting.Key, &setting.ValueJSON, &updated); err != nil {
			return nil, err
		}
		setting.UpdatedAt = timeFromValue(updated)
		settings = append(settings, setting)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return settings, nil
}

type UsageRepository struct {
	store *Store
}

func (s *Store) Usage() *UsageRepository {
	return &UsageRepository{store: s}
}

func (r *UsageRepository) Record(ctx context.Context, usage domain.RequestUsage) (domain.RequestUsage, error) {
	if usage.ID == "" {
		usage.ID = newID()
	}
	if usage.CreatedAt.IsZero() {
		usage.CreatedAt = time.Now().UTC()
	}
	err := r.store.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO request_usage(
				id, session_id, provider, model, endpoint, prompt_tokens,
				completion_tokens, total_tokens, cost, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			usage.ID, usage.SessionID, usage.Provider, usage.Model, usage.Endpoint,
			usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, usage.Cost,
			timeValue(usage.CreatedAt),
		)
		return err
	})
	if err != nil {
		return domain.RequestUsage{}, fmt.Errorf("usage: record: %w", err)
	}
	return usage, nil
}

func (r *UsageRepository) ListBySession(ctx context.Context, sessionID string, limit int) ([]domain.RequestUsage, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id, session_id, provider, model, endpoint, prompt_tokens,
			completion_tokens, total_tokens, cost, created_at
		FROM request_usage WHERE session_id = ?
		ORDER BY created_at DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("usage: list: %w", err)
	}
	defer rows.Close()
	var usages []domain.RequestUsage
	for rows.Next() {
		var usage domain.RequestUsage
		var created int64
		if err := rows.Scan(
			&usage.ID, &usage.SessionID, &usage.Provider, &usage.Model,
			&usage.Endpoint, &usage.PromptTokens, &usage.CompletionTokens,
			&usage.TotalTokens, &usage.Cost, &created,
		); err != nil {
			return nil, err
		}
		usage.CreatedAt = timeFromValue(created)
		usages = append(usages, usage)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return usages, nil
}

func (r *UsageRepository) SumBySession(ctx context.Context, sessionID string) (float64, error) {
	var total float64
	err := r.store.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(cost), 0) FROM request_usage WHERE session_id = ?",
		sessionID,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("usage: sum: %w", err)
	}
	return total, nil
}
