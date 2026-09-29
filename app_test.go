package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/capture"
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
