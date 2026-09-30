package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/service"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

const appEventPrefix = "call_analyzer:"

type App struct {
	mu        sync.RWMutex
	ctx       context.Context
	readiness ReadinessSnapshot
	service   *service.Service
	store     *storage.Store
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
	return NewAppWithDependencies(store, newPlatformSource(), service.NewDefaultSecretStore(), nil), nil
}

func NewAppWithDependencies(
	store *storage.Store,
	source capture.Source,
	secrets service.SecretStore,
	router *openrouter.Client,
) *App {
	app := &App{
		store:   store,
		service: service.New(store, source, secrets, router),
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
	a.emit("backend.ready", a.service.Config(ctx))
}

func (a *App) shutdown(ctx context.Context) {
	_ = a.service.StopCapture(ctx)
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
	}, nil
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
	lesson, err := a.service.StartLessonWithMetadata(
		a.context(),
		target,
		time.Time{},
		storage.LessonMetadata{
			ConsentRecorded: true,
			Target:          target,
			STTModel:        settings.STTModel,
			AnalyzerModel:   settings.AnalyzerModel,
			RetentionPolicy: settings.AudioRetention,
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

func (a *App) checkReadiness(ctx context.Context, target string) error {
	target = normalizeTarget(target)
	if target == "" {
		target = "zoom"
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
