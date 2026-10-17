package chinese

import "testing"

func TestToSimplifiedConvertsTraditional(t *testing.T) {
	got := ToSimplified("我們下週學習這個詞彙。")
	if got != "我们下周学习这个词汇。" {
		t.Fatalf("simplified form = %q", got)
	}
	if got := ToSimplified("我们下周学习。"); got != "我们下周学习。" {
		t.Fatalf("simplified text changed: %q", got)
	}
	if got := ToSimplified("Marked moment"); got != "Marked moment" {
		t.Fatalf("non-Chinese text changed: %q", got)
	}
}

func TestSentencePinyin(t *testing.T) {
	got := SentencePinyin("我们下周可以把这个方案再讨论一下。")
	if got != "wǒ men xià zhōu kě yǐ bǎ zhè gè fāng àn zài tǎo lùn yī xià。" {
		t.Fatalf("pinyin = %q", got)
	}
	if got := SentencePinyin("Marked moment"); got != "" {
		t.Fatalf("non-Chinese pinyin = %q", got)
	}
	if got := SentencePinyin("好的，我先整理一下重点。"); got != "hǎo de， wǒ xiān zhěng lǐ yī xià zhòng diǎn。" {
		t.Fatalf("punctuated pinyin = %q", got)
	}
}
