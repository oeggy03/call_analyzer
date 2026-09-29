package main

import (
	"context"
	"strings"
	"time"

	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/vocabulary"
)

func validTarget(target string) bool {
	switch target {
	case "zoom", "us.zoom.xos", "googleMeet", "teams":
		return true
	default:
		return false
	}
}

func normalizeTarget(target string) string {
	if target == "us.zoom.xos" {
		return "zoom"
	}
	return target
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
			isMoment := source == "system/mark"
			if isMoment || source == "" {
				source = "system"
			}
			transcriptLines = append(transcriptLines, TranscriptLine{
				ID:        segment.ID,
				Text:      segment.Text,
				Source:    source,
				Timestamp: timestamp(lesson.StartedAt.Add(time.Duration(segment.StartMS) * time.Millisecond)),
				IsMoment:  isMoment,
			})
		}
	}
	mic, remote := a.service.Levels()
	cost, err := a.service.CurrentCost(ctx)
	if err != nil {
		return AppSnapshot{}, err
	}
	projected := cost
	if running && !lesson.StartedAt.IsZero() {
		elapsed := time.Since(lesson.StartedAt)
		if elapsed > 0 {
			projectionWindow := elapsed
			if projectionWindow < 5*time.Minute {
				projectionWindow = 5 * time.Minute
			}
			runRate := cost / projectionWindow.Hours()
			if runRate > projected {
				projected = runRate
			}
		}
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
		lessonState.ID = lesson.ID
		lessonState.StartedAt = timestamp(lesson.StartedAt)
		lessonState.Error = a.service.LessonError()
		if running {
			lessonState.Status = "live"
		} else if lesson.EndedAt == nil && lessonState.Error != "" {
			lessonState.Status = "error"
		}
	}
	uiCandidates := a.makeCandidateSnapshots(ctx, candidates)
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
			ProjectedUSD:  projected,
			HardBudgetUSD: settings.HardBudgetUSD,
			Warning:       cost >= 0.35,
			HardExceeded:  cost >= settings.HardBudgetUSD,
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

func (a *App) makeCandidateSnapshots(ctx context.Context, entries []domain.VocabularyEntry) []CandidateSnapshot {
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
		example, examplePinyin, translation := firstExample(entry)
		status := "pending"
		switch entry.Status {
		case domain.VocabularyStatusConfirmed:
			status = "confirmed"
		case domain.VocabularyStatusRejected, domain.VocabularyStatusMerged:
			status = "rejected"
		}
		provenance := string(entry.Provenance)
		candidateTimestamp := entry.CreatedAt
		if len(entry.Evidence) > 0 {
			evidence := entry.Evidence[0]
			if lesson, err := a.store.Lessons().Get(ctx, evidence.LessonID); err == nil {
				candidateTimestamp = lesson.StartedAt.Add(time.Duration(evidence.StartMS) * time.Millisecond)
			}
			if segment, err := a.store.Transcripts().Get(ctx, evidence.SegmentID); err == nil {
				switch {
				case strings.HasPrefix(segment.Source, "system/ocr"):
					provenance = "Screen OCR"
				case segment.Source == "remote":
					provenance = "Remote transcript"
				case segment.Source == "microphone":
					provenance = "Microphone transcript"
				case segment.Source != "":
					provenance = "Lesson transcript"
				}
			}
		}
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
			ExamplePinyin:      examplePinyin,
			ExampleTranslation: translation,
			Provenance:         provenance,
			Confidence:         entry.Confidence,
			Evidence:           firstEvidence(entry),
			Timestamp:          timestamp(candidateTimestamp),
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
	seenCount := len(entry.Evidence)
	if count, observedAt, err := a.service.ObservationStats(ctx, entry.ID); err == nil {
		seenCount = count
		if observedAt.After(lastSeen) {
			lastSeen = observedAt
		}
	}
	if state, err := a.service.GetStudyState(ctx, entry.ID); err == nil {
		if state.Repetitions >= 5 {
			status = "mastered"
		} else if state.DueAt != nil && !state.DueAt.After(time.Now()) {
			status = "review"
		}
	}
	meaning, partOfSpeech, classifier := firstSense(entry)
	example, examplePinyin, exampleTranslation := firstExample(entry)
	return VocabularySnapshot{
		ID:                 entry.ID,
		Simplified:         entry.Simplified,
		Traditional:        entry.Traditional,
		Pinyin:             firstNonEmpty(entry.MarkedPinyin, entry.Reading),
		Meaning:            meaning,
		PartOfSpeech:       partOfSpeech,
		Classifier:         classifier,
		Example:            example,
		ExamplePinyin:      examplePinyin,
		ExampleTranslation: exampleTranslation,
		Status:             status,
		Tags:               tagNames(entry.Tags),
		LastSeen:           timestamp(lastSeen),
		SeenCount:          seenCount,
	}
}

func firstSense(entry domain.VocabularyEntry) (string, string, string) {
	if len(entry.Senses) == 0 {
		return "", "", ""
	}
	return entry.Senses[0].Gloss, entry.Senses[0].PartOfSpeech, entry.Senses[0].Classifier
}

func firstExample(entry domain.VocabularyEntry) (string, string, string) {
	if len(entry.Examples) == 0 {
		return "", "", ""
	}
	example := entry.Examples[0]
	pinyin := example.Reading
	if _, marked, err := vocabulary.CanonicalPinyin(example.Reading); err == nil {
		pinyin = marked
	}
	return example.Simplified, pinyin, example.Translation
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
