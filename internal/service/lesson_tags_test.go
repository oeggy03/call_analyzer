package service

import (
	"context"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/storage"
	"github.com/oeggy03/call_analyzer/internal/vocabulary"
)

func TestNormalizeLessonTags(t *testing.T) {
	at := time.Date(2026, 10, 11, 18, 53, 40, 0, time.Local)
	if got := DefaultLessonTag(at); got != "lesson_2026-10-11_18-53" {
		t.Fatalf("default tag = %q", got)
	}
	if got := NormalizeLessonTags("   ", at); got != "lesson_2026-10-11_18-53" {
		t.Fatalf("blank tag = %q", got)
	}
	got := NormalizeLessonTags("  office , lesson_2026-10-11_18-53 , office ", at)
	if got != "office,lesson_2026-10-11_18-53" {
		t.Fatalf("normalized tags = %q", got)
	}
	long := NormalizeLessonTags("one,two,three,four,five,six,seven,eight,nine", at)
	if long != "one,two,three,four,five,six,seven,eight" {
		t.Fatalf("tag cap = %q", long)
	}
}

func TestAutomaticExtractionAppliesLessonTags(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lesson, err := store.Lessons().StartWithMetadata(ctx, "lesson", time.Time{}, storage.LessonMetadata{
		LessonTags: "lesson_2026-10-11_18-53,office",
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Lessons().Get(ctx, lesson.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.LessonTags != "lesson_2026-10-11_18-53,office" {
		t.Fatalf("stored tags = %q", saved.LessonTags)
	}

	vocabularyService := NewVocabularyService(store, vocabulary.NewEmbeddedDictionary())
	candidates, err := vocabularyService.ProcessExtraction(ctx, lesson.ID, "", "你好，谢谢", vocabulary.Extraction{
		Candidates: []vocabulary.Candidate{{
			Simplified: "谢谢",
			Reading:    "xie4 xie5",
			Evidence:   []domain.Evidence{{Text: "谢谢"}},
			Confidence: .9,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified: "你好",
		Reading:    "ni3 hao3",
		Evidence:   []domain.Evidence{{LessonID: lesson.ID, Text: "你好"}},
		Confidence: .9,
		Senses:     []domain.Sense{{Gloss: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err = store.Vocabulary().Confirm(ctx, confirmed.ID)
	if err != nil {
		t.Fatal(err)
	}

	svc := &Service{store: store}
	tagged, err := svc.tagAutomaticEntries(ctx, lesson.ID, append(candidates, confirmed))
	if err != nil {
		t.Fatal(err)
	}
	if len(tagged) != 2 {
		t.Fatalf("tagged entries = %#v", tagged)
	}
	if len(tagged[0].Tags) != 2 || tagged[0].Tags[0].Name != "lesson_2026-10-11_18-53" || tagged[0].Tags[1].Name != "office" {
		t.Fatalf("candidate tags = %#v", tagged[0].Tags)
	}
	if len(tagged[1].Tags) != 0 {
		t.Fatalf("confirmed vocabulary was retagged: %#v", tagged[1].Tags)
	}

	again, err := svc.tagAutomaticEntries(ctx, lesson.ID, tagged[:1])
	if err != nil {
		t.Fatal(err)
	}
	if len(again[0].Tags) != 2 {
		t.Fatalf("repeated tagging duplicated tags: %#v", again[0].Tags)
	}
}
