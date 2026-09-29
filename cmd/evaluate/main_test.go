package main

import (
	"testing"

	"github.com/oeggy03/call_analyzer/internal/evaluation"
)

func TestSummarizeUsesMicroAveragesAndAudioOnlyCER(t *testing.T) {
	summary := summarize([]caseResult{
		{
			CharacterError: 0.20,
			AudioEvaluated: true,
			VocabularyScore: evaluation.SetScore{
				Matched: 2, Expected: 2, Predicted: 2,
			},
			CostUSD:   0.01,
			LatencyMS: 100,
		},
		{
			CharacterError: 0,
			AudioEvaluated: false,
			VocabularyScore: evaluation.SetScore{
				Matched: 0, Expected: 0, Predicted: 1,
			},
			CostUSD:   0.02,
			LatencyMS: 300,
		},
	})
	if summary.AudioCases != 1 || summary.CharacterErrorRate != 0.20 {
		t.Fatalf("unexpected audio summary: %+v", summary)
	}
	if summary.VocabularyPrecision != 2.0/3.0 || summary.VocabularyRecall != 1 {
		t.Fatalf("unexpected vocabulary summary: %+v", summary)
	}
	if summary.P95LatencyMS != 300 || summary.TotalCostUSD != 0.03 {
		t.Fatalf("unexpected cost/latency summary: %+v", summary)
	}
	if summary.MeetsVocabularyTargets {
		t.Fatalf("low precision should fail acceptance target: %+v", summary)
	}
}
