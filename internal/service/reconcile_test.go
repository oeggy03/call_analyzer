package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

func TestTranscriptWindowsUseThirtySecondsAndFiveSecondOverlap(t *testing.T) {
	segments := []domain.TranscriptSegment{
		{StartMS: 0, EndMS: 30_000, Text: "一"},
		{StartMS: 30_000, EndMS: 60_000, Text: "二"},
		{StartMS: 60_000, EndMS: 65_000, Text: "三"},
	}
	windows := transcriptWindows(segments)
	if len(windows) != 3 {
		t.Fatalf("expected three reconciliation windows, got %#v", windows)
	}
	if windows[0].StartMS != 0 || windows[0].EndMS != 30_000 ||
		windows[1].StartMS != 25_000 || windows[1].EndMS != 55_000 {
		t.Fatalf("unexpected reconciliation bounds %#v", windows)
	}
	if !strings.Contains(windows[1].Text, "二") || !strings.Contains(windows[2].Text, "三") {
		t.Fatalf("overlapping windows lost transcript text %#v", windows)
	}
}

func TestStopReconcilesMissedVocabularyWithoutReinsertingTranscript(t *testing.T) {
	var reconciliationCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/audio/transcriptions":
			_, _ = w.Write([]byte(`{"text":"错过词"}`))
		case "/api/v1/chat/completions":
			var payload struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode chat request: %v", err)
				return
			}
			prompt := ""
			if len(payload.Messages) > 0 {
				prompt = payload.Messages[len(payload.Messages)-1].Content
			}
			if strings.Contains(prompt, "post-lesson reconciliation") {
				reconciliationCalls.Add(1)
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"candidates\":[{\"simplified\":\"错过\",\"traditional\":\"錯過\",\"reading\":\"cuo4 guo4\",\"evidence\":[{\"text\":\"错过\"}],\"confidence\":0.9,\"senses\":[],\"examples\":[]}]}"}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"candidates\":[]}"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secrets := NewMemorySecretStore()
	if err := secrets.Set(ctx, secretOpenRouterAPIKey, "test-key"); err != nil {
		t.Fatal(err)
	}
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL:    server.URL,
		MaxRetries: 1,
		RetryBase:  time.Millisecond,
	}, APIKeySecretProvider{Store: secrets, Name: secretOpenRouterAPIKey})
	if err != nil {
		t.Fatal(err)
	}
	source := capture.NewMockSource()
	svc := New(store, source, secrets, router)
	if err := svc.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	lesson, err := svc.StartLesson(ctx, "lesson", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.StartCapture(ctx, lesson.ID); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC()
	for i := 0; i < 8; i++ {
		if err := source.Feed(capture.AudioFrame{
			Timestamp:  base.Add(time.Duration(i) * 2 * time.Second),
			Samples:    []int16{1_000, 1_000},
			SampleRate: 1,
			Channels:   1,
			Source:     "remote",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.StopAndEndCurrentLesson(ctx); err != nil {
		t.Fatal(err)
	}
	if reconciliationCalls.Load() == 0 {
		t.Fatal("post-lesson reconciliation did not run")
	}
	entries, err := svc.ListCandidates(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Simplified != "错过" {
		t.Fatalf("missed vocabulary was not recovered: %#v", entries)
	}
	var transcriptCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM transcript_segments WHERE lesson_id = ?", lesson.ID,
	).Scan(&transcriptCount); err != nil {
		t.Fatal(err)
	}
	if transcriptCount != 1 {
		t.Fatalf("reconciliation reinserted transcript text: %d segments", transcriptCount)
	}
}

func TestTranscriptWindowsCoverEntireLongLesson(t *testing.T) {
	segments := make([]domain.TranscriptSegment, 0, 120)
	for minute := int64(0); minute < 60; minute++ {
		start := minute * 60_000
		segments = append(segments, domain.TranscriptSegment{
			StartMS: start,
			EndMS:   start + 10_000,
			Text:    fmt.Sprintf("第%d分钟", minute+1),
		})
	}
	windows := transcriptWindows(segments)
	if len(windows) <= 24 {
		t.Fatalf("long lesson was truncated to %d reconciliation windows", len(windows))
	}
	last := windows[len(windows)-1]
	if last.StartMS > segments[len(segments)-1].StartMS ||
		last.EndMS < segments[len(segments)-1].EndMS {
		t.Fatalf("last lesson segment is not covered: window=%+v segment=%+v", last, segments[len(segments)-1])
	}
}
