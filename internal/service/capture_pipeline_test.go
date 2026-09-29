package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/audio"
	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
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

func TestAutomaticAnalysisBatchesTwentySecondsOfTranscript(t *testing.T) {
	segments := []domain.TranscriptSegment{
		{StartMS: 0, EndMS: 8_000, Text: "第一句"},
		{StartMS: 9_000, EndMS: 19_000, Text: "第二句"},
	}
	if automaticBatchReady(segments) {
		t.Fatal("automatic analyzer ran before the twenty-second cadence")
	}
	segments = append(segments, domain.TranscriptSegment{StartMS: 19_000, EndMS: 22_000, Text: "第三句"})
	if !automaticBatchReady(segments) {
		t.Fatal("automatic analyzer did not run after accumulating twenty seconds")
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

	if !session.budgetExhausted {
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

func testAudioFrame(timestamp time.Time, sample int16) capture.AudioFrame {
	return capture.AudioFrame{
		Timestamp:  timestamp,
		Samples:    []int16{sample, sample},
		SampleRate: 1,
		Channels:   1,
		Source:     "remote",
	}
}
