package macos

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/capture"
)

const tinyPCM16WAV = "UklGRigAAABXQVZFZm10IBAAAAABAAEAgD4AAAB9AAACABAAZGF0YQQAAAABAP//"

func TestDecodePCM16WAVAndRejectBadSizes(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(tinyPCM16WAV)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodePCM16WAV(data)
	if err != nil {
		t.Fatalf("decode valid fixture: %v", err)
	}
	if decoded.SampleRate != 16_000 || decoded.Channels != 1 {
		t.Fatalf("unexpected format: %+v", decoded)
	}
	if len(decoded.Samples) != 2 || decoded.Samples[0] != 1 || decoded.Samples[1] != -1 {
		t.Fatalf("unexpected samples: %+v", decoded.Samples)
	}

	data[4]--
	if _, err := decodePCM16WAV(data); err == nil {
		t.Fatal("expected RIFF size validation error")
	}
}

func TestDefaultConfigAndMarkBeforeStart(t *testing.T) {
	source := NewSource(Config{})
	if source.config.BundleID != defaultBundleID {
		t.Fatalf("default bundle id = %q", source.config.BundleID)
	}
	if source.config.ChunkSeconds != 10 {
		t.Fatalf("default chunk seconds = %d", source.config.ChunkSeconds)
	}
	if _, err := source.Mark(context.Background(), "before-start"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected Mark-before-start error, got %v", err)
	}
}

func TestSourceWithFakeHelperDeliversAndCleansAudio(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("fake native helper process test runs on macOS")
	}
	fixture := fakeSourceHelper(t)
	parentSpool := t.TempDir()
	source := NewSource(Config{
		HelperPath:   fixture,
		BundleID:     "example.app",
		SpoolDir:     parentSpool,
		ChunkSeconds: 1,
	})
	frames := make(chan capture.AudioFrame, 1)
	if err := source.Start(context.Background(), func(frame capture.AudioFrame) {
		frames <- frame
	}); err != nil {
		t.Fatalf("start source: %v", err)
	}

	select {
	case frame := <-frames:
		if frame.Source != "remote" || frame.SampleRate != 16_000 || frame.Channels != 1 {
			t.Fatalf("unexpected frame: %+v", frame)
		}
		if len(frame.Samples) != 2 || frame.Samples[0] != 1 || frame.Samples[1] != -1 {
			t.Fatalf("unexpected samples: %+v", frame.Samples)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for audio callback")
	}

	stopContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := source.Stop(stopContext); err != nil {
		t.Fatalf("stop source: %v", err)
	}
	entries, err := os.ReadDir(parentSpool)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("session spool was not cleaned: %+v", entries)
	}
}

func TestSourceReportsMalformedWAVAndCleansChunk(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("fake native helper process test runs on macOS")
	}
	fixture := fakeSourceHelper(t)
	parentSpool := t.TempDir()
	source := NewSource(Config{
		HelperPath:  fixture,
		BundleID:    "example.app",
		SpoolDir:    parentSpool,
		Environment: []string{"FAKE_INVALID_WAV=1"},
	})
	events := make(chan capture.Event, 2)
	source.SetEventHandler(func(event capture.Event) {
		events <- event
	})
	if err := source.Start(context.Background(), func(capture.AudioFrame) {
		t.Error("malformed WAV should not produce an audio frame")
	}); err != nil {
		t.Fatalf("start source: %v", err)
	}

	select {
	case event := <-events:
		if event.Kind != "error" {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for malformed WAV event")
	}
	stopContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = source.Stop(stopContext)
	entries, err := os.ReadDir(parentSpool)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("malformed session spool was not cleaned: %+v", entries)
	}
}

func TestSourceCancellationCleansSession(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("fake native helper process test runs on macOS")
	}
	fixture := fakeSourceHelper(t)
	parentSpool := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	source := NewSource(Config{
		HelperPath: fixture,
		BundleID:   "example.app",
		SpoolDir:   parentSpool,
	})
	if err := source.Start(ctx, func(capture.AudioFrame) {}); err != nil {
		cancel()
		t.Fatalf("start source: %v", err)
	}
	cancel()
	stopContext, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	if err := source.Stop(stopContext); err != nil {
		t.Fatalf("stop canceled source: %v", err)
	}
	entries, err := os.ReadDir(parentSpool)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("canceled session spool was not cleaned: %+v", entries)
	}
}

func fakeSourceHelper(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "fake-source-helper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
