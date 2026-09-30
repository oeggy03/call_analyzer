package main

import "time"

type StartLessonInput struct {
	Target  string `json:"target"`
	Consent bool   `json:"consent"`
}

type ManualVocabularyInput struct {
	Simplified         string   `json:"simplified"`
	Traditional        string   `json:"traditional,omitempty"`
	Pinyin             string   `json:"pinyin,omitempty"`
	Meaning            string   `json:"meaning,omitempty"`
	PartOfSpeech       string   `json:"partOfSpeech,omitempty"`
	Classifier         string   `json:"classifier,omitempty"`
	Example            string   `json:"example,omitempty"`
	ExamplePinyin      string   `json:"examplePinyin,omitempty"`
	ExampleTranslation string   `json:"exampleTranslation,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	AiGenerated        bool     `json:"aiGenerated,omitempty"`
}

type ManualVocabularyDraft struct {
	Simplified         string   `json:"simplified"`
	Traditional        string   `json:"traditional,omitempty"`
	Pinyin             string   `json:"pinyin,omitempty"`
	Meaning            string   `json:"meaning,omitempty"`
	PartOfSpeech       string   `json:"partOfSpeech,omitempty"`
	Classifier         string   `json:"classifier,omitempty"`
	Example            string   `json:"example,omitempty"`
	ExamplePinyin      string   `json:"examplePinyin,omitempty"`
	ExampleTranslation string   `json:"exampleTranslation,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	AiGenerated        bool     `json:"aiGenerated,omitempty"`
	Model              string   `json:"model"`
	Cost               float64  `json:"cost"`
}

type CandidateEditPatch struct {
	Simplified         *string   `json:"simplified,omitempty"`
	Traditional        *string   `json:"traditional,omitempty"`
	Pinyin             *string   `json:"pinyin,omitempty"`
	Meaning            *string   `json:"meaning,omitempty"`
	PartOfSpeech       *string   `json:"partOfSpeech,omitempty"`
	Classifier         *string   `json:"classifier,omitempty"`
	Example            *string   `json:"example,omitempty"`
	ExamplePinyin      *string   `json:"examplePinyin,omitempty"`
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
	Readiness         ReadinessSnapshot    `json:"readiness"`
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

type ReadinessSnapshot struct {
	Checked         bool   `json:"checked"`
	TargetAvailable bool   `json:"targetAvailable"`
	Error           string `json:"error,omitempty"`
	CheckedAt       string `json:"checkedAt,omitempty"`
}

type LessonSnapshot struct {
	ID        string `json:"id"`
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
	ExamplePinyin      string   `json:"examplePinyin"`
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
	ID                 string   `json:"id"`
	Simplified         string   `json:"simplified"`
	Traditional        string   `json:"traditional,omitempty"`
	Pinyin             string   `json:"pinyin"`
	Meaning            string   `json:"meaning"`
	PartOfSpeech       string   `json:"partOfSpeech,omitempty"`
	Classifier         string   `json:"classifier,omitempty"`
	Example            string   `json:"example"`
	ExamplePinyin      string   `json:"examplePinyin"`
	ExampleTranslation string   `json:"exampleTranslation"`
	Status             string   `json:"status"`
	Tags               []string `json:"tags"`
	LastSeen           string   `json:"lastSeen"`
	SeenCount          int      `json:"seenCount"`
}

type CostSummary struct {
	CurrentUSD    float64 `json:"currentUsd"`
	ProjectedUSD  float64 `json:"projectedUsd"`
	HardBudgetUSD float64 `json:"hardBudgetUsd"`
	Warning       bool    `json:"warning"`
	HardExceeded  bool    `json:"hardExceeded"`
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
