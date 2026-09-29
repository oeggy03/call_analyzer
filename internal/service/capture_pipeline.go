package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/oeggy03/call_analyzer/internal/audio"
	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/transcript"
	"github.com/oeggy03/call_analyzer/internal/vocabulary"
)

const (
	captureFrameQueueSize = 64
	captureJobQueueSize   = 16
	chunkDuration         = 15 * time.Second
	chunkOverlap          = 2 * time.Second
	vadThreshold          = 0.01
)

type analysisJob struct {
	window    audio.AudioWindow
	source    string
	manual    bool
	text      string
	timestamp time.Time
}

type sourceAccumulator struct {
	frames []audio.Frame
	voiced bool
}

type captureSession struct {
	ctx          context.Context
	cancel       context.CancelFunc
	manualCtx    context.Context
	manualCancel context.CancelFunc
	stopCh       chan struct{}
	stopOnce     sync.Once
	frames       chan capture.AudioFrame
	events       chan capture.Event
	jobs         chan analysisJob
	done         chan struct{}
	manualWG     sync.WaitGroup
	accumulators map[string]*sourceAccumulator
	lastText     map[string]string
	ring         *audio.RingBuffer
	lessonID     string
	lessonStart  time.Time
	sessionID    string
}

func newCaptureSession(parent context.Context, ring *audio.RingBuffer, lessonID string, lessonStart time.Time) *captureSession {
	ctx, cancel := context.WithCancel(parent)
	manualCtx, manualCancel := context.WithCancel(ctx)
	return &captureSession{
		ctx:          ctx,
		cancel:       cancel,
		manualCtx:    manualCtx,
		manualCancel: manualCancel,
		stopCh:       make(chan struct{}),
		frames:       make(chan capture.AudioFrame, captureFrameQueueSize),
		events:       make(chan capture.Event, captureFrameQueueSize),
		jobs:         make(chan analysisJob, captureJobQueueSize),
		done:         make(chan struct{}),
		accumulators: make(map[string]*sourceAccumulator),
		lastText:     make(map[string]string),
		ring:         ring,
		lessonID:     lessonID,
		lessonStart:  lessonStart,
		sessionID:    uuid.NewString(),
	}
}

func (s *captureSession) start(owner *Service) {
	go s.runFrames(owner)
	go s.runProcessor(owner)
}

func (s *captureSession) stop(ctx context.Context) {
	s.stopOnce.Do(func() {
		s.manualCancel()
		close(s.stopCh)
	})
	select {
	case <-s.done:
	case <-ctx.Done():
		s.cancel()
	}
	s.cancel()
}

func (s *captureSession) enqueueFrame(frame capture.AudioFrame) bool {
	select {
	case s.frames <- frame:
		return true
	default:
		return false
	}
}

func (s *captureSession) enqueueEvent(event capture.Event) bool {
	select {
	case s.events <- event:
		return true
	default:
		return false
	}
}

func (s *captureSession) enqueueJob(job analysisJob, required bool) bool {
	if required {
		select {
		case s.jobs <- job:
			return true
		case <-s.manualCtx.Done():
			return false
		}
	}
	select {
	case s.jobs <- job:
		return true
	default:
		return false
	}
}

func (s *captureSession) runFrames(owner *Service) {
	defer func() {
		s.flush(owner)
		s.manualWG.Wait()
		close(s.jobs)
	}()
	for {
		select {
		case frame := <-s.frames:
			s.handleFrame(owner, frame)
		case event := <-s.events:
			s.handleEvent(owner, event)
		case <-s.stopCh:
			for {
				select {
				case frame := <-s.frames:
					s.handleFrame(owner, frame)
				case event := <-s.events:
					s.handleEvent(owner, event)
				default:
					return
				}
			}
		case <-s.ctx.Done():
			s.manualCancel()
			return
		}
	}
}

func (s *captureSession) handleFrame(owner *Service, frame capture.AudioFrame) {
	_, microphoneEnabled := owner.captureSettings()
	if (frame.Source == "" || frame.Source == "microphone") && !microphoneEnabled {
		return
	}
	owner.updateAudioLevel(frame)
	// The manual-mark ring uses the teacher/remote track. Mixing overlapping
	// microphone and remote frames by concatenation would corrupt timing.
	if frame.Source != "microphone" {
		if err := s.ring.Append(audio.Frame{
			Timestamp:  frame.Timestamp,
			Samples:    frame.Samples,
			SampleRate: frame.SampleRate,
			Channels:   frame.Channels,
		}); err != nil {
			owner.diagnostic("capture frame dropped", err)
			return
		}
	}
	s.accumulate(owner, frame)
}

func (s *captureSession) handleEvent(owner *Service, event capture.Event) {
	if !strings.EqualFold(event.Kind, "ocr") || strings.TrimSpace(event.Text) == "" {
		return
	}
	ocrEnabled, _ := owner.captureSettings()
	if !ocrEnabled {
		return
	}
	source := event.Source
	if source == "" {
		source = "system/ocr"
	}
	if !strings.Contains(source, "/") {
		source = "system/" + source
	}
	if !s.enqueueJob(analysisJob{
		source:    source,
		text:      event.Text,
		timestamp: event.Timestamp,
	}, false) {
		owner.diagnostic("OCR analysis queue full", nil)
	}
}

func (s *captureSession) runProcessor(owner *Service) {
	defer close(s.done)
	for job := range s.jobs {
		owner.processAnalysisJob(s.ctx, s, job)
	}
}

func (s *captureSession) accumulate(owner *Service, frame capture.AudioFrame) {
	source := frame.Source
	if source == "" {
		source = "microphone"
	}
	acc := s.accumulators[source]
	if acc == nil {
		acc = &sourceAccumulator{}
		s.accumulators[source] = acc
	}
	acc.frames = append(acc.frames, audio.Frame{
		Timestamp:  frame.Timestamp,
		Samples:    append([]int16(nil), frame.Samples...),
		SampleRate: frame.SampleRate,
		Channels:   frame.Channels,
	})
	acc.voiced = acc.voiced || audio.VoiceActive(frame.Samples, vadThreshold)
	for framesDuration(acc.frames) >= chunkDuration {
		window, remaining := splitAccumulator(acc.frames, chunkDuration, chunkOverlap)
		voiced := acc.voiced
		acc.frames = remaining
		acc.voiced = accumulatorVoiced(remaining)
		if voiced && window.End.After(window.Start) {
			if !s.enqueueJob(analysisJob{window: window, source: source}, false) {
				owner.diagnostic("audio analysis queue full", nil)
			}
		}
	}
}

func (s *captureSession) flush(owner *Service) {
	for source, acc := range s.accumulators {
		if len(acc.frames) == 0 {
			continue
		}
		window := combineFrames(acc.frames)
		if acc.voiced && window.End.After(window.Start) {
			if !s.enqueueJob(analysisJob{window: window, source: source}, false) {
				owner.diagnostic("audio flush queue full", nil)
			}
		}
		acc.frames = nil
		acc.voiced = false
	}
}

func framesDuration(frames []audio.Frame) time.Duration {
	if len(frames) == 0 {
		return 0
	}
	first := frames[0].Timestamp
	last := frames[len(frames)-1]
	return last.Timestamp.Add(frameDuration(last)).Sub(first)
}

func frameDuration(frame audio.Frame) time.Duration {
	if frame.SampleRate <= 0 || frame.Channels <= 0 {
		return 0
	}
	return time.Duration(float64(len(frame.Samples)/frame.Channels) / float64(frame.SampleRate) * float64(time.Second))
}

func combineFrames(frames []audio.Frame) audio.AudioWindow {
	if len(frames) == 0 {
		return audio.AudioWindow{}
	}
	first := frames[0]
	samples := make([]int16, 0)
	for _, frame := range frames {
		samples = append(samples, frame.Samples...)
	}
	end := first.Timestamp.Add(time.Duration(float64(len(samples)/first.Channels) / float64(first.SampleRate) * float64(time.Second)))
	return audio.AudioWindow{
		Start:      first.Timestamp,
		End:        end,
		Samples:    samples,
		SampleRate: first.SampleRate,
		Channels:   first.Channels,
	}
}

func splitAccumulator(frames []audio.Frame, maxDuration, overlap time.Duration) (audio.AudioWindow, []audio.Frame) {
	window := combineFrames(frames)
	if window.SampleRate <= 0 || window.Channels <= 0 {
		return window, nil
	}
	maxFrames := int(maxDuration.Seconds() * float64(window.SampleRate))
	overlapFrames := int(overlap.Seconds() * float64(window.SampleRate))
	totalFrames := len(window.Samples) / window.Channels
	if totalFrames <= maxFrames {
		return window, nil
	}
	takeSamples := maxFrames * window.Channels
	window.Samples = append([]int16(nil), window.Samples[:takeSamples]...)
	window.End = window.Start.Add(time.Duration(float64(maxFrames) / float64(window.SampleRate) * float64(time.Second)))
	retainFrom := (maxFrames - overlapFrames) * window.Channels
	remainingSamples := append([]int16(nil), combineFrames(frames).Samples[retainFrom:]...)
	remainingStart := window.Start.Add(time.Duration(float64(maxFrames-overlapFrames) / float64(window.SampleRate) * float64(time.Second)))
	return window, []audio.Frame{{
		Timestamp:  remainingStart,
		Samples:    remainingSamples,
		SampleRate: window.SampleRate,
		Channels:   window.Channels,
	}}
}

func accumulatorVoiced(frames []audio.Frame) bool {
	for _, frame := range frames {
		if audio.VoiceActive(frame.Samples, vadThreshold) {
			return true
		}
	}
	return false
}

func (s *Service) processAnalysisJob(ctx context.Context, session *captureSession, job analysisJob) {
	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router == nil {
		s.diagnostic("OpenRouter is not configured", nil)
		return
	}

	text := strings.TrimSpace(job.text)
	var usage openrouter.Usage
	if text == "" {
		wav, err := job.window.WAV()
		if err != nil {
			s.diagnostic("WAV encoding failed", err)
			return
		}
		result, err := router.Transcribe(ctx, wav, "lesson.wav")
		usage = result.Usage
		if usage.Cost > 0 {
			if recordErr := s.recordUsage(ctx, session.sessionID, router.ASRModel(), "/api/v1/audio/transcriptions", usage); recordErr != nil {
				s.diagnostic("recording transcription usage failed", recordErr)
			}
		}
		if err != nil {
			s.diagnostic("transcription failed", err)
			return
		}
		text = strings.TrimSpace(result.Text)
	}
	if text == "" {
		return
	}
	source := job.source
	if source == "" {
		source = "microphone"
	}
	previous := session.lastText[source]
	merged := transcript.ReconcileOverlap(previous, text)
	delta := merged
	if previous != "" && strings.HasPrefix(merged, previous) {
		delta = strings.TrimSpace(strings.TrimPrefix(merged, previous))
	}
	session.lastText[source] = merged
	if delta == "" {
		return
	}

	start := job.window.Start
	end := job.window.End
	if start.IsZero() {
		start = job.timestamp
		end = job.timestamp
		if start.IsZero() {
			start = time.Now().UTC()
			end = start
		}
	}
	segment, err := s.store.Transcripts().Insert(ctx, domain.TranscriptSegment{
		LessonID:   session.lessonID,
		StartMS:    start.Sub(session.lessonStart).Milliseconds(),
		EndMS:      end.Sub(session.lessonStart).Milliseconds(),
		Text:       delta,
		Source:     source,
		Confidence: 1,
	})
	if err != nil {
		s.diagnostic("persisting transcript failed", err)
		return
	}
	s.emit("transcript.segment", segment)
	if err := s.analyzeTranscript(ctx, session, router, segment, job.manual); err != nil {
		s.diagnostic("vocabulary extraction failed", err)
	}
}

func (s *Service) analyzeTranscript(
	ctx context.Context,
	session *captureSession,
	router *openrouter.Client,
	segment domain.TranscriptSegment,
	manual bool,
) error {
	prompt := fmt.Sprintf(
		"Analyze this Mandarin lesson transcript for useful teachable vocabulary. "+
			"Return only the requested JSON schema. Include only words or short phrases "+
			"whose exact evidence appears in the transcript. Prefer practical business "+
			"Mandarin, include simplified/traditional forms, numbered pinyin, one "+
			"concise English sense (use an empty classifier when not applicable), and "+
			"one natural Chinese example with pinyin and English translation. "+
			"Transcript evidence: %s", segment.Text,
	)
	response, err := router.ChatJSON(ctx, openrouter.ChatRequest{
		Prompt:     prompt,
		System:     "You are a conservative Mandarin teacher. Never invent evidence or auto-confirm vocabulary.",
		SchemaName: "mandarin_vocabulary_extraction",
		Schema:     vocabularyExtractionSchema(),
	})
	if response.Usage.Cost > 0 {
		if recordErr := s.recordUsage(ctx, session.sessionID, router.ChatModel(), "/api/v1/chat/completions", response.Usage); recordErr != nil {
			s.diagnostic("recording analyzer usage failed", recordErr)
		}
	}
	if err != nil {
		return err
	}
	extraction, err := ParseExtractionJSON(response.Content)
	if err != nil {
		return err
	}
	vocabularyService := NewVocabularyService(s.store, nil)
	var entries []domain.VocabularyEntry
	if manual {
		entries, err = vocabularyService.ProcessManualExtraction(ctx, session.lessonID, segment.ID, segment.Text, extraction)
	} else {
		entries, err = vocabularyService.ProcessExtraction(ctx, session.lessonID, segment.ID, segment.Text, extraction)
	}
	if err == nil {
		s.emit("vocabulary.changed", entries)
	}
	return err
}

func vocabularyExtractionSchema() map[string]any {
	return vocabulary.ExtractionSchema()
}

func (s *Service) recordUsage(ctx context.Context, sessionID, model, endpoint string, usage openrouter.Usage) error {
	if s.store == nil {
		return errors.New("service: storage is not configured")
	}
	_, err := s.store.Usage().Record(ctx, domain.RequestUsage{
		SessionID:        sessionID,
		Provider:         "openrouter",
		Model:            model,
		Endpoint:         endpoint,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		Cost:             usage.Cost,
	})
	return err
}

func (s *Service) diagnostic(message string, err error) {
	payload := map[string]string{"message": message}
	if err != nil {
		payload["error"] = err.Error()
	}
	s.emit("diagnostic", payload)
}
