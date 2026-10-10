package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

type LessonTranscriptSummary = storage.LessonTranscriptSummary

type LessonTranscript struct {
	Lesson   LessonTranscriptSummary
	Segments []domain.TranscriptSegment
}

func (s *Service) ListLessonTranscripts(
	ctx context.Context,
	limit int,
) ([]LessonTranscriptSummary, error) {
	if s.store == nil {
		return nil, errors.New("lesson transcripts: storage is not configured")
	}
	return s.store.Lessons().ListSummaries(ctx, limit)
}

func (s *Service) GetLessonTranscript(
	ctx context.Context,
	lessonID string,
) (LessonTranscript, error) {
	if s.store == nil {
		return LessonTranscript{}, errors.New("lesson transcripts: storage is not configured")
	}
	lessonID = strings.TrimSpace(lessonID)
	if lessonID == "" {
		return LessonTranscript{}, errors.New("lesson transcript: lesson id is required")
	}
	lesson, err := s.store.Lessons().Get(ctx, lessonID)
	if errors.Is(err, sql.ErrNoRows) {
		return LessonTranscript{}, fmt.Errorf(
			"lesson transcript: lesson %q was not found: %w",
			lessonID,
			err,
		)
	}
	if err != nil {
		return LessonTranscript{}, fmt.Errorf("lesson transcript: load lesson: %w", err)
	}
	segments, err := s.store.Transcripts().List(ctx, lessonID)
	if err != nil {
		return LessonTranscript{}, fmt.Errorf("lesson transcript: load segments: %w", err)
	}
	return LessonTranscript{
		Lesson: LessonTranscriptSummary{
			ID:           lesson.ID,
			Title:        lesson.Title,
			StartedAt:    lesson.StartedAt,
			EndedAt:      lesson.EndedAt,
			SegmentCount: len(segments),
			FinalCost:    lesson.FinalCost,
		},
		Segments: segments,
	}, nil
}
