package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/service"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

func TestLessonTranscriptHistoryMapsSummariesAndLines(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := NewAppWithDependencies(
		store,
		capture.NewMockSource(),
		service.NewMemorySecretStore(),
		nil,
	)
	if err := app.service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}

	startedAt := time.UnixMilli(1_000).UTC()
	older, err := store.Lessons().Start(ctx, "older lesson", startedAt)
	if err != nil {
		t.Fatal(err)
	}
	endedAt := time.UnixMilli(5_000).UTC()
	if _, err := store.Lessons().EndWithCost(ctx, older.ID, endedAt, .25); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transcripts().InsertBatch(ctx, []domain.TranscriptSegment{
		{
			LessonID: older.ID,
			StartMS:  500,
			EndMS:    700,
			Text:     "Marked moment",
			Source:   "system/mark",
		},
		{
			LessonID: older.ID,
			StartMS:  1_500,
			EndMS:    1_700,
			Text:     "system note",
		},
		{
			LessonID: older.ID,
			StartMS:  2_000,
			EndMS:    2_500,
			Text:     "remote line",
			Source:   "remote",
		},
	}); err != nil {
		t.Fatal(err)
	}
	newer, err := store.Lessons().Start(ctx, "newer lesson", time.UnixMilli(2_000).UTC())
	if err != nil {
		t.Fatal(err)
	}

	summaries, err := app.ListLessonTranscripts(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 || summaries[0].ID != newer.ID || summaries[1].ID != older.ID {
		t.Fatalf("unexpected transcript history order: %#v", summaries)
	}
	if summaries[1].StartedAt != timestamp(startedAt) ||
		summaries[1].EndedAt != timestamp(endedAt) ||
		summaries[1].SegmentCount != 3 ||
		summaries[1].FinalCost != .25 {
		t.Fatalf("unexpected transcript summary: %#v", summaries[1])
	}

	transcript, err := app.GetLessonTranscript(older.ID)
	if err != nil {
		t.Fatal(err)
	}
	if transcript.Lesson.ID != older.ID || transcript.Lesson.Title != "older lesson" ||
		len(transcript.Lines) != 3 {
		t.Fatalf("unexpected historical transcript: %#v", transcript)
	}
	if transcript.Lines[0].Source != "system" ||
		!transcript.Lines[0].IsMoment ||
		transcript.Lines[0].Timestamp != timestamp(startedAt.Add(500*time.Millisecond)) {
		t.Fatalf("marked moment was mapped incorrectly: %#v", transcript.Lines[0])
	}
	if transcript.Lines[1].Source != "system" || transcript.Lines[1].IsMoment {
		t.Fatalf("empty source was mapped incorrectly: %#v", transcript.Lines[1])
	}
	if transcript.Lines[2].Source != "remote" || transcript.Lines[2].IsMoment {
		t.Fatalf("remote source was mapped incorrectly: %#v", transcript.Lines[2])
	}
}

func TestGetLessonTranscriptReturnsEmptyLinesAndClearUnknownError(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := NewAppWithDependencies(
		store,
		capture.NewMockSource(),
		service.NewMemorySecretStore(),
		nil,
	)
	if err := app.service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	lesson, err := store.Lessons().Start(ctx, "empty", time.UnixMilli(100).UTC())
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := app.GetLessonTranscript(lesson.ID)
	if err != nil {
		t.Fatal(err)
	}
	if transcript.Lines == nil || len(transcript.Lines) != 0 {
		t.Fatalf("empty transcript did not return an empty lines slice: %#v", transcript.Lines)
	}
	_, err = app.GetLessonTranscript("missing-lesson")
	if err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("unknown lesson error was not clear: %v", err)
	}
}
