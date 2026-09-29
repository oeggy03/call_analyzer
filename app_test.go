package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/service"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

func TestAppCapturePipelineContract(t *testing.T) {
	var transcriptions atomic.Int32
	var chats atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/audio/transcriptions":
			transcriptions.Add(1)
			_, _ = w.Write([]byte(`{"text":"你好","usage":{"cost":0.01}}`))
		case "/api/v1/chat/completions":
			chats.Add(1)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode chat request: %v", err)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"candidates\":[{\"simplified\":\"你好\",\"traditional\":\"你好\",\"reading\":\"ni3 hao3\",\"evidence\":[{\"text\":\"你好\"}],\"confidence\":0.95,\"senses\":[{\"gloss\":\"hello\",\"partOfSpeech\":\"greeting\",\"classifier\":\"\"}],\"examples\":[{\"simplified\":\"老师，你好！\",\"traditional\":\"老師，你好！\",\"reading\":\"lao3 shi1 ni3 hao3\",\"translation\":\"Hello, teacher!\",\"generated\":true}]}]}"}}],"usage":{"cost":0.01}}`))
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
	source := capture.NewMockSource()
	secrets := service.NewMemorySecretStore()
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL:    server.URL,
		MaxRetries: 1,
		RetryBase:  time.Millisecond,
	}, service.APIKeySecretProvider{Store: secrets, Name: "OPENROUTER_API_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	app := NewAppWithDependencies(store, source, secrets, router)
	if err := app.service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	apiKey := "test-key"
	ocrEnabled := true
	if _, err := app.SaveSettings(SettingsPatch{
		OpenRouterKey: &apiKey,
		OCREnabled:    &ocrEnabled,
	}); err != nil {
		t.Fatal(err)
	}
	started, err := app.StartLesson(StartLessonInput{Target: "zoom", Consent: true})
	if err != nil {
		t.Fatal(err)
	}
	if started.Lesson.Status != "live" {
		t.Fatalf("lesson did not start: %#v", started.Lesson)
	}

	base := time.Now().UTC()
	if err := source.FeedEvent(capture.Event{
		Kind:      "ocr",
		Source:    "ocr",
		Timestamp: base,
		Text:      "你好",
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		if err := source.Feed(capture.AudioFrame{
			Timestamp:  base.Add(time.Duration(i) * time.Second),
			Samples:    []int16{1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000},
			SampleRate: 10,
			Channels:   1,
			Source:     "microphone",
		}); err != nil {
			t.Fatal(err)
		}
	}
	stopped, err := app.StopLesson()
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Lesson.Status != "idle" {
		t.Fatalf("lesson did not stop: %#v", stopped.Lesson)
	}
	if transcriptions.Load() == 0 || chats.Load() == 0 {
		t.Fatalf("pipeline did not call OpenRouter: transcriptions=%d chats=%d", transcriptions.Load(), chats.Load())
	}
	if len(stopped.Transcript) == 0 || stopped.Transcript[0].Text != "你好" {
		t.Fatalf("transcript was not persisted: %#v", stopped.Transcript)
	}
	foundOCR := false
	for _, line := range stopped.Transcript {
		foundOCR = foundOCR || line.Source == "system/ocr"
	}
	if !foundOCR {
		t.Fatalf("OCR event was not fused into transcript: %#v", stopped.Transcript)
	}
	if len(stopped.Candidates) == 0 || stopped.Candidates[0].Bucket != "highConfidence" {
		t.Fatalf("candidate was not extracted: %#v", stopped.Candidates)
	}
	if stopped.Candidates[0].Meaning != "hello" ||
		stopped.Candidates[0].ExampleTranslation != "Hello, teacher!" {
		t.Fatalf("candidate enrichment was not persisted: %#v", stopped.Candidates[0])
	}
	if stopped.Settings.STTModel != openrouter.DefaultASRModel ||
		stopped.Settings.AnalyzerModel != openrouter.DefaultChatModel {
		t.Fatalf("unexpected default models: %#v", stopped.Settings)
	}
}

func TestAppContractRejectsMissingConsent(t *testing.T) {
	store, err := storage.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := NewAppWithDependencies(store, capture.NewMockSource(), service.NewMemorySecretStore(), nil)
	if _, err := app.StartLesson(StartLessonInput{Target: "zoom", Consent: false}); err == nil {
		t.Fatal("expected consent validation error")
	}
}

type unavailableZoomSource struct {
	*capture.MockSource
}

func (s unavailableZoomSource) ListTargets(context.Context) ([]capture.Target, error) {
	return []capture.Target{
		{ID: "us.zoom.xos", Name: "Zoom", Available: false},
	}, nil
}

func TestStartLessonPreflightsKeyAndZoomBeforePersistence(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secrets := service.NewMemorySecretStore()
	source := unavailableZoomSource{MockSource: capture.NewMockSource()}
	app := NewAppWithDependencies(store, source, secrets, nil)

	if _, err := app.StartLesson(StartLessonInput{Target: "zoom", Consent: true}); err == nil ||
		!strings.Contains(err.Error(), "API key") {
		t.Fatalf("expected missing-key preflight error, got %v", err)
	}
	var lessons int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM lessons").Scan(&lessons); err != nil {
		t.Fatal(err)
	}
	if lessons != 0 {
		t.Fatalf("missing-key preflight created %d lessons", lessons)
	}

	if err := secrets.Set(ctx, "OPENROUTER_API_KEY", "test-key"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartLesson(StartLessonInput{Target: "zoom", Consent: true}); err == nil ||
		!strings.Contains(err.Error(), "open Zoom") {
		t.Fatalf("expected unavailable Zoom preflight error, got %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM lessons").Scan(&lessons); err != nil {
		t.Fatal(err)
	}
	if lessons != 0 {
		t.Fatalf("unavailable-target preflight created %d lessons", lessons)
	}
}

func TestSnapshotProjectsLiveCostAtOneHourRate(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secrets := service.NewMemorySecretStore()
	if err := secrets.Set(ctx, "OPENROUTER_API_KEY", "test-key"); err != nil {
		t.Fatal(err)
	}
	source := capture.NewMockSource()
	app := NewAppWithDependencies(store, source, secrets, nil)
	if err := app.service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	app.service.SetTarget("zoom")
	startedAt := time.Now().UTC().Add(-30 * time.Minute)
	lesson, err := app.service.StartLessonWithMetadata(ctx, "lesson", startedAt, storage.LessonMetadata{
		ConsentRecorded: true,
		Target:          "zoom",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.service.StartCapture(ctx, lesson.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.service.RecordUsage(ctx, domain.RequestUsage{
		SessionID: app.service.CurrentSessionID(),
		Provider:  "openrouter",
		Model:     "test",
		Cost:      0.10,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := app.GetAppSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Lesson.ID != lesson.ID || snapshot.Lesson.Status != "live" {
		t.Fatalf("lesson identity/status missing from snapshot: %#v", snapshot.Lesson)
	}
	if snapshot.Cost.CurrentUSD != .10 ||
		snapshot.Cost.ProjectedUSD < snapshot.Cost.CurrentUSD ||
		snapshot.Cost.ProjectedUSD < .15 ||
		snapshot.Cost.Warning || snapshot.Cost.HardExceeded {
		t.Fatalf("unexpected live cost summary: %#v", snapshot.Cost)
	}
	if err := app.service.StopCapture(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := app.service.EndLesson(ctx, lesson.ID, time.Time{}); err != nil {
		t.Fatal(err)
	}
}
