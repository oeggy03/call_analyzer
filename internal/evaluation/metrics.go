package evaluation

import (
	"strings"
	"unicode"
)

type SetScore struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	Matched   int     `json:"matched"`
	Expected  int     `json:"expected"`
	Predicted int     `json:"predicted"`
}

// CharacterErrorRate is a language-agnostic edit-distance score that works
// better than whitespace-delimited WER for Mandarin.
func CharacterErrorRate(expected, actual string) float64 {
	want := []rune(normalize(expected))
	got := []rune(normalize(actual))
	if len(want) == 0 {
		if len(got) == 0 {
			return 0
		}
		return 1
	}
	return float64(editDistance(want, got)) / float64(len(want))
}

func ScoreWords(expected, predicted []string) SetScore {
	want := makeSet(expected)
	got := makeSet(predicted)
	matched := 0
	for value := range got {
		if _, ok := want[value]; ok {
			matched++
		}
	}
	score := SetScore{
		Matched:   matched,
		Expected:  len(want),
		Predicted: len(got),
	}
	if len(got) > 0 {
		score.Precision = float64(matched) / float64(len(got))
	}
	if len(want) > 0 {
		score.Recall = float64(matched) / float64(len(want))
	}
	return score
}

func normalize(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func makeSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalize(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func editDistance(left, right []rune) int {
	previous := make([]int, len(right)+1)
	for i := range previous {
		previous[i] = i
	}
	for i, l := range left {
		current := make([]int, len(right)+1)
		current[0] = i + 1
		for j, r := range right {
			cost := 1
			if l == r {
				cost = 0
			}
			current[j+1] = min(
				current[j]+1,
				previous[j+1]+1,
				previous[j]+cost,
			)
		}
		previous = current
	}
	return previous[len(right)]
}

func min(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
