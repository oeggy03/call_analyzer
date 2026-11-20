package selection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/openrouter"
)

type runnerTestHandler struct {
	lookupResult LookupResult
	lookupErr    error
	addErr       error
	regenErr     error

	lookupSurface    string
	lookupContext    string
	addedLine        GlossLine
	regenFields      CardFields
	regenInstruction string
	savedFields      CardFields
	draft            CardDraft
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

func (h *runnerTestHandler) AddSelection(_ context.Context, line GlossLine) (CardDraft, error) {
	h.addedLine = line
	if h.addErr != nil {
		return CardDraft{}, h.addErr
	}
	if h.draft.Fields.Simplified == "" && h.draft.Fields.Pinyin == "" {
		h.draft = CardDraft{
			Fields: CardFields{
				Simplified: line.Simplified,
				Pinyin:     line.Pinyin,
				Meaning:    line.Meaning,
				Example:    "我在走路。",
			},
		}
	}
	return h.draft, nil
}

func (h *runnerTestHandler) RegenerateSelection(
	_ context.Context,
	fields CardFields,
	instruction string,
) (CardDraft, error) {
	h.regenFields = fields
	h.regenInstruction = instruction
	if h.regenErr != nil {
		return CardDraft{}, h.regenErr
	}
	if h.draft.Fields.Simplified == "" && h.draft.Fields.Pinyin == "" {
		h.draft = CardDraft{
			Fields: CardFields{
				Simplified:         fields.Simplified,
				Pinyin:             fields.Pinyin,
				Meaning:            fields.Meaning,
				Example:            "我们一起去散步吧。",
				ExamplePinyin:      "wǒ men yì qǐ qù sàn bù ba",
				ExampleTranslation: "Let's go for a walk together.",
				AIGenerated:        true,
			},
		}
	}
	return h.draft, nil
}

func (h *runnerTestHandler) SaveSelection(_ context.Context, fields CardFields) error {
	h.savedFields = fields
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

func TestServeLookupAcceptsNullContextAndBounds(t *testing.T) {
	handler := &runnerTestHandler{
		lookupResult: LookupResult{
			Surface: "学习",
			Lines: []GlossLine{{
				Surface:     "学习",
				Simplified:  "学习",
				Pinyin:      "xué xí",
				Meaning:     "to study",
				Action:      "add",
				Destination: "add-word",
			}},
		},
	}
	var output bytes.Buffer
	input := `{"type":"lookup","id":"7","surface":"学习","context":null,"bounds":null}` + "\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, handler)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	if handler.lookupSurface != "学习" || handler.lookupContext != "" {
		t.Fatalf("null context was not accepted: surface=%q context=%q", handler.lookupSurface, handler.lookupContext)
	}
	var response resultMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, output.String())
	}
	if response.Type != "result" || response.ID != "7" || len(response.Lines) != 1 {
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
		response.Message != lookupFailureMessage ||
		response.Detail != "Provider failed" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeAddSuccessWritesDraft(t *testing.T) {
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

	var response draftMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "draft" || response.ID != "add-1" || response.Draft.Fields.Pinyin != "xíng" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeRegenerateSuccessWritesDraftWithInstruction(t *testing.T) {
	handler := &runnerTestHandler{}
	var output bytes.Buffer
	input := `{"type":"regenerate","id":"regen-1","fields":{"simplified":"行","pinyin":"xíng","meaning":"to walk","example":"我在走路。"},"instruction":"use a casual spoken example"}` + "\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, handler)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	if handler.regenFields.Simplified != "行" || handler.regenInstruction != "use a casual spoken example" {
		t.Fatalf("unexpected regenerate arguments: fields=%#v instruction=%q", handler.regenFields, handler.regenInstruction)
	}

	var response draftMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "draft" || response.ID != "regen-1" || response.Draft.Fields.Example != "我们一起去散步吧。" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeRegenerateErrorWritesFriendlyMessageAndDetail(t *testing.T) {
	handler := &runnerTestHandler{
		regenErr: fmt.Errorf("manual vocabulary regeneration failed: %w", context.DeadlineExceeded),
	}
	var output bytes.Buffer
	input := `{"type":"regenerate","id":"regen-2","fields":{"simplified":"行","pinyin":"xíng","meaning":"to walk"},"instruction":"simpler"}` + "\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, handler)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	var response errorMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "error" || response.ID != "regen-2" ||
		response.Message != regenerateFailureMessage ||
		response.Detail != "Request timed out while regenerating the word card." {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeSaveWritesSaved(t *testing.T) {
	handler := &runnerTestHandler{}
	var output bytes.Buffer
	input := `{"type":"save","id":"save-1","fields":{"simplified":"行","pinyin":"xíng","meaning":"to walk","example":"我在走路。","examplePinyin":"wǒ zài zǒu lù","exampleTranslation":"I am walking."}}` + "\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, handler)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	if handler.savedFields.Simplified != "行" || handler.savedFields.Example != "我在走路。" {
		t.Fatalf("unexpected saved fields: %#v", handler.savedFields)
	}
	var response savedMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "saved" || response.ID != "save-1" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeAddErrorWritesFriendlyMessageAndDetail(t *testing.T) {
	handler := &runnerTestHandler{
		addErr: fmt.Errorf("manual vocabulary generation failed: %w", &openrouter.HTTPError{
			StatusCode: 404,
			Body:       `{"error":{"message":"No endpoints found that can handle the requested parameters.","code":404},"user_id":"org_123"}`,
		}),
	}
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
		response.Message != addFailureMessage ||
		response.Detail != "OpenRouter HTTP 404: No endpoints found that can handle the requested parameters." {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeSaveErrorWritesFriendlyMessageAndDetail(t *testing.T) {
	handler := &runnerTestHandler{
		addErr: errors.New("manual vocabulary: example sentence must contain Chinese characters"),
	}
	var output bytes.Buffer
	input := `{"type":"save","id":"save-1","fields":{"simplified":"行","pinyin":"xíng","meaning":"to walk","example":"walk","examplePinyin":"xíng","exampleTranslation":"walk"}}` + "\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, handler)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	var response errorMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "error" || response.ID != "save-1" ||
		response.Message != saveFailureMessage ||
		response.Detail != "Example sentence must contain Chinese characters" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestFailureDetailFormatsCommonErrors(t *testing.T) {
	tests := []struct {
		name string
		op   string
		err  error
		want string
	}{
		{
			name: "openrouter provider metadata raw json",
			op:   "add",
			err: fmt.Errorf("manual vocabulary generation failed: %w", &openrouter.HTTPError{
				StatusCode: 502,
				Body:       `{"error":{"message":"Provider returned error","metadata":{"provider_name":"DeepInfra","raw":"{\"error\":{\"message\":\"context length exceeded\"}}"}}}`,
			}),
			want: "OpenRouter HTTP 502 (DeepInfra): context length exceeded",
		},
		{
			name: "openrouter plain text body",
			op:   "add",
			err:  &openrouter.HTTPError{StatusCode: 503, Body: "service unavailable\n"},
			want: "OpenRouter HTTP 503: service unavailable",
		},
		{
			name: "openrouter empty body",
			op:   "add",
			err:  &openrouter.HTTPError{StatusCode: 500},
			want: "OpenRouter HTTP 500",
		},
		{
			name: "add timeout",
			op:   "add",
			err:  fmt.Errorf("manual vocabulary generation failed: %w", context.DeadlineExceeded),
			want: "Request timed out while generating the word card.",
		},
		{
			name: "lookup timeout",
			op:   "lookup",
			err:  fmt.Errorf("selection lookup failed: %w", context.DeadlineExceeded),
			want: "Lookup request timed out.",
		},
		{
			name: "network url error",
			op:   "add",
			err: fmt.Errorf("manual vocabulary generation failed: %w", &url.Error{
				Op:  "Post",
				URL: "https://openrouter.ai/api/v1/chat/completions",
				Err: errors.New("dial tcp: lookup openrouter.ai: no such host"),
			}),
			want: "Network error: dial tcp: lookup openrouter.ai: no such host",
		},
		{
			name: "missing api key",
			op:   "add",
			err:  errors.New("manual vocabulary generation failed: OpenRouter API key is unavailable; add it in Settings: openrouter: API key is not configured"),
			want: "OpenRouter API key is unavailable. Add it in Settings.",
		},
		{
			name: "invalid model json",
			op:   "add",
			err:  errors.New("manual vocabulary: parse model response: service: decode manual vocabulary generation: invalid character '`' looking for beginning of value"),
			want: "Model returned invalid JSON: invalid character '`' looking for beginning of value",
		},
		{
			name: "missing model field",
			op:   "add",
			err:  errors.New(`manual vocabulary: parse model response: service: manual vocabulary generation entry is missing required field "example"`),
			want: `Model response is missing required field "example"`,
		},
		{
			name: "unusable model entry strips retry suffix",
			op:   "add",
			err:  errors.New("manual vocabulary: model did not return a usable entry; try generating again"),
			want: "Model did not return a usable entry.",
		},
		{
			name: "pinyin too long",
			op:   "add",
			err:  errors.New("manual vocabulary: pinyin is too long"),
			want: "Pinyin is too long",
		},
		{
			name: "redacts bearer tokens",
			op:   "add",
			err:  errors.New("unexpected header Authorization: Bearer sk-or-v1-secret-token"),
			want: "Unexpected header Authorization: Bearer [redacted]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := failureDetail(test.err, test.op)
			if got != test.want {
				t.Fatalf("failureDetail() = %q, want %q", got, test.want)
			}
		})
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

func (*deadlineHandler) AddSelection(context.Context, GlossLine) (CardDraft, error) {
	return CardDraft{}, nil
}

func (*deadlineHandler) RegenerateSelection(context.Context, CardFields, string) (CardDraft, error) {
	return CardDraft{}, nil
}

func (*deadlineHandler) SaveSelection(context.Context, CardFields) error {
	return nil
}
