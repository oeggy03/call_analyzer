// Package capture defines the narrow boundary owned by the native capture
// implementation. The macOS agent can implement Source without importing the
// rest of the backend.
package capture

import (
	"context"
	"errors"
	"sync"
	"time"
)

type AudioFrame struct {
	Timestamp  time.Time
	Samples    []int16
	SampleRate int
	Channels   int
	Source     string
}

type Moment struct {
	ID    string    `json:"id"`
	Label string    `json:"label"`
	At    time.Time `json:"at"`
}

type Source interface {
	Start(context.Context, func(AudioFrame)) error
	Stop(context.Context) error
	Mark(context.Context, string) (Moment, error)
}

type Permission string

const (
	PermissionUnknown Permission = "unknown"
	PermissionGranted Permission = "granted"
	PermissionDenied  Permission = "denied"
)

type Target struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

type Event struct {
	Kind      string    `json:"kind"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text,omitempty"`
	Speaker   string    `json:"speaker,omitempty"`
	Code      string    `json:"code,omitempty"`
	Fatal     bool      `json:"fatal,omitempty"`
}

// PermissionSource and TargetSource are optional capabilities. Keeping them
// separate lets the native adapter grow without widening the core Source
// interface.
type PermissionSource interface {
	RequestPermission(context.Context) (Permission, error)
}

type TargetSource interface {
	ListTargets(context.Context) ([]Target, error)
}

// EventSource carries richer native events such as OCR without making the
// shared capture package depend on the macOS implementation.
type EventSource interface {
	SetEventHandler(func(Event))
}

// Options are applied immediately before a source starts. Platform adapters
// may reject changes while running.
type Options struct {
	TargetID   string
	Microphone bool
	OCR        bool
}

type ConfigurableSource interface {
	Configure(Options) error
}

// MockSource is a safe, silent source for development and tests. Feed can be
// used by tests to push deterministic frames through the same callback used by
// the native source.
type MockSource struct {
	mu      sync.Mutex
	running bool
	onFrame func(AudioFrame)
	onEvent func(Event)
	nextID  int
}

func NewMockSource() *MockSource {
	return &MockSource{}
}

func (s *MockSource) Start(ctx context.Context, onFrame func(AudioFrame)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = true
	s.onFrame = onFrame
	return nil
}

func (s *MockSource) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.onFrame = nil
	return nil
}

func (s *MockSource) Mark(ctx context.Context, label string) (Moment, error) {
	if err := ctx.Err(); err != nil {
		return Moment{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return Moment{}, errors.New("capture: source is not running")
	}
	s.nextID++
	return Moment{
		ID:    "mock-moment-" + formatID(s.nextID),
		Label: label,
		At:    time.Now().UTC(),
	}, nil
}

func (s *MockSource) Feed(frame AudioFrame) error {
	s.mu.Lock()
	onFrame := s.onFrame
	running := s.running
	s.mu.Unlock()
	if !running || onFrame == nil {
		return errors.New("capture: source is not running")
	}
	onFrame(frame)
	return nil
}

func (s *MockSource) RequestPermission(ctx context.Context) (Permission, error) {
	if err := ctx.Err(); err != nil {
		return PermissionUnknown, err
	}
	return PermissionGranted, nil
}

func (s *MockSource) ListTargets(ctx context.Context) ([]Target, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []Target{
		{ID: "zoom", Name: "Zoom", Available: true},
		{ID: "googleMeet", Name: "Google Meet", Available: true},
		{ID: "teams", Name: "Microsoft Teams", Available: true},
	}, nil
}

func (s *MockSource) SetEventHandler(handler func(Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEvent = handler
}

func (s *MockSource) FeedEvent(event Event) error {
	s.mu.Lock()
	onEvent := s.onEvent
	running := s.running
	s.mu.Unlock()
	if !running || onEvent == nil {
		return errors.New("capture: source is not running")
	}
	onEvent(event)
	return nil
}

func formatID(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var reversed [20]byte
	i := len(reversed)
	for value > 0 {
		i--
		reversed[i] = digits[value%10]
		value /= 10
	}
	return string(reversed[i:])
}
