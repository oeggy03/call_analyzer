package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testKeyProvider struct{}

func (testKeyProvider) APIKey(context.Context) (string, error) {
	return "test-secret", nil
}

func TestTranscribeAndChatRequestContracts(t *testing.T) {
	var sawAudio atomic.Bool
	var sawChat atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("missing authorization")
		}
		switch r.URL.Path {
		case "/api/v1/audio/transcriptions":
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("multipart: %v", err)
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			fields := map[string]string{}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("part: %v", err)
					return
				}
				data, _ := io.ReadAll(part)
				if part.FormName() == "file" && len(data) > 0 {
					sawAudio.Store(true)
				}
				fields[part.FormName()] = string(data)
			}
			if fields["model"] != DefaultASRModel {
				t.Errorf("unexpected ASR model %q", fields["model"])
			}
			if fields["zdr"] != "true" {
				t.Errorf("transcription did not require ZDR: %#v", fields)
			}
			var provider map[string]any
			if err := json.Unmarshal([]byte(fields["provider"]), &provider); err != nil {
				t.Errorf("invalid transcription provider controls: %v", err)
			}
			if provider["data_collection"] != "deny" || provider["zdr"] != true {
				t.Errorf("unexpected transcription provider controls: %#v", provider)
			}
			sawAudio.Store(sawAudio.Load() && fields["file"] != "")
			_, _ = w.Write([]byte(`{"text":"你好","usage":{"cost":0.10}}`))
		case "/api/v1/chat/completions":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode chat: %v", err)
				return
			}
			if payload["model"] != DefaultChatModel || payload["zdr"] != true {
				t.Errorf("unexpected chat controls: %#v", payload)
			}
			provider, _ := payload["provider"].(map[string]any)
			if provider["require_parameters"] != true || provider["data_collection"] != "deny" || provider["zdr"] != true {
				t.Errorf("unexpected provider controls: %#v", provider)
			}
			format, _ := payload["response_format"].(map[string]any)
			schema, _ := format["json_schema"].(map[string]any)
			if format["type"] != "json_schema" || schema["strict"] != true {
				t.Errorf("unexpected JSON schema controls: %#v", format)
			}
			sawChat.Store(true)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"candidates\":[]}"}}],"usage":{"cost":0.10}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, RetryBase: time.Millisecond}, testKeyProvider{})
	if err != nil {
		t.Fatal(err)
	}
	transcription, err := client.Transcribe(context.Background(), []byte("wav"), "sample.wav")
	if err != nil {
		t.Fatal(err)
	}
	if transcription.Text != "你好" || !sawAudio.Load() {
		t.Fatalf("unexpected transcription %#v, sawAudio=%v", transcription, sawAudio.Load())
	}
	response, err := client.ChatJSON(context.Background(), ChatRequest{
		Prompt:     "extract",
		SchemaName: "test",
		Schema:     map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != `{"candidates":[]}` || !sawChat.Load() {
		t.Fatalf("unexpected chat response %#v", response)
	}
	if got := client.BudgetStatus().Spent; got != .2 {
		t.Fatalf("expected budget .2, got %v", got)
	}
}

func TestRetriesRateLimit(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "try again", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"text":"ok","usage":{"cost":0.01}}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{
		BaseURL:    server.URL,
		MaxRetries: 1,
		RetryBase:  time.Millisecond,
	}, testKeyProvider{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Transcribe(context.Background(), []byte("wav"), "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "ok" || attempts.Load() != 2 {
		t.Fatalf("unexpected retry result=%#v attempts=%d", result, attempts.Load())
	}
}

func TestBudgetDoesNotStopTheSession(t *testing.T) {
	budget := NewBudget(.35, .50)
	if status := budget.Add(.35); !status.Warning || status.HardExceeded {
		t.Fatalf("unexpected warning status %#v", status)
	}
	if status := budget.Add(.15); status.HardExceeded {
		t.Fatalf("cost must not stop the session: %#v", status)
	}
	if err := budget.Check(); err != nil {
		t.Fatalf("budget check stopped the session: %v", err)
	}
}

func TestMultipartFilename(t *testing.T) {
	var filename string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, _ := r.MultipartReader()
		part, _ := reader.NextPart()
		filename = part.FileName()
		_, _ = w.Write([]byte(`{"text":"ok"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL}, testKeyProvider{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Transcribe(context.Background(), []byte("wav"), "audio.wav")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filename, "audio.wav") {
		t.Fatalf("unexpected filename %q", filename)
	}
}

func TestChatVisionJSONRetriesReasoningWithoutZDR(t *testing.T) {
	var attempts atomic.Int32
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
		if _, ok := payload["zdr"]; ok {
			t.Errorf("vision request unexpectedly included top-level zdr: %#v", payload)
		}
		if _, ok := payload["data_collection"]; ok {
			t.Errorf("vision request unexpectedly included top-level data_collection: %#v", payload)
		}
		if _, ok := payload["provider"]; ok {
			t.Errorf("vision request unexpectedly included provider controls: %#v", payload)
		}
		messages, _ := payload["messages"].([]any)
		if len(messages) != 1 {
			t.Fatalf("unexpected vision messages: %#v", payload["messages"])
		}
		userMessage, _ := messages[0].(map[string]any)
		content, _ := userMessage["content"].([]any)
		if len(content) != 2 {
			t.Fatalf("unexpected vision content: %#v", userMessage["content"])
		}
		imagePart, _ := content[1].(map[string]any)
		imageURL, _ := imagePart["image_url"].(map[string]any)
		url, _ := imageURL["url"].(string)
		if !strings.HasPrefix(url, "data:image/jpeg;base64,") ||
			!strings.HasSuffix(url, "/9j/AWgB") {
			t.Errorf("image data URL was not encoded as JPEG base64: %q", url)
		}
		attempt := attempts.Add(1)
		if attempt == 1 {
			reasoning, ok := payload["reasoning"].(map[string]any)
			if !ok || reasoning["effort"] != "none" {
				t.Errorf("first vision request did not disable reasoning: %#v", payload["reasoning"])
			}
			http.Error(w, "reasoning parameter is unsupported", http.StatusBadRequest)
			return
		}
		if _, ok := payload["reasoning"]; ok {
			t.Errorf("reasoning was not omitted on retry: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"entries\":[]}"}}],"usage":{"cost":0.07}}`))
	}))
	defer server.Close()

	budget := NewBudget(.01, .02)
	budget.Add(.02)
	client, err := NewClient(Config{
		BaseURL:    server.URL,
		MaxRetries: 1,
		RetryBase:  time.Millisecond,
		Budget:     budget,
	}, testKeyProvider{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.ChatVisionJSON(context.Background(), VisionChatRequest{
		Prompt:    "read the photo",
		ImageJPEG: []byte{0xff, 0xd8, 0xff, 0x01, 0x68, 0x01},
		Schema:    map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != `{"entries":[]}` || response.Usage.Cost != 0.07 {
		t.Fatalf("unexpected vision response: %#v", response)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected one reasoning fallback retry, got %d attempts", attempts.Load())
	}
	if got := client.BudgetStatus().Spent; got != .02 {
		t.Fatalf("vision call changed lesson budget: %v", got)
	}
}
