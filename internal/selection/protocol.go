package selection

type Bounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type GlossLine struct {
	Surface            string   `json:"surface"`
	Simplified         string   `json:"simplified"`
	Traditional        string   `json:"traditional,omitempty"`
	Pinyin             string   `json:"pinyin"`
	Meaning            string   `json:"meaning"`
	PartOfSpeech       string   `json:"partOfSpeech,omitempty"`
	Classifier         string   `json:"classifier,omitempty"`
	Example            string   `json:"example,omitempty"`
	ExamplePinyin      string   `json:"examplePinyin,omitempty"`
	ExampleTranslation string   `json:"exampleTranslation,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	VocabularyID       string   `json:"vocabularyId,omitempty"`
	Action             string   `json:"action"`
	Destination        string   `json:"destination"`
}

// CardFields is the vocabulary card edited and saved from the tooltip.
type CardFields struct {
	Simplified         string   `json:"simplified"`
	Traditional        string   `json:"traditional,omitempty"`
	Pinyin             string   `json:"pinyin"`
	Meaning            string   `json:"meaning"`
	PartOfSpeech       string   `json:"partOfSpeech,omitempty"`
	Classifier         string   `json:"classifier,omitempty"`
	Example            string   `json:"example,omitempty"`
	ExamplePinyin      string   `json:"examplePinyin,omitempty"`
	ExampleTranslation string   `json:"exampleTranslation,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	AIGenerated        bool     `json:"aiGenerated,omitempty"`
}

// CardVariant is one generated reading the tooltip can apply to the card.
type CardVariant struct {
	Traditional        string `json:"traditional,omitempty"`
	Pinyin             string `json:"pinyin"`
	Meaning            string `json:"meaning"`
	PartOfSpeech       string `json:"partOfSpeech,omitempty"`
	Classifier         string `json:"classifier,omitempty"`
	Example            string `json:"example,omitempty"`
	ExamplePinyin      string `json:"examplePinyin,omitempty"`
	ExampleTranslation string `json:"exampleTranslation,omitempty"`
}

// CardDraft is the generated or saved card shown inside the tooltip.
type CardDraft struct {
	Fields      CardFields    `json:"fields"`
	NeedsChoice bool          `json:"needsChoice"`
	ReadOnly    bool          `json:"readOnly"`
	Variants    []CardVariant `json:"variants,omitempty"`
}

type LookupResult struct {
	Surface      string      `json:"surface"`
	Lines        []GlossLine `json:"lines,omitempty"`
	Notice       string      `json:"notice,omitempty"`
	ExistingTags []string    `json:"existingTags,omitempty"`
}
