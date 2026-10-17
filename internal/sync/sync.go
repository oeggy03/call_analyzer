// Package sync contains an opt-in outbox dispatcher. The default syncer is
// disabled, so local SQLite remains the source of truth until a hosted API is
// deliberately configured.
package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/oeggy03/call_analyzer/internal/domain"
)

type Syncer interface {
	Enabled() bool
	Sync(context.Context, []domain.OutboxEvent) error
}

type DisabledSyncer struct{}

func (DisabledSyncer) Enabled() bool {
	return false
}

func (DisabledSyncer) Sync(context.Context, []domain.OutboxEvent) error {
	return nil
}

type Dispatcher struct {
	outbox storageOutbox
	syncer Syncer
}

type storageOutbox interface {
	ListPending(context.Context, int) ([]domain.OutboxEvent, error)
	MarkDelivered(context.Context, string) error
	MarkFailed(context.Context, string, error) error
}

func NewDispatcher(outbox storageOutbox, syncer Syncer) *Dispatcher {
	if syncer == nil {
		syncer = DisabledSyncer{}
	}
	return &Dispatcher{outbox: outbox, syncer: syncer}
}

func (d *Dispatcher) Dispatch(ctx context.Context, limit int) (int, error) {
	if d == nil || d.outbox == nil || d.syncer == nil || !d.syncer.Enabled() {
		return 0, nil
	}
	events, err := d.outbox.ListPending(ctx, limit)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	if err := d.syncer.Sync(ctx, events); err != nil {
		for _, event := range events {
			_ = d.outbox.MarkFailed(ctx, event.ID, err)
		}
		return 0, err
	}
	for _, event := range events {
		if err := d.outbox.MarkDelivered(ctx, event.ID); err != nil {
			return 0, err
		}
	}
	return len(events), nil
}

type TokenProvider interface {
	Token(context.Context) (string, error)
}

type HTTPConfig struct {
	Endpoint   string
	HTTPClient *http.Client
	Token      TokenProvider
}

type HTTPSyncer struct {
	endpoint string
	client   *http.Client
	token    TokenProvider
}

func NewHTTPSyncer(config HTTPConfig) (*HTTPSyncer, error) {
	if strings.TrimSpace(config.Endpoint) == "" {
		return nil, errors.New("sync: endpoint is required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &HTTPSyncer{
		endpoint: strings.TrimRight(config.Endpoint, "/"),
		client:   config.HTTPClient,
		token:    config.Token,
	}, nil
}

func (s *HTTPSyncer) Enabled() bool {
	return s != nil && s.endpoint != ""
}

func (s *HTTPSyncer) Sync(ctx context.Context, events []domain.OutboxEvent) error {
	if !s.Enabled() {
		return nil
	}
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("sync: marshal %s: %w", event.ID, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("sync: request %s: %w", event.ID, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", event.IdempotencyKey)
		if s.token != nil {
			token, err := s.token.Token(ctx)
			if err != nil {
				return err
			}
			if strings.TrimSpace(token) != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}
		response, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("sync: send %s: %w", event.ID, err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			return readErr
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("sync: HTTP %d for %s: %s", response.StatusCode, event.ID, strings.TrimSpace(string(body)))
		}
	}
	return nil
}
