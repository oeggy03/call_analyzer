package capture

import (
	"context"
	"testing"
	"time"
)

func TestMockSourceLifecycle(t *testing.T) {
	source := NewMockSource()
	ctx := context.Background()
	frames := make(chan AudioFrame, 1)
	if err := source.Start(ctx, func(frame AudioFrame) { frames <- frame }); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Mark(ctx, "moment"); err != nil {
		t.Fatal(err)
	}
	input := AudioFrame{
		Timestamp:  time.Now().UTC(),
		Samples:    []int16{1, 2},
		SampleRate: 2,
		Channels:   1,
	}
	if err := source.Feed(input); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-frames:
		if len(frame.Samples) != len(input.Samples) {
			t.Fatalf("unexpected frame %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("mock frame was not delivered")
	}
	if err := source.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Mark(ctx, "after stop"); err == nil {
		t.Fatal("expected marking a stopped source to fail")
	}
}
