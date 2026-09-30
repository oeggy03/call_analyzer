// Package openrouter is a small, dependency-free HTTP client for the
// OpenRouter audio and chat endpoints.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL   = "https://openrouter.ai"
	DefaultASRModel  = "qwen/qwen3-asr-1.7b"
	DefaultChatModel = "qwen/qwen3-32b"
)

var ErrBudgetExceeded = errors.New("openrouter: session budget exceeded")

type APIKeyProvider interface {
	APIKey(context.Context) (string, error)
}

type Config struct {
	BaseURL    string
	ASRModel   string
	ChatModel  string
	HTTPClient *http.Client
	MaxRetries int
	RetryBase  time.Duration
	Budget     *Budget
}

type Client struct {
	baseURL    *url.URL
	asrModel   string
	chatModel  string
	httpClient *http.Client
	maxRetries int
	retryBase  time.Duration
	key        APIKeyProvider
	budget     *Budget
}

func NewClient(config Config, key APIKeyProvider) (*Client, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("openrouter: invalid base URL %q", base)
	}
	if key == nil {
		return nil, errors.New("openrouter: API key provider is required")
	}
	if config.ASRModel == "" {
		config.ASRModel = DefaultASRModel
	}
	if config.ChatModel == "" {
		config.ChatModel = DefaultChatModel
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 90 * time.Second}
	}
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryBase <= 0 {
		config.RetryBase = 250 * time.Millisecond
	}
	if config.Budget == nil {
		config.Budget = NewBudget(0.35, 0.50)
	}
	return &Client{
		baseURL:    parsed,
		asrModel:   config.ASRModel,
		chatModel:  config.ChatModel,
		httpClient: config.HTTPClient,
		maxRetries: config.MaxRetries,
		retryBase:  config.RetryBase,
		key:        key,
		budget:     config.Budget,
	}, nil
}

func (c *Client) ASRModel() string {
	return c.asrModel
}

func (c *Client) ChatModel() string {
	return c.chatModel
}

func (c *Client) WithModels(asrModel, chatModel string, budget *Budget) *Client {
	copy := *c
	if asrModel != "" {
		copy.asrModel = asrModel
	}
	if chatModel != "" {
		copy.chatModel = chatModel
	}
	if budget != nil {
		copy.budget = budget
	}
	return &copy
}

func (c *Client) BudgetStatus() BudgetStatus {
	return c.budget.Status()
}

type Transcription struct {
	Text     string                 `json:"text"`
	Duration float64                `json:"duration,omitempty"`
	Segments []TranscriptionSegment `json:"segments,omitempty"`
	Usage    Usage                  `json:"usage,omitempty"`
}

type TranscriptionSegment struct {
	ID    int     `json:"id,omitempty"`
	Start float64 `json:"start,omitempty"`
	End   float64 `json:"end,omitempty"`
	Text  string  `json:"text"`
}

func (c *Client) Transcribe(ctx context.Context, audio []byte, filename string) (Transcription, error) {
	if len(audio) == 0 {
		return Transcription{}, errors.New("openrouter: audio is empty")
	}
	if filename == "" {
		filename = "audio.wav"
	}
	var result Transcription
	err := c.doWithRetry(ctx, func() (*http.Request, error) {
		key, err := c.apiKey(ctx)
		if err != nil {
			return nil, err
		}
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		filePart, err := writer.CreateFormFile("file", filename)
		if err != nil {
			return nil, err
		}
		if _, err := filePart.Write(audio); err != nil {
			return nil, err
		}
		if err := writer.WriteField("model", c.asrModel); err != nil {
			return nil, err
		}
		if err := writer.WriteField("zdr", "true"); err != nil {
			return nil, err
		}
		provider, err := json.Marshal(providerRules{
			RequireParameters: true,
			DataCollection:    "deny",
			ZDR:               true,
		})
		if err != nil {
			return nil, err
		}
		if err := writer.WriteField("provider", string(provider)); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.endpoint("/api/v1/audio/transcriptions"), &body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		return req, nil
	}, &result)
	if err == nil {
		if status := c.budget.Add(result.Usage.Cost); status.HardExceeded {
			return result, ErrBudgetExceeded
		}
	}
	return result, err
}

type ChatRequest struct {
	Prompt      string
	System      string
	SchemaName  string
	Schema      map[string]any
	Temperature *float64
}

type ChatResponse struct {
	Content string `json:"content"`
	Usage   Usage  `json:"usage"`
}

type chatEnvelope struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

type chatPayload struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	ResponseFormat responseFormat `json:"response_format"`
	Provider       providerRules  `json:"provider"`
	ZDR            bool           `json:"zdr"`
	Temperature    *float64       `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type       string     `json:"type"`
	JSONSchema jsonSchema `json:"json_schema"`
}

type jsonSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type providerRules struct {
	RequireParameters bool   `json:"require_parameters"`
	DataCollection    string `json:"data_collection"`
	ZDR               bool   `json:"zdr"`
}

func (c *Client) ChatJSON(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return ChatResponse{}, errors.New("openrouter: prompt is empty")
	}
	if request.SchemaName == "" {
		request.SchemaName = "call_analyzer_extraction"
	}
	if request.Schema == nil {
		return ChatResponse{}, errors.New("openrouter: strict JSON schema is required")
	}
	system := request.System
	var result chatEnvelope
	err := c.doWithRetry(ctx, func() (*http.Request, error) {
		key, err := c.apiKey(ctx)
		if err != nil {
			return nil, err
		}
		messages := make([]chatMessage, 0, 2)
		if system != "" {
			messages = append(messages, chatMessage{Role: "system", Content: system})
		}
		messages = append(messages, chatMessage{Role: "user", Content: request.Prompt})
		payload, err := json.Marshal(chatPayload{
			Model:    c.chatModel,
			Messages: messages,
			ResponseFormat: responseFormat{
				Type: "json_schema",
				JSONSchema: jsonSchema{
					Name:   request.SchemaName,
					Strict: true,
					Schema: request.Schema,
				},
			},
			Provider: providerRules{
				RequireParameters: true,
				DataCollection:    "deny",
				ZDR:               true,
			},
			ZDR:         true,
			Temperature: request.Temperature,
		})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.endpoint("/api/v1/chat/completions"), bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}, &result)
	if err != nil {
		return ChatResponse{}, err
	}
	if status := c.budget.Add(result.Usage.Cost); status.HardExceeded {
		return ChatResponse{Usage: result.Usage}, ErrBudgetExceeded
	}
	if len(result.Choices) == 0 {
		return ChatResponse{}, errors.New("openrouter: chat response has no choices")
	}
	content, err := decodeMessageContent(result.Choices[0].Message.Content)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{Content: content, Usage: result.Usage}, nil
}

func decodeMessageContent(raw json.RawMessage) (string, error) {
	var content string
	if err := json.Unmarshal(raw, &content); err == nil {
		return content, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var builder strings.Builder
		for _, part := range parts {
			builder.WriteString(part.Text)
		}
		return builder.String(), nil
	}
	return "", errors.New("openrouter: unsupported chat message content")
}

type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost"`
}

func (c *Client) apiKey(ctx context.Context) (string, error) {
	key, err := c.key.APIKey(ctx)
	if err != nil {
		return "", fmt.Errorf("openrouter: API key: %w", err)
	}
	if strings.TrimSpace(key) == "" {
		return "", errors.New("openrouter: API key is not configured")
	}
	return key, nil
}

func (c *Client) endpoint(path string) string {
	endpoint := *c.baseURL
	basePath := strings.TrimRight(endpoint.Path, "/")
	if strings.HasSuffix(basePath, "/api/v1") && strings.HasPrefix(path, "/api/v1") {
		path = strings.TrimPrefix(path, "/api/v1")
	}
	endpoint.Path = basePath + path
	return endpoint.String()
}

func (c *Client) doWithRetry(ctx context.Context, requestFactory func() (*http.Request, error), output any) error {
	if err := c.budget.Check(); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := requestFactory()
		if err != nil {
			return err
		}
		response, err := c.httpClient.Do(req)
		if err != nil {
			if !isRetryableError(err) || attempt == c.maxRetries {
				return err
			}
			if err := waitRetry(ctx, c.retryBase, attempt); err != nil {
				return err
			}
			lastErr = err
			continue
		}
		data, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			if attempt == c.maxRetries {
				return readErr
			}
			lastErr = readErr
			if err := waitRetry(ctx, c.retryBase, attempt); err != nil {
				return err
			}
			continue
		}
		if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated {
			if err := json.Unmarshal(data, output); err != nil {
				return fmt.Errorf("openrouter: decode response: %w", err)
			}
			return nil
		}
		lastErr = &HTTPError{StatusCode: response.StatusCode, Body: string(data)}
		if !retryableStatus(response.StatusCode) || attempt == c.maxRetries {
			return lastErr
		}
		delay := retryDelay(response, c.retryBase, attempt)
		if err := waitRetryDuration(ctx, delay); err != nil {
			return err
		}
	}
	return lastErr
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 512 {
		body = body[:512]
	}
	if body == "" {
		return fmt.Sprintf("openrouter: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("openrouter: HTTP %d: %s", e.StatusCode, body)
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func isRetryableError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		strings.Contains(strings.ToLower(err.Error()), "connection reset")
}

func retryDelay(response *http.Response, base time.Duration, attempt int) time.Duration {
	if value := response.Header.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return base * time.Duration(1<<attempt)
}

func waitRetry(ctx context.Context, base time.Duration, attempt int) error {
	return waitRetryDuration(ctx, base*time.Duration(1<<attempt))
}

func waitRetryDuration(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type BudgetStatus struct {
	Spent        float64 `json:"spent"`
	WarnAt       float64 `json:"warnAt"`
	HardAt       float64 `json:"hardAt"`
	Warning      bool    `json:"warning"`
	HardExceeded bool    `json:"hardExceeded"`
}

type Budget struct {
	mu     sync.Mutex
	spent  float64
	warnAt float64
	hardAt float64
	warned bool
}

func NewBudget(warnAt, hardAt float64) *Budget {
	if warnAt <= 0 {
		warnAt = 0.35
	}
	if hardAt <= 0 {
		hardAt = 0.50
	}
	if warnAt >= hardAt {
		warnAt = hardAt * 0.7
	}
	return &Budget{warnAt: warnAt, hardAt: hardAt}
}

func (b *Budget) Check() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.spent >= b.hardAt {
		return ErrBudgetExceeded
	}
	return nil
}

func (b *Budget) Add(cost float64) BudgetStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	if cost > 0 {
		b.spent += cost
	}
	if b.spent >= b.warnAt {
		b.warned = true
	}
	return b.statusLocked()
}

func (b *Budget) Status() BudgetStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.statusLocked()
}

func (b *Budget) statusLocked() BudgetStatus {
	return BudgetStatus{
		Spent:        b.spent,
		WarnAt:       b.warnAt,
		HardAt:       b.hardAt,
		Warning:      b.warned || b.spent >= b.warnAt,
		HardExceeded: b.spent >= b.hardAt,
	}
}
