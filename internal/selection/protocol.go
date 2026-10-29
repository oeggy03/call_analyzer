package selection

type Bounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type GlossLine struct {
	Surface      string `json:"surface"`
	Simplified   string `json:"simplified"`
	Traditional  string `json:"traditional,omitempty"`
	Pinyin       string `json:"pinyin"`
	Meaning      string `json:"meaning"`
	VocabularyID string `json:"vocabularyId,omitempty"`
	Action       string `json:"action"`
	Destination  string `json:"destination"`
}

type LookupResult struct {
	Surface string      `json:"surface"`
	Lines   []GlossLine `json:"lines,omitempty"`
	Notice  string      `json:"notice,omitempty"`
}
