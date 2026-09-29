package audio

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	DefaultPreRoll  = 25 * time.Second
	DefaultPostRoll = 10 * time.Second
)

var (
	ErrWindowNotReady = errors.New("audio: post-roll window is not ready")
	ErrOutsideBuffer  = errors.New("audio: requested window is outside the ring buffer")
	ErrInvalidFrame   = errors.New("audio: invalid audio frame")
	ErrInvalidWindow  = errors.New("audio: invalid audio window")
)

type Frame struct {
	Timestamp  time.Time
	Samples    []int16
	SampleRate int
	Channels   int
}

type AudioWindow struct {
	Start      time.Time
	End        time.Time
	Samples    []int16
	SampleRate int
	Channels   int
}

type RingBuffer struct {
	mu       sync.Mutex
	capacity time.Duration
	frames   []Frame
	notify   chan struct{}
}

func NewRingBuffer(capacity time.Duration) (*RingBuffer, error) {
	if capacity <= 0 {
		return nil, errors.New("audio: ring capacity must be positive")
	}
	return &RingBuffer{
		capacity: capacity,
		notify:   make(chan struct{}),
	}, nil
}

func (r *RingBuffer) Capacity() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.capacity
}

func (r *RingBuffer) Append(frame Frame) error {
	if frame.Timestamp.IsZero() || frame.SampleRate <= 0 || frame.Channels <= 0 || len(frame.Samples) == 0 {
		return ErrInvalidFrame
	}
	if len(frame.Samples)%frame.Channels != 0 {
		return fmt.Errorf("%w: sample count is not divisible by channels", ErrInvalidFrame)
	}
	frame.Timestamp = frame.Timestamp.UTC()
	frame.Samples = append([]int16(nil), frame.Samples...)
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.frames) > 0 {
		last := r.frames[len(r.frames)-1]
		if frame.Timestamp.Before(last.Timestamp) {
			return fmt.Errorf("%w: timestamps must be monotonic", ErrInvalidFrame)
		}
	}
	r.frames = append(r.frames, frame)
	latest := frame.Timestamp.Add(frameDuration(frame))
	cutoff := latest.Add(-r.capacity)
	first := 0
	for first < len(r.frames)-1 {
		if r.frames[first].Timestamp.Add(frameDuration(r.frames[first])).After(cutoff) {
			break
		}
		first++
	}
	if first > 0 {
		r.frames = append([]Frame(nil), r.frames[first:]...)
	}
	if len(r.frames) > 0 && r.frames[0].Timestamp.Before(cutoff) {
		firstFrame := r.frames[0]
		framesToDrop := int(math.Ceil(cutoff.Sub(firstFrame.Timestamp).Seconds() * float64(firstFrame.SampleRate)))
		totalFrames := len(firstFrame.Samples) / firstFrame.Channels
		if framesToDrop >= totalFrames {
			framesToDrop = totalFrames - 1
		}
		if framesToDrop > 0 {
			sampleOffset := framesToDrop * firstFrame.Channels
			firstFrame.Samples = append([]int16(nil), firstFrame.Samples[sampleOffset:]...)
			firstFrame.Timestamp = firstFrame.Timestamp.Add(
				time.Duration(float64(framesToDrop) / float64(firstFrame.SampleRate) * float64(time.Second)),
			)
			r.frames[0] = firstFrame
		}
	}
	close(r.notify)
	r.notify = make(chan struct{})
	return nil
}

func (r *RingBuffer) Snapshot(start, end time.Time) (AudioWindow, error) {
	if !end.After(start) {
		return AudioWindow{}, ErrInvalidWindow
	}
	start = start.UTC()
	end = end.UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked(start, end)
}

func (r *RingBuffer) snapshotLocked(start, end time.Time) (AudioWindow, error) {
	if len(r.frames) == 0 {
		return AudioWindow{}, ErrOutsideBuffer
	}
	firstStart := r.frames[0].Timestamp
	lastEnd := r.frames[len(r.frames)-1].Timestamp.Add(frameDuration(r.frames[len(r.frames)-1]))
	if start.Before(firstStart) {
		return AudioWindow{}, ErrOutsideBuffer
	}
	if end.After(lastEnd) {
		return AudioWindow{}, ErrWindowNotReady
	}

	window := AudioWindow{
		Start: start,
		End:   end,
	}
	var found bool
	for _, frame := range r.frames {
		frameStart := frame.Timestamp
		frameEnd := frameStart.Add(frameDuration(frame))
		if !frameEnd.After(start) || !frameStart.Before(end) {
			continue
		}
		if !found {
			window.SampleRate = frame.SampleRate
			window.Channels = frame.Channels
			found = true
		} else if window.SampleRate != frame.SampleRate || window.Channels != frame.Channels {
			return AudioWindow{}, fmt.Errorf("%w: sample format changed", ErrInvalidWindow)
		}
		overlapStart := maxTime(start, frameStart)
		overlapEnd := minTime(end, frameEnd)
		samplesPerSecond := float64(frame.SampleRate * frame.Channels)
		from := int(math.Floor(overlapStart.Sub(frameStart).Seconds() * samplesPerSecond))
		to := int(math.Ceil(overlapEnd.Sub(frameStart).Seconds() * samplesPerSecond))
		from = maxInt(0, minInt(from, len(frame.Samples)))
		to = maxInt(from, minInt(to, len(frame.Samples)))
		window.Samples = append(window.Samples, frame.Samples[from:to]...)
	}
	if !found || len(window.Samples) == 0 {
		return AudioWindow{}, ErrOutsideBuffer
	}
	return window, nil
}

func (r *RingBuffer) SnapshotAround(mark time.Time, preRoll, postRoll time.Duration) (AudioWindow, error) {
	if preRoll < 0 || postRoll < 0 {
		return AudioWindow{}, ErrInvalidWindow
	}
	return r.Snapshot(mark.Add(-preRoll), mark.Add(postRoll))
}

func (r *RingBuffer) Bounds() (oldest, newest time.Time, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.frames) == 0 {
		return time.Time{}, time.Time{}, false
	}
	oldest = r.frames[0].Timestamp
	newest = r.frames[len(r.frames)-1].Timestamp.Add(frameDuration(r.frames[len(r.frames)-1]))
	return oldest, newest, true
}

func (r *RingBuffer) WaitForWindowClamped(
	ctx context.Context,
	mark, minimumStart time.Time,
	preRoll, postRoll time.Duration,
) (AudioWindow, error) {
	if preRoll < 0 || postRoll < 0 {
		return AudioWindow{}, ErrInvalidWindow
	}
	for {
		oldest, _, ok := r.Bounds()
		if ok {
			start := mark.Add(-preRoll)
			if start.Before(minimumStart) {
				start = minimumStart
			}
			if start.Before(oldest) {
				start = oldest
			}
			window, err := r.Snapshot(start, mark.Add(postRoll))
			if err == nil {
				return window, nil
			}
			if !errors.Is(err, ErrWindowNotReady) && !errors.Is(err, ErrOutsideBuffer) {
				return AudioWindow{}, err
			}
		}
		r.mu.Lock()
		notify := r.notify
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return AudioWindow{}, ctx.Err()
		case <-notify:
		}
	}
}

func (r *RingBuffer) WaitForWindow(ctx context.Context, mark time.Time, preRoll, postRoll time.Duration) (AudioWindow, error) {
	for {
		window, err := r.SnapshotAround(mark, preRoll, postRoll)
		if err == nil {
			return window, nil
		}
		if !errors.Is(err, ErrWindowNotReady) {
			return AudioWindow{}, err
		}
		r.mu.Lock()
		notify := r.notify
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return AudioWindow{}, ctx.Err()
		case <-notify:
		}
	}
}

func frameDuration(frame Frame) time.Duration {
	return time.Duration(float64(len(frame.Samples)/frame.Channels) / float64(frame.SampleRate) * float64(time.Second))
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
