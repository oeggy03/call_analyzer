package selection

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var ErrAlreadyRunning = errors.New("selection helper is already running")

const (
	lookupFailureMessage = "Couldn't look this up. Try again."
	addFailureMessage    = "Couldn't add this word. Try again."
	lookupTimeout        = 20 * time.Second
)

// Handler contains the service operations used by the selection helper.
type Handler interface {
	LookupSelection(
		ctx context.Context,
		surface, sentenceContext string,
	) (LookupResult, error)
	AddSelection(ctx context.Context, line GlossLine) error
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
	Type    string     `json:"type"`
	ID      string     `json:"id"`
	Surface string     `json:"surface"`
	Context *string    `json:"context"`
	Bounds  *Bounds    `json:"bounds"`
	Line    *GlossLine `json:"line"`
}

func (r helperRequest) sentenceContext() string {
	if r.Context == nil {
		return ""
	}
	return *r.Context
}

type resultMessage struct {
	Type    string      `json:"type"`
	ID      string      `json:"id"`
	Surface string      `json:"surface"`
	Lines   []GlossLine `json:"lines"`
	Notice  string      `json:"notice"`
}

type errorMessage struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Message string `json:"message"`
}

type dismissMessage struct {
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
				recordWriteError(write(resultMessage{
					Type:    "result",
					ID:      request.ID,
					Surface: result.Surface,
					Lines:   lines,
					Notice:  result.Notice,
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
				callCtx, callCancel := context.WithTimeout(serveCtx, lookupTimeout)
				defer callCancel()

				if err := handler.AddSelection(callCtx, *request.Line); err != nil {
					recordWriteError(write(errorMessage{
						Type:    "error",
						ID:      request.ID,
						Message: addFailureMessage,
					}))
					return
				}
				recordWriteError(write(dismissMessage{
					Type: "dismiss",
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
