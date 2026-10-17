package macos

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestEventJSONRoundTrip(t *testing.T) {
	input := []byte(`{"type":"audio_chunk","source":"remote","path":"/tmp/remote.wav","start_ms":0,"end_ms":1000,"sample_rate":16000,"channels":1}`)
	var event Event
	if err := json.Unmarshal(input, &event); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if event.Type != "audio_chunk" || event.Source != "remote" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.SampleRate != 16000 || event.Channels != 1 {
		t.Fatalf("unexpected audio format: %+v", event)
	}
}

func TestNonDarwinStubIsExplicit(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("stub is only used on non-darwin systems")
	}
	adapter := NewAdapter(Config{})
	if _, err := adapter.ListTargets(context.Background()); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("expected unsupported-platform error, got %v", err)
	}
}

func TestDarwinHelperParsingAndMalformedLines(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native helper process tests run on darwin")
	}
	fixture, err := filepath.Abs(filepath.Join("testdata", "fake-helper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fixture, 0o755); err != nil {
		t.Fatal(err)
	}

	adapter := NewAdapter(Config{HelperPath: fixture})
	targets, err := adapter.ListTargets(context.Background())
	if err != nil {
		t.Fatalf("list targets: %v", err)
	}
	if len(targets) != 1 || targets[0].BundleID != "example.app" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	if len(targets[0].Windows) != 1 || targets[0].Windows[0].WindowID != 7 {
		t.Fatalf("unexpected windows: %+v", targets[0].Windows)
	}

	events, err := adapter.Start(context.Background(), CaptureOptions{
		BundleID:     "example.app",
		SpoolDir:     t.TempDir(),
		ChunkSeconds: 1,
	})
	if err != nil {
		t.Fatalf("start helper: %v", err)
	}
	var got []Event
	for event := range events {
		got = append(got, event)
	}
	if len(got) != 2 {
		t.Fatalf("expected malformed error and ready event, got %+v", got)
	}
	if got[0].Type != "error" || got[0].Code != "malformed_json" {
		t.Fatalf("unexpected malformed-line event: %+v", got[0])
	}
	if got[1].Type != "ready" {
		t.Fatalf("unexpected ready event: %+v", got[1])
	}
}

func TestDarwinStopIsIdempotentWhenNotRunning(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native helper process tests run on darwin")
	}
	adapter := NewAdapter(Config{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := adapter.Stop(ctx); err != nil {
		t.Fatalf("stop without process: %v", err)
	}
}
