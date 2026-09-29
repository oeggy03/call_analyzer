//go:build darwin

package macos

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Adapter struct {
	config Config

	mu     sync.Mutex
	cmd    *exec.Cmd
	events chan Event
	done   chan struct{}
	cancel context.CancelFunc
}

func NewAdapter(config Config) *Adapter {
	return &Adapter{config: config}
}

// ListTargets asks the helper for its current ScreenCaptureKit shareable
// content. The helper is short-lived for this command.
func (a *Adapter) ListTargets(ctx context.Context) ([]Target, error) {
	helper, err := resolveHelperPath(a.config)
	if err != nil {
		return nil, err
	}
	cmd := a.command(ctx, helper, "list")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create helper stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create helper stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start capture helper: %w", err)
	}

	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(os.Stderr, stderr)
		close(stderrDone)
	}()

	targets := make([]Target, 0)
	var parseErr error
	scanner := newHelperScanner(stdout)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			if parseErr == nil {
				parseErr = MalformedLineError{LineNumber: lineNumber}
			}
			continue
		}
		if event.Type == "" {
			if parseErr == nil {
				parseErr = MalformedLineError{LineNumber: lineNumber}
			}
			continue
		}
		switch event.Type {
		case "target":
			targets = append(targets, Target{
				BundleID:        event.BundleID,
				ApplicationName: event.ApplicationName,
				PID:             event.PID,
				Windows:         event.Windows,
			})
		case "error":
			if parseErr == nil {
				parseErr = fmt.Errorf("capture helper: %s", event.Message)
			}
		}
	}
	if err := scanner.Err(); err != nil && parseErr == nil {
		parseErr = fmt.Errorf("read capture helper stdout: %w", err)
	}
	waitErr := cmd.Wait()
	<-stderrDone
	if parseErr != nil {
		return targets, parseErr
	}
	if waitErr != nil {
		return targets, HelperExitError{Err: waitErr}
	}
	return targets, nil
}

// Start starts a long-lived capture helper and returns its decoded event
// stream. A canceled context terminates the helper through exec.CommandContext.
func (a *Adapter) Start(
	ctx context.Context,
	options CaptureOptions,
) (EventStream, error) {
	if err := validateCaptureOptions(options); err != nil {
		return nil, err
	}
	if options.ChunkSeconds == 0 {
		options.ChunkSeconds = 2
	}
	helper, err := resolveHelperPath(a.config)
	if err != nil {
		return nil, err
	}

	args := []string{
		"capture",
		"--bundle-id", options.BundleID,
		"--spool", options.SpoolDir,
		"--chunk-seconds", strconv.Itoa(options.ChunkSeconds),
	}
	if options.Microphone {
		args = append(args, "--microphone")
	}
	if options.OCR {
		args = append(args, "--ocr")
	}
	runCtx, cancel := context.WithCancel(ctx)
	cmd := a.command(runCtx, helper, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create helper stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create helper stderr pipe: %w", err)
	}

	a.mu.Lock()
	if a.cmd != nil {
		a.mu.Unlock()
		cancel()
		return nil, ErrAlreadyRunning
	}
	if err := cmd.Start(); err != nil {
		a.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("start capture helper: %w", err)
	}
	events := make(chan Event, 32)
	done := make(chan struct{})
	a.cmd = cmd
	a.events = events
	a.done = done
	a.cancel = cancel
	a.mu.Unlock()

	go a.consume(runCtx, cancel, cmd, stdout, stderr, events, done)
	return EventStream(events), nil
}

// Stop asks the native process to flush and exit. The helper handles SIGTERM
// by stopping its ScreenCaptureKit stream and finalizing open WAV chunks.
func (a *Adapter) Stop(ctx context.Context) error {
	a.mu.Lock()
	cmd := a.cmd
	done := a.done
	cancel := a.cancel
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		if !errors.Is(err, os.ErrProcessDone) && !strings.Contains(err.Error(), "already finished") {
			return fmt.Errorf("signal capture helper: %w", err)
		}
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		if cancel != nil {
			cancel()
		}
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-ctx.Done():
		}
		return ctx.Err()
	}
}

func (a *Adapter) consume(
	ctx context.Context,
	cancel context.CancelFunc,
	cmd *exec.Cmd,
	stdout io.ReadCloser,
	stderr io.ReadCloser,
	events chan Event,
	done chan struct{},
) {
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(os.Stderr, stderr)
		close(stderrDone)
	}()

	scanner := newHelperScanner(stdout)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			if !sendEvent(ctx, events, Event{
				Type:    "error",
				Code:    "malformed_json",
				Message: fmt.Sprintf("helper stdout line %d was not valid JSON", lineNumber),
			}) {
				break
			}
			continue
		}
		if event.Type == "" {
			if !sendEvent(ctx, events, Event{
				Type:    "error",
				Code:    "malformed_event",
				Message: fmt.Sprintf("helper stdout line %d did not include a type", lineNumber),
			}) {
				break
			}
			continue
		}
		if !sendEvent(ctx, events, event) {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		_ = sendEvent(ctx, events, Event{
			Type:    "error",
			Code:    "stdout_read",
			Message: fmt.Sprintf("could not read helper stdout: %v", err),
		})
	}
	waitErr := cmd.Wait()
	<-stderrDone
	if waitErr != nil && ctx.Err() == nil {
		_ = sendEvent(ctx, events, Event{
			Type:    "error",
			Code:    "helper_exit",
			Message: HelperExitError{Err: waitErr}.Error(),
		})
	}
	close(events)
	close(done)

	a.mu.Lock()
	if a.cmd == cmd {
		a.cmd = nil
		a.events = nil
		a.done = nil
		a.cancel = nil
	}
	a.mu.Unlock()
	cancel()
}

func (a *Adapter) command(ctx context.Context, helper string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, helper, args...)
	// Give the helper a chance to flush its final WAV chunk when the parent
	// context is canceled. WaitDelay remains a hard backstop for a wedged
	// helper that ignores SIGTERM.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = a.config.WorkingDir
	if len(a.config.Environment) > 0 {
		cmd.Env = append(os.Environ(), a.config.Environment...)
	}
	return cmd
}

func validateCaptureOptions(options CaptureOptions) error {
	if strings.TrimSpace(options.BundleID) == "" {
		return fmt.Errorf("bundle id is required")
	}
	if strings.TrimSpace(options.SpoolDir) == "" {
		return fmt.Errorf("spool directory is required")
	}
	if options.ChunkSeconds != 0 &&
		(options.ChunkSeconds < 1 || options.ChunkSeconds > 3600) {
		return fmt.Errorf("chunk seconds must be between 1 and 3600")
	}
	if err := os.MkdirAll(options.SpoolDir, 0o755); err != nil {
		return fmt.Errorf("create spool directory: %w", err)
	}
	return nil
}

func newHelperScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	return scanner
}

func sendEvent(ctx context.Context, events chan<- Event, event Event) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func resolveHelperPath(config Config) (string, error) {
	candidates := make([]string, 0, 6)
	if config.HelperPath != "" {
		candidates = append(candidates, config.HelperPath)
	}
	for _, key := range []string{
		"CALL_ANALYZER_CAPTURE_HELPER",
		"CALL_ANALYZER_CAPTURE_BIN",
	} {
		if value := os.Getenv(key); value != "" {
			candidates = append(candidates, value)
		}
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(
			filepath.Dir(executable),
			"call-analyzer-capture",
		))
	}
	candidates = append(candidates, filepath.Join("build", "bin", "call-analyzer-capture"))

	var lastErr error
	for _, candidate := range candidates {
		if !strings.ContainsRune(candidate, os.PathSeparator) {
			if path, err := exec.LookPath(candidate); err == nil {
				return path, nil
			} else {
				lastErr = err
				continue
			}
		}
		path := candidate
		if !filepath.IsAbs(path) {
			base := config.WorkingDir
			if base == "" {
				base, _ = os.Getwd()
			}
			path = filepath.Join(base, path)
		}
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return "", fmt.Errorf(
		"could not locate call-analyzer-capture helper; set CALL_ANALYZER_CAPTURE_HELPER or Config.HelperPath: %w",
		lastErr,
	)
}
