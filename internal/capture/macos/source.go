package macos

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oeggy03/call_analyzer/internal/capture"
)

const defaultBundleID = "us.zoom.xos"

var helperReadyTimeout = 20 * time.Second

// Source adapts the native helper to capture.Source. It intentionally keeps
// the parent's pre/post-roll buffering out of this package.
type Source struct {
	adapter *Adapter
	config  Config

	mu          sync.Mutex
	running     bool
	onFrame     func(capture.AudioFrame)
	eventHandle func(capture.Event)
	nativeEvent NativeEventHandler
	sessionDir  string
	eventsDone  chan struct{}
	startedAt   time.Time
	markCounter uint64
}

var (
	_ capture.Source             = (*Source)(nil)
	_ capture.PermissionSource   = (*Source)(nil)
	_ capture.TargetSource       = (*Source)(nil)
	_ capture.EventSource        = (*Source)(nil)
	_ capture.ConfigurableSource = (*Source)(nil)
)

// NewSource creates a shared capture-contract adapter.
func NewSource(config Config) *Source {
	config = normalizeConfig(config)
	parent, prefix := sessionSpoolLocation(config.SpoolDir)
	_ = cleanupStaleSessionSpools(parent, prefix)
	return &Source{
		adapter: NewAdapter(config),
		config:  config,
	}
}

// Configure applies privacy-sensitive capture options before a session starts.
func (s *Source) Configure(options capture.Options) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return ErrAlreadyRunning
	}
	switch options.TargetID {
	case "", "zoom":
		s.config.BundleID = defaultBundleID
	default:
		return fmt.Errorf("macos capture: unsupported target %q", options.TargetID)
	}
	s.config.Microphone = options.Microphone
	s.config.OCR = options.OCR
	s.adapter = NewAdapter(s.config)
	return nil
}

// SetEventHandler implements capture.EventSource. OCR, state, and error
// events are mapped into the shared event shape.
func (s *Source) SetEventHandler(handler func(capture.Event)) {
	s.mu.Lock()
	s.eventHandle = handler
	s.mu.Unlock()
}

// SetNativeEventHandler exposes the complete helper event for callers that
// need paths, confidence, or error codes not present in capture.Event.
func (s *Source) SetNativeEventHandler(handler NativeEventHandler) {
	s.mu.Lock()
	s.nativeEvent = handler
	s.mu.Unlock()
}

func (s *Source) Start(
	ctx context.Context,
	onFrame func(capture.AudioFrame),
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if onFrame == nil {
		return errors.New("macos capture: audio callback is required")
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	config := s.config
	s.startedAt = time.Now().UTC()
	s.onFrame = onFrame
	s.running = true
	s.mu.Unlock()

	sessionDir, err := createSessionSpool(config.SpoolDir)
	if err != nil {
		s.resetAfterStartFailure()
		return fmt.Errorf("macos capture: create session spool: %w", err)
	}

	s.mu.Lock()
	s.sessionDir = sessionDir
	s.eventsDone = make(chan struct{})
	startedAt := s.startedAt
	done := s.eventsDone
	s.mu.Unlock()

	events, err := s.adapter.Start(ctx, CaptureOptions{
		BundleID:     config.BundleID,
		SpoolDir:     sessionDir,
		Microphone:   config.Microphone,
		OCR:          config.OCR,
		ChunkSeconds: config.ChunkSeconds,
	})
	if err != nil {
		s.cleanupSession(sessionDir)
		s.resetAfterStartFailure()
		return err
	}

	startup := make(chan error, 1)
	go s.consumeEvents(events, done, startup, startedAt, onFrame, sessionDir)
	readyTimer := time.NewTimer(helperReadyTimeout)
	defer readyTimer.Stop()

	select {
	case err := <-startup:
		if err != nil {
			stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = s.Stop(stopContext)
			cancel()
			return err
		}
		return nil
	case <-ctx.Done():
		stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = s.Stop(stopContext)
		cancel()
		return ctx.Err()
	case <-readyTimer.C:
		stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = s.Stop(stopContext)
		cancel()
		return fmt.Errorf("macos capture helper did not become ready within %s", helperReadyTimeout)
	}
}

func (s *Source) consumeEvents(
	events EventStream,
	done chan struct{},
	startup chan<- error,
	startedAt time.Time,
	onFrame func(capture.AudioFrame),
	sessionDir string,
) {
	ready := false
	signalStartup := func(err error) {
		select {
		case startup <- err:
		default:
		}
	}

	for event := range events {
		s.dispatchEvent(event, startedAt)
		if event.Type == "ready" {
			ready = true
			signalStartup(nil)
			continue
		}
		if event.Type == "error" && !ready {
			signalStartup(fmt.Errorf("macos capture helper: %s", event.Message))
			continue
		}
		if event.Type != "audio_chunk" {
			continue
		}

		frame, err := s.readAudioChunk(event, startedAt, sessionDir)
		if err != nil {
			s.dispatchEvent(Event{
				Type:    "error",
				Code:    "audio_chunk",
				Message: err.Error(),
			}, startedAt)
			continue
		}
		onFrame(frame)
	}

	if !ready {
		signalStartup(errors.New("macos capture helper exited before ready"))
	}

	s.mu.Lock()
	s.running = false
	s.onFrame = nil
	s.sessionDir = ""
	s.eventsDone = nil
	s.mu.Unlock()
	s.cleanupSession(sessionDir)
	close(done)
}

func (s *Source) dispatchEvent(event Event, startedAt time.Time) {
	s.mu.Lock()
	handler := s.eventHandle
	nativeHandler := s.nativeEvent
	s.mu.Unlock()
	if nativeHandler != nil {
		nativeHandler(event)
	}
	if handler != nil {
		if mapped, ok := mapParentEvent(event, startedAt); ok {
			handler(mapped)
		}
	}
}

func (s *Source) readAudioChunk(
	event Event,
	startedAt time.Time,
	sessionDir string,
) (capture.AudioFrame, error) {
	path, err := safeChunkPath(event.Path, sessionDir)
	if err != nil {
		return capture.AudioFrame{}, err
	}
	defer func() {
		_ = os.Remove(path)
	}()

	data, err := os.ReadFile(path)
	if err != nil {
		return capture.AudioFrame{}, AudioChunkError{
			Path:   event.Path,
			Reason: err.Error(),
		}
	}
	decoded, err := decodePCM16WAV(data)
	if err != nil {
		return capture.AudioFrame{}, AudioChunkError{
			Path:   event.Path,
			Reason: err.Error(),
		}
	}
	if decoded.SampleRate != 16_000 || decoded.Channels != 1 {
		return capture.AudioFrame{}, AudioChunkError{
			Path: event.Path,
			Reason: fmt.Sprintf(
				"expected 16 kHz mono PCM, got %d Hz/%d channels",
				decoded.SampleRate,
				decoded.Channels,
			),
		}
	}
	if event.SampleRate != 0 && event.SampleRate != decoded.SampleRate {
		return capture.AudioFrame{}, AudioChunkError{
			Path: event.Path,
			Reason: fmt.Sprintf(
				"event sample rate %d does not match WAV sample rate %d",
				event.SampleRate,
				decoded.SampleRate,
			),
		}
	}
	if event.Channels != 0 && event.Channels != decoded.Channels {
		return capture.AudioFrame{}, AudioChunkError{
			Path: event.Path,
			Reason: fmt.Sprintf(
				"event channel count %d does not match WAV channel count %d",
				event.Channels,
				decoded.Channels,
			),
		}
	}
	if event.StartMS < 0 || event.EndMS < event.StartMS {
		return capture.AudioFrame{}, AudioChunkError{
			Path:   event.Path,
			Reason: "invalid helper timestamps",
		}
	}

	frame := capture.AudioFrame{
		Timestamp:  startedAt.Add(time.Duration(event.StartMS) * time.Millisecond),
		Samples:    decoded.Samples,
		SampleRate: decoded.SampleRate,
		Channels:   decoded.Channels,
		Source:     event.Source,
	}
	return frame, nil
}

// Stop requests a graceful helper stop, drains final events, and removes the
// session directory. The configured parent spool directory is preserved.
func (s *Source) Stop(ctx context.Context) error {
	s.mu.Lock()
	running := s.running
	done := s.eventsDone
	s.mu.Unlock()
	if !running {
		return nil
	}

	err := s.adapter.Stop(ctx)
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			if err == nil {
				err = ctx.Err()
			}
		}
	}
	return err
}

// Mark returns a stable per-source moment id. The parent service owns any
// pre/post-roll buffering around this timestamp.
func (s *Source) Mark(ctx context.Context, label string) (capture.Moment, error) {
	if err := ctx.Err(); err != nil {
		return capture.Moment{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return capture.Moment{}, ErrNotRunning
	}
	s.markCounter++
	return capture.Moment{
		ID:    fmt.Sprintf("macos-moment-%d", s.markCounter),
		Label: label,
		At:    time.Now().UTC(),
	}, nil
}

// RequestPermission invokes the helper's shareable-content query. Any TCC
// denial is returned unchanged so callers can display its actionable message.
func (s *Source) RequestPermission(ctx context.Context) (capture.Permission, error) {
	_, err := s.ListTargets(ctx)
	if err != nil {
		return capture.PermissionDenied, err
	}
	return capture.PermissionGranted, nil
}

func (s *Source) ListTargets(ctx context.Context) ([]capture.Target, error) {
	targets, err := s.adapter.ListTargets(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]capture.Target, 0, len(targets))
	for _, target := range targets {
		result = append(result, capture.Target{
			ID:        target.BundleID,
			Name:      target.ApplicationName,
			Available: true,
		})
	}
	return result, nil
}

func (s *Source) resetAfterStartFailure() {
	s.mu.Lock()
	s.running = false
	s.onFrame = nil
	s.sessionDir = ""
	s.eventsDone = nil
	s.mu.Unlock()
}

func (s *Source) cleanupSession(sessionDir string) {
	if sessionDir != "" {
		_ = os.RemoveAll(sessionDir)
	}
}

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.BundleID) == "" {
		config.BundleID = defaultBundleID
	}
	if config.ChunkSeconds == 0 {
		config.ChunkSeconds = 2
	}
	return config
}

func createSessionSpool(parent string) (string, error) {
	parent, prefix := sessionSpoolLocation(parent)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	if err := cleanupStaleSessionSpools(parent, prefix); err != nil {
		return "", err
	}
	sessionDir, err := os.MkdirTemp(parent, prefix)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(
		filepath.Join(sessionDir, ".owner-pid"),
		[]byte(strconv.Itoa(os.Getpid())),
		0o600,
	); err != nil {
		_ = os.RemoveAll(sessionDir)
		return "", err
	}
	return sessionDir, nil
}

func sessionSpoolLocation(parent string) (string, string) {
	if parent == "" {
		return os.TempDir(), "call-analyzer-capture-"
	}
	return parent, "session-"
}

func cleanupStaleSessionSpools(parent, prefix string) error {
	matches, err := filepath.Glob(filepath.Join(parent, prefix+"*"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() {
			continue
		}
		ownerBytes, readErr := os.ReadFile(filepath.Join(path, ".owner-pid"))
		ownerPID, parseErr := strconv.Atoi(strings.TrimSpace(string(ownerBytes)))
		if readErr == nil && parseErr == nil && processIsAlive(ownerPID) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func processIsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		return pid == os.Getpid()
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func safeChunkPath(path, sessionDir string) (string, error) {
	if path == "" {
		return "", AudioChunkError{Reason: "audio_chunk path is empty"}
	}
	cleanPath := filepath.Clean(path)
	relative, err := filepath.Rel(sessionDir, cleanPath)
	if err != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", AudioChunkError{
			Path:   path,
			Reason: "audio_chunk path is outside the session spool",
		}
	}
	return cleanPath, nil
}

type decodedWAV struct {
	SampleRate int
	Channels   int
	Samples    []int16
}

func decodePCM16WAV(data []byte) (decodedWAV, error) {
	if len(data) < 12 {
		return decodedWAV{}, errors.New("file is shorter than a RIFF header")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return decodedWAV{}, errors.New("file is not a RIFF/WAVE file")
	}
	riffSize := int(binary.LittleEndian.Uint32(data[4:8]))
	if riffSize != len(data)-8 {
		return decodedWAV{}, fmt.Errorf(
			"RIFF size %d does not match file size %d",
			riffSize,
			len(data)-8,
		)
	}

	var (
		sampleRate int
		channels   int
		blockAlign int
		byteRate   int
		dataBytes  []byte
		haveFormat bool
		haveData   bool
	)
	offset := 12
	for offset < len(data) {
		if len(data)-offset < 8 {
			return decodedWAV{}, errors.New("truncated WAV chunk header")
		}
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + chunkSize
		if chunkEnd < chunkStart || chunkEnd > len(data) {
			return decodedWAV{}, errors.New("WAV chunk exceeds file size")
		}
		switch chunkID {
		case "fmt ":
			if haveFormat || chunkSize < 16 {
				return decodedWAV{}, errors.New("invalid or duplicate WAV format chunk")
			}
			audioFormat := binary.LittleEndian.Uint16(data[chunkStart : chunkStart+2])
			channels = int(binary.LittleEndian.Uint16(data[chunkStart+2 : chunkStart+4]))
			sampleRate = int(binary.LittleEndian.Uint32(data[chunkStart+4 : chunkStart+8]))
			byteRate = int(binary.LittleEndian.Uint32(data[chunkStart+8 : chunkStart+12]))
			blockAlign = int(binary.LittleEndian.Uint16(data[chunkStart+12 : chunkStart+14]))
			bitsPerSample := binary.LittleEndian.Uint16(data[chunkStart+14 : chunkStart+16])
			if audioFormat != 1 || bitsPerSample != 16 {
				return decodedWAV{}, errors.New("WAV must be uncompressed 16-bit PCM")
			}
			if channels <= 0 || sampleRate <= 0 ||
				blockAlign != channels*2 || byteRate != sampleRate*blockAlign {
				return decodedWAV{}, errors.New("WAV format fields are inconsistent")
			}
			haveFormat = true
		case "data":
			if haveData {
				return decodedWAV{}, errors.New("duplicate WAV data chunk")
			}
			dataBytes = data[chunkStart:chunkEnd]
			haveData = true
		}
		offset = chunkEnd
		if chunkSize%2 != 0 {
			if offset >= len(data) {
				return decodedWAV{}, errors.New("missing WAV chunk padding")
			}
			offset++
		}
	}
	if !haveFormat || !haveData {
		return decodedWAV{}, errors.New("WAV is missing fmt or data chunk")
	}
	if len(dataBytes) == 0 || len(dataBytes)%blockAlign != 0 {
		return decodedWAV{}, errors.New("WAV data size is not frame-aligned")
	}

	samples := make([]int16, len(dataBytes)/2)
	for index := range samples {
		samples[index] = int16(binary.LittleEndian.Uint16(
			dataBytes[index*2 : index*2+2],
		))
	}
	return decodedWAV{
		SampleRate: sampleRate,
		Channels:   channels,
		Samples:    samples,
	}, nil
}

func mapParentEvent(event Event, startedAt time.Time) (capture.Event, bool) {
	timestamp := startedAt
	if event.Timestamp >= 0 {
		timestamp = startedAt.Add(time.Duration(event.Timestamp) * time.Millisecond)
	}
	switch event.Type {
	case "ocr":
		return capture.Event{
			Kind:      "ocr",
			Source:    event.Source,
			Timestamp: timestamp,
			Text:      event.Text,
		}, true
	case "state":
		text := event.State
		if event.Message != "" {
			text = event.Message
		}
		return capture.Event{
			Kind:      "state",
			Source:    event.Source,
			Timestamp: timestamp,
			Text:      text,
		}, true
	case "error":
		return capture.Event{
			Kind:      "error",
			Source:    event.Source,
			Timestamp: timestamp,
			Text:      event.Message,
		}, true
	default:
		return capture.Event{}, false
	}
}
