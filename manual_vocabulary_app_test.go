package main

import (
	"context"
	"testing"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/service"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

func TestSaveManualVocabularyReturnsUpdatedSnapshot(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := NewAppWithDependencies(
		store,
		capture.NewMockSource(),
		service.NewMemorySecretStore(),
		nil,
	)
	if err := app.service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}

	snapshot, err := app.SaveManualVocabulary(ManualVocabularyInput{
		Simplified:         "你好",
		Pinyin:             "nǐ hǎo",
		Meaning:            "hello",
		PartOfSpeech:       "greeting",
		Example:            "你好，老师！",
		ExamplePinyin:      "nǐ hǎo lǎo shī",
		ExampleTranslation: "Hello, teacher!",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Vocabulary) != 1 {
		t.Fatalf("save did not return updated vocabulary: %#v", snapshot.Vocabulary)
	}
	if snapshot.Vocabulary[0].Simplified != "你好" ||
		snapshot.Vocabulary[0].Pinyin != "nǐ hǎo" ||
		snapshot.Vocabulary[0].ExampleTranslation != "Hello, teacher!" {
		t.Fatalf("unexpected vocabulary snapshot: %#v", snapshot.Vocabulary[0])
	}
}
