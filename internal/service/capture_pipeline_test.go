package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/audio"
	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

func TestAccumulatorFinalizesVoicedWindowAfterSilence(t *testing.T) {
	start := time.Unix(10, 0).UTC()
	ring, err := audio.NewRingBuffer(audio.DefaultRingCapacity)
	if err != nil {
		t.Fatal(err)
	}
	session := newCaptureSession(context.Background(), ring, "lesson", start)
	owner := &Service{}

	for i := 0; i < 4; i++ {
		session.accumulate(owner, testAudioFrame(start.Add(time.Duration(i)*2*time.Second), 1_000))
	}
	for i := 4; i < 6; i++ {
		session.accumulate(owner, testAudioFrame(start.Add(time.Duration(i)*2*time.Second), 0))
	}

	select {
	case job := <-session.jobs:
		if job.window.End.Sub(job.window.Start) != 10*time.Second {
			t.Fatalf("unexpected utterance window %s", job.window.End.Sub(job.window.Start))
		}
		if !audio.VoiceActive(job.window.Samples, vadThreshold) {
			t.Fatal("utterance window was silence-only")
		}
	default:
		t.Fatal("voiced buffer was not finalized after silence")
	}
	select {
	case job := <-session.jobs:
		t.Fatalf("unexpected duplicate/silence job %#v", job)
	default:
	}
}

func TestAccumulatorKeepsFifteenSecondChunksAndTwoSecondOverlap(t *testing.T) {
	start := time.Unix(20, 0).UTC()
	ring, err := audio.NewRingBuffer(audio.DefaultRingCapacity)
	if err != nil {
		t.Fatal(err)
	}
	session := newCaptureSession(context.Background(), ring, "lesson", start)
	owner := &Service{}
	for i := 0; i < 9; i++ {
		session.accumulate(owner, testAudioFrame(start.Add(time.Duration(i)*2*time.Second), 1_000))
	}

	first := <-session.jobs
	if got := first.window.End.Sub(first.window.Start); got != 15*time.Second {
		t.Fatalf("expected 15-second chunk, got %s", got)
	}
	if len(session.accumulators["remote"].frames) == 0 {
		t.Fatal("expected overlap frames to remain buffered")
	}
	oldest := session.accumulators["remote"].frames[0].Timestamp
	if got := oldest.Sub(start.Add(13 * time.Second)); got != 0 {
		t.Fatalf("expected 2-second overlap to start at 13s, got %s", got)
	}
}

func TestRecentSpeechContextIsBoundedAndSkipsOCR(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	start := time.Unix(100, 0).UTC()
	lesson, err := store.Lessons().Start(ctx, "lesson", start)
	if err != nil {
		t.Fatal(err)
	}
	segments := []domain.TranscriptSegment{
		{LessonID: lesson.ID, StartMS: 0, EndMS: 10_000, Text: "old speech", Source: "remote"},
		{LessonID: lesson.ID, StartMS: 40_000, EndMS: 50_000, Text: "recent speech", Source: "remote"},
		{LessonID: lesson.ID, StartMS: 50_000, EndMS: 51_000, Text: "screen text", Source: "system/ocr"},
		{LessonID: lesson.ID, StartMS: 51_000, EndMS: 51_000, Text: "Marked moment", Source: "system/mark"},
	}
	if _, err := store.Transcripts().InsertBatch(ctx, segments); err != nil {
		t.Fatal(err)
	}
	svc := New(store, nil, NewMemorySecretStore(), nil)
	contextText, err := svc.recentSpeechContext(ctx, &captureSession{
		lessonID:    lesson.ID,
		lessonStart: start,
	}, start.Add(120*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if contextText != "recent speech" {
		t.Fatalf("unexpected recent speech context %q", contextText)
	}
}

func TestAudioLevelUpdateEmitsLiveSnapshotTrigger(t *testing.T) {
	svc := &Service{}
	events := make(chan string, 1)
	svc.SetEventHook(func(name string, _ any) {
		events <- name
	})
	svc.updateAudioLevel(capture.AudioFrame{
		Samples: []int16{20_000, -20_000},
		Source:  "remote",
	})
	select {
	case name := <-events:
		if name != "capture.levels" {
			t.Fatalf("unexpected event %q", name)
		}
	case <-time.After(time.Second):
		t.Fatal("audio level update did not trigger a UI snapshot event")
	}
}

func TestManualAnalysisKeepsEntireMarkedWindow(t *testing.T) {
	previous := "老师说机会"
	current := "老师说机会，然后解释把握机会"
	merged, analysisText := reconcileAnalysisText(previous, current, true)
	if merged != previous {
		t.Fatalf("manual analysis mutated automatic overlap state: %q", merged)
	}
	if analysisText != current {
		t.Fatalf("manual analysis text = %q, want full marked window %q", analysisText, current)
	}

	_, automaticDelta := reconcileAnalysisText(previous, current, false)
	if automaticDelta == current {
		t.Fatal("automatic overlap reconciliation unexpectedly kept the full repeated window")
	}
}

func TestAutomaticAnalysisBatchesThirtySecondsOfTranscript(t *testing.T) {
	segments := []domain.TranscriptSegment{
		{StartMS: 0, EndMS: 12_000, Text: "第一句"},
		{StartMS: 13_000, EndMS: 29_000, Text: "第二句"},
	}
	if automaticBatchReady(segments) {
		t.Fatal("automatic analyzer ran before the thirty-second cadence")
	}
	segments = append(segments, domain.TranscriptSegment{StartMS: 29_000, EndMS: 32_000, Text: "第三句"})
	if !automaticBatchReady(segments) {
		t.Fatal("automatic analyzer did not run after accumulating thirty seconds")
	}
}

func TestBudgetExhaustionPausesCloudWorkWithoutErrorSpam(t *testing.T) {
	svc := &Service{}
	events := 0
	svc.SetEventHook(func(name string, _ any) {
		if name == "diagnostic" {
			events++
		}
	})
	session := &captureSession{}
	svc.exhaustSessionBudget(session)
	svc.exhaustSessionBudget(session)

	if !session.budgetExhausted.Load() {
		t.Fatal("session was not moved to budget-exhausted state")
	}
	if events != 1 {
		t.Fatalf("budget exhaustion emitted %d errors, want one", events)
	}
	if message := svc.LessonError(); !strings.Contains(message, "Cloud analysis is paused") {
		t.Fatalf("budget error was not actionable: %q", message)
	}
}

func TestManualJobsUsePriorityQueue(t *testing.T) {
	ring, err := audio.NewRingBuffer(audio.DefaultRingCapacity)
	if err != nil {
		t.Fatal(err)
	}
	session := newCaptureSession(context.Background(), ring, "lesson", time.Now())
	if !session.enqueueJob(analysisJob{text: "automatic"}, false) {
		t.Fatal("could not queue automatic job")
	}
	if !session.enqueueJob(analysisJob{text: "manual", manual: true}, true) {
		t.Fatal("could not queue manual job")
	}
	select {
	case job := <-session.priorityJobs:
		if !job.manual || job.text != "manual" {
			t.Fatalf("unexpected priority job: %#v", job)
		}
	default:
		t.Fatal("manual job was not placed on the priority queue")
	}
}

func TestFatalCaptureEventLeavesLessonRecoverableButNotLive(t *testing.T) {
	ring, err := audio.NewRingBuffer(audio.DefaultRingCapacity)
	if err != nil {
		t.Fatal(err)
	}
	session := newCaptureSession(context.Background(), ring, "lesson-1", time.Now())
	svc := &Service{
		session:       session,
		capturing:     true,
		currentLesson: "lesson-1",
	}
	failed := make(chan struct{}, 1)
	svc.SetEventHook(func(name string, _ any) {
		if name == "capture.failed" {
			failed <- struct{}{}
		}
	})
	session.start(svc)
	if !session.enqueueEvent(capture.Event{
		Kind:  "error",
		Code:  "stream_stopped",
		Text:  "ScreenCaptureKit stopped",
		Fatal: true,
	}) {
		t.Fatal("could not queue fatal capture event")
	}
	select {
	case <-failed:
	case <-time.After(time.Second):
		t.Fatal("fatal native failure did not update capture state")
	}
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if svc.capturing {
		t.Fatal("service still reports live capture after native failure")
	}
	if svc.currentLesson != "lesson-1" || svc.session != session {
		t.Fatal("failed lesson was cleared before the user could finish it")
	}
}

func TestFailedAutomaticBatchIsRetainedForRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= 2 {
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"candidates\":[]}"}}]}`))
	}))
	defer server.Close()
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lesson, err := store.Lessons().Start(ctx, "retry", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	segment, err := store.Transcripts().Insert(ctx, domain.TranscriptSegment{
		LessonID: lesson.ID,
		Text:     "测试",
		EndMS:    20_000,
		Source:   "remote",
	})
	if err != nil {
		t.Fatal(err)
	}
	secrets := NewMemorySecretStore()
	if err := secrets.Set(ctx, secretOpenRouterAPIKey, "test-key"); err != nil {
		t.Fatal(err)
	}
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL:    server.URL,
		MaxRetries: 1,
		RetryBase:  time.Millisecond,
	}, APIKeySecretProvider{Store: secrets, Name: secretOpenRouterAPIKey})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(store, nil, secrets, router)
	var lastDiagnostic any
	svc.SetEventHook(func(name string, payload any) {
		if name == "diagnostic" {
			lastDiagnostic = payload
		}
	})
	session := &captureSession{
		ctx:          ctx,
		lessonID:     lesson.ID,
		lessonStart:  lesson.StartedAt,
		sessionID:    lesson.ID,
		pendingSince: time.Now().Add(-automaticAnalysisCadence),
		pendingAuto:  []domain.TranscriptSegment{segment},
	}
	svc.flushAutomaticAnalysis(ctx, session)
	if len(session.pendingAuto) != 1 || session.nextAutoAttempt.IsZero() {
		t.Fatal("failed automatic batch was discarded instead of scheduled for retry")
	}
	session.nextAutoAttempt = time.Time{}
	svc.flushAutomaticAnalysis(ctx, session)
	if len(session.pendingAuto) != 0 || calls.Load() != 3 {
		t.Fatalf("successful retry did not clear batch: pending=%d calls=%d diagnostic=%v", len(session.pendingAuto), calls.Load(), lastDiagnostic)
	}
}

func TestStopTimeoutRetainsLessonForRecovery(t *testing.T) {
	ring, err := audio.NewRingBuffer(audio.DefaultRingCapacity)
	if err != nil {
		t.Fatal(err)
	}
	source := capture.NewMockSource()
	if err := source.Start(context.Background(), func(capture.AudioFrame) {}); err != nil {
		t.Fatal(err)
	}
	session := newCaptureSession(context.Background(), ring, "lesson-1", time.Now())
	svc := &Service{
		source:        source,
		capturing:     true,
		currentLesson: "lesson-1",
		session:       session,
		ring:          ring,
	}
	previousDrainTimeout := sessionCancelDrainTimeout
	sessionCancelDrainTimeout = 10 * time.Millisecond
	t.Cleanup(func() {
		sessionCancelDrainTimeout = previousDrainTimeout
		session.cancel()
	})
	stopContext, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := svc.StopCapture(stopContext); err == nil {
		t.Fatal("expected pending analysis timeout")
	}
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if svc.currentLesson != "lesson-1" || svc.session != session || svc.ring != ring {
		t.Fatal("stop timeout cleared recoverable lesson state")
	}
	if svc.capturing {
		t.Fatal("timed-out stop still reported active capture")
	}
}

func TestSlowVocabularyExtractionDoesNotBlockTranscription(t *testing.T) {
	chatEntered := make(chan struct{}, 1)
	releaseChat := make(chan struct{})
	var sttCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/audio/transcriptions":
			n := sttCalls.Add(1)
			_, _ = w.Write([]byte(`{"text":"第` + string(rune('一'+n-1)) + `句中文"}`))
		case "/api/v1/chat/completions":
			select {
			case chatEntered <- struct{}{}:
			default:
			}
			<-releaseChat
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"candidates\":[]}"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secrets := NewMemorySecretStore()
	if err := secrets.Set(ctx, secretOpenRouterAPIKey, "test-key"); err != nil {
		t.Fatal(err)
	}
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL:   server.URL,
		RetryBase: time.Millisecond,
	}, APIKeySecretProvider{Store: secrets, Name: secretOpenRouterAPIKey})
	if err != nil {
		t.Fatal(err)
	}
	source := capture.NewMockSource()
	svc := New(store, source, secrets, router)
	if err := svc.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	ocrEnabled := true
	if err := svc.ApplySettings(ctx, SettingsPatch{OCREnabled: &ocrEnabled}); err != nil {
		t.Fatal(err)
	}
	emittedSegments := make(chan domain.TranscriptSegment, 8)
	svc.SetEventHook(func(name string, payload any) {
		if name == "transcript.segment" {
			if seg, ok := payload.(domain.TranscriptSegment); ok {
				emittedSegments <- seg
			}
		}
	})

	start := time.Unix(1_000, 0).UTC()
	lesson, err := svc.StartLesson(ctx, "decoupled", start)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.StartCapture(ctx, lesson.ID); err != nil {
		t.Fatal(err)
	}

	// Trigger an immediate background ChatJSON call via an OCR event, and hold
	// ChatJSON open on the server while feeding live audio utterances.
	svc.receiveEvent(capture.Event{
		Kind:      "ocr",
		Source:    "system/ocr",
		Timestamp: start.Add(time.Second),
		Text:      "屏幕词汇",
	})
	select {
	case <-chatEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("background vocabulary extraction did not start")
	}
	// Drain the OCR transcript segment event.
	<-emittedSegments

	// Feed a voiced 8s + 4s silence utterance while ChatJSON is completely blocked.
	for i := 0; i < 4; i++ {
		if err := source.Feed(testAudioFrame(start.Add(time.Duration(i)*2*time.Second), 2_000)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 4; i < 6; i++ {
		if err := source.Feed(testAudioFrame(start.Add(time.Duration(i)*2*time.Second), 0)); err != nil {
			t.Fatal(err)
		}
	}

	select {
	case seg := <-emittedSegments:
		if seg.Source != "remote" || !strings.Contains(seg.Text, "中文") {
			t.Fatalf("unexpected live transcript segment while vocab AI blocked: %#v", seg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("audio transcription was blocked by slow ChatJSON vocabulary extraction")
	}

	close(releaseChat)
	if err := svc.StopCapture(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentTranscriptionsCommitInStrictSequenceOrder(t *testing.T) {
	secondDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/audio/transcriptions":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = file.Close()
			// Identify win1 (smaller 4-sample WAV) vs win2 (larger 8-sample WAV)
			// regardless of which worker goroutine reaches TCP accept first.
			if header.Size < 60 {
				<-secondDone
				_, _ = w.Write([]byte(`{"text":"今天我们学习谈判技巧"}`))
			} else {
				_, _ = w.Write([]byte(`{"text":"谈判技巧非常重要"}`))
				close(secondDone)
			}
		case "/api/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"candidates\":[]}"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secrets := NewMemorySecretStore()
	if err := secrets.Set(ctx, secretOpenRouterAPIKey, "test-key"); err != nil {
		t.Fatal(err)
	}
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL:   server.URL,
		RetryBase: time.Millisecond,
	}, APIKeySecretProvider{Store: secrets, Name: secretOpenRouterAPIKey})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(store, capture.NewMockSource(), secrets, router)
	start := time.Unix(2_000, 0).UTC()
	lesson, err := svc.StartLesson(ctx, "ordered", start)
	if err != nil {
		t.Fatal(err)
	}
	ring, err := audio.NewRingBuffer(audio.DefaultRingCapacity)
	if err != nil {
		t.Fatal(err)
	}
	session := newCaptureSession(ctx, ring, lesson.ID, start)
	session.start(svc)

	win1 := audio.AudioWindow{
		Start: start, End: start.Add(10 * time.Second),
		Samples: []int16{2_000, -2_000, 2_000, -2_000}, SampleRate: 2, Channels: 1,
	}
	win2 := audio.AudioWindow{
		Start: start.Add(8 * time.Second), End: start.Add(18 * time.Second),
		Samples: []int16{2_000, -2_000, 2_000, -2_000, 2_000, -2_000, 2_000, -2_000, 2_000, -2_000, 2_000, -2_000}, SampleRate: 2, Channels: 1,
	}
	if !session.enqueueJob(analysisJob{window: win1, source: "remote"}, false) {
		t.Fatal("enqueue win1 failed")
	}
	if !session.enqueueJob(analysisJob{window: win2, source: "remote"}, false) {
		t.Fatal("enqueue win2 failed")
	}
	if err := session.stop(ctx); err != nil {
		t.Fatal(err)
	}

	segments, err := svc.ListTranscript(ctx, lesson.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 ordered segments, got %#v", segments)
	}
	if segments[0].Text != "今天我们学习谈判技巧" {
		t.Fatalf("segment[0] = %q, want first window intact", segments[0].Text)
	}
	if segments[1].Text != "非常重要" {
		t.Fatalf("segment[1] = %q, want overlap-deduplicated delta \"非常重要\"", segments[1].Text)
	}
}

func TestAccumulatorCoalescesUnderBackpressureWithoutDroppingSpeech(t *testing.T) {
	start := time.Unix(30, 0).UTC()
	ring, err := audio.NewRingBuffer(audio.DefaultRingCapacity)
	if err != nil {
		t.Fatal(err)
	}
	session := newCaptureSession(context.Background(), ring, "lesson", start)
	owner := &Service{}

	// Fill the job queue completely to simulate cloud backpressure.
	for i := 0; i < captureJobQueueSize; i++ {
		if !session.enqueueJob(analysisJob{text: "queued"}, false) {
			t.Fatalf("queue filled early at %d", i)
		}
	}

	// Feed 16 seconds of voiced speech; it must stay buffered instead of dropping.
	for i := 0; i < 8; i++ {
		session.accumulate(owner, testAudioFrame(start.Add(time.Duration(i)*2*time.Second), 1_500))
	}
	if owner.LessonError() != "" {
		t.Fatalf("unexpected drop error within coalesce cap: %q", owner.LessonError())
	}
	if len(session.accumulators["remote"].frames) == 0 {
		t.Fatal("speech frames were dropped instead of coalesced during backpressure")
	}

	// Free one slot and feed one more frame; the buffered 15s window should enqueue.
	<-session.jobs
	session.accumulate(owner, testAudioFrame(start.Add(16*time.Second), 1_500))
	if len(session.jobs) != captureJobQueueSize {
		t.Fatalf("buffered speech was not enqueued once queue capacity freed: len=%d", len(session.jobs))
	}
}

func TestStopAndEndCurrentLessonFinalizesWhenTrailingVocabExtractionTimesOut(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/audio/transcriptions":
			_, _ = w.Write([]byte(`{"text":"保存下来的课程内容","usage":{"cost":0.01}}`))
		case "/api/v1/chat/completions":
			select {
			case <-r.Context().Done():
			case <-serverDone:
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer func() {
		close(serverDone)
		server.Close()
	}()

	prevVocabDrain := vocabStopDrainTimeout
	prevCancelDrain := sessionCancelDrainTimeout
	vocabStopDrainTimeout = 25 * time.Millisecond
	sessionCancelDrainTimeout = 25 * time.Millisecond
	t.Cleanup(func() {
		vocabStopDrainTimeout = prevVocabDrain
		sessionCancelDrainTimeout = prevCancelDrain
	})

	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secrets := NewMemorySecretStore()
	if err := secrets.Set(ctx, secretOpenRouterAPIKey, "test-key"); err != nil {
		t.Fatal(err)
	}
	router, err := openrouter.NewClient(openrouter.Config{
		BaseURL:     server.URL,
		MaxRetries:  0,
		ChatTimeout: 30 * time.Millisecond,
	}, APIKeySecretProvider{Store: secrets, Name: secretOpenRouterAPIKey})
	if err != nil {
		t.Fatal(err)
	}
	source := capture.NewMockSource()
	svc := New(store, source, secrets, router)
	if err := svc.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	start := time.Unix(3_000, 0).UTC()
	lesson, err := svc.StartLesson(ctx, "resilient-stop", start)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.StartCapture(ctx, lesson.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := source.Feed(testAudioFrame(start.Add(time.Duration(i)*2*time.Second), 2_000)); err != nil {
			t.Fatal(err)
		}
	}

	ended, err := svc.StopAndEndCurrentLesson(ctx)
	if err != nil {
		t.Fatalf("StopAndEndCurrentLesson failed when only trailing vocab AI timed out: %v", err)
	}
	if ended.EndedAt == nil {
		t.Fatal("lesson EndedAt was left nil")
	}
	segments, err := svc.ListTranscript(ctx, lesson.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].Text != "保存下来的课程内容" {
		t.Fatalf("captured transcript was not preserved before lesson close: %#v", segments)
	}
}

func testAudioFrame(timestamp time.Time, sample int16) capture.AudioFrame {
	return capture.AudioFrame{
		Timestamp:  timestamp,
		Samples:    []int16{sample, sample},
		SampleRate: 1,
		Channels:   1,
		Source:     "remote",
	}
}
