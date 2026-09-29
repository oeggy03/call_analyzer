package main

import (
	"context"
	"strings"
	"time"

	"github.com/oeggy03/call_analyzer/internal/domain"
)

func validTarget(target string) bool {
	switch target {
	case "zoom", "googleMeet", "teams":
		return true
	default:
		return false
	}
}

func (a *App) buildSnapshot(ctx context.Context) (AppSnapshot, error) {
	health := a.service.Health(ctx)
	settings, err := a.service.Settings(ctx)
	if err != nil {
		return AppSnapshot{}, err
	}
	lesson, running, err := a.service.CurrentLesson(ctx)
	if err != nil {
		return AppSnapshot{}, err
	}
	candidates, err := a.service.ListCandidates(ctx, 500)
	if err != nil {
		return AppSnapshot{}, err
	}
	vocabulary, err := a.service.ListVocabulary(ctx, 500)
	if err != nil {
		return AppSnapshot{}, err
	}
	transcriptLines := []TranscriptLine{}
	lessonID := lesson.ID
	if lessonID != "" {
		segments, err := a.service.ListTranscript(ctx, lessonID)
		if err != nil {
			return AppSnapshot{}, err
		}
		for _, segment := range segments {
			source := segment.Source
			if source == "" {
				source = "system"
			}
			transcriptLines = append(transcriptLines, TranscriptLine{
				ID:        segment.ID,
				Text:      segment.Text,
				Source:    source,
				Timestamp: timestamp(segment.CreatedAt),
			})
		}
	}
	mic, remote := a.service.Levels()
	cost, err := a.service.CurrentCost(ctx)
	if err != nil {
		return AppSnapshot{}, err
	}
	target := a.service.Target()
	if target == "" {
		target = "zoom"
	}
	connection := "connected"
	if !health.OK {
		connection = "offline"
	}
	lessonState := LessonSnapshot{Status: "idle"}
	if lesson.ID != "" {
		lessonState.StartedAt = timestamp(lesson.StartedAt)
		if running {
			lessonState.Status = "live"
		}
	}
	uiCandidates := makeCandidateSnapshots(candidates)
	uiVocabulary := make([]VocabularySnapshot, 0, len(vocabulary))
	for _, entry := range vocabulary {
		uiVocabulary = append(uiVocabulary, a.toVocabularySnapshot(ctx, entry))
	}
	return AppSnapshot{
		Connection:        connection,
		CapturePermission: string(a.service.CapturePermission()),
		Target:            target,
		Lesson:            lessonState,
		Levels:            AudioLevelsSnapshot{Mic: mic, Remote: remote},
		Transcript:        transcriptLines,
		Candidates:        uiCandidates,
		Vocabulary:        uiVocabulary,
		Cost: CostSummary{
			CurrentUSD:    cost,
			ProjectedUSD:  cost,
			HardBudgetUSD: settings.HardBudgetUSD,
			Currency:      "USD",
		},
		Privacy: PrivacyState{
			ZDREnabled:     true,
			AudioRetention: settings.AudioRetention,
		},
		Settings: SettingsSnapshot{
			OpenRouterKeyConfigured: a.service.HasAPIKey(ctx),
			STTModel:                settings.STTModel,
			AnalyzerModel:           settings.AnalyzerModel,
			HardBudgetUSD:           settings.HardBudgetUSD,
			AudioRetention:          settings.AudioRetention,
			OCREnabled:              settings.OCREnabled,
			MicrophoneEnabled:       settings.MicrophoneEnabled,
		},
		LastUpdated: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func makeCandidateSnapshots(entries []domain.VocabularyEntry) []CandidateSnapshot {
	result := make([]CandidateSnapshot, 0, len(entries))
	seen := make(map[string]string)
	for _, entry := range entries {
		bucket := "highConfidence"
		if entry.Manual || entry.Priority == "manual" {
			bucket = "manual"
		} else if entry.Confidence < 0.65 {
			bucket = "lowConfidence"
		}
		key := entry.Simplified + "\x00" + entry.Reading
		duplicateOf := entry.MergedIntoID
		if previous, ok := seen[key]; ok && duplicateOf == "" {
			duplicateOf = previous
			bucket = "possibleDuplicate"
		} else if duplicateOf == "" {
			seen[key] = entry.ID
		}
		meaning, partOfSpeech, classifier := firstSense(entry)
		example, translation := firstExample(entry)
		status := "pending"
		switch entry.Status {
		case domain.VocabularyStatusConfirmed:
			status = "confirmed"
		case domain.VocabularyStatusRejected, domain.VocabularyStatusMerged:
			status = "rejected"
		}
		provenance := string(entry.Provenance)
		if entry.Manual {
			provenance = "Manual mark"
		}
		result = append(result, CandidateSnapshot{
			ID:                 entry.ID,
			Simplified:         entry.Simplified,
			Traditional:        entry.Traditional,
			Pinyin:             firstNonEmpty(entry.MarkedPinyin, entry.Reading),
			Meaning:            meaning,
			PartOfSpeech:       partOfSpeech,
			Classifier:         classifier,
			Example:            example,
			ExampleTranslation: translation,
			Provenance:         provenance,
			Confidence:         entry.Confidence,
			Evidence:           firstEvidence(entry),
			Timestamp:          timestamp(entry.CreatedAt),
			Bucket:             bucket,
			Status:             status,
			DuplicateOf:        duplicateOf,
			Tags:               tagNames(entry.Tags),
		})
	}
	return result
}

func (a *App) toVocabularySnapshot(ctx context.Context, entry domain.VocabularyEntry) VocabularySnapshot {
	status := "learning"
	lastSeen := entry.UpdatedAt
	seenCount := 0
	if state, err := a.service.GetStudyState(ctx, entry.ID); err == nil {
		seenCount = state.Repetitions
		if state.LastReviewedAt != nil {
			lastSeen = *state.LastReviewedAt
		}
		if state.Repetitions >= 5 {
			status = "mastered"
		} else if state.DueAt != nil && !state.DueAt.After(time.Now()) {
			status = "review"
		}
	}
	meaning, partOfSpeech, classifier := firstSense(entry)
	return VocabularySnapshot{
		ID:           entry.ID,
		Simplified:   entry.Simplified,
		Traditional:  entry.Traditional,
		Pinyin:       firstNonEmpty(entry.MarkedPinyin, entry.Reading),
		Meaning:      meaning,
		PartOfSpeech: partOfSpeech,
		Classifier:   classifier,
		Status:       status,
		Tags:         tagNames(entry.Tags),
		LastSeen:     timestamp(lastSeen),
		SeenCount:    seenCount,
	}
}

func firstSense(entry domain.VocabularyEntry) (string, string, string) {
	if len(entry.Senses) == 0 {
		return "", "", ""
	}
	return entry.Senses[0].Gloss, entry.Senses[0].PartOfSpeech, entry.Senses[0].Classifier
}

func firstExample(entry domain.VocabularyEntry) (string, string) {
	if len(entry.Examples) == 0 {
		return "", ""
	}
	return entry.Examples[0].Simplified, entry.Examples[0].Translation
}

func firstEvidence(entry domain.VocabularyEntry) string {
	if len(entry.Evidence) == 0 {
		return ""
	}
	return entry.Evidence[0].Text
}

func tagNames(tags []domain.Tag) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		result = append(result, tag.Name)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
