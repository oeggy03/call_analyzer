package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oeggy03/call_analyzer/internal/domain"
)

func TestSQLiteRepositoriesAndLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "call_analyzer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var foreignKeys int
	if err := store.DB().QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign keys are disabled: %d", foreignKeys)
	}
	var journalMode string
	if err := store.DB().QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" && journalMode != "memory" {
		t.Fatalf("unexpected journal mode %q", journalMode)
	}
	var busyTimeout int
	if err := store.DB().QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("busy timeout is too short: %d", busyTimeout)
	}

	lesson, err := store.Lessons().Start(ctx, "test lesson", timeAt(1000))
	if err != nil {
		t.Fatal(err)
	}
	segment, err := store.Transcripts().Insert(ctx, domain.TranscriptSegment{
		LessonID:   lesson.ID,
		StartMS:    0,
		EndMS:      1000,
		Text:       "你好",
		Confidence: .9,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified:  "你好",
		Traditional: "你好",
		Reading:     "ni3 hao3",
		Confidence:  .8,
		Evidence: []domain.Evidence{{
			LessonID:  lesson.ID,
			SegmentID: segment.ID,
			Text:      "老师说你好",
		}},
		Senses: []domain.Sense{{Gloss: "hello", SortOrder: 0}},
		Examples: []domain.Example{{
			Simplified: "你好",
			Generated:  true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != domain.VocabularyStatusCandidate ||
		len(candidate.Evidence) != 1 || len(candidate.Senses) != 1 || len(candidate.Examples) != 1 {
		t.Fatalf("unexpected candidate %#v", candidate)
	}
	if candidate.Examples[0].Provenance != domain.ProvenanceModel {
		t.Fatalf("generated example provenance was not normalized: %#v", candidate.Examples[0])
	}

	duplicate, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified:  "你好",
		Traditional: "你好",
		Reading:     "ni3 hao3",
		Confidence:  .95,
	})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != candidate.ID {
		t.Fatalf("deduplication created a second entry: %s != %s", duplicate.ID, candidate.ID)
	}
	candidates, err := store.Vocabulary().ListCandidates(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(candidates))
	}

	if _, err := store.Vocabulary().Confirm(ctx, candidate.ID); err != nil {
		t.Fatal(err)
	}
	vocabulary, err := store.Vocabulary().ListVocabulary(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(vocabulary) != 1 || vocabulary[0].ID != candidate.ID {
		t.Fatalf("unexpected confirmed entries %#v", vocabulary)
	}
	var studyState int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM study_state WHERE entry_id = ?", candidate.ID,
	).Scan(&studyState); err != nil {
		t.Fatal(err)
	}
	if studyState != 1 {
		t.Fatalf("expected study state, got %d", studyState)
	}
	review, state, err := store.Study().RecordReview(ctx, candidate.ID, 5, timeAt(3000), "good")
	if err != nil {
		t.Fatal(err)
	}
	if review.EntryID != candidate.ID || state.Repetitions != 1 || state.DueAt == nil {
		t.Fatalf("unexpected review state event=%#v state=%#v", review, state)
	}

	pending, err := store.Outbox().ListPending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) < 2 {
		t.Fatalf("expected candidate and confirmation outbox events, got %d", len(pending))
	}
	if err := store.Outbox().MarkDelivered(ctx, pending[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Outbox().MarkFailed(ctx, pending[1].ID, errors.New("temporary")); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Lessons().End(ctx, lesson.ID, timeAt(2000)); err != nil {
		t.Fatal(err)
	}
	if err := store.Lessons().Delete(ctx, lesson.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transcripts().List(ctx, lesson.ID); err != nil {
		t.Fatal(err)
	}
	var observationCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM observations WHERE lesson_id = ?", lesson.ID,
	).Scan(&observationCount); err != nil {
		t.Fatal(err)
	}
	if observationCount != 0 {
		t.Fatalf("lesson delete did not cascade observations: %d", observationCount)
	}
	preserved, err := store.Vocabulary().Get(ctx, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preserved.Evidence) != 0 {
		t.Fatalf("confirmed vocabulary retained deleted lesson evidence: %#v", preserved.Evidence)
	}
	pending, err = store.Outbox().ListPending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range pending {
		if strings.Contains(event.PayloadJSON, "老师说你好") {
			t.Fatalf("outbox retained deleted lesson text: %s", event.PayloadJSON)
		}
	}
}

func TestLessonDeleteRemovesOrphanCandidate(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lesson, err := store.Lessons().Start(ctx, "private lesson", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	segment, err := store.Transcripts().Insert(ctx, domain.TranscriptSegment{
		LessonID: lesson.ID, EndMS: 1_000, Text: "秘密",
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified: "秘密", Traditional: "秘密", Reading: "mi4 mi4",
		Evidence: []domain.Evidence{{
			LessonID: lesson.ID, SegmentID: segment.ID, Text: "秘密",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Lessons().Delete(ctx, lesson.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Vocabulary().Get(ctx, candidate.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("orphan candidate survived lesson deletion: %v", err)
	}
}

func TestSettingsAndContextCancellation(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	setting, err := store.Settings().Set(ctx, "theme", `{"dark":true}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Settings().Get(ctx, setting.Key)
	if err != nil || got.ValueJSON != setting.ValueJSON {
		t.Fatalf("unexpected setting %#v err=%v", got, err)
	}
	usage, err := store.Usage().Record(ctx, domain.RequestUsage{
		SessionID: "session",
		Provider:  "openrouter",
		Model:     "model",
		Cost:      .12,
	})
	if err != nil || usage.ID == "" {
		t.Fatalf("unexpected usage %#v err=%v", usage, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Lessons().Start(cancelled, "", timeAt(0)); err == nil {
		t.Fatal("expected cancelled context error")
	}
	if _, err := store.Settings().Get(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing setting, got %v", err)
	}
}

func TestLessonRecoveryMetadataAndLatestComplete(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	orphan, err := store.Lessons().StartWithMetadata(ctx, "orphan", timeAt(1000), LessonMetadata{
		ConsentRecorded: true,
		Target:          "zoom",
		STTModel:        "asr",
		AnalyzerModel:   "chat",
		RetentionPolicy: "sessionOnly",
	})
	if err != nil {
		t.Fatal(err)
	}
	latest, err := store.Lessons().Start(ctx, "latest", timeAt(2000))
	if err != nil {
		t.Fatal(err)
	}
	recoveredAt := timeAt(3000)
	count, err := store.Lessons().EndOrphaned(ctx, recoveredAt)
	if err != nil || count != 2 {
		t.Fatalf("unexpected orphan recovery count=%d err=%v", count, err)
	}
	recovered, err := store.Lessons().Get(ctx, orphan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.EndedAt == nil || !recovered.EndedAt.Equal(recoveredAt) ||
		!recovered.ConsentRecorded || recovered.Target != "zoom" ||
		recovered.STTModel != "asr" || recovered.AnalyzerModel != "chat" {
		t.Fatalf("recovery metadata was not preserved: %#v", recovered)
	}
	ended, err := store.Lessons().EndWithCost(ctx, latest.ID, timeAt(4000), .42)
	if err != nil {
		t.Fatal(err)
	}
	if ended.FinalCost != .42 {
		t.Fatalf("final lesson cost was not persisted: %#v", ended)
	}
	gotLatest, err := store.Lessons().LatestComplete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if gotLatest.ID != latest.ID {
		t.Fatalf("latest complete lesson = %s, want %s", gotLatest.ID, latest.ID)
	}
}

func TestCandidateEditRejectAndMerge(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	source, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified:  "学习",
		Traditional: "學習",
		Reading:     "xue2 xi2",
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified:  "學習",
		Traditional: "學習",
		Reading:     "xue2 xi2",
	})
	if err != nil {
		t.Fatal(err)
	}
	confidence := .77
	edited, err := store.Vocabulary().Edit(ctx, source.ID, domain.CandidateEdit{Confidence: &confidence})
	if err != nil || edited.Confidence != confidence {
		t.Fatalf("unexpected edit %#v err=%v", edited, err)
	}
	example := "我学习中文。"
	examplePinyin := "wo3 xue2 xi2 zhong1 wen2"
	translation := "I study Chinese."
	edited, err = store.Vocabulary().Edit(ctx, source.ID, domain.CandidateEdit{
		Example:            &example,
		ExamplePinyin:      &examplePinyin,
		ExampleTranslation: &translation,
	})
	if err != nil || len(edited.Examples) != 1 ||
		edited.Examples[0].Reading != examplePinyin ||
		edited.Examples[0].Translation != translation {
		t.Fatalf("example edit did not persist pinyin: %#v err=%v", edited, err)
	}
	merged, err := store.Vocabulary().Merge(ctx, target.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.ID != source.ID {
		t.Fatalf("merge returned wrong target %#v", merged)
	}
	sourceAfter, err := store.Vocabulary().Get(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sourceAfter.Status != domain.VocabularyStatusMerged || sourceAfter.MergedIntoID != source.ID {
		t.Fatalf("source was not safely merged %#v", sourceAfter)
	}
	rejected, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified: "再见",
		Reading:    "zai4 jian4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Vocabulary().Reject(ctx, rejected.ID); err != nil {
		t.Fatal(err)
	}
	rejectedAfter, err := store.Vocabulary().Get(ctx, rejected.ID)
	if err != nil || rejectedAfter.Status != domain.VocabularyStatusRejected {
		t.Fatalf("unexpected rejected entry %#v err=%v", rejectedAfter, err)
	}
	repeated, err := store.Vocabulary().UpsertCandidate(ctx, domain.VocabularyEntry{
		Simplified: "再见",
		Reading:    "zai4 jian4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Status != domain.VocabularyStatusRejected {
		t.Fatalf("automatic extraction reopened rejected entry: %#v", repeated)
	}
}

func timeAt(milliseconds int64) (value time.Time) {
	return time.UnixMilli(milliseconds).UTC()
}
