package service

import (
	"context"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/storage"
	"github.com/oeggy03/call_analyzer/internal/vocabulary"
)

func TestServiceSecretsCaptureAndExtraction(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := capture.NewMockSource()
	secrets := NewMemorySecretStore()
	svc := New(store, source, secrets, nil)
	if err := svc.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAPIKey(ctx, "secret-value"); err != nil {
		t.Fatal(err)
	}
	if !svc.HasAPIKey(ctx) || !svc.Config(ctx).HasAPIKey {
		t.Fatal("API key should be reported as configured")
	}
	var persistedSecrets int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM settings WHERE value_json LIKE '%secret-value%'",
	).Scan(&persistedSecrets); err != nil {
		t.Fatal(err)
	}
	if persistedSecrets != 0 {
		t.Fatal("API key was persisted in SQLite")
	}

	lesson, err := svc.StartLesson(ctx, "lesson", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.StartCapture(ctx, lesson.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MarkMoment(ctx, "interesting"); err != nil {
		t.Fatal(err)
	}
	if err := source.Feed(capture.AudioFrame{
		Timestamp:  time.Now().UTC(),
		Samples:    []int16{1, 2, 3, 4},
		SampleRate: 4,
		Channels:   1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.StopCapture(ctx); err != nil {
		t.Fatal(err)
	}

	vocabularyService := NewVocabularyService(store, vocabulary.NewEmbeddedDictionary())
	parsed, err := ParseExtractionJSON(`{"candidates":[{"simplified":"你好","traditional":"你好","reading":"ni3 hao3","evidence":[{"text":"你好"}],"confidence":0.9}]}`)
	if err != nil || len(parsed.Candidates) != 1 {
		t.Fatalf("unexpected strict extraction parse %#v err=%v", parsed, err)
	}
	entries, err := vocabularyService.ProcessExtraction(ctx, lesson.ID, "", "你好", vocabulary.Extraction{
		Candidates: []vocabulary.Candidate{{
			Simplified: "你好",
			Reading:    "ni3 hao3",
			Evidence:   []domain.Evidence{{Text: "你好"}},
			Confidence: .9,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Status != domain.VocabularyStatusCandidate {
		t.Fatalf("unexpected extraction result %#v", entries)
	}
}
