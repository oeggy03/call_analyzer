package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/oeggy03/call_analyzer/internal/domain"
)

type fakeOutbox struct {
	mu        sync.Mutex
	events    []domain.OutboxEvent
	delivered []string
	failed    []string
}

func (f *fakeOutbox) ListPending(context.Context, int) ([]domain.OutboxEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.OutboxEvent(nil), f.events...), nil
}

func (f *fakeOutbox) MarkDelivered(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delivered = append(f.delivered, id)
	return nil
}

func (f *fakeOutbox) MarkFailed(_ context.Context, id string, _ error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, id)
	return nil
}

type fakeSyncer struct {
	events []domain.OutboxEvent
}

func (f *fakeSyncer) Enabled() bool {
	return true
}

func (f *fakeSyncer) Sync(_ context.Context, events []domain.OutboxEvent) error {
	f.events = append(f.events, events...)
	return nil
}

func TestDispatcherMarksDelivered(t *testing.T) {
	outbox := &fakeOutbox{events: []domain.OutboxEvent{{ID: "one", IdempotencyKey: "key-one"}}}
	syncer := &fakeSyncer{}
	count, err := NewDispatcher(outbox, syncer).Dispatch(context.Background(), 10)
	if err != nil || count != 1 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}
	if len(syncer.events) != 1 || len(outbox.delivered) != 1 {
		t.Fatalf("unexpected dispatch state sync=%#v delivered=%#v", syncer.events, outbox.delivered)
	}
}

func TestHTTPSyncerUsesIdempotencyKey(t *testing.T) {
	var gotKey, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	syncer, err := NewHTTPSyncer(HTTPConfig{
		Endpoint: server.URL,
		Token:    staticToken("token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = syncer.Sync(context.Background(), []domain.OutboxEvent{{
		ID:             "one",
		IdempotencyKey: "idempotent-one",
		PayloadJSON:    "{}",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "idempotent-one" || gotAuth != "Bearer token" {
		t.Fatalf("unexpected headers key=%q auth=%q", gotKey, gotAuth)
	}
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) {
	return string(s), nil
}
