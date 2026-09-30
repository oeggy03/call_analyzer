package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

type manualTestKeyProvider struct{}

func (manualTestKeyProvider) APIKey(context.Context) (string, error) {
	return "manual-test-key", nil
}

func TestGenerateManualVocabularyUsesStrictModelAndPreservesUserValues(t *testing.T) {
	const generatedResponse = `{"simplified":"学习","traditional":"学习","pinyin":"xue2 xi2","meaning":"generated meaning","partOfSpeech":"verb","classifier":"个","example":"generated example","examplePinyin":"generated pinyin","exampleTranslation":"generated translation","tags":["generated"]}`
	var requestPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&requestPayload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]string{"content": generatedResponse},
			}},
			"usage": map[string]any{
				"prompt_tokens":     17,
				"completion_tokens": 23,
				"total_tokens":      40,
				"cost":              0.0123,
			},
		})
	}))
	defer server.Close()

	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL:    server.URL,
		MaxRetries: 1,
	}, manualTestKeyProvider{})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(store, capture.NewMockSource(), NewMemorySecretStore(), router)
	if err := svc.Initialize(ctx); err != nil {
		t.Fatal(err)
	}

	draft, err := svc.GenerateManualVocabulary(ctx, ManualVocabularyInput{
		Simplified:         " 学习 ",
		Traditional:        "學習",
		Pinyin:             "xue2 xi2",
		Meaning:            "to study",
		PartOfSpeech:       "verb",
		Classifier:         "门",
		Example:            "我学习中文。",
		ExamplePinyin:      "wo3 xue2 zhong1 wen2",
		ExampleTranslation: "I study Chinese.",
		Tags:               []string{"user-tag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Model != openrouter.DefaultChatModel || draft.Cost != 0.0123 {
		t.Fatalf("unexpected model metadata: %#v", draft)
	}
	if draft.Simplified != "学习" ||
		draft.Traditional != "學習" ||
		draft.Pinyin != "xue2 xi2" ||
		draft.Meaning != "to study" ||
		draft.PartOfSpeech != "verb" ||
		draft.Classifier != "门" ||
		draft.Example != "我学习中文。" ||
		draft.ExamplePinyin != "wo3 xue2 zhong1 wen2" ||
		draft.ExampleTranslation != "I study Chinese." ||
		len(draft.Tags) != 1 ||
		draft.Tags[0] != "user-tag" {
		t.Fatalf("user values were not preserved: %#v", draft)
	}
	if draft.AiGenerated {
		t.Fatal("fully user-provided example was marked generated")
	}
	if requestPayload["model"] != openrouter.DefaultChatModel {
		t.Fatalf("unexpected request model: %#v", requestPayload["model"])
	}
	if requestPayload["zdr"] != true {
		t.Fatalf("request did not require ZDR: %#v", requestPayload)
	}
	responseFormat, _ := requestPayload["response_format"].(map[string]any)
	jsonSchema, _ := responseFormat["json_schema"].(map[string]any)
	if responseFormat["type"] != "json_schema" || jsonSchema["strict"] != true {
		t.Fatalf("request did not require strict JSON: %#v", requestPayload["response_format"])
	}
	messages, _ := requestPayload["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("unexpected messages: %#v", requestPayload["messages"])
	}
	userMessage, _ := messages[1].(map[string]any)
	if !strings.Contains(userMessage["content"].(string), `"simplified":"学习"`) {
		t.Fatalf("form was not serialized into the prompt: %#v", userMessage["content"])
	}

	usages, err := store.Usage().ListBySession(ctx, ManualVocabularySessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 ||
		usages[0].Model != openrouter.DefaultChatModel ||
		usages[0].Cost != 0.0123 ||
		usages[0].PromptTokens != 17 ||
		usages[0].CompletionTokens != 23 {
		t.Fatalf("manual usage was not recorded: %#v", usages)
	}
	vocabulary, err := store.Vocabulary().ListAll(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(vocabulary) != 0 {
		t.Fatalf("generation persisted vocabulary: %#v", vocabulary)
	}
}

func TestParseManualVocabularyJSONIsStrict(t *testing.T) {
	valid := `{"simplified":"你好","traditional":"你好","pinyin":"ni3 hao3","meaning":"hello","partOfSpeech":"greeting","classifier":"","example":"你好！","examplePinyin":"ni3 hao3","exampleTranslation":"Hello!","tags":[]}`
	if _, err := ParseManualVocabularyJSON(valid); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseManualVocabularyJSON(strings.TrimSuffix(valid, "}")); err == nil {
		t.Fatal("malformed JSON was accepted")
	}
	if _, err := ParseManualVocabularyJSON(strings.Replace(valid, `,"tags":[]`, "", 1)); err == nil {
		t.Fatal("missing required field was accepted")
	}
	if _, err := ParseManualVocabularyJSON(strings.TrimSuffix(valid, "}") + `,"unexpected":"value"}`); err == nil {
		t.Fatal("unknown field was accepted")
	}
	if _, err := ParseManualVocabularyJSON(valid + valid); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
}

func TestMergeManualVocabularyInputFillsMissingFields(t *testing.T) {
	draft, err := mergeManualVocabularyInput(
		ManualVocabularyInput{Simplified: "学习"},
		ManualVocabularyInput{
			Simplified:         "学习",
			Traditional:        "學習",
			Pinyin:             "xue2 xi2",
			Meaning:            "to study",
			PartOfSpeech:       "verb",
			Example:            "我学习中文。",
			ExamplePinyin:      "wo3 xue2 zhong1 wen2",
			ExampleTranslation: "I study Chinese.",
			Tags:               []string{"study"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Traditional != "學習" ||
		draft.Pinyin != "xue2 xi2" ||
		draft.Meaning != "to study" ||
		draft.Example != "我学习中文。" ||
		len(draft.Tags) != 1 ||
		!draft.AiGenerated {
		t.Fatalf("missing fields were not filled: %#v", draft)
	}
}

func TestManualVocabularyGenerationErrorsAreActionable(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := New(store, capture.NewMockSource(), NewMemorySecretStore(), nil)

	if _, err := svc.GenerateManualVocabulary(ctx, ManualVocabularyInput{}); err == nil ||
		!strings.Contains(err.Error(), "simplified Chinese") {
		t.Fatalf("missing Chinese input was not actionable: %v", err)
	}
	if _, err := svc.GenerateManualVocabulary(ctx, ManualVocabularyInput{Simplified: "hello"}); err == nil ||
		!strings.Contains(err.Error(), "Chinese") {
		t.Fatalf("non-Chinese input was not actionable: %v", err)
	}
	svc.mu.Lock()
	svc.currentLesson = "active-lesson"
	svc.mu.Unlock()
	if _, err := svc.GenerateManualVocabulary(ctx, ManualVocabularyInput{Simplified: "你好"}); err == nil ||
		!strings.Contains(err.Error(), "finish the active lesson") {
		t.Fatalf("active lesson was not protected: %v", err)
	}
	svc.mu.Lock()
	svc.currentLesson = ""
	svc.mu.Unlock()
	if _, err := svc.GenerateManualVocabulary(ctx, ManualVocabularyInput{Simplified: "你好"}); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "api key") {
		t.Fatalf("missing API key was not actionable: %v", err)
	}
}

func TestSaveManualVocabularyCanonicalizesAndRecordsProvenance(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := New(store, capture.NewMockSource(), NewMemorySecretStore(), nil)

	entry, err := svc.SaveManualVocabulary(ctx, ManualVocabularyInput{
		Simplified:         "学习",
		Traditional:        "學習",
		Pinyin:             "xué xí",
		Meaning:            "to study",
		PartOfSpeech:       "verb",
		Classifier:         "门",
		Example:            "我学习中文。",
		ExamplePinyin:      "wǒ xué zhōng wén",
		ExampleTranslation: "I study Chinese.",
		Tags:               []string{"business", "review"},
		AiGenerated:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Status != domain.VocabularyStatusConfirmed ||
		!entry.Manual ||
		entry.Priority != "manual" ||
		entry.Provenance != domain.ProvenanceUser ||
		entry.Reading != "xue2 xi2" ||
		entry.MarkedPinyin != "xué xí" {
		t.Fatalf("manual metadata or canonical pinyin is wrong: %#v", entry)
	}
	if len(entry.Senses) != 1 ||
		entry.Senses[0].Gloss != "to study" ||
		entry.Senses[0].PartOfSpeech != "verb" ||
		entry.Senses[0].Classifier != "门" {
		t.Fatalf("manual sense was not saved: %#v", entry.Senses)
	}
	if len(entry.Examples) != 1 ||
		entry.Examples[0].Reading != "wo3 xue2 zhong1 wen2" ||
		entry.Examples[0].Provenance != domain.ProvenanceModel ||
		!entry.Examples[0].Generated {
		t.Fatalf("generated example provenance was not saved: %#v", entry.Examples)
	}
	if len(entry.Tags) != 2 || entry.Tags[0].Name != "business" || entry.Tags[1].Name != "review" {
		t.Fatalf("manual tags were not saved: %#v", entry.Tags)
	}
}

func TestManualVocabularyDuplicateAndRejectedRestore(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := New(store, capture.NewMockSource(), NewMemorySecretStore(), nil)
	input := ManualVocabularyInput{
		Simplified:         "再见",
		Pinyin:             "zai4 jian4",
		Meaning:            "goodbye",
		Example:            "我们明天再见。",
		ExamplePinyin:      "wo3 men5 ming2 tian1 zai4 jian4",
		ExampleTranslation: "See you tomorrow.",
	}

	first, err := svc.SaveManualVocabulary(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.SaveManualVocabulary(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("confirmed duplicate created a new entry: %s != %s", first.ID, second.ID)
	}
	if len(first.Examples) != 1 ||
		first.Examples[0].Provenance != domain.ProvenanceUser ||
		first.Examples[0].Generated {
		t.Fatalf("user example provenance was not preserved: %#v", first.Examples)
	}
	all, err := store.Vocabulary().ListVocabulary(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("confirmed duplicate was not idempotent: %#v", all)
	}

	rejected, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified: "谢谢",
		Reading:    "xie4 xie4",
		Provenance: domain.ProvenanceModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Vocabulary().Reject(ctx, rejected.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := svc.SaveManualVocabulary(ctx, ManualVocabularyInput{
		Simplified:         "谢谢",
		Pinyin:             "xiè xiè",
		Meaning:            "thank you",
		Example:            "谢谢你的帮助。",
		ExamplePinyin:      "xie4 xie4 ni3 de5 bang1 zhu4",
		ExampleTranslation: "Thank you for your help.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != rejected.ID ||
		restored.Status != domain.VocabularyStatusConfirmed ||
		!restored.Manual {
		t.Fatalf("manual save did not restore rejected entry: %#v", restored)
	}

	automaticRejected, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified: "再见",
		Reading:    "zai4 jian4",
		Provenance: domain.ProvenanceModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if automaticRejected.Status != domain.VocabularyStatusConfirmed {
		t.Fatalf("expected existing manual entry to remain confirmed: %#v", automaticRejected)
	}
	otherRejected, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified: "中文",
		Reading:    "zhong1 wen2",
		Provenance: domain.ProvenanceModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Vocabulary().Reject(ctx, otherRejected.ID); err != nil {
		t.Fatal(err)
	}
	again, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified: "中文",
		Reading:    "zhōng wén",
		Provenance: domain.ProvenanceModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != domain.VocabularyStatusRejected {
		t.Fatalf("automatic upsert revived rejected entry: %#v", again)
	}
}
