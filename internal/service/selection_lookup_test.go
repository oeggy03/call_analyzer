package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/selection"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

func TestLookupSelectionStripsPunctuationAndUsesSavedVocabulary(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected model request", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	entry := saveSelectionEntry(t, store, domain.VocabularyEntry{
		Simplified:  "学习",
		Traditional: "學習",
		Reading:     "xue2 xi2",
		Senses:      []domain.Sense{{Gloss: "to study"}},
	})

	result, err := svc.LookupSelection(ctx, " 「学习，」 ", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Surface != "学习" || result.Notice != "" || len(result.Lines) != 1 {
		t.Fatalf("unexpected punctuation lookup result: %#v", result)
	}
	line := result.Lines[0]
	if line.Surface != "学习" ||
		line.Simplified != entry.Simplified ||
		line.Traditional != entry.Traditional ||
		line.Pinyin != entry.MarkedPinyin ||
		line.Meaning != "to study" ||
		line.Action != "add" ||
		line.Destination != "add-word" {
		t.Fatalf("unexpected saved gloss line: %#v", line)
	}
	if requests.Load() != 0 {
		t.Fatalf("saved vocabulary caused a model request: %d", requests.Load())
	}
}

func TestLookupSelectionMatchesTraditionalSurface(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected model request", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	entry := saveSelectionEntry(t, store, domain.VocabularyEntry{
		Simplified:  "学习",
		Traditional: "學習",
		Reading:     "xue2 xi2",
		Senses:      []domain.Sense{{Gloss: "to study"}},
	})

	result, err := svc.LookupSelection(ctx, "學習", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 1 || result.Lines[0].VocabularyID != entry.ID {
		t.Fatalf("traditional surface did not match: %#v", result)
	}
}

func TestLookupSelectionReturnsDistinctConfirmedReadings(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected model request", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	saveSelectionEntry(t, store, domain.VocabularyEntry{
		Simplified: "行",
		Reading:    "xing2",
		Senses:     []domain.Sense{{Gloss: "to walk"}},
	})
	saveSelectionEntry(t, store, domain.VocabularyEntry{
		Simplified: "行",
		Reading:    "hang2",
		Senses:     []domain.Sense{{Gloss: "profession"}},
	})

	result, err := svc.LookupSelection(ctx, "行", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 2 {
		t.Fatalf("expected two saved readings, got %#v", result)
	}
	if result.Lines[0].Pinyin == result.Lines[1].Pinyin {
		t.Fatalf("saved readings were not distinct: %#v", result.Lines)
	}
	if requests.Load() != 0 {
		t.Fatalf("saved readings caused a model request: %d", requests.Load())
	}
}

func TestLookupSelectionHidesCandidateWithConfirmedReading(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected model request", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	confirmed := saveSelectionEntry(t, store, domain.VocabularyEntry{
		Simplified: "行",
		Reading:    "xing2",
		Senses:     []domain.Sense{{Gloss: "to walk"}},
	})
	candidate, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified:  "行",
		Traditional: "行字",
		Reading:     "xing2",
		Senses:      []domain.Sense{{Gloss: "candidate gloss"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.LookupSelection(ctx, "行", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 1 || result.Lines[0].VocabularyID != confirmed.ID {
		t.Fatalf("candidate was not hidden: candidate=%s result=%#v", candidate.ID, result)
	}
}

func TestLookupSelectionOpensConfirmedEntryWithExample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected model request", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	entry := saveSelectionEntry(t, store, domain.VocabularyEntry{
		Simplified: "学习",
		Reading:    "xue2 xi2",
		Senses:     []domain.Sense{{Gloss: "to study"}},
		Examples: []domain.Example{{
			Simplified:  "我学习中文。",
			Reading:     "wo3 xue2 xi2 zhong1 wen2",
			Translation: "I study Chinese.",
		}},
	})

	result, err := svc.LookupSelection(ctx, "学习", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 1 ||
		result.Lines[0].VocabularyID != entry.ID ||
		result.Lines[0].Action != "open" ||
		result.Lines[0].Destination != "vocabulary" {
		t.Fatalf("confirmed example was not openable: %#v", result)
	}
}

func TestAddSelectionDoesNotTreatMissingGlossAsTypedMeaning(t *testing.T) {
	const manualResponse = `{"resolution":"selected","entry":{"simplified":"学习","traditional":"學習","pinyin":"xué xí","meaning":"to study","partOfSpeech":"verb","classifier":"","example":"我学习中文。","examplePinyin":"wǒ xué xí zhōng wén","exampleTranslation":"I study Chinese.","tags":[]},"variants":[{"traditional":"學習","pinyin":"xué xí","meaning":"to study","partOfSpeech":"verb","classifier":"","example":"我学习中文。","examplePinyin":"wǒ xué xí zhōng wén","exampleTranslation":"I study Chinese."}]}`
	var requestContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode manual request: %v", err)
		}
		messages, _ := payload["messages"].([]any)
		if len(messages) > 0 {
			userMessage, _ := messages[len(messages)-1].(map[string]any)
			requestContent, _ = userMessage["content"].(string)
		}
		writeSelectionChatResponse(w, manualResponse)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	defer store.Close()
	saveSelectionEntry(t, store, domain.VocabularyEntry{
		Simplified: "学习",
		Reading:    "xue2 xi2",
	})
	lookup, err := svc.LookupSelection(ctx, "学习", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(lookup.Lines) != 1 || lookup.Lines[0].Meaning != selectionNoGloss {
		t.Fatalf("missing gloss was not shown on the lookup line: %#v", lookup)
	}

	var event SelectionAddWord
	svc.SetEventHook(func(name string, payload any) {
		if name == "selection.add-word" {
			event, _ = payload.(SelectionAddWord)
		}
	})
	if err := svc.AddSelection(ctx, lookup.Lines[0]); err != nil {
		t.Fatal(err)
	}
	if event.Meaning != "" {
		t.Fatalf("placeholder was copied to the event: %#v", event)
	}
	if strings.Contains(requestContent, selectionNoGloss) {
		t.Fatalf("placeholder was sent as typed meaning: %q", requestContent)
	}
}

func TestLookupSelectionCallsLookupModelOnceAndCachesResult(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode lookup request: %v", err)
		}
		if payload["model"] != openrouter.DefaultLookupModel ||
			payload["max_tokens"] != float64(200) ||
			payload["zdr"] != true {
			t.Errorf("unexpected lookup request: %#v", payload)
		}
		provider, _ := payload["provider"].(map[string]any)
		if provider["data_collection"] != "deny" || provider["zdr"] != true {
			t.Errorf("unexpected lookup provider privacy controls: %#v", provider)
		}
		messages, _ := payload["messages"].([]any)
		if len(messages) != 2 {
			t.Errorf("unexpected lookup messages: %#v", payload["messages"])
		} else {
			userMessage, _ := messages[1].(map[string]any)
			if userMessage["content"] != "Word: 学习\nContext: 我每天学习中文。" {
				t.Errorf("lookup prompt was not labeled: %#v", userMessage["content"])
			}
		}
		writeSelectionChatResponse(w, `{"entries":[{"pinyin":"xué xí","meaning":"to study"}]}`)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	defer store.Close()
	first, err := svc.LookupSelection(ctx, "学习", "我每天学习中文。")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.LookupSelection(ctx, "学习", "我每天学习中文。")
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("lookup cache made %d requests, want 1", requests.Load())
	}
	if len(first.Lines) != 1 || len(second.Lines) != 1 ||
		first.Lines[0].Pinyin != "xué xí" ||
		second.Lines[0].Meaning != "to study" {
		t.Fatalf("unexpected cached lookup results: first=%#v second=%#v", first, second)
	}
	usages, err := store.Usage().ListBySession(ctx, SelectionLookupSessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0].Model != openrouter.DefaultLookupModel {
		t.Fatalf("lookup usage was not recorded once: %#v", usages)
	}
}

func TestLookupSelectionRecordsUsageWhenHTTPCallFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid request"},"usage":{"cost":0.006}}`))
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	defer store.Close()
	if _, err := svc.LookupSelection(ctx, "学习", ""); err == nil {
		t.Fatal("failed lookup did not return an error")
	}
	usages, err := store.Usage().ListBySession(ctx, SelectionLookupSessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 ||
		usages[0].Model != openrouter.DefaultLookupModel ||
		usages[0].Cost != 0.006 {
		t.Fatalf("failed lookup usage was not recorded: %#v", usages)
	}
}

func TestLookupSelectionValidationNoticesAvoidNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected model request", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	defer store.Close()
	tests := []struct {
		name    string
		surface string
		want    string
	}{
		{name: "empty", surface: " 「」 ", want: selectionEmptyNotice},
		{name: "too many Han", surface: strings.Repeat("学", 13), want: selectionShortNotice},
		{name: "internal punctuation", surface: "你好，世界", want: selectionShortNotice},
		{name: "latin", surface: "hello", want: selectionNotChineseNotice},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := svc.LookupSelection(ctx, test.surface, "")
			if err != nil {
				t.Fatal(err)
			}
			if result.Notice != test.want {
				t.Fatalf("notice = %q, want %q; result=%#v", result.Notice, test.want, result)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("validation notices caused model requests: %d", requests.Load())
	}
}

func TestAddSelectionBlocksGenerationDuringActiveLesson(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected model request", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	defer store.Close()
	svc.mu.Lock()
	svc.currentLesson = "active-lesson"
	svc.mu.Unlock()
	var event SelectionAddWord
	var eventCount int
	svc.SetEventHook(func(name string, payload any) {
		if name == "selection.add-word" {
			eventCount++
			event, _ = payload.(SelectionAddWord)
		}
	})

	err := svc.AddSelection(ctx, selection.GlossLine{
		Surface:     "学习",
		Simplified:  "学习",
		Traditional: "學習",
		Pinyin:      "xué xí",
		Meaning:     "to study",
		Action:      "add",
		Destination: "add-word",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 || eventCount != 1 {
		t.Fatalf("active lesson triggered unexpected work: requests=%d events=%d", requests.Load(), eventCount)
	}
	if !event.LessonBlocked || event.Destination != "add-word" || event.Draft != nil ||
		event.Simplified != "学习" || event.Traditional != "學習" {
		t.Fatalf("unexpected blocked selection event: %#v", event)
	}
}

func TestAddSelectionUsesDefaultManualModelAndEmitsDraft(t *testing.T) {
	const manualResponse = `{"resolution":"selected","entry":{"simplified":"学习","traditional":"學習","pinyin":"xué xí","meaning":"to study","partOfSpeech":"verb","classifier":"","example":"我学习中文。","examplePinyin":"wǒ xué xí zhōng wén","exampleTranslation":"I study Chinese.","tags":[]},"variants":[{"traditional":"學習","pinyin":"xué xí","meaning":"to study","partOfSpeech":"verb","classifier":"","example":"我学习中文。","examplePinyin":"wǒ xué xí zhōng wén","exampleTranslation":"I study Chinese."}]}`
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode manual request: %v", err)
		}
		if payload["model"] != openrouter.DefaultChatModel {
			t.Errorf("selection add used lookup model: %#v", payload["model"])
		}
		writeSelectionChatResponse(w, manualResponse)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	defer store.Close()
	var event SelectionAddWord
	svc.SetEventHook(func(name string, payload any) {
		if name == "selection.add-word" {
			event, _ = payload.(SelectionAddWord)
		}
	})

	err := svc.AddSelection(ctx, selection.GlossLine{
		Surface:     "学习",
		Simplified:  "学习",
		Traditional: "學習",
		Pinyin:      "xué xí",
		Meaning:     "to study",
		Action:      "add",
		Destination: "add-word",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || event.Draft == nil || event.LessonBlocked ||
		event.Destination != "add-word" ||
		event.Draft.Model != openrouter.DefaultChatModel {
		t.Fatalf("unexpected generated selection event: requests=%d event=%#v", requests.Load(), event)
	}
}

func TestAddSelectionOpenEmitsEventWithoutHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected model request", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	store, svc := newSelectionTestService(t, server.URL)
	defer store.Close()
	var event SelectionAddWord
	svc.SetEventHook(func(name string, payload any) {
		if name == "selection.add-word" {
			event, _ = payload.(SelectionAddWord)
		}
	})

	err := svc.AddSelection(ctx, selection.GlossLine{
		Surface:      "学习",
		Simplified:   "学习",
		Traditional:  "學習",
		Pinyin:       "xué xí",
		Meaning:      "to study",
		VocabularyID: "saved-id",
		Action:       "open",
		Destination:  "vocabulary",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 ||
		event.Destination != "vocabulary" ||
		event.VocabularyID != "saved-id" ||
		event.Draft != nil ||
		event.LessonBlocked {
		t.Fatalf("unexpected open selection event: requests=%d event=%#v", requests.Load(), event)
	}
}

func newSelectionTestService(t *testing.T, serverURL string) (*storage.Store, *Service) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	secrets := NewMemorySecretStore()
	if err := secrets.Set(ctx, secretOpenRouterAPIKey, "selection-test-key"); err != nil {
		store.Close()
		t.Fatal(err)
	}
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL:    serverURL,
		MaxRetries: 1,
	}, APIKeySecretProvider{Store: secrets, Name: secretOpenRouterAPIKey})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	return store, New(store, capture.NewMockSource(), secrets, router)
}

func saveSelectionEntry(t *testing.T, store *storage.Store, entry domain.VocabularyEntry) domain.VocabularyEntry {
	t.Helper()
	ctx := context.Background()
	saved, err := store.Vocabulary().UpsertCandidate(ctx, entry)
	if err != nil {
		t.Fatal(err)
	}
	saved, err = store.Vocabulary().Confirm(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func writeSelectionChatResponse(w http.ResponseWriter, content string) {
	encoded, _ := json.Marshal(content)
	_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + string(encoded) +
		`}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"cost":0.0042}}`))
}
