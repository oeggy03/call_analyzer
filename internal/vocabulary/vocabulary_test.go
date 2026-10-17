package vocabulary

import (
	"context"
	"errors"
	"testing"

	"github.com/oeggy03/call_analyzer/internal/domain"
)

func TestCanonicalPinyin(t *testing.T) {
	numbered, marked, err := CanonicalPinyin("ni3 hao3")
	if err != nil {
		t.Fatal(err)
	}
	if numbered != "ni3 hao3" || marked != "nǐ hǎo" {
		t.Fatalf("got numbered=%q marked=%q", numbered, marked)
	}
	numbered, marked, err = CanonicalPinyin("lü4")
	if err != nil {
		t.Fatal(err)
	}
	if numbered != "lü4" || marked != "lǜ" {
		t.Fatalf("got numbered=%q marked=%q", numbered, marked)
	}
	numbered, marked, err = CanonicalPinyin("Lao3 shi1, ni3 hao3!")
	if err != nil {
		t.Fatal(err)
	}
	if numbered != "lao3 shi1 ni3 hao3" || marked != "lǎo shī nǐ hǎo" {
		t.Fatalf("sentence punctuation was not normalized: numbered=%q marked=%q", numbered, marked)
	}
}

func TestNormalizeRequiresEvidence(t *testing.T) {
	_, err := NormalizeCandidate(Candidate{
		Simplified: "你好",
		Reading:    "ni3 hao3",
	}, "你好")
	if !errors.Is(err, ErrMissingEvidence) {
		t.Fatalf("expected missing evidence error, got %v", err)
	}
	_, err = NormalizeCandidate(Candidate{
		Simplified: "你好",
		Reading:    "ni3 hao3",
		Evidence:   []domain.Evidence{{Text: "再见"}},
	}, "你好")
	if err == nil {
		t.Fatal("expected evidence mismatch")
	}
}

func TestDeduplicateUsesFormsAndReading(t *testing.T) {
	candidates := []Candidate{
		{Simplified: "行", Traditional: "行", Reading: "xing2", Evidence: []domain.Evidence{{Text: "行"}}, Confidence: .5},
		{Simplified: "行", Traditional: "行", Reading: "xing2", Evidence: []domain.Evidence{{Text: "行"}}, Confidence: .9},
		{Simplified: "形", Traditional: "形", Reading: "xing2", Evidence: []domain.Evidence{{Text: "形"}}, Confidence: .9},
	}
	got, err := Deduplicate(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Confidence != .9 {
		t.Fatalf("unexpected dedup result %#v", got)
	}
}

func TestNormalizeCanonicalizesRequiredExamplePinyin(t *testing.T) {
	entry, err := NormalizeCandidate(Candidate{
		Simplified: "你好",
		Reading:    "ni3 hao3",
		Evidence:   []domain.Evidence{{Text: "你好"}},
		Confidence: .9,
		Examples: []domain.Example{{
			Simplified:  "老师，你好！",
			Reading:     "lao3 shi1, ni3 hao3!",
			Translation: "Hello, teacher!",
		}},
	}, "你好")
	if err != nil {
		t.Fatal(err)
	}
	if got := entry.Examples[0].Reading; got != "lao3 shi1 ni3 hao3" {
		t.Fatalf("example reading was not canonicalized: %q", got)
	}
}

func TestEmbeddedDictionaryAndGeneratedExample(t *testing.T) {
	dictionary := NewEmbeddedDictionary()
	senses, err := dictionary.Lookup(context.Background(), "你好", "你好")
	if err != nil {
		t.Fatal(err)
	}
	if len(senses) != 1 || senses[0].Gloss == "" {
		t.Fatalf("unexpected dictionary result %#v", senses)
	}
	example := GeneratedExample("你好", "你好", "ni3 hao3", "hello")
	if !example.Generated || example.Provenance != domain.ProvenanceModel {
		t.Fatalf("generated example was not marked: %#v", example)
	}
}
