package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
)

const (
	reconciliationWindow  = 30 * time.Second
	reconciliationOverlap = 5 * time.Second
)

type transcriptWindow struct {
	StartMS int64
	EndMS   int64
	Text    string
}

func transcriptWindows(segments []domain.TranscriptSegment) []transcriptWindow {
	if len(segments) == 0 {
		return nil
	}
	var maxEnd int64
	for _, segment := range segments {
		if segment.EndMS > maxEnd {
			maxEnd = segment.EndMS
		}
	}
	stepMS := (reconciliationWindow - reconciliationOverlap).Milliseconds()
	windowMS := reconciliationWindow.Milliseconds()
	result := make([]transcriptWindow, 0)
	for start := int64(0); start <= maxEnd; start += stepMS {
		end := start + windowMS
		var parts []string
		for _, segment := range segments {
			if segment.Source == "system/mark" {
				continue
			}
			if segment.EndMS <= start || segment.StartMS >= end {
				continue
			}
			if text := strings.TrimSpace(segment.Text); text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			if start >= maxEnd {
				break
			}
			continue
		}
		result = append(result, transcriptWindow{
			StartMS: start,
			EndMS:   end,
			Text:    strings.Join(parts, "\n"),
		})
		if end >= maxEnd {
			break
		}
	}
	return result
}

func (s *Service) reconcileLesson(
	ctx context.Context,
	lessonID string,
	sessionID string,
	lessonStart time.Time,
	router *openrouter.Client,
) error {
	if router == nil {
		return errors.New("post-lesson reconciliation skipped: OpenRouter is not configured")
	}
	segments, err := s.store.Transcripts().List(ctx, lessonID)
	if err != nil {
		return fmt.Errorf("load transcript for reconciliation: %w", err)
	}
	windows := transcriptWindows(segments)
	if len(windows) == 0 {
		return nil
	}
	session := &captureSession{
		ctx:         ctx,
		lessonID:    lessonID,
		lessonStart: lessonStart,
		sessionID:   sessionID,
	}
	if session.sessionID == "" {
		session.sessionID = "reconcile-" + uuid.NewString()
	}
	// Keep the planned 30-second/5-second-overlap windows as explicit evidence
	// boundaries, but send them in one request. A one-hour lesson otherwise
	// requires roughly 144 sequential calls and cannot finish when the learner
	// presses Stop. Qwen's context window comfortably fits a lesson transcript,
	// while the existing evidence validator still maps every returned quote to
	// its original persisted segment.
	var prompt strings.Builder
	for _, window := range windows {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Fprintf(&prompt, "\n[post-lesson reconciliation %dms-%dms]\n%s\n",
			window.StartMS, window.EndMS, window.Text)
	}
	validationText := strings.TrimSpace(prompt.String())
	segment := domain.TranscriptSegment{
		LessonID:   lessonID,
		StartMS:    windows[0].StartMS,
		EndMS:      windows[len(windows)-1].EndMS,
		Text:       validationText,
		Source:     "reconciliation",
		Confidence: 1,
	}
	if err := s.analyzeTranscript(ctx, session, router, segment, false, validationText); err != nil {
		if errors.Is(err, openrouter.ErrBudgetExceeded) {
			return err
		}
		return fmt.Errorf("analyze reconciliation transcript: %w", err)
	}
	return nil
}
