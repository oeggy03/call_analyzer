package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/selection"
	"github.com/oeggy03/call_analyzer/internal/service"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

const appEventPrefix = "call_analyzer:"

type App struct {
	mu              sync.RWMutex
	ctx             context.Context
	readiness       ReadinessSnapshot
	service         *service.Service
	store           *storage.Store
	selectionRunner *selection.Runner
}

func NewApp() (*App, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(configDir, "call_analyzer", "call_analyzer.db")
	store, err := storage.Open(context.Background(), dbPath)
	if err != nil {
		return nil, err
	}
	return NewAppWithDependencies(
		store,
		newPlatformSource(),
		service.NewDefaultSecretStore(),
		nil,
		service.NewSupabaseSecretStore(),
	), nil
}

func NewAppWithDependencies(
	store *storage.Store,
	source capture.Source,
	secrets service.SecretStore,
	router *openrouter.Client,
	syncSecrets ...service.SecretStore,
) *App {
	var syncSecretStore service.SecretStore
	if len(syncSecrets) > 0 {
		syncSecretStore = syncSecrets[0]
	}
	app := &App{
		store:           store,
		service:         service.NewWithSyncSecretStore(store, source, secrets, router, syncSecretStore),
		selectionRunner: selection.NewRunner(selection.Config{}),
	}
	app.service.SetEventHook(app.onServiceEvent)
	return app
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
	if err := a.service.Initialize(ctx); err != nil {
		a.emit("backend.error", err.Error())
		return
	}
	if a.selectionRunner != nil {
		if err := a.selectionRunner.Start(ctx, a.service); err != nil {
			a.emit("backend.error", err.Error())
		}
	}
	a.emit("backend.ready", a.service.Config(ctx))
	go a.checkSupabaseAccess()
}

func (a *App) beforeClose(ctx context.Context) bool {
	needsSync, err := a.service.HasUnsyncedVocabulary(ctx)
	if err != nil || !needsSync {
		return false
	}
	choice, dialogErr := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Unsynced vocabulary",
		Message:       "You still have unsynced items. Sync now?",
		Buttons:       []string{"Sync now", "Not now"},
		DefaultButton: "Sync now",
		CancelButton:  "Not now",
	})
	if dialogErr != nil {
		return true
	}
	if choice != "Sync now" {
		a.service.SkipShutdownSync()
		return false
	}
	syncCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.service.SyncNow(syncCtx); err != nil {
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Sync failed",
			Message: "The vocabulary sync did not finish. The app will stay open.",
		})
		return true
	}
	return false
}

func (a *App) shutdown(ctx context.Context) {
	if a.selectionRunner != nil {
		_ = a.selectionRunner.Stop(ctx)
	}
	_ = a.service.StopCapture(ctx)
	flushCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	_ = a.service.Shutdown(flushCtx)
	cancel()
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) context() context.Context {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (a *App) emit(name string, payload any) {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, appEventPrefix+name, payload)
	}
}

func (a *App) onServiceEvent(name string, payload any) {
	if name == "selection.add-word" {
		ctx := a.context()
		runtime.WindowUnminimise(ctx)
		runtime.WindowShow(ctx)
	}
	a.emit(name, payload)
	if name == "snapshot_changed" || name == "capture.levels" {
		return
	}
	if snapshot, err := a.GetAppSnapshot(); err == nil {
		a.emit("snapshot_changed", snapshot)
	}
}

func (a *App) GetAppSnapshot() (AppSnapshot, error) {
	return a.buildSnapshot(a.context())
}

func (a *App) GenerateManualVocabulary(input ManualVocabularyInput) (ManualVocabularyDraft, error) {
	draft, err := a.service.GenerateManualVocabulary(a.context(), service.ManualVocabularyInput{
		Simplified:         input.Simplified,
		Traditional:        input.Traditional,
		Pinyin:             input.Pinyin,
		Meaning:            input.Meaning,
		PartOfSpeech:       input.PartOfSpeech,
		Classifier:         input.Classifier,
		Example:            input.Example,
		ExamplePinyin:      input.ExamplePinyin,
		ExampleTranslation: input.ExampleTranslation,
		Tags:               input.Tags,
		AiGenerated:        input.AiGenerated,
	})
	if err != nil {
		return ManualVocabularyDraft{}, err
	}
	variants := make([]ManualVocabularyVariant, 0, len(draft.Variants))
	for _, variant := range draft.Variants {
		variants = append(variants, ManualVocabularyVariant{
			Traditional:        variant.Traditional,
			Pinyin:             variant.Pinyin,
			Meaning:            variant.Meaning,
			PartOfSpeech:       variant.PartOfSpeech,
			Classifier:         variant.Classifier,
			Example:            variant.Example,
			ExamplePinyin:      variant.ExamplePinyin,
			ExampleTranslation: variant.ExampleTranslation,
		})
	}
	return ManualVocabularyDraft{
		Simplified:         draft.Simplified,
		Traditional:        draft.Traditional,
		Pinyin:             draft.Pinyin,
		Meaning:            draft.Meaning,
		PartOfSpeech:       draft.PartOfSpeech,
		Classifier:         draft.Classifier,
		Example:            draft.Example,
		ExamplePinyin:      draft.ExamplePinyin,
		ExampleTranslation: draft.ExampleTranslation,
		Tags:               draft.Tags,
		AiGenerated:        draft.AiGenerated,
		Model:              draft.Model,
		Cost:               draft.Cost,
		NeedsChoice:        draft.NeedsChoice,
		MatchedContext:     draft.MatchedContext,
		AppliedVariant:     draft.AppliedVariant,
		Variants:           variants,
	}, nil
}

func (a *App) RegenerateManualVocabulary(input RegenerateManualVocabularyInput) (ManualVocabularyDraft, error) {
	draft, err := a.service.RegenerateManualVocabulary(a.context(), service.RegenerateManualVocabularyInput{
		ManualVocabularyInput: service.ManualVocabularyInput{
			Simplified:         input.Simplified,
			Traditional:        input.Traditional,
			Pinyin:             input.Pinyin,
			Meaning:            input.Meaning,
			PartOfSpeech:       input.PartOfSpeech,
			Classifier:         input.Classifier,
			Example:            input.Example,
			ExamplePinyin:      input.ExamplePinyin,
			ExampleTranslation: input.ExampleTranslation,
			Tags:               input.Tags,
			AiGenerated:        input.AiGenerated,
		},
		Instruction: input.Instruction,
	})
	if err != nil {
		return ManualVocabularyDraft{}, err
	}
	return ManualVocabularyDraft{
		Simplified:         draft.Simplified,
		Traditional:        draft.Traditional,
		Pinyin:             draft.Pinyin,
		Meaning:            draft.Meaning,
		PartOfSpeech:       draft.PartOfSpeech,
		Classifier:         draft.Classifier,
		Example:            draft.Example,
		ExamplePinyin:      draft.ExamplePinyin,
		ExampleTranslation: draft.ExampleTranslation,
		Tags:               draft.Tags,
		AiGenerated:        draft.AiGenerated,
		Model:              draft.Model,
		Cost:               draft.Cost,
		NeedsChoice:        draft.NeedsChoice,
		MatchedContext:     draft.MatchedContext,
		AppliedVariant:     draft.AppliedVariant,
	}, nil
}

func (a *App) RecognizeVocabularyPhoto(imageBase64 string) (PhotoVocabularyDraft, error) {
	image, err := decodePhotoImageBase64(imageBase64)
	if err != nil {
		return PhotoVocabularyDraft{}, err
	}
	draft, err := a.service.RecognizeVocabularyPhoto(a.context(), image)
	if err != nil {
		return PhotoVocabularyDraft{}, err
	}
	entries := make([]ManualVocabularyInput, 0, len(draft.Entries))
	for _, entry := range draft.Entries {
		entries = append(entries, ManualVocabularyInput{
			Simplified:         entry.Simplified,
			Traditional:        entry.Traditional,
			Pinyin:             entry.Pinyin,
			Meaning:            entry.Meaning,
			PartOfSpeech:       entry.PartOfSpeech,
			Classifier:         entry.Classifier,
			Example:            entry.Example,
			ExamplePinyin:      entry.ExamplePinyin,
			ExampleTranslation: entry.ExampleTranslation,
			Tags:               entry.Tags,
			AiGenerated:        entry.AiGenerated,
		})
	}
	return PhotoVocabularyDraft{
		Entries: entries,
		Model:   draft.Model,
		Cost:    draft.Cost,
	}, nil
}

func decodePhotoImageBase64(value string) ([]byte, error) {
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		comma := strings.IndexByte(value, ',')
		if comma < 0 {
			return nil, errors.New("invalid image data URL")
		}
		value = value[comma+1:]
	}
	if value == "" {
		return nil, errors.New("image base64 is empty")
	}
	image, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid image base64: %w", err)
	}
	return image, nil
}

func (a *App) SaveManualVocabulary(input ManualVocabularyInput) (AppSnapshot, error) {
	_, err := a.service.SaveManualVocabulary(a.context(), service.ManualVocabularyInput{
		Simplified:         input.Simplified,
		Traditional:        input.Traditional,
		Pinyin:             input.Pinyin,
		Meaning:            input.Meaning,
		PartOfSpeech:       input.PartOfSpeech,
		Classifier:         input.Classifier,
		Example:            input.Example,
		ExamplePinyin:      input.ExamplePinyin,
		ExampleTranslation: input.ExampleTranslation,
		Tags:               input.Tags,
		AiGenerated:        input.AiGenerated,
	})
	if err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) RequestCapturePermission() (AppSnapshot, error) {
	err := a.checkReadiness(a.context(), a.service.Target())
	a.checkSupabaseAccess()
	snapshot, snapshotErr := a.GetAppSnapshot()
	if snapshotErr == nil {
		a.emit("snapshot_changed", snapshot)
	}
	if err != nil {
		return snapshot, err
	}
	if snapshotErr != nil {
		return AppSnapshot{}, snapshotErr
	}
	return snapshot, nil
}

func (a *App) StartLesson(input StartLessonInput) (AppSnapshot, error) {
	if !input.Consent {
		return AppSnapshot{}, errors.New("consent is required before starting a lesson")
	}
	target := normalizeTarget(input.Target)
	if !validTarget(target) {
		return AppSnapshot{}, errors.New("unsupported lesson target")
	}
	if !a.service.HasAPIKey(a.context()) {
		return AppSnapshot{}, errors.New("OpenRouter API key is not configured; add it in Settings before starting a lesson")
	}
	if err := a.checkReadiness(a.context(), target); err != nil {
		if snapshot, snapshotErr := a.GetAppSnapshot(); snapshotErr == nil {
			a.emit("snapshot_changed", snapshot)
		}
		return AppSnapshot{}, err
	}
	a.service.SetTarget(target)
	settings, err := a.service.Settings(a.context())
	if err != nil {
		return AppSnapshot{}, err
	}
	startedAt := time.Now()
	lesson, err := a.service.StartLessonWithMetadata(
		a.context(),
		target,
		startedAt,
		storage.LessonMetadata{
			ConsentRecorded: true,
			Target:          target,
			STTModel:        settings.STTModel,
			AnalyzerModel:   settings.AnalyzerModel,
			RetentionPolicy: settings.AudioRetention,
			LessonTags:      service.NormalizeLessonTags(input.Tags, startedAt),
		},
	)
	if err != nil {
		return AppSnapshot{}, err
	}
	if err := a.service.StartCapture(a.context(), lesson.ID); err != nil {
		_ = a.service.DeleteLesson(a.context(), lesson.ID)
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) StopLesson() (AppSnapshot, error) {
	if _, err := a.service.StopAndEndCurrentLesson(a.context()); err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) MarkMoment() (AppSnapshot, error) {
	if _, err := a.service.MarkMoment(a.context(), "manual"); err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) ConfirmCandidate(candidateID string) (AppSnapshot, error) {
	if _, err := a.service.ConfirmCandidate(a.context(), candidateID); err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) EditCandidate(candidateID string, patch CandidateEditPatch) (AppSnapshot, error) {
	edit := domain.CandidateEdit{
		Simplified:         patch.Simplified,
		Traditional:        patch.Traditional,
		Meaning:            patch.Meaning,
		PartOfSpeech:       patch.PartOfSpeech,
		Classifier:         patch.Classifier,
		Example:            patch.Example,
		ExamplePinyin:      patch.ExamplePinyin,
		ExampleTranslation: patch.ExampleTranslation,
		Tags:               patch.Tags,
	}
	if patch.Pinyin != nil {
		edit.Reading = patch.Pinyin
	}
	if _, err := a.service.EditCandidate(a.context(), candidateID, edit); err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) DeleteLastLesson() (AppSnapshot, error) {
	if err := a.service.DeleteLastLesson(a.context()); err != nil {
		return AppSnapshot{}, err
	}
	snapshot, err := a.GetAppSnapshot()
	if err == nil {
		a.emit("snapshot_changed", snapshot)
	}
	return snapshot, err
}

func (a *App) RejectCandidate(candidateID string) (AppSnapshot, error) {
	if _, err := a.service.RejectCandidate(a.context(), candidateID); err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) MergeCandidates(sourceID, targetID string) (AppSnapshot, error) {
	if _, err := a.service.MergeCandidates(a.context(), sourceID, targetID); err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) DeleteVocabulary(entryID string) (AppSnapshot, error) {
	if err := a.service.DeleteVocabulary(a.context(), entryID); err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) SaveSettings(patch SettingsPatch) (AppSnapshot, error) {
	if a.service.HasActiveLesson() {
		return AppSnapshot{}, errors.New("finish the active lesson before changing settings")
	}
	if patch.OpenRouterKey != nil {
		if strings.TrimSpace(*patch.OpenRouterKey) == "" {
			if err := a.service.ClearAPIKey(a.context()); err != nil {
				return AppSnapshot{}, err
			}
		} else if err := a.service.SetAPIKey(a.context(), *patch.OpenRouterKey); err != nil {
			return AppSnapshot{}, err
		}
	}
	if err := a.service.ApplySettings(a.context(), service.SettingsPatch{
		STTModel:          patch.STTModel,
		AnalyzerModel:     patch.AnalyzerModel,
		VisionModel:       patch.VisionModel,
		HardBudgetUSD:     patch.HardBudgetUSD,
		AudioRetention:    patch.AudioRetention,
		OCREnabled:        patch.OCREnabled,
		MicrophoneEnabled: patch.MicrophoneEnabled,
	}); err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) Refresh() (AppSnapshot, error) {
	return a.GetAppSnapshot()
}

func (a *App) SaveSupabaseKey(key string) (AppSnapshot, error) {
	if err := a.service.SetSupabaseSecret(a.context(), key); err != nil {
		return AppSnapshot{}, err
	}
	return a.GetAppSnapshot()
}

func (a *App) checkSupabaseAccess() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = a.service.CheckSupabaseAccess(ctx)
}

func (a *App) SyncNow() (AppSnapshot, error) {
	err := a.service.SyncNow(a.context())
	snapshot, snapshotErr := a.GetAppSnapshot()
	if snapshotErr != nil {
		return AppSnapshot{}, snapshotErr
	}
	if err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (a *App) ListTags() ([]string, error) {
	return a.service.ListTagNames(a.context())
}

func (a *App) ListLessonTranscripts(limit int) ([]LessonTranscriptSummary, error) {
	summaries, err := a.service.ListLessonTranscripts(a.context(), limit)
	if err != nil {
		return nil, err
	}
	result := make([]LessonTranscriptSummary, 0, len(summaries))
	for _, summary := range summaries {
		result = append(result, LessonTranscriptSummary{
			ID:           summary.ID,
			Title:        summary.Title,
			StartedAt:    timestamp(summary.StartedAt),
			EndedAt:      timestampFromPointer(summary.EndedAt),
			SegmentCount: summary.SegmentCount,
			FinalCost:    summary.FinalCost,
		})
	}
	return result, nil
}

func (a *App) GetLessonTranscript(lessonID string) (LessonTranscript, error) {
	transcript, err := a.service.GetLessonTranscript(a.context(), lessonID)
	if err != nil {
		return LessonTranscript{}, err
	}
	lines := make([]TranscriptLine, 0, len(transcript.Segments))
	for _, segment := range transcript.Segments {
		lines = append(lines, transcriptLineFromSegment(transcript.Lesson.StartedAt, segment))
	}
	return LessonTranscript{
		Lesson: LessonTranscriptSummary{
			ID:           transcript.Lesson.ID,
			Title:        transcript.Lesson.Title,
			StartedAt:    timestamp(transcript.Lesson.StartedAt),
			EndedAt:      timestampFromPointer(transcript.Lesson.EndedAt),
			SegmentCount: len(transcript.Segments),
			FinalCost:    transcript.Lesson.FinalCost,
		},
		Lines: lines,
	}, nil
}

func timestampFromPointer(value *time.Time) string {
	if value == nil {
		return ""
	}
	return timestamp(*value)
}

func (a *App) checkReadiness(ctx context.Context, target string) error {
	target = normalizeTarget(target)
	if target == "" {
		target = "system"
	}
	err := a.service.ValidateTarget(ctx, target)
	readiness := ReadinessSnapshot{
		Checked:         true,
		TargetAvailable: err == nil,
		CheckedAt:       timestamp(time.Now()),
	}
	if err != nil {
		readiness.Error = err.Error()
	}
	a.mu.Lock()
	a.readiness = readiness
	a.mu.Unlock()
	return err
}

func (a *App) readinessSnapshot() ReadinessSnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.readiness
}
