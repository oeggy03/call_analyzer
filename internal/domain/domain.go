// Package domain contains the data contracts shared by the backend layers and
// the Wails bindings. The structs intentionally use JSON-friendly primitives
// and time.Time so Wails can generate stable TypeScript bindings for them.
package domain

import "time"

type Lesson struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type TranscriptSegment struct {
	ID              string    `json:"id"`
	LessonID        string    `json:"lessonId"`
	StartMS         int64     `json:"startMs"`
	EndMS           int64     `json:"endMs"`
	Text            string    `json:"text"`
	SimplifiedText  string    `json:"simplifiedText,omitempty"`
	TraditionalText string    `json:"traditionalText,omitempty"`
	Reading         string    `json:"reading,omitempty"`
	Source          string    `json:"source,omitempty"`
	Confidence      float64   `json:"confidence,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

type VocabularyStatus string

const (
	VocabularyStatusCandidate VocabularyStatus = "candidate"
	VocabularyStatusConfirmed VocabularyStatus = "confirmed"
	VocabularyStatusRejected  VocabularyStatus = "rejected"
	VocabularyStatusMerged    VocabularyStatus = "merged"
)

type Provenance string

const (
	ProvenanceUser       Provenance = "user"
	ProvenanceModel      Provenance = "model"
	ProvenanceDictionary Provenance = "dictionary"
	ProvenanceImported   Provenance = "imported"
)

type Evidence struct {
	ID         string  `json:"id,omitempty"`
	LessonID   string  `json:"lessonId,omitempty"`
	SegmentID  string  `json:"segmentId,omitempty"`
	Text       string  `json:"text"`
	StartMS    int64   `json:"startMs,omitempty"`
	EndMS      int64   `json:"endMs,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type Sense struct {
	ID           string `json:"id"`
	EntryID      string `json:"entryId"`
	Gloss        string `json:"gloss"`
	PartOfSpeech string `json:"partOfSpeech,omitempty"`
	Classifier   string `json:"classifier,omitempty"`
	SortOrder    int    `json:"sortOrder"`
}

type Example struct {
	ID          string     `json:"id"`
	EntryID     string     `json:"entryId"`
	Simplified  string     `json:"simplified"`
	Traditional string     `json:"traditional,omitempty"`
	Reading     string     `json:"reading,omitempty"`
	Translation string     `json:"translation,omitempty"`
	Provenance  Provenance `json:"provenance"`
	Generated   bool       `json:"generated"`
}

type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type VocabularyEntry struct {
	ID           string           `json:"id"`
	Simplified   string           `json:"simplified"`
	Traditional  string           `json:"traditional"`
	Reading      string           `json:"reading"`
	MarkedPinyin string           `json:"markedPinyin,omitempty"`
	Status       VocabularyStatus `json:"status"`
	Provenance   Provenance       `json:"provenance"`
	Confidence   float64          `json:"confidence"`
	TeachingCue  float64          `json:"teachingCue"`
	Manual       bool             `json:"manual"`
	Priority     string           `json:"priority,omitempty"`
	Evidence     []Evidence       `json:"evidence,omitempty"`
	Senses       []Sense          `json:"senses,omitempty"`
	Examples     []Example        `json:"examples,omitempty"`
	Tags         []Tag            `json:"tags,omitempty"`
	MergedIntoID string           `json:"mergedIntoId,omitempty"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

// VocabularyCandidate is an alias so candidate and confirmed records share
// one wire shape while status determines the lifecycle state.
type VocabularyCandidate = VocabularyEntry

type CandidateEdit struct {
	Simplified         *string   `json:"simplified,omitempty"`
	Traditional        *string   `json:"traditional,omitempty"`
	Reading            *string   `json:"reading,omitempty"`
	MarkedPinyin       *string   `json:"markedPinyin,omitempty"`
	Confidence         *float64  `json:"confidence,omitempty"`
	TeachingCue        *float64  `json:"teachingCue,omitempty"`
	Meaning            *string   `json:"meaning,omitempty"`
	PartOfSpeech       *string   `json:"partOfSpeech,omitempty"`
	Classifier         *string   `json:"classifier,omitempty"`
	Example            *string   `json:"example,omitempty"`
	ExampleTranslation *string   `json:"exampleTranslation,omitempty"`
	Tags               *[]string `json:"tags,omitempty"`
}

type Observation struct {
	ID           string    `json:"id"`
	LessonID     string    `json:"lessonId"`
	SegmentID    string    `json:"segmentId,omitempty"`
	EntryID      string    `json:"entryId"`
	EvidenceText string    `json:"evidenceText"`
	StartMS      int64     `json:"startMs,omitempty"`
	EndMS        int64     `json:"endMs,omitempty"`
	Confidence   float64   `json:"confidence,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type StudyState struct {
	EntryID        string     `json:"entryId"`
	DueAt          *time.Time `json:"dueAt,omitempty"`
	IntervalDays   float64    `json:"intervalDays"`
	Ease           float64    `json:"ease"`
	Repetitions    int        `json:"repetitions"`
	Lapses         int        `json:"lapses"`
	LastReviewedAt *time.Time `json:"lastReviewedAt,omitempty"`
}

type ReviewEvent struct {
	ID         string    `json:"id"`
	EntryID    string    `json:"entryId"`
	Rating     int       `json:"rating"`
	ReviewedAt time.Time `json:"reviewedAt"`
	Metadata   string    `json:"metadata,omitempty"`
}

type OutboxEvent struct {
	ID             string     `json:"id"`
	EventType      string     `json:"eventType"`
	AggregateType  string     `json:"aggregateType"`
	AggregateID    string     `json:"aggregateId"`
	PayloadJSON    string     `json:"payloadJson"`
	IdempotencyKey string     `json:"idempotencyKey"`
	CreatedAt      time.Time  `json:"createdAt"`
	DeliveredAt    *time.Time `json:"deliveredAt,omitempty"`
	Attempts       int        `json:"attempts"`
	LastError      string     `json:"lastError,omitempty"`
}

type Setting struct {
	Key       string    `json:"key"`
	ValueJSON string    `json:"valueJson"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RequestUsage struct {
	ID               string    `json:"id"`
	SessionID        string    `json:"sessionId"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	Endpoint         string    `json:"endpoint"`
	PromptTokens     int       `json:"promptTokens"`
	CompletionTokens int       `json:"completionTokens"`
	TotalTokens      int       `json:"totalTokens"`
	Cost             float64   `json:"cost"`
	CreatedAt        time.Time `json:"createdAt"`
}

type Health struct {
	OK       bool   `json:"ok"`
	Database string `json:"database"`
	Capture  string `json:"capture"`
	Version  string `json:"version"`
}

type ConfigView struct {
	DatabasePath string `json:"databasePath"`
	ASRModel     string `json:"asrModel"`
	ChatModel    string `json:"chatModel"`
	SyncEnabled  bool   `json:"syncEnabled"`
	HasAPIKey    bool   `json:"hasApiKey"`
}
