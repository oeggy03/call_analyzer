//go:build darwin

package selection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Start launches the helper when it is available. A missing helper is not an
// application startup failure because development builds may omit native
// helpers.
func (r *Runner) Start(ctx context.Context, handler Handler) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if handler == nil {
		return errors.New("selection helper handler is nil")
	}

	helper, err := resolveHelperPath(r.config)
	if err != nil {
		return err
	}
	if helper == "" {
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, helper)
	cmd.Env = helperEnvironment()
	cmd.Stderr = io.Discard
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 2 * time.Second

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("create selection helper stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("create selection helper stdout: %w", err)
	}

	r.mu.Lock()
	if r.cmd != nil {
		r.mu.Unlock()
		cancel()
		return ErrAlreadyRunning
	}
	if err := cmd.Start(); err != nil {
		r.mu.Unlock()
		cancel()
		return fmt.Errorf("start selection helper: %w", err)
	}
	done := make(chan struct{})
	r.cmd = cmd
	r.stdin = stdin
	r.done = done
	r.cancel = cancel
	r.mu.Unlock()

	go r.consume(runCtx, cmd, stdin, stdout, handler, done)
	return nil
}

// Stop closes the helper protocol, requests a graceful exit, and applies a
// short hard-stop backstop for a helper that does not honor termination.
func (r *Runner) Stop(context.Context) error {
	r.mu.Lock()
	cmd := r.cmd
	stdin := r.stdin
	done := r.done
	cancel := r.cancel
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if stdin != nil {
		_ = stdin.Close()
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil &&
		!processAlreadyFinished(err) {
		// The helper may have exited immediately after stdin closed. There is
		// no useful recovery at shutdown, so continue to the wait/kill path.
	}
	if cancel != nil {
		cancel()
	}

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
		return nil
	}
}

func (r *Runner) consume(
	ctx context.Context,
	cmd *exec.Cmd,
	stdin io.WriteCloser,
	stdout io.ReadCloser,
	handler Handler,
	done chan struct{},
) {
	_ = Serve(ctx, stdout, stdin, handler)
	_ = stdin.Close()
	_ = cmd.Wait()

	r.mu.Lock()
	cancel := r.cancel
	if r.cmd == cmd {
		r.cmd = nil
		r.stdin = nil
		r.done = nil
		r.cancel = nil
	}
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	close(done)
}

func processAlreadyFinished(err error) bool {
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already finished") ||
		strings.Contains(message, "no such process")
}

func resolveHelperPath(config Config) (string, error) {
	candidates := make([]string, 0, 4)
	if strings.TrimSpace(config.HelperPath) != "" {
		candidates = append(candidates, config.HelperPath)
	}
	if value := strings.TrimSpace(os.Getenv("CALL_ANALYZER_LOOKUP_HELPER")); value != "" {
		candidates = append(candidates, value)
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(
			filepath.Dir(executable),
			"call-analyzer-lookup",
		))
	}
	candidates = append(candidates, filepath.Join("build", "bin", "call-analyzer-lookup"))

	for _, candidate := range candidates {
		if !strings.ContainsRune(candidate, os.PathSeparator) {
			if path, err := exec.LookPath(candidate); err == nil {
				return path, nil
			}
			continue
		}
		path := candidate
		if !filepath.IsAbs(path) {
			if workingDirectory, err := os.Getwd(); err == nil {
				path = filepath.Join(workingDirectory, path)
			}
		}
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", nil
}
