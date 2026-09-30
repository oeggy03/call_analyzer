package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/oeggy03/call_analyzer/internal/openrouter"
)

const (
	settingSTTModel       = "stt_model"
	settingAnalyzerModel  = "analyzer_model"
	settingHardBudget     = "hard_budget_usd"
	settingAudioRetention = "audio_retention"
	settingOCREnabled     = "ocr_enabled"
	settingMicEnabled     = "microphone_enabled"
)

type RuntimeSettings struct {
	STTModel          string
	AnalyzerModel     string
	HardBudgetUSD     float64
	AudioRetention    string
	OCREnabled        bool
	MicrophoneEnabled bool
}

type SettingsPatch struct {
	STTModel          *string
	AnalyzerModel     *string
	HardBudgetUSD     *float64
	AudioRetention    *string
	OCREnabled        *bool
	MicrophoneEnabled *bool
}

func defaultRuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		STTModel:          openrouter.DefaultASRModel,
		AnalyzerModel:     openrouter.DefaultChatModel,
		HardBudgetUSD:     0.50,
		AudioRetention:    "sessionOnly",
		OCREnabled:        false,
		MicrophoneEnabled: true,
	}
}

func (s *Service) loadSettings(ctx context.Context) error {
	settings := defaultRuntimeSettings()
	if value, err := s.store.Settings().Get(ctx, settingSTTModel); err == nil {
		_ = json.Unmarshal([]byte(value.ValueJSON), &settings.STTModel)
	}
	if value, err := s.store.Settings().Get(ctx, settingAnalyzerModel); err == nil {
		_ = json.Unmarshal([]byte(value.ValueJSON), &settings.AnalyzerModel)
	}
	if value, err := s.store.Settings().Get(ctx, settingHardBudget); err == nil {
		_ = json.Unmarshal([]byte(value.ValueJSON), &settings.HardBudgetUSD)
	}
	if value, err := s.store.Settings().Get(ctx, settingAudioRetention); err == nil {
		_ = json.Unmarshal([]byte(value.ValueJSON), &settings.AudioRetention)
	}
	if value, err := s.store.Settings().Get(ctx, settingOCREnabled); err == nil {
		_ = json.Unmarshal([]byte(value.ValueJSON), &settings.OCREnabled)
	}
	if value, err := s.store.Settings().Get(ctx, settingMicEnabled); err == nil {
		_ = json.Unmarshal([]byte(value.ValueJSON), &settings.MicrophoneEnabled)
	}
	if settings.STTModel == "" {
		settings.STTModel = openrouter.DefaultASRModel
	}
	if settings.AnalyzerModel == "" || isLegacyNonZDRAnalyzerModel(settings.AnalyzerModel) {
		settings.AnalyzerModel = openrouter.DefaultChatModel
	}
	if settings.HardBudgetUSD <= 0 || settings.HardBudgetUSD > 0.50 {
		settings.HardBudgetUSD = 0.50
	}
	if settings.AudioRetention == "" {
		settings.AudioRetention = "sessionOnly"
	}
	s.mu.Lock()
	s.settings = settings
	s.settingsLoaded = true
	s.syncEnabled = false
	s.mu.Unlock()
	for key, value := range map[string]any{
		settingSTTModel:       settings.STTModel,
		settingAnalyzerModel:  settings.AnalyzerModel,
		settingHardBudget:     settings.HardBudgetUSD,
		settingAudioRetention: settings.AudioRetention,
		settingOCREnabled:     settings.OCREnabled,
		settingMicEnabled:     settings.MicrophoneEnabled,
	} {
		encoded, _ := json.Marshal(value)
		if _, err := s.store.Settings().Set(ctx, key, string(encoded)); err != nil {
			return err
		}
	}
	return s.rebuildRouter(settings)
}

func (s *Service) rebuildRouter(settings RuntimeSettings) error {
	s.mu.RLock()
	existing := s.router
	capturing := s.capturing
	s.mu.RUnlock()
	if existing != nil {
		var budget *openrouter.Budget
		if !capturing {
			budget = openrouter.NewBudget(0.35, settings.HardBudgetUSD)
		}
		s.mu.Lock()
		s.router = existing.WithModels(settings.STTModel, settings.AnalyzerModel, budget)
		s.mu.Unlock()
		s.emit("config.changed", s.Config(context.Background()))
		return nil
	}
	return s.ConfigureOpenRouter(openrouter.Config{
		ASRModel:  settings.STTModel,
		ChatModel: settings.AnalyzerModel,
		Budget:    openrouter.NewBudget(0.35, settings.HardBudgetUSD),
	})
}

func (s *Service) Settings(ctx context.Context) (RuntimeSettings, error) {
	if err := s.loadSettingsIfNeeded(ctx); err != nil {
		return RuntimeSettings{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings, nil
}

func (s *Service) loadSettingsIfNeeded(ctx context.Context) error {
	s.mu.RLock()
	loaded := s.settingsLoaded
	s.mu.RUnlock()
	if loaded {
		return nil
	}
	return s.loadSettings(ctx)
}

func (s *Service) ApplySettings(ctx context.Context, patch SettingsPatch) error {
	if s.store == nil {
		return errors.New("service: storage is not configured")
	}
	if err := s.loadSettingsIfNeeded(ctx); err != nil {
		return err
	}
	s.mu.RLock()
	next := s.settings
	s.mu.RUnlock()
	if patch.STTModel != nil {
		next.STTModel = strings.TrimSpace(*patch.STTModel)
	}
	if patch.AnalyzerModel != nil {
		next.AnalyzerModel = strings.TrimSpace(*patch.AnalyzerModel)
	}
	if patch.HardBudgetUSD != nil {
		next.HardBudgetUSD = *patch.HardBudgetUSD
	}
	if patch.AudioRetention != nil {
		next.AudioRetention = *patch.AudioRetention
	}
	if patch.OCREnabled != nil {
		next.OCREnabled = *patch.OCREnabled
	}
	if patch.MicrophoneEnabled != nil {
		next.MicrophoneEnabled = *patch.MicrophoneEnabled
	}
	if next.STTModel == "" || next.AnalyzerModel == "" {
		return errors.New("service: model names cannot be empty")
	}
	if next.HardBudgetUSD <= 0 || next.HardBudgetUSD > 0.50 {
		return errors.New("service: hard budget must be greater than zero and at most 0.50 USD")
	}
	switch next.AudioRetention {
	case "sessionOnly", "oneDay", "sevenDays":
	default:
		return errors.New("service: invalid audio retention")
	}
	values := map[string]any{
		settingSTTModel:       next.STTModel,
		settingAnalyzerModel:  next.AnalyzerModel,
		settingHardBudget:     next.HardBudgetUSD,
		settingAudioRetention: next.AudioRetention,
		settingOCREnabled:     next.OCREnabled,
		settingMicEnabled:     next.MicrophoneEnabled,
	}
	for key, value := range values {
		encoded, _ := json.Marshal(value)
		if _, err := s.store.Settings().Set(ctx, key, string(encoded)); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.settings = next
	s.settingsLoaded = true
	s.mu.Unlock()
	if err := s.rebuildRouter(next); err != nil {
		return err
	}
	s.emit("settings.changed", next)
	return nil
}

func isLegacyNonZDRAnalyzerModel(model string) bool {
	switch strings.TrimSpace(model) {
	case "qwen/qwen3.8-flash", "qwen/qwen3.8-max-0902":
		return true
	default:
		return false
	}
}
