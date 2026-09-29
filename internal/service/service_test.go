package service

import (
	"context"
	"strings"
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
	svc.mu.RLock()
	ringCapacity := svc.ring.Capacity()
	svc.mu.RUnlock()
	if ringCapacity != 60*time.Second {
		t.Fatalf("manual-mark ring capacity = %s, want 60s", ringCapacity)
	}
	if _, err := svc.MarkMoment(ctx, "interesting"); err != nil {
		t.Fatal(err)
	}
	markedTranscript, err := svc.ListTranscript(ctx, lesson.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(markedTranscript) != 1 || markedTranscript[0].Source != "system/mark" {
		t.Fatalf("manual mark was not persisted in the lesson timeline: %#v", markedTranscript)
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

func TestVocabularyExtractionKeepsValidCandidatesWhenOneIsMalformed(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lesson, err := store.Lessons().Start(ctx, "lesson", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := NewVocabularyService(store, vocabulary.NewEmbeddedDictionary()).
		ProcessExtraction(ctx, lesson.ID, "", "你好，谢谢", vocabulary.Extraction{
			Candidates: []vocabulary.Candidate{
				{
					Simplified: "坏", Reading: "not-pinyin!",
					Evidence: []domain.Evidence{{Text: "你好"}}, Confidence: .8,
				},
				{
					Simplified: "谢谢", Reading: "xie4 xie5",
					Evidence: []domain.Evidence{{Text: "谢谢"}}, Confidence: .9,
				},
			},
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Simplified != "谢谢" {
		t.Fatalf("valid candidate was lost with malformed peer: %#v", entries)
	}
}

func TestExtractionPopulatesEvidenceFromTranscriptWindow(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lesson, err := store.Lessons().Start(ctx, "lesson", time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	segment, err := store.Transcripts().Insert(ctx, domain.TranscriptSegment{
		LessonID:   lesson.ID,
		StartMS:    8_000,
		EndMS:      12_000,
		Text:       "我们今天学习中文",
		Confidence: .87,
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := NewVocabularyService(store, nil).ProcessExtraction(
		ctx,
		lesson.ID,
		segment.ID,
		segment.Text,
		vocabulary.Extraction{Candidates: []vocabulary.Candidate{{
			Simplified: "学习",
			Reading:    "xue2 xi2",
			Evidence:   []domain.Evidence{{Text: "学习"}},
			Confidence: .9,
		}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || len(entries[0].Evidence) != 1 {
		t.Fatalf("unexpected extraction %#v", entries)
	}
	evidence := entries[0].Evidence[0]
	if evidence.LessonID != lesson.ID || evidence.SegmentID != segment.ID ||
		evidence.StartMS != segment.StartMS || evidence.EndMS != segment.EndMS ||
		evidence.Confidence != segment.Confidence {
		t.Fatalf("evidence window was not populated: %#v", evidence)
	}
}

func TestNativeCaptureErrorsAreExposedOnLesson(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := capture.NewMockSource()
	secrets := NewMemorySecretStore()
	if err := secrets.Set(ctx, secretOpenRouterAPIKey, "test-key"); err != nil {
		t.Fatal(err)
	}
	svc := New(store, source, secrets, nil)
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
	diagnostics := make(chan struct{}, 1)
	svc.SetEventHook(func(name string, _ any) {
		if name == "diagnostic" {
			diagnostics <- struct{}{}
		}
	})
	if err := source.FeedEvent(capture.Event{
		Kind: "error",
		Text: "Screen Recording permission was lost",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-diagnostics:
	case <-time.After(time.Second):
		t.Fatal("native error was not surfaced")
	}
	if !strings.Contains(svc.LessonError(), "Screen Recording") {
		t.Fatalf("lesson error did not retain native error: %q", svc.LessonError())
	}
	if err := svc.StopCapture(ctx); err != nil {
		t.Fatal(err)
	}
	if got, running, err := svc.CurrentLesson(ctx); err != nil || running || got.ID != lesson.ID {
		t.Fatalf("last lesson was not retained: %#v running=%v err=%v", got, running, err)
	}
}

func TestDeleteLastLessonRefusesLiveAndClearsReferences(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := capture.NewMockSource()
	secrets := NewMemorySecretStore()
	if err := secrets.Set(ctx, secretOpenRouterAPIKey, "test-key"); err != nil {
		t.Fatal(err)
	}
	svc := New(store, source, secrets, nil)
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
	if err := svc.DeleteLastLesson(ctx); err == nil {
		t.Fatal("expected live lesson deletion to be refused")
	}
	if err := svc.StopCapture(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EndLesson(ctx, lesson.ID, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteLastLesson(ctx); err != nil {
		t.Fatal(err)
	}
	if got, running, err := svc.CurrentLesson(ctx); err != nil || running || got.ID != "" {
		t.Fatalf("deleted lesson still referenced: %#v running=%v err=%v", got, running, err)
	}
}
