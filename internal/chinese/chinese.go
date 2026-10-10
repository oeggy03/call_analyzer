// Package chinese converts Mandarin text into the forms this app shows
// learners: simplified characters and spaced, tone-marked pinyin.
package chinese

import (
	"strings"
	"sync"
	"unicode"

	"github.com/longbridgeapp/opencc"
	"github.com/mozillazg/go-pinyin"
)

var (
	converterOnce sync.Once
	converter     *opencc.OpenCC
	converterErr  error
)

// ToSimplified returns the simplified-character form of text. Text that is
// already simplified is unchanged. Conversion failure leaves the original
// text in place so a transcript line is never dropped.
func ToSimplified(text string) string {
	if strings.TrimSpace(text) == "" || !ContainsHan(text) {
		return text
	}
	converterOnce.Do(func() {
		converter, converterErr = opencc.New("t2s")
	})
	if converterErr != nil || converter == nil {
		return text
	}
	converted, err := converter.Convert(text)
	if err != nil || strings.TrimSpace(converted) == "" {
		return text
	}
	return converted
}

// SentencePinyin returns spaced Hanyu Pinyin with tone marks for the Han
// characters in text. Punctuation stays attached to the preceding syllable.
// Text without Han characters returns an empty string.
func SentencePinyin(text string) string {
	if !ContainsHan(text) {
		return ""
	}
	args := pinyin.NewArgs()
	args.Style = pinyin.Tone
	args.Fallback = func(r rune, _ pinyin.Args) []string {
		return []string{string(r)}
	}
	var b strings.Builder
	var latin strings.Builder
	flushLatin := func() {
		if latin.Len() == 0 {
			return
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(latin.String())
		latin.Reset()
	}
	for _, group := range pinyin.Pinyin(text, args) {
		if len(group) == 0 {
			continue
		}
		token := group[0]
		if strings.TrimSpace(token) == "" {
			flushLatin()
			continue
		}
		runes := []rune(token)
		if len(runes) == 1 && (unicode.IsPunct(runes[0]) || unicode.IsSymbol(runes[0])) {
			flushLatin()
			b.WriteString(token)
			continue
		}
		if len(runes) == 1 && runes[0] <= unicode.MaxASCII && (unicode.IsLetter(runes[0]) || unicode.IsDigit(runes[0])) {
			latin.WriteString(token)
			continue
		}
		flushLatin()
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(token)
	}
	flushLatin()
	return strings.TrimSpace(b.String())
}

func ContainsHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
