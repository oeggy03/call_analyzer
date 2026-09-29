// Package transcript contains deterministic transcript post-processing that
// does not depend on a model or a platform audio API.
package transcript

import (
	"strings"
	"unicode"
)

var punctuation = strings.NewReplacer(
	"。", ".", "，", ",", "、", ",", "！", "!", "？", "?",
	"：", ":", "；", ";", "（", "(", "）", ")", "【", "[", "】", "]",
	"「", "\"", "」", "\"", "“", "\"", "”", "\"", "’", "'",
)

func NormalizeChinese(text string) string {
	text = punctuation.Replace(text)
	runes := []rune(strings.TrimSpace(text))
	out := make([]rune, 0, len(runes))
	pendingSpace := false
	var previous rune
	for _, current := range runes {
		if unicode.IsSpace(current) {
			pendingSpace = len(out) > 0
			continue
		}
		current = unicode.ToLower(current)
		if pendingSpace && shouldInsertSpace(previous, current) {
			out = append(out, ' ')
		}
		out = append(out, current)
		previous = current
		pendingSpace = false
	}
	return strings.TrimSpace(string(out))
}

func ReconcileOverlap(previous, incoming string) string {
	left := NormalizeChinese(previous)
	right := NormalizeChinese(incoming)
	if left == "" {
		return right
	}
	if right == "" || strings.HasSuffix(left, right) {
		return left
	}
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	maxOverlap := len(leftRunes)
	if len(rightRunes) < maxOverlap {
		maxOverlap = len(rightRunes)
	}
	overlap := 0
	for size := maxOverlap; size > 0; size-- {
		matches := true
		for i := 0; i < size; i++ {
			if leftRunes[len(leftRunes)-size+i] != rightRunes[i] {
				matches = false
				break
			}
		}
		if matches {
			overlap = size
			break
		}
	}
	if overlap == len(rightRunes) {
		return left
	}
	return joinNormalized(left, string(rightRunes[overlap:]))
}

func Merge(parts ...string) string {
	var merged string
	for _, part := range parts {
		merged = ReconcileOverlap(merged, part)
	}
	return merged
}

func shouldInsertSpace(previous, current rune) bool {
	if previous == 0 || previous == ' ' {
		return false
	}
	if isCJK(previous) && isCJK(current) {
		return false
	}
	if isPunctuation(current) {
		return false
	}
	if isPunctuation(previous) {
		return !isCJK(current) && !isOpeningPunctuation(previous)
	}
	return true
}

func joinNormalized(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	if shouldInsertSpace(leftRunes[len(leftRunes)-1], rightRunes[0]) {
		return left + " " + right
	}
	return left + right
}

func isCJK(value rune) bool {
	return (value >= '\u3400' && value <= '\u4dbf') ||
		(value >= '\u4e00' && value <= '\u9fff') ||
		(value >= '\uf900' && value <= '\ufaff')
}

func isPunctuation(value rune) bool {
	return unicode.IsPunct(value) || unicode.IsSymbol(value)
}

func isOpeningPunctuation(value rune) bool {
	switch value {
	case '(', '[', '{', '"', '\'', '“', '‘':
		return true
	default:
		return false
	}
}
