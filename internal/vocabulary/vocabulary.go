// Package vocabulary contains conservative candidate extraction and
// normalization. It deliberately produces candidates only; confirmation is a
// separate user action in storage/service.
package vocabulary

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/transcript"
)

type Candidate struct {
	Simplified   string            `json:"simplified"`
	Traditional  string            `json:"traditional"`
	Reading      string            `json:"reading"`
	MarkedPinyin string            `json:"markedPinyin,omitempty"`
	Evidence     []domain.Evidence `json:"evidence"`
	Confidence   float64           `json:"confidence"`
	Frequency    int               `json:"frequency,omitempty"`
	TeachingCue  float64           `json:"teachingCue,omitempty"`
	Senses       []domain.Sense    `json:"senses,omitempty"`
	Examples     []domain.Example  `json:"examples,omitempty"`
}

type Extraction struct {
	Candidates []Candidate `json:"candidates"`
}

var ErrMissingEvidence = errors.New("vocabulary: candidate requires transcript evidence")

func ExtractionSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"candidates"},
		"properties": map[string]any{
			"candidates": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"simplified", "traditional", "reading", "evidence", "confidence", "senses", "examples"},
					"properties": map[string]any{
						"simplified":  map[string]any{"type": "string"},
						"traditional": map[string]any{"type": "string"},
						"reading":     map[string]any{"type": "string"},
						"evidence": map[string]any{
							"type":     "array",
							"minItems": 1,
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"required":             []string{"text"},
								"properties": map[string]any{
									"text": map[string]any{"type": "string"},
								},
							},
						},
						"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
						"senses": map[string]any{
							"type":     "array",
							"minItems": 1,
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"required":             []string{"gloss", "partOfSpeech", "classifier"},
								"properties": map[string]any{
									"gloss":        map[string]any{"type": "string"},
									"partOfSpeech": map[string]any{"type": "string"},
									"classifier":   map[string]any{"type": "string"},
								},
							},
						},
						"examples": map[string]any{
							"type":     "array",
							"minItems": 1,
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"required":             []string{"simplified", "traditional", "reading", "translation", "generated"},
								"properties": map[string]any{
									"simplified":  map[string]any{"type": "string"},
									"traditional": map[string]any{"type": "string"},
									"reading":     map[string]any{"type": "string"},
									"translation": map[string]any{"type": "string"},
									"generated":   map[string]any{"type": "boolean"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func NormalizeCandidate(candidate Candidate, transcriptText string) (domain.VocabularyEntry, error) {
	if strings.TrimSpace(candidate.Simplified) == "" {
		return domain.VocabularyEntry{}, errors.New("vocabulary: simplified form is required")
	}
	if strings.TrimSpace(candidate.Traditional) == "" {
		candidate.Traditional = candidate.Simplified
	}
	if strings.TrimSpace(transcriptText) == "" || len(candidate.Evidence) == 0 {
		return domain.VocabularyEntry{}, ErrMissingEvidence
	}
	for _, evidence := range candidate.Evidence {
		if strings.TrimSpace(evidence.Text) == "" {
			return domain.VocabularyEntry{}, ErrMissingEvidence
		}
		if !strings.Contains(transcript.NormalizeChinese(transcriptText), transcript.NormalizeChinese(evidence.Text)) {
			return domain.VocabularyEntry{}, fmt.Errorf("vocabulary: evidence %q is not present in transcript", evidence.Text)
		}
	}
	numbered, marked, err := CanonicalPinyin(candidate.Reading)
	if err != nil {
		return domain.VocabularyEntry{}, err
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 || math.IsNaN(candidate.Confidence) {
		return domain.VocabularyEntry{}, errors.New("vocabulary: confidence must be between 0 and 1")
	}
	for i := range candidate.Examples {
		if strings.TrimSpace(candidate.Examples[i].Simplified) == "" ||
			strings.TrimSpace(candidate.Examples[i].Reading) == "" ||
			strings.TrimSpace(candidate.Examples[i].Translation) == "" {
			return domain.VocabularyEntry{}, errors.New("vocabulary: example sentence, pinyin, and translation are required")
		}
		exampleReading, _, err := CanonicalPinyin(candidate.Examples[i].Reading)
		if err != nil {
			return domain.VocabularyEntry{}, fmt.Errorf("vocabulary: example pinyin: %w", err)
		}
		candidate.Examples[i].Reading = exampleReading
		if candidate.Examples[i].Generated {
			candidate.Examples[i].Provenance = domain.ProvenanceModel
		}
	}
	return domain.VocabularyEntry{
		Simplified:   candidate.Simplified,
		Traditional:  candidate.Traditional,
		Reading:      numbered,
		MarkedPinyin: marked,
		Status:       domain.VocabularyStatusCandidate,
		Provenance:   domain.ProvenanceModel,
		Confidence:   candidate.Confidence,
		TeachingCue:  ScoreTeachingCue(candidate),
		Evidence:     append([]domain.Evidence(nil), candidate.Evidence...),
		Senses:       append([]domain.Sense(nil), candidate.Senses...),
		Examples:     append([]domain.Example(nil), candidate.Examples...),
	}, nil
}

func ScoreTeachingCue(candidate Candidate) float64 {
	confidence := clamp01(candidate.Confidence)
	evidence := clamp01(float64(len(candidate.Evidence)) / 3)
	frequency := clamp01(float64(candidate.Frequency) / 5)
	length := len([]rune(strings.TrimSpace(candidate.Simplified)))
	novelty := 1.0
	if length == 0 {
		novelty = 0
	} else if length > 6 {
		novelty = 0.6
	}
	score := confidence*0.55 + evidence*0.2 + frequency*0.15 + novelty*0.1
	return math.Round(clamp01(score)*1000) / 1000
}

func Deduplicate(candidates []Candidate) ([]Candidate, error) {
	type grouped struct {
		candidate Candidate
		index     int
	}
	groups := make(map[string]grouped)
	order := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		numbered, marked, err := CanonicalPinyin(candidate.Reading)
		if err != nil {
			return nil, err
		}
		candidate.Reading = numbered
		candidate.MarkedPinyin = marked
		if strings.TrimSpace(candidate.Traditional) == "" {
			candidate.Traditional = candidate.Simplified
		}
		key := candidate.Simplified + "\x00" + candidate.Traditional + "\x00" + numbered
		group, exists := groups[key]
		if !exists {
			groups[key] = grouped{candidate: candidate, index: len(order)}
			order = append(order, key)
			continue
		}
		group.candidate.Evidence = append(group.candidate.Evidence, candidate.Evidence...)
		group.candidate.Senses = append(group.candidate.Senses, candidate.Senses...)
		group.candidate.Examples = append(group.candidate.Examples, candidate.Examples...)
		if candidate.Confidence > group.candidate.Confidence {
			group.candidate.Confidence = candidate.Confidence
		}
		if candidate.Frequency > group.candidate.Frequency {
			group.candidate.Frequency = candidate.Frequency
		}
		group.candidate.TeachingCue = ScoreTeachingCue(group.candidate)
		groups[key] = group
	}
	sort.SliceStable(order, func(i, j int) bool {
		return groups[order[i]].index < groups[order[j]].index
	})
	result := make([]Candidate, 0, len(order))
	for _, key := range order {
		result = append(result, groups[key].candidate)
	}
	return result, nil
}

func DedupKey(simplified, traditional, numberedReading string) string {
	// Forms are always part of the key. In particular, reading alone is never
	// enough to merge two entries such as 行 (xing2) and 形 (xing2).
	return simplified + "\x00" + traditional + "\x00" + numberedReading
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

type Dictionary interface {
	Lookup(context.Context, string, string) ([]domain.Sense, error)
}

type EmbeddedDictionary struct {
	entries map[string][]domain.Sense
}

func NewEmbeddedDictionary() *EmbeddedDictionary {
	return &EmbeddedDictionary{entries: map[string][]domain.Sense{
		"你好": {
			{Gloss: "hello; hi", PartOfSpeech: "greeting", SortOrder: 0},
		},
		"谢谢": {
			{Gloss: "thanks; thank you", PartOfSpeech: "verb", SortOrder: 0},
		},
		"再见": {
			{Gloss: "goodbye", PartOfSpeech: "greeting", SortOrder: 0},
		},
		"学习": {
			{Gloss: "to study; to learn", PartOfSpeech: "verb", SortOrder: 0},
		},
		"中文": {
			{Gloss: "Chinese language", PartOfSpeech: "noun", SortOrder: 0},
		},
	}}
}

func (d *EmbeddedDictionary) Lookup(ctx context.Context, simplified, traditional string) ([]domain.Sense, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	senses, ok := d.entries[simplified]
	if !ok {
		senses, ok = d.entries[traditional]
	}
	if !ok {
		return []domain.Sense{}, nil
	}
	result := make([]domain.Sense, len(senses))
	copy(result, senses)
	return result, nil
}

func GeneratedExample(simplified, traditional, reading, translation string) domain.Example {
	return domain.Example{
		Simplified:  simplified,
		Traditional: traditional,
		Reading:     reading,
		Translation: translation,
		Provenance:  domain.ProvenanceModel,
		Generated:   true,
	}
}

var toneMarks = map[rune]struct {
	base rune
	tone int
}{
	'ā': {'a', 1}, 'á': {'a', 2}, 'ǎ': {'a', 3}, 'à': {'a', 4},
	'ē': {'e', 1}, 'é': {'e', 2}, 'ě': {'e', 3}, 'è': {'e', 4},
	'ī': {'i', 1}, 'í': {'i', 2}, 'ǐ': {'i', 3}, 'ì': {'i', 4},
	'ō': {'o', 1}, 'ó': {'o', 2}, 'ǒ': {'o', 3}, 'ò': {'o', 4},
	'ū': {'u', 1}, 'ú': {'u', 2}, 'ǔ': {'u', 3}, 'ù': {'u', 4},
	'ǖ': {'ü', 1}, 'ǘ': {'ü', 2}, 'ǚ': {'ü', 3}, 'ǜ': {'ü', 4},
	'Ā': {'a', 1}, 'Á': {'a', 2}, 'Ǎ': {'a', 3}, 'À': {'a', 4},
	'Ē': {'e', 1}, 'É': {'e', 2}, 'Ě': {'e', 3}, 'È': {'e', 4},
	'Ī': {'i', 1}, 'Í': {'i', 2}, 'Ǐ': {'i', 3}, 'Ì': {'i', 4},
	'Ō': {'o', 1}, 'Ó': {'o', 2}, 'Ǒ': {'o', 3}, 'Ò': {'o', 4},
	'Ū': {'u', 1}, 'Ú': {'u', 2}, 'Ǔ': {'u', 3}, 'Ù': {'u', 4},
	'Ǖ': {'ü', 1}, 'Ǘ': {'ü', 2}, 'Ǚ': {'ü', 3}, 'Ǜ': {'ü', 4},
}

var markedVowels = map[string]rune{
	"a1": 'ā', "a2": 'á', "a3": 'ǎ', "a4": 'à',
	"e1": 'ē', "e2": 'é', "e3": 'ě', "e4": 'è',
	"i1": 'ī', "i2": 'í', "i3": 'ǐ', "i4": 'ì',
	"o1": 'ō', "o2": 'ó', "o3": 'ǒ', "o4": 'ò',
	"u1": 'ū', "u2": 'ú', "u3": 'ǔ', "u4": 'ù',
	"ü1": 'ǖ', "ü2": 'ǘ', "ü3": 'ǚ', "ü4": 'ǜ',
}

func CanonicalPinyin(input string) (numbered, marked string, err error) {
	input = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(input, "u:", "ü")))
	if input == "" {
		return "", "", errors.New("vocabulary: pinyin reading is required")
	}
	tokens := splitPinyin(input)
	numberedTokens := make([]string, 0, len(tokens))
	markedTokens := make([]string, 0, len(tokens))
	for _, token := range tokens {
		n, m, err := canonicalSyllable(token)
		if err != nil {
			return "", "", err
		}
		numberedTokens = append(numberedTokens, n)
		markedTokens = append(markedTokens, m)
	}
	return strings.Join(numberedTokens, " "), strings.Join(markedTokens, " "), nil
}

func splitPinyin(input string) []string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || r == '·'
	})
	var result []string
	for _, field := range fields {
		start := 0
		runes := []rune(field)
		for i, r := range runes {
			if r >= '0' && r <= '5' {
				result = append(result, string(runes[start:i+1]))
				start = i + 1
			}
		}
		if start < len(runes) {
			result = append(result, string(runes[start:]))
		}
	}
	return result
}

func canonicalSyllable(token string) (string, string, error) {
	if token == "" {
		return "", "", errors.New("vocabulary: empty pinyin syllable")
	}
	runes := []rune(strings.ToLower(token))
	tone := 0
	base := make([]rune, 0, len(runes))
	for _, r := range runes {
		if r >= '0' && r <= '5' {
			if tone != 0 {
				return "", "", fmt.Errorf("vocabulary: invalid pinyin syllable %q", token)
			}
			tone = int(r - '0')
			continue
		}
		if converted, ok := toneMarks[r]; ok {
			if tone != 0 && tone != converted.tone {
				return "", "", fmt.Errorf("vocabulary: conflicting tone in %q", token)
			}
			tone = converted.tone
			base = append(base, converted.base)
			continue
		}
		if r == 'v' {
			r = 'ü'
		}
		if !isPinyinLetter(r) {
			return "", "", fmt.Errorf("vocabulary: invalid pinyin syllable %q", token)
		}
		base = append(base, r)
	}
	if len(base) == 0 {
		return "", "", fmt.Errorf("vocabulary: invalid pinyin syllable %q", token)
	}
	if !validPinyinBase(string(base)) {
		return "", "", fmt.Errorf("vocabulary: invalid pinyin syllable %q", token)
	}
	if tone == 0 {
		tone = 5
	}
	if tone > 5 {
		return "", "", fmt.Errorf("vocabulary: invalid tone in %q", token)
	}
	plain := string(base)
	numbered := plain + strconv.Itoa(tone)
	if tone == 5 {
		return numbered, plain, nil
	}
	priority := []rune{'a', 'e', 'o'}
	markIndex := -1
	for _, vowel := range priority {
		for i, r := range base {
			if r == vowel {
				markIndex = i
				break
			}
		}
		if markIndex >= 0 {
			break
		}
	}
	if markIndex < 0 {
		for i := len(base) - 1; i >= 0; i-- {
			if base[i] == 'i' || base[i] == 'u' || base[i] == 'ü' {
				markIndex = i
				break
			}
		}
	}
	if markIndex < 0 {
		return "", "", fmt.Errorf("vocabulary: no vowel in %q", token)
	}
	markedBase := append([]rune(nil), base...)
	marked, ok := markedVowels[string([]rune{markedBase[markIndex]})+strconv.Itoa(tone)]
	if !ok {
		return "", "", fmt.Errorf("vocabulary: cannot mark syllable %q", token)
	}
	markedBase[markIndex] = marked
	return numbered, string(markedBase), nil
}

func isPinyinLetter(value rune) bool {
	return (value >= 'a' && value <= 'z') || value == 'ü'
}

func validPinyinBase(value string) bool {
	finals := map[string]struct{}{
		"a": {}, "ai": {}, "an": {}, "ang": {}, "ao": {},
		"e": {}, "ei": {}, "en": {}, "eng": {}, "er": {},
		"i": {}, "ia": {}, "ian": {}, "iang": {}, "iao": {},
		"ie": {}, "in": {}, "ing": {}, "iong": {}, "iu": {},
		"o": {}, "ong": {}, "ou": {},
		"u": {}, "ua": {}, "uai": {}, "uan": {}, "uang": {},
		"ue": {}, "ui": {}, "un": {}, "uo": {},
		"ü": {}, "üan": {}, "üe": {}, "ün": {},
	}
	initials := []string{
		"zh", "ch", "sh",
		"b", "c", "d", "f", "g", "h", "j", "k", "l",
		"m", "n", "p", "q", "r", "s", "t", "w", "x", "y", "z",
		"",
	}
	for _, initial := range initials {
		if !strings.HasPrefix(value, initial) {
			continue
		}
		if _, ok := finals[strings.TrimPrefix(value, initial)]; ok {
			return true
		}
	}
	return false
}
