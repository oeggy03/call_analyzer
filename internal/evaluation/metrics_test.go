package evaluation

import "testing"

func TestCharacterErrorRateMandarin(t *testing.T) {
	if got := CharacterErrorRate("这个方案 很常见。", "这个方案很常见"); got != 0 {
		t.Fatalf("expected punctuation-insensitive match, got %f", got)
	}
	if got := CharacterErrorRate("你好", "你号"); got != 0.5 {
		t.Fatalf("expected one of two characters wrong, got %f", got)
	}
}

func TestScoreWords(t *testing.T) {
	score := ScoreWords([]string{"方案", "常见"}, []string{"方案", "重点"})
	if score.Precision != 0.5 || score.Recall != 0.5 || score.Matched != 1 {
		t.Fatalf("unexpected score: %+v", score)
	}
	abstention := ScoreWords(nil, nil)
	if abstention.Precision != 1 || abstention.Recall != 1 {
		t.Fatalf("correct abstention should score perfectly: %+v", abstention)
	}
}
