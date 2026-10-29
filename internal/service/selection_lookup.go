package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/oeggy03/call_analyzer/internal/chinese"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/selection"
	"github.com/oeggy03/call_analyzer/internal/vocabulary"
)

const (
	SelectionLookupSessionID = "selection-lookup"

	selectionLookupTimeout       = 12 * time.Second
	selectionLookupCacheCapacity = 64
	selectionLookupMaxLines      = 3
	selectionLookupMaxMeaning    = 500
	selectionLookupMaxTokens     = 400

	selectionEmptyNotice      = "Highlight Chinese text, then press ⌘⌥5."
	selectionNotChineseNotice = "Select Chinese text."
	selectionFailedNotice     = "Couldn't look this up. Try again."
	selectionNoGloss          = "No English gloss saved"
)

type SelectionAddWord struct {
	Simplified    string                 `json:"simplified"`
	Traditional   string                 `json:"traditional,omitempty"`
	Pinyin        string                 `json:"pinyin"`
	Meaning       string                 `json:"meaning"`
	VocabularyID  string                 `json:"vocabularyId,omitempty"`
	Destination   string                 `json:"destination"`
	LessonBlocked bool                   `json:"lessonBlocked"`
	Draft         *ManualVocabularyDraft `json:"draft,omitempty"`
}

type selectionLookupCache struct {
	mu      sync.Mutex
	entries map[string][]selection.GlossLine
	order   []string
}

func (c *selectionLookupCache) get(key string) ([]selection.GlossLine, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	lines, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.touchLocked(key)
	return cloneGlossLines(lines), true
}

func (c *selectionLookupCache) put(key string, lines []selection.GlossLine) {
	if len(lines) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string][]selection.GlossLine)
	}
	if _, exists := c.entries[key]; !exists {
		if len(c.order) >= selectionLookupCacheCapacity {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
	}
	c.entries[key] = cloneGlossLines(lines)
	c.touchLocked(key)
}

func (c *selectionLookupCache) touchLocked(key string) {
	for index, existing := range c.order {
		if existing != key {
			continue
		}
		c.order = append(c.order[:index], c.order[index+1:]...)
		break
	}
	c.order = append(c.order, key)
}

func cloneGlossLines(lines []selection.GlossLine) []selection.GlossLine {
	return append([]selection.GlossLine(nil), lines...)
}

func (s *Service) LookupSelection(
	ctx context.Context,
	surface string,
	sentenceContext string,
) (selection.LookupResult, error) {
	normalized, notice := normalizeSelectionSurface(surface)
	result := selection.LookupResult{Surface: normalized}
	if notice != "" {
		result.Notice = notice
		return result, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.store == nil {
		return result, errors.New("selection lookup: storage is not configured")
	}
	tagNames, err := s.ListTagNames(ctx)
	if err != nil {
		return result, err
	}
	result.ExistingTags = tagNames

	simplified := chinese.ToSimplified(normalized)
	contextText := normalizeSelectionContext(
		sentenceContext,
		normalized,
		strings.TrimSpace(surface),
	)
	forms := []string{normalized}
	if simplified != normalized {
		forms = append(forms, simplified)
	}
	localEntries, err := s.store.Vocabulary().ListMatchingSurface(ctx, forms)
	if err != nil {
		return result, fmt.Errorf("selection lookup: storage: %w", err)
	}
	if len(localEntries) > 0 {
		result.Lines = selectionGlossLines(normalized, localEntries)
		return result, nil
	}

	cacheKey := simplified + "\n" + contextText
	if cached, ok := s.selectionCache.get(cacheKey); ok {
		result.Lines = cached
		return result, nil
	}

	router, err := s.selectionLookupRouter()
	if err != nil {
		return result, err
	}
	lookupContext, cancel := context.WithTimeout(ctx, selectionLookupTimeout)
	defer cancel()
	response, callErr := router.ChatJSON(lookupContext, openrouter.ChatRequest{
		Prompt:           selectionLookupPrompt(normalized, contextText),
		System:           selectionLookupSystemPrompt,
		SchemaName:       "selection_lookup",
		Schema:           selectionLookupSchema(),
		Model:            openrouter.DefaultLookupModel,
		MaxTokens:        selectionLookupMaxTokens,
		Temperature:      floatPointer(0),
		DisableReasoning: true,
	})
	usageErr := s.recordUsage(
		ctx,
		SelectionLookupSessionID,
		openrouter.DefaultLookupModel,
		"/api/v1/chat/completions",
		response.Usage,
	)
	if callErr != nil {
		lookupErr := fmt.Errorf("selection lookup failed: %w", callErr)
		if usageErr != nil {
			lookupErr = errors.Join(
				lookupErr,
				fmt.Errorf("selection lookup: record OpenRouter usage: %w", usageErr),
			)
		}
		return result, lookupErr
	}
	if usageErr != nil {
		return result, fmt.Errorf(
			"selection lookup: record OpenRouter usage: %w",
			usageErr,
		)
	}

	entries, err := parseSelectionLookupJSON(response.Content)
	if err != nil {
		return result, fmt.Errorf("selection lookup: parse model response: %w", err)
	}
	lines := make([]selection.GlossLine, 0, minInt(len(entries), selectionLookupMaxLines))
	for _, entry := range entries {
		pinyin := strings.TrimSpace(entry.Pinyin)
		meaning := strings.TrimSpace(entry.Meaning)
		if pinyin == "" || meaning == "" {
			continue
		}
		lines = append(lines, selection.GlossLine{
			Surface:     normalized,
			Simplified:  simplified,
			Pinyin:      pinyin,
			Meaning:     meaning,
			Action:      "add",
			Destination: "add-word",
		})
		if len(lines) == selectionLookupMaxLines {
			break
		}
	}
	if len(lines) == 0 {
		result.Notice = selectionFailedNotice
		return result, nil
	}
	s.selectionCache.put(cacheKey, lines)
	result.Lines = lines
	return result, nil
}

func (s *Service) AddSelection(
	ctx context.Context,
	line selection.GlossLine,
) (selection.CardDraft, error) {
	if s == nil {
		return selection.CardDraft{}, errors.New("selection lookup: service is not configured")
	}
	if line.Action == "open" {
		return selection.CardDraft{
			Fields:   cardFieldsFromLine(line),
			ReadOnly: true,
		}, nil
	}
	if line.Action != "add" {
		return selection.CardDraft{}, fmt.Errorf("selection lookup: unsupported action %q", line.Action)
	}

	simplified := line.Simplified
	if strings.TrimSpace(simplified) == "" {
		simplified = chinese.ToSimplified(strings.TrimSpace(line.Surface))
	}
	meaning := line.Meaning
	if strings.TrimSpace(meaning) == selectionNoGloss {
		meaning = ""
	}
	draft, err := s.generateManualVocabulary(ctx, ManualVocabularyInput{
		Simplified:  simplified,
		Traditional: line.Traditional,
		Pinyin:      line.Pinyin,
		Meaning:     meaning,
	}, false)
	if err != nil {
		return selection.CardDraft{}, err
	}
	return cardDraftFromManual(draft), nil
}

func (s *Service) ListTagNames(ctx context.Context) ([]string, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("selection lookup: storage is not configured")
	}
	names, err := s.store.Vocabulary().ListTagNames(ctx)
	if err != nil {
		return nil, err
	}
	if names == nil {
		return []string{}, nil
	}
	return names, nil
}

func (s *Service) SaveSelection(ctx context.Context, fields selection.CardFields) error {
	if s == nil {
		return errors.New("selection lookup: service is not configured")
	}
	_, err := s.SaveManualVocabulary(ctx, ManualVocabularyInput{
		Simplified:         fields.Simplified,
		Traditional:        fields.Traditional,
		Pinyin:             fields.Pinyin,
		Meaning:            fields.Meaning,
		PartOfSpeech:       fields.PartOfSpeech,
		Classifier:         fields.Classifier,
		Example:            fields.Example,
		ExamplePinyin:      fields.ExamplePinyin,
		ExampleTranslation: fields.ExampleTranslation,
		Tags:               fields.Tags,
		AiGenerated:        fields.AIGenerated,
	})
	return err
}

func cardFieldsFromLine(line selection.GlossLine) selection.CardFields {
	meaning := line.Meaning
	if strings.TrimSpace(meaning) == selectionNoGloss {
		meaning = ""
	}
	return selection.CardFields{
		Simplified:         line.Simplified,
		Traditional:        line.Traditional,
		Pinyin:             line.Pinyin,
		Meaning:            meaning,
		PartOfSpeech:       line.PartOfSpeech,
		Classifier:         line.Classifier,
		Example:            line.Example,
		ExamplePinyin:      line.ExamplePinyin,
		ExampleTranslation: line.ExampleTranslation,
		Tags:               line.Tags,
	}
}

func cardDraftFromManual(draft ManualVocabularyDraft) selection.CardDraft {
	fields := selection.CardFields{
		Simplified:         draft.Simplified,
		Traditional:        draft.Traditional,
		Pinyin:             draft.Pinyin,
		Meaning:            draft.Meaning,
		PartOfSpeech:       draft.PartOfSpeech,
		Classifier:         draft.Classifier,
		Example:            draft.Example,
		ExamplePinyin:      draft.ExamplePinyin,
		ExampleTranslation: draft.ExampleTranslation,
		Tags:               draft.Tags,
		AIGenerated:        strings.TrimSpace(draft.Example) != "",
	}
	variants := make([]selection.CardVariant, 0, len(draft.Variants))
	for _, variant := range draft.Variants {
		variants = append(variants, selection.CardVariant{
			Traditional:        variant.Traditional,
			Pinyin:             variant.Pinyin,
			Meaning:            variant.Meaning,
			PartOfSpeech:       variant.PartOfSpeech,
			Classifier:         variant.Classifier,
			Example:            variant.Example,
			ExamplePinyin:      variant.ExamplePinyin,
			ExampleTranslation: variant.ExampleTranslation,
		})
	}
	return selection.CardDraft{
		Fields:      fields,
		NeedsChoice: draft.NeedsChoice,
		Variants:    variants,
	}
}

func (s *Service) selectionLookupRouter() (*openrouter.Client, error) {
	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router == nil {
		return nil, errors.New(
			"selection lookup: OpenRouter is not configured; add an API key in Settings",
		)
	}
	return router, nil
}

func normalizeSelectionSurface(surface string) (string, string) {
	normalized := strings.TrimSpace(surface)
	for {
		next := strings.TrimSpace(strings.TrimFunc(normalized, selectionEdgeTrim))
		if next == normalized {
			break
		}
		normalized = next
	}
	if normalized == "" {
		return normalized, selectionEmptyNotice
	}
	if !chinese.ContainsHan(normalized) {
		return normalized, selectionNotChineseNotice
	}
	return normalized, ""
}

func selectionEdgeTrim(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func normalizeSelectionContext(value, normalizedSurface, trimmedSurface string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 80 {
		value = string(runes[:80])
	}
	if value == "" ||
		value == normalizedSurface ||
		value == trimmedSurface ||
		!chinese.ContainsHan(value) {
		return ""
	}
	return value
}

func selectionGlossLines(
	normalizedSurface string,
	entries []domain.VocabularyEntry,
) []selection.GlossLine {
	confirmedReadings := make(map[string]struct{})
	for _, entry := range entries {
		if entry.Status == domain.VocabularyStatusConfirmed {
			confirmedReadings[entry.Reading] = struct{}{}
		}
	}
	lines := make([]selection.GlossLine, 0, minInt(len(entries), selectionLookupMaxLines))
	for _, entry := range entries {
		if entry.Status == domain.VocabularyStatusCandidate {
			if _, exists := confirmedReadings[entry.Reading]; exists {
				continue
			}
		}
		action := "add"
		destination := "add-word"
		if len(entry.Examples) > 0 {
			action = "open"
			switch entry.Status {
			case domain.VocabularyStatusConfirmed:
				destination = "vocabulary"
			case domain.VocabularyStatusCandidate:
				destination = "inbox"
			}
		}
		pinyin := strings.TrimSpace(entry.MarkedPinyin)
		if pinyin == "" {
			pinyin = strings.TrimSpace(entry.Reading)
		}
		example, examplePinyin, exampleTranslation := selectionExample(entry)
		partOfSpeech := ""
		classifier := ""
		if len(entry.Senses) > 0 {
			partOfSpeech = strings.TrimSpace(entry.Senses[0].PartOfSpeech)
			classifier = strings.TrimSpace(entry.Senses[0].Classifier)
		}
		lines = append(lines, selection.GlossLine{
			Surface:            normalizedSurface,
			Simplified:         entry.Simplified,
			Traditional:        entry.Traditional,
			Pinyin:             pinyin,
			Meaning:            selectionMeaning(entry),
			PartOfSpeech:       partOfSpeech,
			Classifier:         classifier,
			Example:            example,
			ExamplePinyin:      examplePinyin,
			ExampleTranslation: exampleTranslation,
			Tags:               selectionTagNames(entry),
			VocabularyID:       entry.ID,
			Action:             action,
			Destination:        destination,
		})
		if len(lines) == selectionLookupMaxLines {
			break
		}
	}
	return lines
}

func selectionExample(entry domain.VocabularyEntry) (string, string, string) {
	if len(entry.Examples) == 0 {
		return "", "", ""
	}
	example := entry.Examples[0]
	reading := strings.TrimSpace(example.Reading)
	if _, marked, err := vocabulary.CanonicalPinyin(reading); err == nil && strings.TrimSpace(marked) != "" {
		reading = marked
	}
	return strings.TrimSpace(example.Simplified), reading, strings.TrimSpace(example.Translation)
}

func selectionTagNames(entry domain.VocabularyEntry) []string {
	if len(entry.Tags) == 0 {
		return nil
	}
	names := make([]string, 0, len(entry.Tags))
	for _, tag := range entry.Tags {
		name := strings.TrimSpace(tag.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

func selectionMeaning(entry domain.VocabularyEntry) string {
	glosses := make([]string, 0, len(entry.Senses))
	for _, sense := range entry.Senses {
		gloss := strings.TrimSpace(sense.Gloss)
		if gloss != "" {
			glosses = append(glosses, gloss)
		}
	}
	if len(glosses) == 0 {
		return selectionNoGloss
	}
	meaning := strings.Join(glosses, "; ")
	runes := []rune(meaning)
	if len(runes) > selectionLookupMaxMeaning {
		meaning = string(runes[:selectionLookupMaxMeaning])
	}
	return meaning
}

func selectionLookupPrompt(normalizedSurface, contextText string) string {
	if contextText == "" {
		return "Word: " + normalizedSurface
	}
	return "Word: " + normalizedSurface + "\nContext: " + contextText
}

const selectionLookupSystemPrompt = `You are a concise Mandarin dictionary. The selection may be one word or a longer phrase. Return tone-marked Hanyu Pinyin for the whole selection and a short English gloss of the whole selection. Return one entry when the meaning is clear. When a short selection has several common readings, return up to three. Do not include examples, notes, or character forms.`

func selectionLookupSchema() map[string]any {
	entrySchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"pinyin", "meaning"},
		"properties": map[string]any{
			"pinyin":  map[string]any{"type": "string"},
			"meaning": map[string]any{"type": "string"},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"entries"},
		"properties": map[string]any{
			"entries": map[string]any{
				"type":  "array",
				"items": entrySchema,
			},
		},
	}
}

type selectionLookupModelResponse struct {
	Entries []selectionLookupModelEntry `json:"entries"`
}

type selectionLookupModelEntry struct {
	Pinyin  string `json:"pinyin"`
	Meaning string `json:"meaning"`
}

func parseSelectionLookupJSON(payload string) ([]selectionLookupModelEntry, error) {
	var response selectionLookupModelResponse
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode selection lookup: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, errors.New("selection lookup has trailing JSON")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode trailing selection lookup: %w", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return nil, fmt.Errorf("decode selection lookup fields: %w", err)
	}
	entriesField, ok := fields["entries"]
	if !ok || strings.TrimSpace(string(entriesField)) == "null" {
		return nil, errors.New("selection lookup is missing required field \"entries\"")
	}
	var rawEntries []json.RawMessage
	if err := json.Unmarshal(entriesField, &rawEntries); err != nil {
		return nil, fmt.Errorf("decode selection lookup entries: %w", err)
	}
	if len(rawEntries) != len(response.Entries) {
		return nil, errors.New("selection lookup entries are invalid")
	}
	for index, rawEntry := range rawEntries {
		entryFields := rawObjectFields(rawEntry)
		if err := validateManualVocabularyJSONFields(
			entryFields,
			fmt.Sprintf("selection lookup entry %d", index+1),
			[]string{"pinyin", "meaning"},
		); err != nil {
			return nil, err
		}
	}
	return response.Entries, nil
}

func floatPointer(value float64) *float64 {
	return &value
}
