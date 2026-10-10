package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/service"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

func TestRecognizeVocabularyPhotoDecodesDataURLAndMapsDraft(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"entries\":[{\"simplified\":\"你好\",\"traditional\":\"你好\",\"pinyin\":\"nǐ hǎo\",\"meaning\":\"hello\",\"partOfSpeech\":\"greeting\",\"classifier\":\"\",\"example\":\"你好！\",\"examplePinyin\":\"nǐ hǎo\",\"exampleTranslation\":\"Hello!\",\"tags\":[\"greeting\"]}]}"}}],"usage":{"cost":0.009}}`))
	}))
	defer server.Close()

	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secrets := service.NewMemorySecretStore()
	if err := secrets.Set(ctx, "OPENROUTER_API_KEY", "app-photo-key"); err != nil {
		t.Fatal(err)
	}
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL: server.URL,
	}, service.APIKeySecretProvider{Store: secrets, Name: "OPENROUTER_API_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	app := NewAppWithDependencies(store, capture.NewMockSource(), secrets, router)
	if err := app.service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}

	jpeg := []byte{0xff, 0xd8, 0xff, 0x01}
	encoded := base64.StdEncoding.EncodeToString(jpeg)
	draft, err := app.RecognizeVocabularyPhoto(
		" \n data:image/jpeg;base64, " + encoded + " \n",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Entries) != 1 ||
		draft.Entries[0].Simplified != "你好" ||
		!draft.Entries[0].AiGenerated ||
		draft.Model != openrouter.DefaultVisionModel ||
		draft.Cost != 0.009 {
		t.Fatalf("unexpected Wails photo draft: %#v", draft)
	}
}

func TestRecognizeVocabularyPhotoRejectsInvalidBase64(t *testing.T) {
	app := &App{}
	_, err := app.RecognizeVocabularyPhoto("data:image/jpeg;base64,not base64!")
	if err == nil || !strings.Contains(err.Error(), "invalid image base64") {
		t.Fatalf("invalid base64 was accepted: %v", err)
	}
}
