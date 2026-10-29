package selection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type runnerTestHandler struct {
	lookupResult LookupResult
	lookupErr    error
	addErr       error

	lookupSurface string
	lookupContext string
	addedLine     GlossLine
}

func (h *runnerTestHandler) LookupSelection(
	_ context.Context,
	surface, sentenceContext string,
) (LookupResult, error) {
	h.lookupSurface = surface
	h.lookupContext = sentenceContext
	if h.lookupErr != nil {
		return LookupResult{}, h.lookupErr
	}
	return h.lookupResult, nil
}

func (h *runnerTestHandler) AddSelection(_ context.Context, line GlossLine) error {
	h.addedLine = line
	return h.addErr
}

func TestHelperEnvironmentOmitsSecrets(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "dummy-openrouter-key")
	t.Setenv("SUPABASE_SECRET_KEY", "dummy-supabase-key")

	for _, entry := range helperEnvironment() {
		key, _, _ := strings.Cut(entry, "=")
		if key == "OPENROUTER_API_KEY" || key == "SUPABASE_SECRET_KEY" {
			t.Fatalf("helper environment contains %s", key)
		}
	}
}

func TestServeLookupWritesResult(t *testing.T) {
	handler := &runnerTestHandler{
		lookupResult: LookupResult{
			Surface: "行",
			Lines: []GlossLine{{
				Surface:     "行",
				Simplified:  "行",
				Pinyin:      "xíng",
				Meaning:     "to walk",
				Action:      "add",
				Destination: "add-word",
			}},
		},
	}
	var output bytes.Buffer
	err := Serve(
		context.Background(),
		strings.NewReader(`{"type":"lookup","id":"1","surface":"行","context":"去银行","bounds":{"x":1,"y":2,"width":3,"height":4}}`+"\n"),
		&output,
		handler,
	)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	if handler.lookupSurface != "行" || handler.lookupContext != "去银行" {
		t.Fatalf("unexpected lookup arguments: surface=%q context=%q", handler.lookupSurface, handler.lookupContext)
	}

	var response resultMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "result" || response.ID != "1" ||
		response.Surface != "行" || len(response.Lines) != 1 {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeLookupErrorWritesFriendlyMessage(t *testing.T) {
	handler := &runnerTestHandler{lookupErr: errors.New("provider failed")}
	var output bytes.Buffer
	err := Serve(
		context.Background(),
		strings.NewReader(`{"type":"lookup","id":"lookup-1","surface":"行"}`+"\n"),
		&output,
		handler,
	)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	var response errorMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "error" || response.ID != "lookup-1" ||
		response.Message != lookupFailureMessage {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeAddSuccessWritesDismiss(t *testing.T) {
	handler := &runnerTestHandler{}
	var output bytes.Buffer
	input := `{"type":"add","id":"add-1","line":{"surface":"行","simplified":"行","pinyin":"xíng","meaning":"to walk","action":"add","destination":"add-word"}}` + "\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, handler)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	if handler.addedLine.Surface != "行" || handler.addedLine.Pinyin != "xíng" {
		t.Fatalf("unexpected added line: %#v", handler.addedLine)
	}

	var response dismissMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "dismiss" || response.ID != "add-1" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeAddErrorWritesFriendlyMessage(t *testing.T) {
	handler := &runnerTestHandler{addErr: errors.New("save failed")}
	var output bytes.Buffer
	input := `{"type":"add","id":"add-1","line":{"surface":"行","simplified":"行","pinyin":"xíng","meaning":"to walk","action":"add","destination":"add-word"}}` + "\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, handler)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	var response errorMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "error" || response.ID != "add-1" ||
		response.Message != addFailureMessage {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeIgnoresMalformedAndStaleLines(t *testing.T) {
	handler := &runnerTestHandler{
		lookupResult: LookupResult{Surface: "行"},
	}
	var output bytes.Buffer
	input := strings.Join([]string{
		"{not json",
		`null`,
		`{"type":"unknown","id":"old"}`,
		`{"type":"lookup","surface":"missing-id"}`,
		`{"type":"add","id":"missing-line"}`,
		`{"type":"lookup","id":"current","surface":"行"}`,
	}, "\n") + "\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, handler)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	var response resultMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID != "current" {
		t.Fatalf("unexpected response id: %q", response.ID)
	}
}

func TestServeUsesPerRequestTimeout(t *testing.T) {
	handler := &deadlineHandler{}
	var output bytes.Buffer
	err := Serve(
		context.Background(),
		strings.NewReader(`{"type":"lookup","id":"1","surface":"行"}`+"\n"),
		&output,
		handler,
	)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	if !handler.deadline.After(time.Now()) {
		t.Fatalf("handler did not receive a future deadline: %v", handler.deadline)
	}
}

type deadlineHandler struct {
	deadline time.Time
}

func (h *deadlineHandler) LookupSelection(ctx context.Context, _, _ string) (LookupResult, error) {
	var ok bool
	h.deadline, ok = ctx.Deadline()
	if !ok {
		return LookupResult{}, errors.New("missing deadline")
	}
	return LookupResult{Surface: "行"}, nil
}

func (*deadlineHandler) AddSelection(context.Context, GlossLine) error {
	return nil
}
