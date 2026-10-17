package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

func TestRecognizeVocabularyPhotoParsesEntriesAndRecordsUsage(t *testing.T) {
	const generatedResponse = `{"entries":[
		{"simplified":"学习","traditional":"學習","pinyin":"xué xí","meaning":"to study","partOfSpeech":"verb","classifier":"","example":"我学习中文。","examplePinyin":"wǒ xué xí zhōng wén","exampleTranslation":"I study Chinese.","tags":["study"]},
		{"simplified":"你好","traditional":"你好","pinyin":"nǐ hǎo","meaning":"hello","partOfSpeech":"greeting","classifier":"","example":"你好！","examplePinyin":"nǐ hǎo","exampleTranslation":"Hello!","tags":["greeting"]}
	]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode vision request: %v", err)
			return
		}
		if payload["model"] != openrouter.DefaultVisionModel {
			t.Errorf("unexpected vision model: %#v", payload["model"])
		}
		for _, field := range []string{"zdr", "provider", "data_collection"} {
			if _, ok := payload[field]; ok {
				t.Errorf("vision request included lesson privacy field %q: %#v", field, payload)
			}
		}
		format, _ := payload["response_format"].(map[string]any)
		schema, _ := format["json_schema"].(map[string]any)
		photoSchema, _ := schema["schema"].(map[string]any)
		entriesSchema, _ := photoSchema["properties"].(map[string]any)
		entries, _ := entriesSchema["entries"].(map[string]any)
		if entries["maxItems"] != float64(12) {
			t.Errorf("photo schema did not cap entries: %#v", entries)
		}
		messages, _ := payload["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("unexpected vision messages: %#v", payload["messages"])
		}
		user, _ := messages[1].(map[string]any)
		parts, _ := user["content"].([]any)
		if len(parts) != 2 {
			t.Fatalf("unexpected user content: %#v", user["content"])
		}
		textPart, _ := parts[0].(map[string]any)
		text, _ := textPart["text"].(string)
		if !strings.Contains(text, "at most 12") {
			t.Errorf("photo prompt did not mention the 12-entry cap: %q", text)
		}
		if !strings.Contains(text, "do not turn a whole sentence into the headword") {
			t.Errorf("photo prompt did not discourage sentence headwords: %q", text)
		}
		imagePart, _ := parts[1].(map[string]any)
		imageURL, _ := imagePart["image_url"].(map[string]any)
		if !strings.HasPrefix(imageURL["url"].(string), "data:image/jpeg;base64,") {
			t.Fatalf("missing JPEG data URL: %#v", imagePart)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` +
			strconvQuote(generatedResponse) +
			`}}],"usage":{"prompt_tokens":11,"completion_tokens":19,"total_tokens":30,"cost":0.0123}}`))
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newPhotoVocabularyTestService(t, server.URL)
	defer store.Close()
	svc.mu.Lock()
	svc.currentLesson = "live-lesson"
	svc.mu.Unlock()

	draft, err := svc.RecognizeVocabularyPhoto(ctx, validVocabularyPhotoJPEG())
	if err != nil {
		t.Fatal(err)
	}
	if draft.Model != openrouter.DefaultVisionModel || draft.Cost != 0.0123 {
		t.Fatalf("unexpected photo draft metadata: %#v", draft)
	}
	if len(draft.Entries) != 2 {
		t.Fatalf("expected two photo entries, got %#v", draft.Entries)
	}
	if draft.Entries[0].Simplified != "学习" ||
		draft.Entries[0].Traditional != "學習" ||
		draft.Entries[0].Pinyin != "xué xí" ||
		!draft.Entries[0].AiGenerated ||
		len(draft.Entries[0].Tags) != 0 ||
		len(draft.Entries[1].Tags) != 0 ||
		draft.Entries[1].Simplified != "你好" {
		t.Fatalf("photo entries were not normalized: %#v", draft.Entries)
	}
	usages, err := store.Usage().ListBySession(ctx, PhotoVocabularySessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 ||
		usages[0].Model != openrouter.DefaultVisionModel ||
		usages[0].Endpoint != "/api/v1/chat/completions" ||
		usages[0].PromptTokens != 11 ||
		usages[0].CompletionTokens != 19 ||
		usages[0].Cost != 0.0123 {
		t.Fatalf("photo usage was not recorded: %#v", usages)
	}
	vocabulary, err := store.Vocabulary().ListAll(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(vocabulary) != 0 {
		t.Fatalf("photo recognition persisted vocabulary: %#v", vocabulary)
	}
}

func TestParsePhotoVocabularyJSONCapsHanEntries(t *testing.T) {
	words := []string{
		"学习",
		"你好",
		"计划",
		"方案",
		"会议",
		"重点",
		"工作",
		"时间",
		"问题",
		"结果",
		"意见",
		"建议",
		"例子",
	}
	entry := func(word string) manualVocabularyModelResponse {
		return manualVocabularyModelResponse{
			Simplified:         word,
			Traditional:        word,
			Pinyin:             "xué xí",
			Meaning:            "meaning",
			PartOfSpeech:       "noun",
			Classifier:         "",
			Example:            "这是一个例子。",
			ExamplePinyin:      "zhè shì yí gè lì zi",
			ExampleTranslation: "This is an example.",
			Tags:               []string{"ignored"},
		}
	}
	entries := make([]manualVocabularyModelResponse, 0, len(words)+1)
	for index, word := range words {
		if index == 5 {
			entries = append(entries, entry("hello"))
		}
		entries = append(entries, entry(word))
	}
	payload, err := json.Marshal(photoVocabularyModelResponse{Entries: entries})
	if err != nil {
		t.Fatal(err)
	}

	got, err := parsePhotoVocabularyJSON(string(payload))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 12 {
		t.Fatalf("expected 12 Han entries, got %d: %#v", len(got), got)
	}
	for index, want := range words[:12] {
		if got[index].Simplified != want {
			t.Fatalf("entry %d = %q, want %q; got %#v", index, got[index].Simplified, want, got)
		}
	}
}

func TestRecognizeVocabularyPhotoRejectsInvalidJPEG(t *testing.T) {
	svc := New(nil, nil, NewMemorySecretStore(), nil)
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "empty", data: nil, want: "empty"},
		{name: "non-jpeg", data: []byte("not an image"), want: "JPEG"},
		{
			name: "oversize",
			data: append(validVocabularyPhotoJPEG(), make([]byte, maxVocabularyPhotoJPEGBytes)...),
			want: "1.5 MiB",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := svc.RecognizeVocabularyPhoto(context.Background(), test.data)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unexpected JPEG validation error: %v", err)
			}
		})
	}
}

func TestRecognizeVocabularyPhotoEmptyRecognitionRecordsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"entries\":[]}"}}],"usage":{"cost":0.004}}`))
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newPhotoVocabularyTestService(t, server.URL)
	defer store.Close()
	_, err := svc.RecognizeVocabularyPhoto(ctx, validVocabularyPhotoJPEG())
	if err == nil || !strings.Contains(err.Error(), "no Chinese words") {
		t.Fatalf("empty recognition did not return a clear error: %v", err)
	}
	usages, err := store.Usage().ListBySession(ctx, PhotoVocabularySessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0].Cost != 0.004 {
		t.Fatalf("empty recognition usage was not recorded: %#v", usages)
	}
}

func TestRecognizeVocabularyPhotoRecordsUsageWhenCallFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":{"message":"invalid request"},"usage":{"cost":0.006}}`))
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newPhotoVocabularyTestService(t, server.URL)
	defer store.Close()
	_, err := svc.RecognizeVocabularyPhoto(ctx, validVocabularyPhotoJPEG())
	if err == nil || !strings.Contains(err.Error(), "recognition failed") {
		t.Fatalf("failed photo call did not return an error: %v", err)
	}
	usages, err := store.Usage().ListBySession(ctx, PhotoVocabularySessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0].Cost != 0.006 {
		t.Fatalf("failed photo usage was not recorded: %#v", usages)
	}
}

func TestVisionModelSettingsDefaultAndValidation(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Settings().Set(ctx, settingVisionModel, `"unknown/model"`); err != nil {
		t.Fatal(err)
	}
	svc := New(store, capture.NewMockSource(), NewMemorySecretStore(), nil)
	if err := svc.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	settings, err := svc.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.VisionModel != openrouter.DefaultVisionModel {
		t.Fatalf("unknown vision model was not reset: %#v", settings)
	}
	persisted, err := store.Settings().Get(ctx, settingVisionModel)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ValueJSON != `"`+openrouter.DefaultVisionModel+`"` {
		t.Fatalf("default vision model was not persisted: %s", persisted.ValueJSON)
	}
	invalid := "not-allowed"
	if err := svc.ApplySettings(ctx, SettingsPatch{VisionModel: &invalid}); err == nil ||
		!strings.Contains(err.Error(), "vision model") {
		t.Fatalf("invalid vision model was accepted: %v", err)
	}
}

func newPhotoVocabularyTestService(
	t *testing.T,
	serverURL string,
) (*storage.Store, *Service) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	secrets := NewMemorySecretStore()
	if err := secrets.Set(ctx, secretOpenRouterAPIKey, "photo-test-key"); err != nil {
		store.Close()
		t.Fatal(err)
	}
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL: serverURL,
	}, APIKeySecretProvider{Store: secrets, Name: secretOpenRouterAPIKey})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	svc := New(store, capture.NewMockSource(), secrets, router)
	if err := svc.Initialize(ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}
	return store, svc
}

func validVocabularyPhotoJPEG() []byte {
	return []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0xff, 0xd9}
}

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
