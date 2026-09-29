package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func TestRingBufferBoundsAndPostRoll(t *testing.T) {
	ring, err := NewRingBuffer(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(0, 0).UTC()
	for i := 0; i < 8; i++ {
		if err := ring.Append(Frame{
			Timestamp:  start.Add(time.Duration(i) * time.Second),
			Samples:    []int16{int16(i), int16(i), int16(i), int16(i)},
			SampleRate: 4,
			Channels:   1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ring.Snapshot(start, start.Add(time.Second)); !errors.Is(err, ErrOutsideBuffer) {
		t.Fatalf("expected old data to be evicted, got %v", err)
	}
	if _, err := ring.SnapshotAround(start.Add(7*time.Second), time.Second, 2*time.Second); !errors.Is(err, ErrWindowNotReady) {
		t.Fatalf("expected post-roll to be pending, got %v", err)
	}
	if err := ring.Append(Frame{
		Timestamp:  start.Add(8 * time.Second),
		Samples:    []int16{8, 8, 8, 8},
		SampleRate: 4,
		Channels:   1,
	}); err != nil {
		t.Fatal(err)
	}
	window, err := ring.SnapshotAround(start.Add(7*time.Second), time.Second, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Samples) != 12 {
		t.Fatalf("expected 12 samples, got %d", len(window.Samples))
	}
}

func TestWaitForWindow(t *testing.T) {
	ring, err := NewRingBuffer(10 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(100, 0).UTC()
	if err := ring.Append(Frame{
		Timestamp:  start,
		Samples:    []int16{1, 1},
		SampleRate: 2,
		Channels:   1,
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan AudioWindow, 1)
	go func() {
		window, _ := ring.WaitForWindow(ctx, start, 0, 2*time.Second)
		done <- window
	}()
	time.Sleep(10 * time.Millisecond)
	if err := ring.Append(Frame{
		Timestamp:  start.Add(time.Second),
		Samples:    []int16{2, 2},
		SampleRate: 2,
		Channels:   1,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case window := <-done:
		if len(window.Samples) != 4 {
			t.Fatalf("expected 4 samples, got %d", len(window.Samples))
		}
	case <-ctx.Done():
		t.Fatal("window did not become ready")
	}
}

func TestWAVAndVAD(t *testing.T) {
	wav, err := EncodeWAV([]int16{0, 32767, -32768}, 8000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("invalid WAV header: %q", wav[:12])
	}
	if got := binary.LittleEndian.Uint32(wav[40:44]); got != 6 {
		t.Fatalf("unexpected data size %d", got)
	}
	if !VoiceActive([]int16{32767, -32768}, 0.5) {
		t.Fatal("loud samples should be active")
	}
	if VoiceActive([]int16{0, 1}, 0.1) {
		t.Fatal("quiet samples should be inactive")
	}
}

func TestChunkPolicyKeepsBoundedOverlap(t *testing.T) {
	sampleRate := 10
	samples := make([]int16, 30*sampleRate)
	window := AudioWindow{
		Start:      time.Unix(0, 0).UTC(),
		End:        time.Unix(30, 0).UTC(),
		Samples:    samples,
		SampleRate: sampleRate,
		Channels:   1,
	}
	chunks, err := DefaultChunkPolicy().Chunk(window)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	for i, chunk := range chunks {
		duration := chunk.End.Sub(chunk.Start)
		if duration < 8*time.Second || duration > 15*time.Second {
			t.Fatalf("chunk %d duration %s is outside policy", i, duration)
		}
		if i > 0 {
			overlap := chunks[i-1].End.Sub(chunk.Start)
			if overlap < time.Second || overlap > 2*time.Second {
				t.Fatalf("chunk %d overlap %s is outside policy", i, overlap)
			}
		}
	}
}
