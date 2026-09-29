package audio

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

func EncodeWAV(samples []int16, sampleRate, channels int) ([]byte, error) {
	if sampleRate <= 0 || channels <= 0 {
		return nil, errors.New("audio: sample rate and channels must be positive")
	}
	if len(samples)%channels != 0 {
		return nil, errors.New("audio: sample count is not divisible by channels")
	}
	dataSize := len(samples) * 2
	fileSize := 44 + dataSize
	wav := make([]byte, fileSize)
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(fileSize-8))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(wav[24:28], uint32(sampleRate))
	byteRate := sampleRate * channels * 2
	binary.LittleEndian.PutUint32(wav[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(wav[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(dataSize))
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(wav[44+i*2:], uint16(sample))
	}
	return wav, nil
}

func (window AudioWindow) WAV() ([]byte, error) {
	return EncodeWAV(window.Samples, window.SampleRate, window.Channels)
}

func Energy(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, sample := range samples {
		normalized := float64(sample) / 32768
		sum += normalized * normalized
	}
	return math.Sqrt(sum / float64(len(samples)))
}

func VoiceActive(samples []int16, threshold float64) bool {
	if threshold < 0 {
		threshold = 0
	}
	return Energy(samples) >= threshold
}

type ChunkPolicy struct {
	MinDuration time.Duration
	MaxDuration time.Duration
	Overlap     time.Duration
}

func DefaultChunkPolicy() ChunkPolicy {
	return ChunkPolicy{
		MinDuration: 8 * time.Second,
		MaxDuration: 15 * time.Second,
		Overlap:     2 * time.Second,
	}
}

func (p ChunkPolicy) validate() error {
	if p.MinDuration <= 0 || p.MaxDuration < p.MinDuration {
		return errors.New("audio: invalid chunk duration bounds")
	}
	if p.MinDuration < 8*time.Second || p.MaxDuration > 15*time.Second {
		return errors.New("audio: chunk policy must stay within 8-15 seconds")
	}
	if p.Overlap < time.Second || p.Overlap > 2*time.Second || p.Overlap >= p.MaxDuration {
		return errors.New("audio: overlap must be between 1 and 2 seconds")
	}
	return nil
}

type Chunk struct {
	Start      time.Time
	End        time.Time
	Samples    []int16
	SampleRate int
	Channels   int
}

func (p ChunkPolicy) Chunk(window AudioWindow) ([]Chunk, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if !window.End.After(window.Start) || window.SampleRate <= 0 || window.Channels <= 0 {
		return nil, ErrInvalidWindow
	}
	frames := len(window.Samples) / window.Channels
	if frames == 0 || len(window.Samples)%window.Channels != 0 {
		return nil, ErrInvalidWindow
	}
	minFrames := durationFrames(p.MinDuration, window.SampleRate)
	maxFrames := durationFrames(p.MaxDuration, window.SampleRate)
	overlapFrames := durationFrames(p.Overlap, window.SampleRate)
	if frames <= maxFrames {
		return []Chunk{{
			Start:      window.Start,
			End:        window.End,
			Samples:    append([]int16(nil), window.Samples...),
			SampleRate: window.SampleRate,
			Channels:   window.Channels,
		}}, nil
	}
	count := (frames - overlapFrames + (maxFrames - overlapFrames) - 1) / (maxFrames - overlapFrames)
	if count < 2 {
		count = 2
	}
	chunkFrames := (frames + (count-1)*overlapFrames + count - 1) / count
	if chunkFrames > maxFrames {
		chunkFrames = maxFrames
	}
	if chunkFrames < minFrames {
		chunkFrames = minFrames
	}
	step := chunkFrames - overlapFrames
	chunks := make([]Chunk, 0, count)
	for i := 0; i < count; i++ {
		startFrame := i * step
		if startFrame >= frames {
			break
		}
		endFrame := startFrame + chunkFrames
		if endFrame > frames {
			endFrame = frames
		}
		if i == count-1 {
			endFrame = frames
			if endFrame-startFrame > maxFrames {
				startFrame = endFrame - maxFrames
			}
		}
		start := window.Start.Add(time.Duration(float64(startFrame) / float64(window.SampleRate) * float64(time.Second)))
		end := window.Start.Add(time.Duration(float64(endFrame) / float64(window.SampleRate) * float64(time.Second)))
		sampleStart := startFrame * window.Channels
		sampleEnd := endFrame * window.Channels
		chunks = append(chunks, Chunk{
			Start:      start,
			End:        end,
			Samples:    append([]int16(nil), window.Samples[sampleStart:sampleEnd]...),
			SampleRate: window.SampleRate,
			Channels:   window.Channels,
		})
	}
	return chunks, nil
}

func durationFrames(duration time.Duration, sampleRate int) int {
	return int(math.Round(duration.Seconds() * float64(sampleRate)))
}
