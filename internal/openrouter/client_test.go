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

func TestBudgetHardStop(t *testing.T) {
	budget := NewBudget(.35, .50)
	if status := budget.Add(.35); !status.Warning || status.HardExceeded {
		t.Fatalf("unexpected warning status %#v", status)
	}
	if status := budget.Add(.15); !status.HardExceeded {
		t.Fatalf("expected hard status %#v", status)
	}
	if err := budget.Check(); err != ErrBudgetExceeded {
		t.Fatalf("expected hard budget error, got %v", err)
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
