package main

import "time"

type StartLessonInput struct {
	Target  string `json:"target"`
	Consent bool   `json:"consent"`
}

type CandidateEditPatch struct {
	Simplified         *string   `json:"simplified,omitempty"`
	Traditional        *string   `json:"traditional,omitempty"`
	Pinyin             *string   `json:"pinyin,omitempty"`
	Meaning            *string   `json:"meaning,omitempty"`
	PartOfSpeech       *string   `json:"partOfSpeech,omitempty"`
	Classifier         *string   `json:"classifier,omitempty"`
	Example            *string   `json:"example,omitempty"`
	ExampleTranslation *string   `json:"exampleTranslation,omitempty"`
	Tags               *[]string `json:"tags,omitempty"`
}

type SettingsPatch struct {
	OpenRouterKey     *string  `json:"openRouterKey,omitempty"`
	STTModel          *string  `json:"sttModel,omitempty"`
	AnalyzerModel     *string  `json:"analyzerModel,omitempty"`
	HardBudgetUSD     *float64 `json:"hardBudgetUsd,omitempty"`
	AudioRetention    *string  `json:"audioRetention,omitempty"`
	OCREnabled        *bool    `json:"ocrEnabled,omitempty"`
	MicrophoneEnabled *bool    `json:"microphoneEnabled,omitempty"`
}

type AppSnapshot struct {
	Connection        string               `json:"connection"`
	CapturePermission string               `json:"capturePermission"`
	Target            string               `json:"target"`
	Lesson            LessonSnapshot       `json:"lesson"`
	Levels            AudioLevelsSnapshot  `json:"levels"`
	Transcript        []TranscriptLine     `json:"transcript"`
	Candidates        []CandidateSnapshot  `json:"candidates"`
	Vocabulary        []VocabularySnapshot `json:"vocabulary"`
	Cost              CostSummary          `json:"cost"`
	Privacy           PrivacyState         `json:"privacy"`
	Settings          SettingsSnapshot     `json:"settings"`
	LastUpdated       string               `json:"lastUpdated"`
}

type LessonSnapshot struct {
	Status    string `json:"status"`
	StartedAt string `json:"startedAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

type AudioLevelsSnapshot struct {
	Mic    float64 `json:"mic"`
	Remote float64 `json:"remote"`
}

type TranscriptLine struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
	Speaker   string `json:"speaker,omitempty"`
	IsMoment  bool   `json:"isMoment,omitempty"`
}

type CandidateSnapshot struct {
	ID                 string   `json:"id"`
	Simplified         string   `json:"simplified"`
	Traditional        string   `json:"traditional,omitempty"`
	Pinyin             string   `json:"pinyin"`
	Meaning            string   `json:"meaning"`
	PartOfSpeech       string   `json:"partOfSpeech,omitempty"`
	Classifier         string   `json:"classifier,omitempty"`
	Example            string   `json:"example"`
	ExampleTranslation string   `json:"exampleTranslation"`
	Provenance         string   `json:"provenance"`
	Confidence         float64  `json:"confidence"`
	Evidence           string   `json:"evidence"`
	Timestamp          string   `json:"timestamp"`
	Bucket             string   `json:"bucket"`
	Status             string   `json:"status"`
	DuplicateOf        string   `json:"duplicateOf,omitempty"`
	Tags               []string `json:"tags"`
}

type VocabularySnapshot struct {
	ID           string   `json:"id"`
	Simplified   string   `json:"simplified"`
	Traditional  string   `json:"traditional,omitempty"`
	Pinyin       string   `json:"pinyin"`
	Meaning      string   `json:"meaning"`
	PartOfSpeech string   `json:"partOfSpeech,omitempty"`
	Classifier   string   `json:"classifier,omitempty"`
	Status       string   `json:"status"`
	Tags         []string `json:"tags"`
	LastSeen     string   `json:"lastSeen"`
	SeenCount    int      `json:"seenCount"`
}

type CostSummary struct {
	CurrentUSD    float64 `json:"currentUsd"`
	ProjectedUSD  float64 `json:"projectedUsd"`
	HardBudgetUSD float64 `json:"hardBudgetUsd"`
	Currency      string  `json:"currency"`
}

type PrivacyState struct {
	ZDREnabled     bool   `json:"zdrEnabled"`
	AudioRetention string `json:"audioRetention"`
}

type SettingsSnapshot struct {
	OpenRouterKeyConfigured bool    `json:"openRouterKeyConfigured"`
	STTModel                string  `json:"sttModel"`
	AnalyzerModel           string  `json:"analyzerModel"`
	HardBudgetUSD           float64 `json:"hardBudgetUsd"`
	AudioRetention          string  `json:"audioRetention"`
	OCREnabled              bool    `json:"ocrEnabled"`
	MicrophoneEnabled       bool    `json:"microphoneEnabled"`
}

func timestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
