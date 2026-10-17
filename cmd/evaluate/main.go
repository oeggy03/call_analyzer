package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oeggy03/call_analyzer/internal/evaluation"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/service"
	"github.com/oeggy03/call_analyzer/internal/vocabulary"
)

type manifest struct {
	Cases []fixture `json:"cases"`
}

type fixture struct {
	Name               string   `json:"name"`
	AudioPath          string   `json:"audioPath,omitempty"`
	ExpectedTranscript string   `json:"expectedTranscript"`
	ExpectedWords      []string `json:"expectedWords"`
}

type caseResult struct {
	Name            string              `json:"name"`
	Transcript      string              `json:"transcript"`
	CharacterError  float64             `json:"characterErrorRate"`
	Vocabulary      []string            `json:"vocabulary"`
	VocabularyScore evaluation.SetScore `json:"vocabularyScore"`
	CostUSD         float64             `json:"costUsd"`
	LatencyMS       int64               `json:"latencyMs"`
	AudioEvaluated  bool                `json:"audioEvaluated"`
}

type report struct {
	Cases   []caseResult  `json:"cases"`
	Summary reportSummary `json:"summary"`
}

type reportSummary struct {
	Cases                  int     `json:"cases"`
	AudioCases             int     `json:"audioCases"`
	CharacterErrorRate     float64 `json:"characterErrorRate,omitempty"`
	VocabularyPrecision    float64 `json:"vocabularyPrecision"`
	VocabularyRecall       float64 `json:"vocabularyRecall"`
	TotalCostUSD           float64 `json:"totalCostUsd"`
	P95LatencyMS           int64   `json:"p95LatencyMs"`
	MeetsVocabularyTargets bool    `json:"meetsVocabularyTargets"`
}

type envKey struct{}

func (envKey) APIKey(context.Context) (string, error) {
	value := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if value == "" {
		return "", errors.New("OPENROUTER_API_KEY is not set")
	}
	return value, nil
}

func main() {
	manifestPath := flag.String("manifest", "fixtures/evaluation/manifest.json", "evaluation manifest")
	execute := flag.Bool("execute", false, "send requests to OpenRouter")
	asrModel := flag.String("asr-model", openrouter.DefaultASRModel, "OpenRouter transcription model")
	chatModel := flag.String("chat-model", openrouter.DefaultChatModel, "OpenRouter analyzer model")
	flag.Parse()

	data, err := os.ReadFile(*manifestPath)
	fatalIf(err)
	var suite manifest
	fatalIf(json.Unmarshal(data, &suite))
	if len(suite.Cases) == 0 {
		fatalIf(errors.New("manifest has no cases"))
	}
	baseDir := filepath.Dir(*manifestPath)
	for _, item := range suite.Cases {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.ExpectedTranscript) == "" {
			fatalIf(errors.New("every fixture needs a name and expectedTranscript"))
		}
		if item.AudioPath != "" {
			if _, err := os.Stat(filepath.Join(baseDir, item.AudioPath)); err != nil {
				fatalIf(fmt.Errorf("%s: %w", item.Name, err))
			}
		}
	}
	if !*execute {
		fmt.Printf("validated %d fixtures; pass -execute to make billable OpenRouter requests\n", len(suite.Cases))
		return
	}

	client, err := openrouter.NewClient(openrouter.Config{
		ASRModel:  *asrModel,
		ChatModel: *chatModel,
		Budget:    openrouter.NewBudget(0.35, 0.50),
	}, envKey{})
	fatalIf(err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	results := make([]caseResult, 0, len(suite.Cases))
	for _, item := range suite.Cases {
		started := time.Now()
		transcriptText := item.ExpectedTranscript
		cost := 0.0
		if item.AudioPath != "" {
			audio, err := os.ReadFile(filepath.Join(baseDir, item.AudioPath))
			fatalIf(err)
			response, err := client.Transcribe(ctx, audio, filepath.Base(item.AudioPath))
			fatalIf(err)
			transcriptText = response.Text
			cost += response.Usage.Cost
		}
		chat, err := client.ChatJSON(ctx, openrouter.ChatRequest{
			System:     "You are a conservative Mandarin teacher. Never invent evidence.",
			Prompt:     "Extract only explicitly taught Mandarin vocabulary from this transcript. Evidence must be an exact substring: " + transcriptText,
			SchemaName: "mandarin_vocabulary_evaluation",
			Schema:     vocabulary.ExtractionSchema(),
		})
		fatalIf(err)
		cost += chat.Usage.Cost
		extraction, err := service.ParseExtractionJSON(chat.Content)
		fatalIf(err)
		words := make([]string, 0, len(extraction.Candidates))
		for _, candidate := range extraction.Candidates {
			words = append(words, candidate.Simplified)
		}
		results = append(results, caseResult{
			Name:            item.Name,
			Transcript:      transcriptText,
			CharacterError:  evaluation.CharacterErrorRate(item.ExpectedTranscript, transcriptText),
			Vocabulary:      words,
			VocabularyScore: evaluation.ScoreWords(item.ExpectedWords, words),
			CostUSD:         cost,
			LatencyMS:       time.Since(started).Milliseconds(),
			AudioEvaluated:  item.AudioPath != "",
		})
	}
	output, err := json.MarshalIndent(report{
		Cases:   results,
		Summary: summarize(results),
	}, "", "  ")
	fatalIf(err)
	fmt.Println(string(output))
}

func summarize(results []caseResult) reportSummary {
	summary := reportSummary{Cases: len(results)}
	var (
		matched   int
		expected  int
		predicted int
		latencies = make([]int64, 0, len(results))
	)
	for _, result := range results {
		matched += result.VocabularyScore.Matched
		expected += result.VocabularyScore.Expected
		predicted += result.VocabularyScore.Predicted
		summary.TotalCostUSD += result.CostUSD
		latencies = append(latencies, result.LatencyMS)
		if result.AudioEvaluated {
			summary.AudioCases++
			summary.CharacterErrorRate += result.CharacterError
		}
	}
	if predicted == 0 && expected == 0 {
		summary.VocabularyPrecision = 1
		summary.VocabularyRecall = 1
	} else {
		if predicted > 0 {
			summary.VocabularyPrecision = float64(matched) / float64(predicted)
		}
		if expected > 0 {
			summary.VocabularyRecall = float64(matched) / float64(expected)
		}
	}
	if summary.AudioCases > 0 {
		summary.CharacterErrorRate /= float64(summary.AudioCases)
	}
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		index := (95*len(latencies) + 99) / 100
		summary.P95LatencyMS = latencies[index-1]
	}
	summary.MeetsVocabularyTargets =
		summary.VocabularyRecall >= 0.90 && summary.VocabularyPrecision >= 0.85
	return summary
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
