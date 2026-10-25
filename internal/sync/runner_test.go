package sync

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Prefer string
	Body   string
	APIKey string
	Auth   string
}

type requestRecorder struct {
	mu       sync.Mutex
	requests []recordedRequest
	status   int
	body     string
}

type testSecretStore struct {
	value string
}

func (s testSecretStore) Get(ctx context.Context, _ string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.value == "" {
		return "", errors.New("missing test secret")
	}
	return s.value, nil
}

func (r *requestRecorder) handler(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	r.requests = append(r.requests, recordedRequest{
		Method: req.Method,
		Path:   req.URL.Path,
		Query:  req.URL.RawQuery,
		Prefer: req.Header.Get("Prefer"),
		Body:   string(body),
		APIKey: req.Header.Get("apikey"),
		Auth:   req.Header.Get("Authorization"),
	})
	status, responseBody := r.status, r.body
	r.mu.Unlock()
	if status == 0 {
		if req.Method == http.MethodPost && req.URL.Path == "/rest/v1/tags" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(responseBody))
}

func (r *requestRecorder) snapshot() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.requests...)
}

func newTestServer(recorder *requestRecorder) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(recorder.handler))
}

func newVocabularyStore(t *testing.T) (*storage.Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, ctx
}

func confirmedEntry(t *testing.T, ctx context.Context, store *storage.Store) domain.VocabularyEntry {
	t.Helper()
	entry, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		ID:          uuid.NewString(),
		Simplified:  "学习",
		Traditional: "學習",
		Reading:     "xue2 xi2",
		Senses: []domain.Sense{{
			Gloss:     "to study",
			SortOrder: 0,
		}},
		Examples: []domain.Example{{
			Simplified:  "我学习中文。",
			Reading:     "wo3 xue2 xi2 zhong1 wen2",
			Translation: "I study Chinese.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Vocabulary().AddTags(ctx, entry.ID, []string{"study"}); err != nil {
		t.Fatal(err)
	}
	entry, err = store.Vocabulary().Confirm(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func candidateEntry(t *testing.T, ctx context.Context, store *storage.Store) domain.VocabularyEntry {
	t.Helper()
	entry, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		ID:          uuid.NewString(),
		Simplified:  "方案",
		Traditional: "方案",
		Reading:     "fang1 an4",
	})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func newConfiguredRunner(
	t *testing.T,
	store *storage.Store,
	ctx context.Context,
	serverURL string,
	now func() time.Time,
) (*Runner, string) {
	t.Helper()
	secret := "fixture-" + uuid.NewString()
	secrets := testSecretStore{value: secret}
	return NewRunner(store, secrets, RunnerOptions{
		BaseURL:     serverURL,
		QuietPeriod: 10 * time.Minute,
		Now:         now,
	}), secret
}

func TestRunnerQuietPeriodHoldsConfirmedAndSkipsCandidateHTTP(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	confirmed := confirmedEntry(t, ctx, store)
	candidate := candidateEntry(t, ctx, store)

	recorder := &requestRecorder{}
	server := newTestServer(recorder)
	defer server.Close()
	now := time.Now().UTC()
	runner, _ := newConfiguredRunner(t, store, ctx, server.URL, func() time.Time {
		return now
	})

	if err := runner.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if requests := recorder.snapshot(); len(requests) != 0 {
		t.Fatalf("quiet period made %d HTTP requests", len(requests))
	}
	status := runner.Status()
	if status.Status != SyncStatusWaiting || status.PendingCount != 3 {
		t.Fatalf("unexpected waiting status: %#v", status)
	}

	now = now.Add(11 * time.Minute)
	if err := runner.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	requests := recorder.snapshot()
	for _, request := range requests {
		if strings.Contains(request.Body, candidate.ID) {
			t.Fatalf("candidate was uploaded: method=%s path=%s", request.Method, request.Path)
		}
	}
	pending, err := store.Outbox().ListPendingVocabulary(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("eligible outbox rows remain pending: %#v", pending)
	}
	_ = confirmed
}

func TestRunnerSyncNowUpsertsCurrentEntryAndInitializesStudyState(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	entry := confirmedEntry(t, ctx, store)
	recorder := &requestRecorder{}
	server := newTestServer(recorder)
	defer server.Close()
	runner, secret := newConfiguredRunner(t, store, ctx, server.URL, time.Now)

	if err := runner.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	requests := recorder.snapshot()
	if len(requests) == 0 {
		t.Fatal("SyncNow made no HTTP requests")
	}
	var vocabularyRequest, studyRequest bool
	for _, request := range requests {
		if request.Path == "/rest/v1/vocabulary_entries" {
			vocabularyRequest = true
			if request.Method != http.MethodPost ||
				!strings.Contains(request.Query, "on_conflict=id") ||
				request.Prefer != "resolution=merge-duplicates,return=minimal" ||
				!strings.Contains(request.Body, `"merged_into_id":null`) {
				t.Fatal("vocabulary upsert did not meet the PostgREST contract")
			}
		}
		if request.Path == "/rest/v1/study_state" {
			studyRequest = true
			if request.Prefer != "resolution=ignore-duplicates,return=minimal" ||
				!strings.Contains(request.Query, "on_conflict=entry_id") {
				t.Fatal("study-state upsert did not meet the PostgREST contract")
			}
		}
		if request.APIKey != secret ||
			request.Auth != "Bearer "+secret {
			t.Fatalf("secret headers were not sent as required for %s %s", request.Method, request.Path)
		}
	}
	if !vocabularyRequest || !studyRequest {
		t.Fatalf("missing required requests in %d HTTP calls", len(requests))
	}
	if runner.Status().Status != SyncStatusIdle || runner.Status().PendingCount != 0 {
		t.Fatalf("unexpected completed status: %#v", runner.Status())
	}
	_ = entry
}

func TestRunnerDeletesWhenLocalEntryIsGone(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	entry := confirmedEntry(t, ctx, store)
	if _, err := store.DB().ExecContext(ctx, "DELETE FROM vocabulary_entries WHERE id = ?", entry.ID); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{}
	server := newTestServer(recorder)
	defer server.Close()
	runner, _ := newConfiguredRunner(t, store, ctx, server.URL, time.Now)

	if err := runner.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	var deleted bool
	for _, request := range recorder.snapshot() {
		if request.Method == http.MethodDelete &&
			request.Path == "/rest/v1/vocabulary_entries" &&
			strings.Contains(request.Query, "id=eq."+entry.ID) {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("missing upstream delete after %d requests", len(recorder.snapshot()))
	}
}

func TestRunnerMissingSecretMakesNoHTTPCall(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	confirmedEntry(t, ctx, store)
	recorder := &requestRecorder{}
	server := newTestServer(recorder)
	defer server.Close()
	runner := NewRunner(store, testSecretStore{}, RunnerOptions{
		BaseURL: server.URL,
		Now:     time.Now,
	})

	if err := runner.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	if requests := recorder.snapshot(); len(requests) != 0 {
		t.Fatalf("missing secret made %d HTTP requests", len(requests))
	}
	if status := runner.Status(); status.Status != SyncStatusNotConfigured {
		t.Fatalf("unexpected missing-secret status: %#v", status)
	}
}

func TestRunnerRedactsSecretFromHTTPError(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	confirmedEntry(t, ctx, store)
	recorder := &requestRecorder{
		status: http.StatusBadGateway,
	}
	server := newTestServer(recorder)
	defer server.Close()
	runner, secret := newConfiguredRunner(t, store, ctx, server.URL, time.Now)
	recorder.mu.Lock()
	recorder.body = " upstream rejected " + secret + " "
	recorder.mu.Unlock()

	err := runner.SyncNow(ctx)
	if err == nil || !strings.Contains(err.Error(), "502") ||
		strings.Contains(err.Error(), secret) {
		t.Fatal("sync error did not meet status and redaction contract")
	}
	for _, request := range recorder.snapshot() {
		if strings.Contains(request.Method+" "+request.Path+" "+request.Body, secret) {
			t.Fatal("secret appeared in the asserted request log")
		}
	}
}

func TestRunnerReviewDoesNotBlockVocabularyTimer(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	entry := confirmedEntry(t, ctx, store)
	pending, err := store.Outbox().ListPendingVocabulary(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range pending {
		if event.AggregateID == entry.ID && event.EventType != "study.review.recorded" {
			if err := store.Outbox().MarkDelivered(ctx, event.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, _, err := store.Study().RecordReview(ctx, entry.ID, 5, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{}
	server := newTestServer(recorder)
	defer server.Close()
	runner, _ := newConfiguredRunner(t, store, ctx, server.URL, time.Now)

	if err := runner.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(recorder.snapshot()) != 0 {
		t.Fatal("review-only outbox caused an HTTP request")
	}
	if runner.Status().Status != SyncStatusIdle {
		t.Fatalf("review-only outbox did not remain idle: %#v", runner.Status())
	}
}

func TestRunnerReportsUploadablePendingVocabulary(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	runner := NewRunner(store, nil, RunnerOptions{})
	pending, err := runner.HasUploadablePending(ctx)
	if err != nil || pending {
		t.Fatalf("empty outbox reported pending work: %v %v", pending, err)
	}
	candidateEntry(t, ctx, store)
	pending, err = runner.HasUploadablePending(ctx)
	if err != nil || pending {
		t.Fatalf("candidate outbox reported an upload: %v %v", pending, err)
	}
	confirmedEntry(t, ctx, store)
	pending, err = runner.HasUploadablePending(ctx)
	if err != nil || !pending {
		t.Fatalf("confirmed outbox did not report an upload: %v %v", pending, err)
	}
}

func TestRunnerMergeRepublishesConfirmedTargetOnly(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	target := confirmedEntry(t, ctx, store)
	source := candidateEntry(t, ctx, store)
	if _, err := store.Vocabulary().Merge(ctx, source.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{}
	server := newTestServer(recorder)
	defer server.Close()
	runner, _ := newConfiguredRunner(t, store, ctx, server.URL, time.Now)

	if err := runner.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	sawTarget := false
	for _, request := range recorder.snapshot() {
		if request.Method != http.MethodPost || request.Path != "/rest/v1/vocabulary_entries" {
			continue
		}
		if strings.Contains(request.Body, source.ID) {
			t.Fatal("merged source was uploaded")
		}
		if strings.Contains(request.Body, target.ID) {
			sawTarget = true
		}
	}
	if !sawTarget {
		t.Fatal("confirmed merge target was not uploaded")
	}
}

func TestStartPullsConfirmedVocabulary(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	entryID := uuid.NewString()
	senseID := uuid.NewString()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/rest/v1/vocabulary_entries" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[{
				"id":"`+entryID+`",
				"simplified":"电脑",
				"traditional":"電腦",
				"reading":"dian4 nao3",
				"marked_pinyin":"diàn nǎo",
				"status":"confirmed",
				"provenance":"user",
				"confidence":1,
				"teaching_cue":0,
				"manual":true,
				"priority":"manual",
				"merged_into_id":null,
				"created_at":"2026-10-20T00:00:00Z",
				"updated_at":"2026-10-20T00:00:00Z",
				"senses":[{"id":"`+senseID+`","gloss":"computer","part_of_speech":"noun","classifier":"台","sort_order":0}],
				"examples":[],
				"entry_tags":[{"tags":{"id":"`+uuid.NewString()+`","name":"tech"}}]
			}]`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	runner, _ := newConfiguredRunner(t, store, ctx, server.URL, time.Now)
	runner.Start(ctx)
	t.Cleanup(func() { _ = runner.StopWithoutFlush(context.Background()) })

	deadline := time.Now().Add(2 * time.Second)
	for runner.Status().PullStatus != PullStatusIdle {
		if runner.Status().PullStatus == PullStatusError {
			t.Fatal(runner.Status().PullError)
		}
		if time.Now().After(deadline) {
			t.Fatalf("launch pull did not finish: %#v", runner.Status())
		}
		time.Sleep(15 * time.Millisecond)
	}
	if runner.Status().PullChangeCount != 1 {
		t.Fatalf("pull change count = %d", runner.Status().PullChangeCount)
	}
	entry, err := store.Vocabulary().Get(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Senses) != 1 || entry.Senses[0].Gloss != "computer" {
		t.Fatalf("pulled senses = %#v", entry.Senses)
	}
	if len(entry.Tags) != 1 || entry.Tags[0].Name != "tech" {
		t.Fatalf("pulled tags = %#v", entry.Tags)
	}
}

func TestPullKeepsNewerLocalWordAndPendingUpload(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	local := confirmedEntry(t, ctx, store)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/rest/v1/vocabulary_entries" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[{
				"id":"`+local.ID+`",
				"simplified":"`+local.Simplified+`",
				"traditional":"`+local.Traditional+`",
				"reading":"`+local.Reading+`",
				"marked_pinyin":"`+local.MarkedPinyin+`",
				"status":"confirmed",
				"provenance":"user",
				"confidence":1,
				"teaching_cue":0,
				"manual":true,
				"priority":"manual",
				"created_at":"2020-01-01T00:00:00Z",
				"updated_at":"2020-01-01T00:00:00Z",
				"senses":[{"id":"`+uuid.NewString()+`","gloss":"replaced","part_of_speech":"","classifier":"","sort_order":0}],
				"examples":[],
				"entry_tags":[]
			}]`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	runner, _ := newConfiguredRunner(t, store, ctx, server.URL, time.Now)
	if err := runner.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	entry, err := store.Vocabulary().Get(ctx, local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Senses) == 0 || entry.Senses[0].Gloss != "to study" {
		t.Fatalf("newer local sense was replaced: %#v", entry.Senses)
	}
	pending, err := store.Outbox().ListPendingVocabulary(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) == 0 {
		t.Fatal("pending local upload was acknowledged")
	}
}

func TestPullAppliesNewerRemoteWord(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	local := confirmedEntry(t, ctx, store)
	if _, err := store.DB().ExecContext(ctx, `
		UPDATE vocabulary_entries SET updated_at = ? WHERE id = ?`,
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), local.ID,
	); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/rest/v1/vocabulary_entries" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[{
				"id":"`+local.ID+`",
				"simplified":"`+local.Simplified+`",
				"traditional":"`+local.Traditional+`",
				"reading":"`+local.Reading+`",
				"marked_pinyin":"`+local.MarkedPinyin+`",
				"status":"confirmed",
				"provenance":"user",
				"confidence":1,
				"teaching_cue":0,
				"manual":true,
				"priority":"manual",
				"created_at":"2020-01-01T00:00:00Z",
				"updated_at":"2026-10-20T00:00:00Z",
				"senses":[{"id":"`+uuid.NewString()+`","gloss":"replaced","part_of_speech":"","classifier":"","sort_order":0}],
				"examples":[],
				"entry_tags":[]
			}]`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	runner, _ := newConfiguredRunner(t, store, ctx, server.URL, time.Now)
	if err := runner.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	entry, err := store.Vocabulary().Get(ctx, local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Senses) != 1 || entry.Senses[0].Gloss != "replaced" {
		t.Fatalf("newer remote sense was not applied: %#v", entry.Senses)
	}
	pending, err := store.Outbox().ListPendingVocabulary(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range pending {
		if event.AggregateID == local.ID {
			t.Fatal("replaced local word still has a pending upload")
		}
	}
}

func TestPullRemovesDeliveredWordMissingUpstream(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	local := confirmedEntry(t, ctx, store)
	candidate := candidateEntry(t, ctx, store)
	pending, err := store.Outbox().ListPendingVocabulary(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range pending {
		if event.AggregateID != local.ID {
			continue
		}
		if err := store.Outbox().MarkDelivered(ctx, event.ID); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	defer server.Close()
	runner, _ := newConfiguredRunner(t, store, ctx, server.URL, time.Now)
	if err := runner.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Vocabulary().Get(ctx, local.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("delivered word missing upstream remains: %v", err)
	}
	if _, err := store.Vocabulary().Get(ctx, candidate.ID); err != nil {
		t.Fatal(err)
	}
	if runner.Status().PullChangeCount != 1 {
		t.Fatalf("removed count = %d", runner.Status().PullChangeCount)
	}
}

func TestSyncNowDoesNotPull(t *testing.T) {
	store, ctx := newVocabularyStore(t)
	confirmedEntry(t, ctx, store)
	recorder := &requestRecorder{}
	server := newTestServer(recorder)
	defer server.Close()
	runner, _ := newConfiguredRunner(t, store, ctx, server.URL, func() time.Time {
		return time.Now().UTC().Add(11 * time.Minute)
	})
	if err := runner.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	for _, request := range recorder.snapshot() {
		if request.Method == http.MethodGet && strings.Contains(request.Path, "vocabulary_entries") {
			t.Fatalf("sync now pulled vocabulary: %s", request.Path)
		}
	}
	if runner.Status().PullStatus != "" {
		t.Fatalf("sync now set pull status %q", runner.Status().PullStatus)
	}
}

func TestProbeReadWriteChecksVocabularyAndTag(t *testing.T) {
	var sawRead, sawWrite, sawDelete bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/rest/v1/vocabulary_entries":
			sawRead = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[]`)
		case req.Method == http.MethodPost && req.URL.Path == "/rest/v1/tags":
			sawWrite = true
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), accessProbeTagName) {
				t.Errorf("probe tag missing from %s", body)
			}
			w.WriteHeader(http.StatusNoContent)
		case req.Method == http.MethodDelete && req.URL.Path == "/rest/v1/tags":
			sawDelete = true
			if !strings.Contains(req.URL.RawQuery, "name=eq."+accessProbeTagName) {
				t.Errorf("delete query = %s", req.URL.RawQuery)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := NewPostgRESTClient(server.URL, "fixture-secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ProbeReadWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !sawRead || !sawWrite || !sawDelete {
		t.Fatalf("read=%v write=%v delete=%v", sawRead, sawWrite, sawDelete)
	}
}

func TestProbeReadWriteRejectsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"bad"}`)
	}))
	defer server.Close()
	client, err := NewPostgRESTClient(server.URL, "fixture-secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	err = client.ProbeReadWrite(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatal(err)
	}
}
