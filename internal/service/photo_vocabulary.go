package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/oeggy03/call_analyzer/internal/openrouter"
)

const (
	// PhotoVocabularySessionID is deliberately stable because photo
	// recognition is not attached to a lesson foreign key.
	PhotoVocabularySessionID = "photo-vocabulary"

	maxVocabularyPhotoJPEGBytes = 1_572_864
	photoVocabularyTemperature  = 0.1
)

type PhotoVocabularyDraft struct {
	Entries []ManualVocabularyInput `json:"entries"`
	Model   string                  `json:"model"`
	Cost    float64                 `json:"cost"`
}

type photoVocabularyModelResponse struct {
	Entries []manualVocabularyModelResponse `json:"entries"`
}

func (s *Service) RecognizeVocabularyPhoto(
	ctx context.Context,
	jpeg []byte,
) (PhotoVocabularyDraft, error) {
	if err := validateVocabularyPhotoJPEG(jpeg); err != nil {
		return PhotoVocabularyDraft{}, err
	}
	router, err := s.photoVocabularyRouter(ctx)
	if err != nil {
		return PhotoVocabularyDraft{}, err
	}
	model := router.VisionModel()
	temperature := photoVocabularyTemperature
	response, callErr := router.ChatVisionJSON(ctx, openrouter.VisionChatRequest{
		System:      photoVocabularySystemPrompt,
		Prompt:      photoVocabularyPrompt,
		ImageJPEG:   jpeg,
		SchemaName:  "photo_mandarin_vocabulary",
		Schema:      photoVocabularySchema(),
		Temperature: &temperature,
	})
	usageErr := s.recordUsage(
		ctx,
		PhotoVocabularySessionID,
		model,
		"/api/v1/chat/completions",
		response.Usage,
	)
	if callErr != nil {
		message := "photo vocabulary recognition failed"
		if strings.Contains(strings.ToLower(callErr.Error()), "api key") {
			message = "photo vocabulary recognition failed: OpenRouter API key is unavailable; add it in Settings"
		}
		recognitionErr := fmt.Errorf("%s: %w", message, callErr)
		if usageErr != nil {
			recognitionErr = errors.Join(
				recognitionErr,
				fmt.Errorf("photo vocabulary: record OpenRouter usage: %w", usageErr),
			)
		}
		return PhotoVocabularyDraft{}, recognitionErr
	}
	if usageErr != nil {
		return PhotoVocabularyDraft{}, fmt.Errorf(
			"photo vocabulary: record OpenRouter usage: %w",
			usageErr,
		)
	}
	entries, err := parsePhotoVocabularyJSON(response.Content)
	if err != nil {
		return PhotoVocabularyDraft{}, fmt.Errorf(
			"photo vocabulary: parse model response: %w",
			err,
		)
	}
	if len(entries) == 0 {
		return PhotoVocabularyDraft{}, errors.New(
			"photo vocabulary: no Chinese words were recognized in the photo",
		)
	}
	return PhotoVocabularyDraft{
		Entries: entries,
		Model:   model,
		Cost:    response.Usage.Cost,
	}, nil
}

func validateVocabularyPhotoJPEG(jpeg []byte) error {
	if len(jpeg) == 0 {
		return errors.New("photo vocabulary: image is empty")
	}
	if len(jpeg) < 3 || jpeg[0] != 0xff || jpeg[1] != 0xd8 || jpeg[2] != 0xff {
		return errors.New("photo vocabulary: image must be a JPEG")
	}
	if len(jpeg) > maxVocabularyPhotoJPEGBytes {
		return errors.New("photo vocabulary: JPEG image must be at most 1.5 MiB")
	}
	return nil
}

func (s *Service) photoVocabularyRouter(ctx context.Context) (*openrouter.Client, error) {
	if s.store == nil {
		return nil, errors.New("photo vocabulary: storage is not configured")
	}
	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router == nil {
		if err := s.loadSettingsIfNeeded(ctx); err != nil {
			return nil, fmt.Errorf("photo vocabulary: load settings: %w", err)
		}
		s.mu.RLock()
		router = s.router
		s.mu.RUnlock()
	}
	if router == nil {
		return nil, errors.New(
			"photo vocabulary: OpenRouter is not configured; add an API key in Settings",
		)
	}
	return router, nil
}

func parsePhotoVocabularyJSON(payload string) ([]ManualVocabularyInput, error) {
	var response photoVocabularyModelResponse
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode photo vocabulary: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, errors.New("photo vocabulary has trailing JSON")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode trailing photo vocabulary: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return nil, fmt.Errorf("decode photo vocabulary fields: %w", err)
	}
	entriesField, ok := fields["entries"]
	if !ok || strings.TrimSpace(string(entriesField)) == "null" {
		return nil, errors.New("photo vocabulary is missing required field \"entries\"")
	}

	entries := make([]ManualVocabularyInput, 0, minInt(len(response.Entries), 12))
	for _, entry := range response.Entries {
		input := ManualVocabularyInput{
			Simplified:         entry.Simplified,
			Traditional:        entry.Traditional,
			Pinyin:             entry.Pinyin,
			Meaning:            entry.Meaning,
			PartOfSpeech:       entry.PartOfSpeech,
			Classifier:         entry.Classifier,
			Example:            entry.Example,
			ExamplePinyin:      entry.ExamplePinyin,
			ExampleTranslation: entry.ExampleTranslation,
			Tags:               entry.Tags,
			AiGenerated:        true,
		}
		if !containsHan(input.Simplified) {
			continue
		}
		normalized, err := normalizeManualVocabularyInput(input)
		if err != nil {
			continue
		}
		normalized.AiGenerated = true
		normalized.Tags = nil
		entries = append(entries, normalized)
		if len(entries) == 12 {
			break
		}
	}
	return entries, nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

const photoVocabularyPrompt = `Look at the supplied photo and pick out specific Chinese vocabulary that is visibly written in the image. Return each item as its own entry. Prefer words and short set phrases (typically one to six characters, or a visible vocabulary-list item). Skip punctuation, page numbers, isolated particles such as 的 了 吗, and do not turn a whole sentence into the headword. If a visible sentence illustrates a chosen word, put that sentence in the example field instead. Do not invent words that are not visible, and do not add related vocabulary that the image does not contain. Copy any visible pinyin and English meaning exactly when present. Return the word in simplified characters in the simplified field, and put the traditional form in the traditional field. Write the example sentence in simplified Chinese even when the photo uses traditional characters. For each chosen item, fill missing traditional form, spaced Hanyu Pinyin (write "xué xí", never "xuéxí"), concise meaning, part of speech, classifier (use an empty string when none applies), one natural simplified-Chinese example, example pinyin, and example translation. Return tags as an empty array and do not invent tags. Return at most 12 items, choosing the most useful distinct vocabulary if more text is visible. If no Chinese vocabulary is visible, return an empty entries array. Return only the strict JSON object requested by the schema.`

const photoVocabularySystemPrompt = `You are a careful Mandarin vocabulary picker. Read the image literally. Select specific words and short phrases whose characters are visibly present. Never hallucinate related words. Do not return full sentences as headwords. Preserve visible pinyin and English. Return simplified characters for the word and the example, and keep traditional characters in the traditional field. Complete missing learning fields. Separate every pinyin syllable with a space. Return tags as an empty array.`

func photoVocabularySchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"entries"},
		"properties": map[string]any{
			"entries": map[string]any{
				"type":     "array",
				"maxItems": 12,
				"items":    manualVocabularySchema(),
			},
		},
	}
}
