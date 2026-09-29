// Package macos provides the Go-side adapter for the macOS ScreenCaptureKit
// helper. It intentionally owns only process control and JSON-line decoding;
// capture policy and media processing remain in the native helper.
package macos

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrUnsupportedPlatform = errors.New("macOS capture is unavailable on this platform")
	ErrAlreadyRunning      = errors.New("macOS capture helper is already running")
	ErrNotRunning          = errors.New("macOS capture helper is not running")
)

// Config controls how the native helper is located and launched.
type Config struct {
	// HelperPath overrides helper discovery. Relative paths are resolved from
	// WorkingDir when it is set, otherwise from the current working directory.
	HelperPath string
	// WorkingDir is the helper process working directory.
	WorkingDir string
	// Environment is appended to the current environment for the helper.
	Environment []string
	// BundleID selects the running application. It defaults to Zoom's bundle
	// identifier when empty.
	BundleID string
	// SpoolDir is the parent directory for one private capture session. When
	// empty, a private temporary directory is used.
	SpoolDir     string
	Microphone   bool
	OCR          bool
	ChunkSeconds int
}

// CaptureOptions are translated into native helper command-line flags.
type CaptureOptions struct {
	BundleID     string
	SpoolDir     string
	Microphone   bool
	OCR          bool
	ChunkSeconds int
}

// Event is the local integration contract for one helper JSON-line event.
// It is deliberately independent of internal/capture/capture.go so this
// adapter can be integrated without changing another package's ownership.
type Event struct {
	Type            string   `json:"type"`
	BundleID        string   `json:"bundle_id,omitempty"`
	ApplicationName string   `json:"application_name,omitempty"`
	PID             int32    `json:"pid,omitempty"`
	Windows         []Window `json:"windows,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	State           string   `json:"state,omitempty"`
	Source          string   `json:"source,omitempty"`
	Path            string   `json:"path,omitempty"`
	StartMS         int64    `json:"start_ms,omitempty"`
	EndMS           int64    `json:"end_ms,omitempty"`
	SampleRate      int      `json:"sample_rate,omitempty"`
	Channels        int      `json:"channels,omitempty"`
	Text            string   `json:"text,omitempty"`
	Timestamp       int64    `json:"timestamp,omitempty"`
	Confidence      float64  `json:"confidence,omitempty"`
	Code            string   `json:"code,omitempty"`
	Message         string   `json:"message,omitempty"`
	Actionable      bool     `json:"actionable,omitempty"`
}

// Window describes one capturable application window.
type Window struct {
	WindowID uint32  `json:"window_id"`
	Title    string  `json:"title"`
	OnScreen bool    `json:"on_screen"`
	Active   bool    `json:"active"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
}

// Target is one running application and its currently visible windows.
type Target struct {
	BundleID        string
	ApplicationName string
	PID             int32
	Windows         []Window
}

// EventStream is closed after the helper exits and all stdout has been read.
type EventStream <-chan Event

// NativeEventHandler receives the helper's wire events for callers that need
// fields not represented by capture.Event. Source.SetEventHandler exposes the
// shared capture.Event handler when available.
type NativeEventHandler func(Event)

// AudioChunkError identifies a malformed or unsupported helper WAV chunk.
type AudioChunkError struct {
	Path   string
	Reason string
}

func (e AudioChunkError) Error() string {
	if e.Path == "" {
		return "macOS capture audio chunk is invalid: " + e.Reason
	}
	return fmt.Sprintf("macOS capture audio chunk %q is invalid: %s", e.Path, e.Reason)
}

// MalformedLineError identifies a helper stdout line that was not valid JSON.
type MalformedLineError struct {
	LineNumber int
}

func (e MalformedLineError) Error() string {
	return "macOS capture helper emitted malformed JSON"
}

// HelperExitError reports a non-zero helper exit after the event stream ends.
type HelperExitError struct {
	Err error
}

func (e HelperExitError) Error() string {
	if e.Err == nil {
		return "macOS capture helper exited unsuccessfully"
	}
	return "macOS capture helper exited unsuccessfully: " + e.Err.Error()
}

func (e HelperExitError) Unwrap() error {
	return e.Err
}

// EventTimestamp returns the timestamp used by the helper, when present.
func EventTimestamp(event Event) time.Duration {
	return time.Duration(event.Timestamp) * time.Millisecond
}
