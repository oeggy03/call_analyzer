package selection

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/oeggy03/call_analyzer/internal/openrouter"
)

var ErrAlreadyRunning = errors.New("selection helper is already running")

const (
	lookupFailureMessage     = "Couldn't look this up."
	addFailureMessage        = "Couldn't add this word."
	regenerateFailureMessage = "Couldn't regenerate this card."
	saveFailureMessage       = "Couldn't save this word."
	lookupTimeout            = 20 * time.Second
	addTimeout               = 60 * time.Second
	maxErrorDetailRunes      = 220
)

// Handler contains the service operations used by the selection helper.
type Handler interface {
	LookupSelection(
		ctx context.Context,
		surface, sentenceContext string,
	) (LookupResult, error)
	AddSelection(ctx context.Context, line GlossLine) (CardDraft, error)
	RegenerateSelection(ctx context.Context, fields CardFields, instruction string) (CardDraft, error)
	SaveSelection(ctx context.Context, fields CardFields) error
}

// Config controls how the native selection helper is located.
type Config struct {
	HelperPath string
}

// Runner owns one long-lived native selection helper process.
//
// The process-specific implementation is selected by the platform files. The
// shared fields live here so App can hold the same runner on every platform.
type Runner struct {
	config Config

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	done   chan struct{}
	cancel context.CancelFunc
}

// NewRunner creates a selection helper runner.
func NewRunner(config Config) *Runner {
	return &Runner{config: config}
}

func helperEnvironment() []string {
	environment := os.Environ()
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if key == "OPENROUTER_API_KEY" || key == "SUPABASE_SECRET_KEY" {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

type helperRequest struct {
	Type        string      `json:"type"`
	ID          string      `json:"id"`
	Surface     string      `json:"surface"`
	Context     *string     `json:"context"`
	Bounds      *Bounds     `json:"bounds"`
	Line        *GlossLine  `json:"line"`
	Fields      *CardFields `json:"fields"`
	Instruction string      `json:"instruction,omitempty"`
}

func failureDetail(err error, op string) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return timeoutDetail(op)
	}
	if errors.Is(err, context.Canceled) {
		return "Request was canceled."
	}
	if errors.Is(err, openrouter.ErrBudgetExceeded) {
		return "OpenRouter budget limit was exceeded."
	}
	var httpErr *openrouter.HTTPError
	if errors.As(err, &httpErr) {
		return redactSecrets(openRouterHTTPErrorDetail(httpErr))
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return timeoutDetail(op)
		}
		if urlErr.Err != nil {
			if inner := cleanInlineText(urlErr.Err.Error()); inner != "" {
				return truncateRunes(redactSecrets("Network error: "+inner), maxErrorDetailRunes)
			}
		}
	}

	text := cleanInlineText(err.Error())
	text = stripInternalPrefixes(text)
	text = rewriteKnownError(text)
	text = stripRetrySuffix(text)
	text = redactSecrets(text)
	text = capitalizeFirst(text)
	return truncateRunes(text, maxErrorDetailRunes)
}

func timeoutDetail(op string) string {
	switch op {
	case "add":
		return "Request timed out while generating the word card."
	case "regenerate":
		return "Request timed out while regenerating the word card."
	case "lookup":
		return "Lookup request timed out."
	case "save":
		return "Save request timed out."
	default:
		return "Request timed out."
	}
}

type openRouterErrorPayload struct {
	Error struct {
		Message  string `json:"message"`
		Metadata struct {
			Raw          string `json:"raw"`
			ProviderName string `json:"provider_name"`
		} `json:"metadata"`
	} `json:"error"`
	Message string `json:"message"`
}

func openRouterHTTPErrorDetail(httpErr *openrouter.HTTPError) string {
	if httpErr == nil {
		return ""
	}
	var payload openRouterErrorPayload
	if json.Unmarshal([]byte(httpErr.Body), &payload) == nil {
		msg := cleanInlineText(payload.Error.Message)
		if msg == "" {
			msg = cleanInlineText(payload.Message)
		}
		provider := cleanInlineText(payload.Error.Metadata.ProviderName)
		rawDetail := extractNestedErrorMessage(payload.Error.Metadata.Raw)
		if msg != "" {
			if rawDetail != "" && strings.EqualFold(msg, "provider returned error") {
				msg = rawDetail
			} else if rawDetail != "" && !strings.Contains(strings.ToLower(msg), strings.ToLower(rawDetail)) {
				msg = msg + " (" + rawDetail + ")"
			}
			if provider != "" {
				return truncateRunes(
					fmt.Sprintf("OpenRouter HTTP %d (%s): %s", httpErr.StatusCode, provider, msg),
					maxErrorDetailRunes,
				)
			}
			return truncateRunes(
				fmt.Sprintf("OpenRouter HTTP %d: %s", httpErr.StatusCode, msg),
				maxErrorDetailRunes,
			)
		}
	}
	if body := cleanInlineText(httpErr.Body); body != "" {
		return truncateRunes(
			fmt.Sprintf("OpenRouter HTTP %d: %s", httpErr.StatusCode, body),
			maxErrorDetailRunes,
		)
	}
	return fmt.Sprintf("OpenRouter HTTP %d", httpErr.StatusCode)
}

func extractNestedErrorMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var nested struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}
	if json.Unmarshal([]byte(raw), &nested) == nil {
		for _, candidate := range []string{nested.Error.Message, nested.Message, nested.Detail} {
			if cleaned := cleanInlineText(candidate); cleaned != "" {
				return cleaned
			}
		}
	}
	var flat struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &flat) == nil {
		if cleaned := cleanInlineText(flat.Error); cleaned != "" {
			return cleaned
		}
	}
	return cleanInlineText(raw)
}

func stripInternalPrefixes(text string) string {
	prefixes := []string{
		"manual vocabulary regeneration failed: ",
		"manual vocabulary generation failed: ",
		"manual vocabulary: ",
		"selection lookup failed: ",
		"selection lookup: ",
		"parse model response: ",
		"service: ",
		"openrouter: ",
		"save: ",
	}
	for {
		trimmed := text
		for _, prefix := range prefixes {
			if len(trimmed) >= len(prefix) && strings.EqualFold(trimmed[:len(prefix)], prefix) {
				trimmed = strings.TrimSpace(trimmed[len(prefix):])
			}
		}
		if trimmed == text {
			return text
		}
		text = trimmed
	}
}

func rewriteKnownError(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.HasPrefix(lower, "openrouter api key is unavailable; add it in settings"):
		return "OpenRouter API key is unavailable. Add it in Settings."
	case strings.HasPrefix(lower, "api key is not configured"):
		return "OpenRouter API key is not configured. Add it in Settings."
	case strings.HasPrefix(lower, "chat response has no choices"):
		return "Model returned an empty response."
	}
	for _, decodePrefix := range []string{
		"decode manual vocabulary generation: ",
		"decode trailing manual vocabulary generation: ",
		"decode manual vocabulary: ",
		"decode selection lookup: ",
	} {
		if strings.HasPrefix(lower, decodePrefix) {
			return "Model returned invalid JSON: " + strings.TrimSpace(text[len(decodePrefix):])
		}
	}
	for _, schemaPrefix := range []string{
		"manual vocabulary generation entry ",
		"manual vocabulary generation variant ",
		"manual vocabulary generation ",
		"selection lookup entry ",
		"selection lookup ",
	} {
		if strings.HasPrefix(lower, schemaPrefix) {
			return "Model response " + strings.TrimSpace(text[len(schemaPrefix):])
		}
	}
	return text
}

func stripRetrySuffix(text string) string {
	lower := strings.ToLower(text)
	const suffix = "; try generating again"
	if strings.HasSuffix(lower, suffix) {
		return strings.TrimSpace(text[:len(text)-len(suffix)]) + "."
	}
	return text
}

func cleanInlineText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func redactSecrets(text string) string {
	lower := strings.ToLower(text)
	index := strings.Index(lower, "bearer ")
	for index >= 0 {
		tokenStart := index + len("bearer ")
		tokenEnd := tokenStart
		for tokenEnd < len(text) && !unicode.IsSpace(rune(text[tokenEnd])) &&
			text[tokenEnd] != '"' && text[tokenEnd] != '\'' && text[tokenEnd] != ',' {
			tokenEnd++
		}
		text = text[:tokenStart] + "[redacted]" + text[tokenEnd:]
		lower = strings.ToLower(text)
		nextOffset := tokenStart + len("[redacted]")
		rel := strings.Index(lower[nextOffset:], "bearer ")
		if rel < 0 {
			break
		}
		index = nextOffset + rel
	}
	return text
}

func capitalizeFirst(text string) string {
	if text == "" {
		return ""
	}
	runes := []rune(text)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func (r helperRequest) sentenceContext() string {
	if r.Context == nil {
		return ""
	}
	return *r.Context
}

type resultMessage struct {
	Type         string      `json:"type"`
	ID           string      `json:"id"`
	Surface      string      `json:"surface"`
	Lines        []GlossLine `json:"lines"`
	Notice       string      `json:"notice"`
	ExistingTags []string    `json:"existingTags,omitempty"`
}

type errorMessage struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

type dismissMessage struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type draftMessage struct {
	Type  string    `json:"type"`
	ID    string    `json:"id"`
	Draft CardDraft `json:"draft"`
}

type savedMessage struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Serve translates newline-delimited helper requests into parent responses.
//
// Each request gets its own timeout and may complete independently of newer
// requests. The helper tags every request with an id and ignores responses
// for ids that are no longer current.
func Serve(ctx context.Context, in io.Reader, out io.Writer, handler Handler) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if in == nil {
		return errors.New("selection helper input is nil")
	}
	if out == nil {
		return errors.New("selection helper output is nil")
	}
	if handler == nil {
		return errors.New("selection helper handler is nil")
	}

	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		scanner = bufio.NewScanner(in)
		wg      sync.WaitGroup
		writeMu sync.Mutex
	)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	write := func(message any) error {
		data, err := json.Marshal(message)
		if err != nil {
			return err
		}
		data = append(data, '\n')

		writeMu.Lock()
		defer writeMu.Unlock()
		for len(data) > 0 {
			n, err := out.Write(data)
			if err != nil {
				return err
			}
			if n <= 0 {
				return io.ErrShortWrite
			}
			data = data[n:]
		}
		return nil
	}

	var writeErrMu sync.Mutex
	var writeErr error
	recordWriteError := func(err error) {
		if err == nil {
			return
		}
		writeErrMu.Lock()
		if writeErr == nil {
			writeErr = err
		}
		writeErrMu.Unlock()
	}

	for scanner.Scan() {
		var request helperRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil ||
			request.Type == "" || request.ID == "" {
			continue
		}

		switch request.Type {
		case "lookup":
			request := request
			wg.Add(1)
			go func() {
				defer wg.Done()
				callCtx, callCancel := context.WithTimeout(serveCtx, lookupTimeout)
				defer callCancel()

				result, err := handler.LookupSelection(
					callCtx,
					request.Surface,
					request.sentenceContext(),
				)
				if err != nil {
					recordWriteError(write(errorMessage{
						Type:    "error",
						ID:      request.ID,
						Message: lookupFailureMessage,
						Detail:  failureDetail(err, "lookup"),
					}))
					return
				}
				if result.Surface == "" {
					result.Surface = request.Surface
				}
				lines := result.Lines
				if lines == nil {
					lines = []GlossLine{}
				}
				tags := result.ExistingTags
				if tags == nil {
					tags = []string{}
				}
				recordWriteError(write(resultMessage{
					Type:         "result",
					ID:           request.ID,
					Surface:      result.Surface,
					Lines:        lines,
					Notice:       result.Notice,
					ExistingTags: tags,
				}))
			}()
		case "add":
			if request.Line == nil {
				continue
			}
			request := request
			wg.Add(1)
			go func() {
				defer wg.Done()
				callCtx, callCancel := context.WithTimeout(serveCtx, addTimeout)
				defer callCancel()

				draft, err := handler.AddSelection(callCtx, *request.Line)
				if err != nil {
					recordWriteError(write(errorMessage{
						Type:    "error",
						ID:      request.ID,
						Message: addFailureMessage,
						Detail:  failureDetail(err, "add"),
					}))
					return
				}
				recordWriteError(write(draftMessage{
					Type:  "draft",
					ID:    request.ID,
					Draft: draft,
				}))
			}()
		case "regenerate":
			if request.Fields == nil {
				continue
			}
			request := request
			wg.Add(1)
			go func() {
				defer wg.Done()
				callCtx, callCancel := context.WithTimeout(serveCtx, addTimeout)
				defer callCancel()

				draft, err := handler.RegenerateSelection(
					callCtx,
					*request.Fields,
					request.Instruction,
				)
				if err != nil {
					recordWriteError(write(errorMessage{
						Type:    "error",
						ID:      request.ID,
						Message: regenerateFailureMessage,
						Detail:  failureDetail(err, "regenerate"),
					}))
					return
				}
				recordWriteError(write(draftMessage{
					Type:  "draft",
					ID:    request.ID,
					Draft: draft,
				}))
			}()
		case "save":
			if request.Fields == nil {
				continue
			}
			request := request
			wg.Add(1)
			go func() {
				defer wg.Done()
				callCtx, callCancel := context.WithTimeout(serveCtx, addTimeout)
				defer callCancel()

				if err := handler.SaveSelection(callCtx, *request.Fields); err != nil {
					recordWriteError(write(errorMessage{
						Type:    "error",
						ID:      request.ID,
						Message: saveFailureMessage,
						Detail:  failureDetail(err, "save"),
					}))
					return
				}
				recordWriteError(write(savedMessage{
					Type: "saved",
					ID:   request.ID,
				}))
			}()
		}
	}

	scanErr := scanner.Err()
	workersDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(workersDone)
	}()

	if scanErr != nil || ctx.Err() != nil {
		cancel()
		select {
		case <-workersDone:
		case <-time.After(2 * time.Second):
		}
	} else {
		<-workersDone
		cancel()
	}

	if scanErr != nil {
		return scanErr
	}
	writeErrMu.Lock()
	defer writeErrMu.Unlock()
	return writeErr
}
