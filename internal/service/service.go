package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/oeggy03/call_analyzer/internal/audio"
	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/openrouter"
	"github.com/oeggy03/call_analyzer/internal/storage"
	syncer "github.com/oeggy03/call_analyzer/internal/sync"
	"github.com/oeggy03/call_analyzer/internal/vocabulary"
)

const (
	secretOpenRouterAPIKey = "OPENROUTER_API_KEY"
	backendVersion         = "0.1.0"
)

type EventHook func(string, any)

type Service struct {
	store                *storage.Store
	source               capture.Source
	secrets              SecretStore
	router               *openrouter.Client
	syncSecrets          SecretStore
	syncRunner           *syncer.Runner
	skipShutdownSync     bool
	supabaseBaseURL      string
	supabaseHTTP         *http.Client
	supabaseAccessStatus string
	supabaseAccessDetail string

	mu              sync.RWMutex
	capturing       bool
	currentLesson   string
	lastLesson      string
	lessonError     string
	ring            *audio.RingBuffer
	session         *captureSession
	sessionID       string
	sessionLessonID string
	lessonStarted   time.Time
	target          string
	permission      capture.Permission
	micLevel        float64
	remoteLevel     float64
	lastLevelEmit   time.Time
	settings        RuntimeSettings
	settingsLoaded  bool
	eventHook       EventHook
	selectionCache  selectionLookupCache
	databasePath    string
	syncEnabled     bool
}

func New(store *storage.Store, source capture.Source, secrets SecretStore, router *openrouter.Client) *Service {
	return NewWithSyncSecretStore(store, source, secrets, router, nil)
}

func NewWithSyncSecretStore(
	store *storage.Store,
	source capture.Source,
	secrets SecretStore,
	router *openrouter.Client,
	syncSecrets SecretStore,
) *Service {
	usedDefaultSecrets := secrets == nil
	if secrets == nil {
		secrets = NewDefaultSecretStore()
	}
	if syncSecrets == nil {
		if usedDefaultSecrets {
			syncSecrets = NewSupabaseSecretStore()
		} else {
			syncSecrets = secrets
		}
	}
	databasePath := ""
	if store != nil {
		databasePath = store.Path()
	}
	return &Service{
		store:        store,
		source:       source,
		secrets:      secrets,
		syncSecrets:  syncSecrets,
		router:       router,
		databasePath: databasePath,
		settings:     defaultRuntimeSettings(),
	}
}

func (s *Service) SetEventHook(hook EventHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventHook = hook
}

func (s *Service) emit(name string, payload any) {
	s.mu.RLock()
	hook := s.eventHook
	s.mu.RUnlock()
	if hook != nil {
		hook(name, payload)
	}
}

func (s *Service) Store() *storage.Store {
	return s.store
}

func (s *Service) Initialize(ctx context.Context) error {
	if s.store == nil {
		return errors.New("service: storage is not configured")
	}
	if err := s.store.DB().PingContext(ctx); err != nil {
		return fmt.Errorf("service: initialize storage: %w", err)
	}
	if err := s.loadSettingsIfNeeded(ctx); err != nil {
		return fmt.Errorf("service: load settings: %w", err)
	}
	if _, err := s.store.Lessons().EndOrphaned(ctx, time.Now().UTC()); err != nil {
		return fmt.Errorf("service: recover lessons: %w", err)
	}
	if lesson, err := s.store.Lessons().Latest(ctx); err == nil {
		s.mu.Lock()
		s.lastLesson = lesson.ID
		s.mu.Unlock()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("service: load latest lesson: %w", err)
	}
	s.startSync(ctx)
	return nil
}

func (s *Service) startSync(ctx context.Context) {
	s.mu.Lock()
	if s.syncRunner != nil {
		s.mu.Unlock()
		return
	}
	runner := syncer.NewRunner(s.store, s.syncSecrets, syncer.RunnerOptions{
		OnStatusChange: func(status syncer.Status) {
			s.emit("sync.changed", status)
		},
	})
	s.syncRunner = runner
	s.mu.Unlock()
	runner.Start(ctx)
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.RLock()
	runner := s.syncRunner
	skip := s.skipShutdownSync
	s.mu.RUnlock()
	if runner == nil {
		return nil
	}
	if skip {
		return runner.StopWithoutFlush(ctx)
	}
	return runner.Stop(ctx)
}

func (s *Service) SkipShutdownSync() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.skipShutdownSync = true
	s.mu.Unlock()
}

func (s *Service) HasUnsyncedVocabulary(ctx context.Context) (bool, error) {
	s.mu.RLock()
	runner := s.syncRunner
	s.mu.RUnlock()
	if runner == nil || !runner.Status().Configured {
		return false, nil
	}
	return runner.HasUploadablePending(ctx)
}

func (s *Service) SyncNow(ctx context.Context) error {
	s.mu.RLock()
	runner := s.syncRunner
	s.mu.RUnlock()
	if runner == nil {
		return nil
	}
	return runner.SyncNow(ctx)
}

func (s *Service) SyncStatus(context.Context) syncer.Status {
	s.mu.RLock()
	runner := s.syncRunner
	s.mu.RUnlock()
	if runner == nil {
		return syncer.Status{Status: syncer.SyncStatusNotConfigured}
	}
	return runner.Status()
}

func (s *Service) SupabaseAccess() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.supabaseAccessStatus
	if status == "" {
		status = "unchecked"
	}
	return status, s.supabaseAccessDetail
}

// SetSupabaseSecret stores the cloud vocabulary key in the app secret store.
// On macOS that store writes the Keychain after the environment. An environment
// value still wins on the next read.
func (s *Service) SetSupabaseSecret(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("cloud vocabulary key is required")
	}
	if s.syncSecrets == nil {
		return errors.New("cloud vocabulary key storage is not configured")
	}
	if err := s.syncSecrets.Set(ctx, syncer.SupabaseSecretName, key); err != nil {
		return errors.New("this computer cannot store a cloud vocabulary key; set SUPABASE_SECRET_KEY in the environment")
	}
	stored, err := s.syncSecrets.Get(ctx, syncer.SupabaseSecretName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(stored) != key {
		_ = s.CheckSupabaseAccess(ctx)
		return errors.New("SUPABASE_SECRET_KEY is set in the environment and overrides the saved key")
	}
	return s.CheckSupabaseAccess(ctx)
}

// CheckSupabaseAccess reports whether the saved key can read and write cloud vocabulary.
func (s *Service) CheckSupabaseAccess(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.RLock()
	wasBroken := s.supabaseAccessStatus == "missing" || s.supabaseAccessStatus == "invalid"
	runner := s.syncRunner
	baseURL := s.supabaseBaseURL
	httpClient := s.supabaseHTTP
	s.mu.RUnlock()

	s.setSupabaseAccess("checking", "Checking cloud vocabulary access.")
	if s.syncSecrets == nil {
		s.setSupabaseAccess("missing", "Add the cloud vocabulary key for this computer.")
		return nil
	}
	secret, err := s.syncSecrets.Get(ctx, syncer.SupabaseSecretName)
	if err != nil || strings.TrimSpace(secret) == "" {
		s.setSupabaseAccess("missing", "Add the cloud vocabulary key for this computer.")
		return nil
	}
	client, err := syncer.NewPostgRESTClient(baseURL, strings.TrimSpace(secret), httpClient)
	if err != nil {
		s.setSupabaseAccess("invalid", "The cloud vocabulary key could not be used.")
		return nil
	}
	if err := client.ProbeReadWrite(ctx); err != nil {
		s.setSupabaseAccess("invalid", supabaseAccessFailure(err))
		return nil
	}
	s.setSupabaseAccess("ready", "Cloud vocabulary can be read and written.")
	if runner == nil || !runner.Running() {
		s.resumeSync(ctx)
	} else if wasBroken {
		go func() {
			pullCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_ = runner.Pull(pullCtx)
		}()
	}
	return nil
}

func (s *Service) resumeSync(ctx context.Context) {
	s.mu.RLock()
	runner := s.syncRunner
	s.mu.RUnlock()
	if runner == nil {
		s.startSync(ctx)
		return
	}
	if !runner.Running() {
		runner.Start(ctx)
	}
}

func (s *Service) setSupabaseAccess(status, detail string) {
	s.mu.Lock()
	changed := s.supabaseAccessStatus != status || s.supabaseAccessDetail != detail
	s.supabaseAccessStatus = status
	s.supabaseAccessDetail = detail
	s.mu.Unlock()
	if changed {
		s.emit("supabase.access", map[string]string{"status": status})
	}
}

func supabaseAccessFailure(err error) string {
	message := ""
	if err != nil {
		message = err.Error()
	}
	if strings.Contains(message, "HTTP 401") || strings.Contains(message, "HTTP 403") {
		return "This key cannot read and write cloud vocabulary."
	}
	return "Cloud vocabulary could not be reached."
}

func (s *Service) Health(ctx context.Context) domain.Health {
	health := domain.Health{
		OK:      false,
		Capture: "stopped",
		Version: backendVersion,
	}
	if s.store == nil {
		health.Database = "unconfigured"
		return health
	}
	if err := s.store.DB().PingContext(ctx); err != nil {
		health.Database = "error"
	} else {
		health.Database = "ok"
		health.OK = true
	}
	s.mu.RLock()
	if s.capturing {
		health.Capture = "running"
	}
	s.mu.RUnlock()
	return health
}

func (s *Service) Config(ctx context.Context) domain.ConfigView {
	view := domain.ConfigView{
		DatabasePath: s.databasePath,
		SyncEnabled:  s.syncEnabled,
	}
	if s.router != nil {
		view.ASRModel = s.router.ASRModel()
		view.ChatModel = s.router.ChatModel()
	} else {
		view.ASRModel = openrouter.DefaultASRModel
		view.ChatModel = openrouter.DefaultChatModel
	}
	view.HasAPIKey = s.HasAPIKey(ctx)
	return view
}

func (s *Service) SetAPIKey(ctx context.Context, key string) error {
	if err := s.secrets.Set(ctx, secretOpenRouterAPIKey, strings.TrimSpace(key)); err != nil {
		return err
	}
	s.mu.RLock()
	routerReady := s.router != nil
	settings := s.settings
	loaded := s.settingsLoaded
	s.mu.RUnlock()
	if !routerReady {
		if !loaded {
			if err := s.loadSettingsIfNeeded(ctx); err != nil {
				return err
			}
			s.mu.RLock()
			settings = s.settings
			s.mu.RUnlock()
		}
		if err := s.rebuildRouter(settings); err != nil {
			return err
		}
	}
	s.emit("config.changed", s.Config(ctx))
	return nil
}

func (s *Service) ClearAPIKey(ctx context.Context) error {
	if err := s.secrets.Delete(ctx, secretOpenRouterAPIKey); err != nil {
		return err
	}
	if s.HasAPIKey(ctx) {
		return errors.New("OpenRouter API key is supplied by the environment and cannot be cleared in the app")
	}
	s.emit("config.changed", s.Config(ctx))
	return nil
}

func (s *Service) HasAPIKey(ctx context.Context) bool {
	_, err := s.secrets.Get(ctx, secretOpenRouterAPIKey)
	return err == nil
}

func (s *Service) ConfigureOpenRouter(config openrouter.Config) error {
	client, err := openrouter.NewClient(config, APIKeySecretProvider{
		Store: s.secrets,
		Name:  secretOpenRouterAPIKey,
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.router = client
	s.mu.Unlock()
	s.emit("config.changed", s.Config(context.Background()))
	return nil
}

func (s *Service) StartLesson(ctx context.Context, title string, startedAt time.Time) (domain.Lesson, error) {
	s.mu.RLock()
	target := s.target
	s.mu.RUnlock()
	metadata := storage.LessonMetadata{Target: target}
	settings, err := s.Settings(ctx)
	if err != nil {
		return domain.Lesson{}, err
	}
	metadata.STTModel = settings.STTModel
	metadata.AnalyzerModel = settings.AnalyzerModel
	metadata.RetentionPolicy = settings.AudioRetention
	return s.StartLessonWithMetadata(ctx, title, startedAt, metadata)
}

func (s *Service) StartLessonWithMetadata(
	ctx context.Context,
	title string,
	startedAt time.Time,
	metadata storage.LessonMetadata,
) (domain.Lesson, error) {
	if s.store == nil {
		return domain.Lesson{}, errors.New("service: storage is not configured")
	}
	lesson, err := s.store.Lessons().StartWithMetadata(ctx, title, startedAt, metadata)
	if err == nil {
		s.mu.Lock()
		s.lastLesson = lesson.ID
		s.lessonError = ""
		s.mu.Unlock()
		s.emit("lesson.started", lesson)
	}
	return lesson, err
}

func (s *Service) EndLesson(ctx context.Context, id string, endedAt time.Time) (domain.Lesson, error) {
	if s.store == nil {
		return domain.Lesson{}, errors.New("service: storage is not configured")
	}
	finalCost := 0.0
	s.mu.RLock()
	sessionID := s.sessionID
	sessionLessonID := s.sessionLessonID
	s.mu.RUnlock()
	if sessionID != "" && sessionLessonID == id {
		if cost, costErr := s.store.Usage().SumBySession(ctx, sessionID); costErr == nil {
			finalCost = cost
		}
	}
	lesson, err := s.store.Lessons().EndWithCost(ctx, id, endedAt, finalCost)
	if err == nil {
		s.mu.Lock()
		if s.sessionLessonID == id {
			s.sessionID = ""
			s.sessionLessonID = ""
		}
		s.mu.Unlock()
		s.emit("lesson.ended", lesson)
	}
	return lesson, err
}

func (s *Service) ListLessons(ctx context.Context, limit int) ([]domain.Lesson, error) {
	if s.store == nil {
		return nil, errors.New("service: storage is not configured")
	}
	return s.store.Lessons().List(ctx, limit)
}

func (s *Service) DeleteLesson(ctx context.Context, id string) error {
	if s.store == nil {
		return errors.New("service: storage is not configured")
	}
	err := s.store.Lessons().Delete(ctx, id)
	if err == nil {
		s.mu.Lock()
		if s.currentLesson == id {
			s.currentLesson = ""
		}
		if s.lastLesson == id {
			s.lastLesson = ""
		}
		if s.sessionLessonID == id {
			s.sessionID = ""
			s.sessionLessonID = ""
		}
		if s.lessonError != "" && s.currentLesson == "" {
			s.lessonError = ""
		}
		s.mu.Unlock()
		if latest, latestErr := s.store.Lessons().Latest(ctx); latestErr == nil {
			s.mu.Lock()
			if s.currentLesson == "" {
				s.lastLesson = latest.ID
			}
			s.mu.Unlock()
		} else if !errors.Is(latestErr, sql.ErrNoRows) {
			return latestErr
		}
		s.emit("lesson.deleted", id)
	}
	return err
}

func (s *Service) DeleteLastLesson(ctx context.Context) error {
	if s.store == nil {
		return errors.New("service: storage is not configured")
	}
	s.mu.RLock()
	live := s.capturing || s.currentLesson != ""
	s.mu.RUnlock()
	if live {
		return errors.New("service: cannot delete the last lesson while capture is live")
	}
	lesson, err := s.store.Lessons().LatestComplete(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("service: no completed lesson to delete")
		}
		return err
	}
	if err := s.store.Lessons().Delete(ctx, lesson.ID); err != nil {
		return err
	}
	s.mu.Lock()
	s.currentLesson = ""
	s.lastLesson = ""
	s.session = nil
	s.sessionID = ""
	s.sessionLessonID = ""
	s.ring = nil
	s.lessonStarted = time.Time{}
	s.lessonError = ""
	s.mu.Unlock()
	if latest, latestErr := s.store.Lessons().Latest(ctx); latestErr == nil {
		s.mu.Lock()
		s.lastLesson = latest.ID
		s.mu.Unlock()
	} else if !errors.Is(latestErr, sql.ErrNoRows) {
		return latestErr
	}
	s.emit("lesson.deleted", lesson.ID)
	return nil
}

func (s *Service) StartCapture(ctx context.Context, lessonID string) error {
	if s.source == nil {
		err := errors.New("service: capture source is not configured")
		s.diagnostic(err.Error(), nil)
		return err
	}
	if err := s.loadSettingsIfNeeded(ctx); err != nil {
		s.diagnostic("loading capture settings failed", err)
		return err
	}
	s.mu.Lock()
	if s.router != nil {
		s.router = s.router.WithModels(
			s.settings.STTModel,
			s.settings.AnalyzerModel,
			openrouter.NewBudget(0.35, s.settings.HardBudgetUSD),
		).WithVisionModel(s.settings.VisionModel)
	}
	s.mu.Unlock()
	if lessonID == "" {
		err := errors.New("service: lesson id is required")
		s.diagnostic(err.Error(), nil)
		return err
	}
	lesson, err := s.store.Lessons().Get(ctx, lessonID)
	if err != nil {
		err = fmt.Errorf("service: load lesson: %w", err)
		s.diagnostic(err.Error(), nil)
		return err
	}
	ring, err := audio.NewRingBuffer(audio.DefaultRingCapacity)
	if err != nil {
		return err
	}
	session := newCaptureSession(ctx, ring, lessonID, lesson.StartedAt)
	s.mu.Lock()
	if s.capturing {
		s.mu.Unlock()
		return errors.New("service: capture is already running")
	}
	s.currentLesson = lessonID
	s.lastLesson = lessonID
	s.ring = ring
	s.lessonStarted = lesson.StartedAt
	s.sessionID = session.sessionID
	s.sessionLessonID = lessonID
	s.session = session
	s.capturing = true
	s.micLevel = 0
	s.remoteLevel = 0
	s.lastLevelEmit = time.Time{}
	s.mu.Unlock()
	if eventSource, ok := s.source.(capture.EventSource); ok {
		eventSource.SetEventHandler(s.receiveEvent)
	}
	if configurable, ok := s.source.(capture.ConfigurableSource); ok {
		if err := configurable.Configure(capture.Options{
			TargetID:   s.Target(),
			Microphone: s.settings.MicrophoneEnabled,
			OCR:        s.settings.OCREnabled,
		}); err != nil {
			session.cancel()
			s.mu.Lock()
			s.capturing = false
			s.currentLesson = ""
			s.ring = nil
			s.session = nil
			s.mu.Unlock()
			s.diagnostic("capture configuration failed", err)
			return err
		}
	}
	if err := s.source.Start(ctx, s.receiveFrame); err != nil {
		session.cancel()
		s.mu.Lock()
		s.capturing = false
		s.currentLesson = ""
		s.ring = nil
		s.session = nil
		s.mu.Unlock()
		s.diagnostic("starting capture failed", err)
		return err
	}
	session.start(s)
	s.emit("capture.started", map[string]string{"lessonId": lessonID})
	return nil
}

func (s *Service) StopCapture(ctx context.Context) error {
	if s.source == nil {
		err := errors.New("service: capture source is not configured")
		s.diagnostic(err.Error(), nil)
		return err
	}
	s.mu.RLock()
	running := s.capturing
	lessonID := s.currentLesson
	session := s.session
	s.mu.RUnlock()
	if !running && session == nil {
		return nil
	}
	sourceErr := s.source.Stop(ctx)
	var sessionErr error
	if session != nil {
		sessionErr = session.stop(ctx)
	}
	s.mu.Lock()
	s.capturing = false
	if sourceErr == nil && sessionErr == nil {
		s.currentLesson = ""
		s.session = nil
		s.ring = nil
	}
	s.micLevel = 0
	s.remoteLevel = 0
	s.lastLevelEmit = time.Time{}
	s.mu.Unlock()
	if sourceErr != nil {
		s.diagnostic("stopping capture failed", sourceErr)
	}
	if sessionErr != nil {
		s.diagnostic("finishing pending lesson analysis timed out", sessionErr)
	}
	if sourceErr == nil && sessionErr == nil {
		s.emit("capture.stopped", map[string]string{"lessonId": lessonID})
	} else {
		s.emit("capture.stop_failed", map[string]string{"lessonId": lessonID})
	}
	return errors.Join(sourceErr, sessionErr)
}

func (s *Service) StopAndEndCurrentLesson(ctx context.Context) (domain.Lesson, error) {
	s.mu.RLock()
	lessonID := s.currentLesson
	lessonStart := s.lessonStarted
	sessionID := s.sessionID
	router := s.router
	s.mu.RUnlock()
	if lessonID == "" {
		return domain.Lesson{}, errors.New("service: no active lesson")
	}
	flushCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	stopErr := s.StopCapture(flushCtx)
	if stopErr != nil {
		return domain.Lesson{}, stopErr
	}
	if router == nil || !router.BudgetStatus().HardExceeded {
		reconcileCtx, reconcileCancel := context.WithTimeout(ctx, 20*time.Second)
		reconcileErr := s.reconcileLesson(
			reconcileCtx,
			lessonID,
			sessionID,
			lessonStart,
			router,
		)
		reconcileCancel()
		if reconcileErr != nil && !errors.Is(reconcileErr, context.Canceled) {
			s.diagnostic("post-lesson reconciliation failed", reconcileErr)
		}
	}
	lesson, endErr := s.EndLesson(ctx, lessonID, time.Time{})
	return lesson, endErr
}

func (s *Service) captureSessionFailed(session *captureSession) {
	s.mu.Lock()
	if s.session != session {
		s.mu.Unlock()
		return
	}
	s.capturing = false
	s.micLevel = 0
	s.remoteLevel = 0
	s.lastLevelEmit = time.Time{}
	lessonID := s.currentLesson
	s.mu.Unlock()
	s.emit("capture.failed", map[string]string{"lessonId": lessonID})
}

func (s *Service) receiveFrame(frame capture.AudioFrame) {
	s.mu.RLock()
	session := s.session
	s.mu.RUnlock()
	if session == nil || !session.enqueueFrame(frame) {
		if session != nil {
			s.diagnostic("capture frame queue full", nil)
		}
		return
	}
}

func (s *Service) receiveEvent(event capture.Event) {
	s.mu.RLock()
	session := s.session
	s.mu.RUnlock()
	if session == nil || !session.enqueueEvent(event) {
		if session != nil {
			s.diagnostic("capture event queue full", nil)
		}
	}
}

func (s *Service) updateAudioLevel(frame capture.AudioFrame) {
	level := audio.Energy(frame.Samples)
	if level > 1 {
		level = 1
	}
	s.mu.Lock()
	switch frame.Source {
	case "remote", "system", "system/ocr":
		s.remoteLevel = level
	default:
		s.micLevel = level
	}
	shouldEmit := time.Since(s.lastLevelEmit) >= 500*time.Millisecond
	if shouldEmit {
		s.lastLevelEmit = time.Now()
	}
	micLevel := s.micLevel
	remoteLevel := s.remoteLevel
	s.mu.Unlock()
	if shouldEmit {
		s.emit("capture.levels", map[string]float64{
			"mic":    micLevel,
			"remote": remoteLevel,
		})
	}
}

func (s *Service) MarkMoment(ctx context.Context, label string) (capture.Moment, error) {
	if s.source == nil {
		return capture.Moment{}, errors.New("service: capture source is not configured")
	}
	moment, err := s.source.Mark(ctx, label)
	if err == nil {
		s.mu.RLock()
		session := s.session
		s.mu.RUnlock()
		if session == nil {
			return capture.Moment{}, errors.New("service: no active lesson")
		}
		offset := moment.At.Sub(session.lessonStart).Milliseconds()
		if offset < 0 {
			offset = 0
		}
		marker, markerErr := s.store.Transcripts().Insert(ctx, domain.TranscriptSegment{
			LessonID:   session.lessonID,
			StartMS:    offset,
			EndMS:      offset,
			Text:       "Marked moment",
			Source:     "system/mark",
			Confidence: 1,
		})
		if markerErr != nil {
			return capture.Moment{}, fmt.Errorf("service: persist lesson mark: %w", markerErr)
		}
		s.emit("transcript.segment", marker)
		session.manualWG.Add(1)
		go s.waitForManualMoment(session, moment)
		s.emit("capture.moment", moment)
	} else {
		s.diagnostic("manual mark failed", err)
	}
	return moment, err
}

func (s *Service) waitForManualMoment(session *captureSession, moment capture.Moment) {
	defer session.manualWG.Done()
	window, err := session.ring.WaitForWindowClamped(
		session.manualCtx,
		moment.At,
		session.lessonStart,
		audio.DefaultPreRoll,
		audio.DefaultPostRoll,
	)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			window, fallbackErr := session.ring.SnapshotClamped(
				moment.At,
				session.lessonStart,
				audio.DefaultPreRoll,
				audio.DefaultPostRoll,
			)
			if fallbackErr == nil {
				if !session.enqueueJob(analysisJob{
					window: window,
					source: "system/manual",
					manual: true,
				}, true) {
					s.diagnostic("manual moment queue full", nil)
				}
				return
			}
			s.diagnostic("manual moment window unavailable", fallbackErr)
			return
		}
		if !errors.Is(err, context.Canceled) {
			s.diagnostic("manual moment window failed", err)
		}
		return
	}
	if !session.enqueueJob(analysisJob{
		window: window,
		source: "system/manual",
		manual: true,
	}, true) {
		s.diagnostic("manual moment queue full", nil)
	}
}

func (s *Service) RequestCapturePermission(ctx context.Context) (capture.Permission, error) {
	if permissionSource, ok := s.source.(capture.PermissionSource); ok {
		permission, err := permissionSource.RequestPermission(ctx)
		s.mu.Lock()
		s.permission = permission
		s.mu.Unlock()
		return permission, err
	}
	s.mu.Lock()
	s.permission = capture.PermissionGranted
	s.mu.Unlock()
	return capture.PermissionGranted, nil
}

func (s *Service) ListTargets(ctx context.Context) ([]capture.Target, error) {
	if targetSource, ok := s.source.(capture.TargetSource); ok {
		return targetSource.ListTargets(ctx)
	}
	return []capture.Target{
		{ID: "zoom", Name: "Zoom", Available: true},
		{ID: "googleMeet", Name: "Google Meet", Available: true},
		{ID: "teams", Name: "Microsoft Teams", Available: true},
	}, nil
}

func (s *Service) ValidateTarget(ctx context.Context, target string) error {
	targets, err := s.ListTargets(ctx)
	if err != nil {
		s.mu.Lock()
		s.permission = capture.PermissionDenied
		s.mu.Unlock()
		return fmt.Errorf("capture setup check failed: %w", err)
	}
	s.mu.Lock()
	s.permission = capture.PermissionGranted
	s.mu.Unlock()
	if systemAudioTarget(target) {
		return nil
	}
	for _, candidate := range targets {
		if candidate.Available && candidate.ID == target {
			return nil
		}
	}
	return fmt.Errorf("open %s before starting a lesson, then try again", target)
}

func systemAudioTarget(target string) bool {
	switch strings.TrimSpace(target) {
	case "", "system", "zoom", "us.zoom.xos", "googleMeet", "teams":
		return true
	default:
		return false
	}
}

func (s *Service) CapturePermission() capture.Permission {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.permission == "" {
		return capture.PermissionUnknown
	}
	return s.permission
}

func (s *Service) SetTarget(target string) {
	if target == "us.zoom.xos" {
		target = "zoom"
	}
	s.mu.Lock()
	s.target = target
	s.mu.Unlock()
}

func (s *Service) Target() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.target
}

func (s *Service) Levels() (mic, remote float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.micLevel, s.remoteLevel
}

func (s *Service) captureSettings() (ocrEnabled, microphoneEnabled bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.OCREnabled, s.settings.MicrophoneEnabled
}

func (s *Service) CurrentLesson(ctx context.Context) (domain.Lesson, bool, error) {
	s.mu.RLock()
	id := s.currentLesson
	running := s.capturing
	if id == "" {
		id = s.lastLesson
	}
	s.mu.RUnlock()
	if id == "" {
		return domain.Lesson{}, false, nil
	}
	lesson, err := s.store.Lessons().Get(ctx, id)
	return lesson, running, err
}

func (s *Service) LessonError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lessonError
}

func (s *Service) CurrentSessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

func (s *Service) HasActiveLesson() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentLesson != ""
}

func (s *Service) CurrentCost(ctx context.Context) (float64, error) {
	sessionID := s.CurrentSessionID()
	if sessionID == "" {
		lesson, _, err := s.CurrentLesson(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, nil
			}
			return 0, err
		}
		return lesson.FinalCost, nil
	}
	return s.store.Usage().SumBySession(ctx, sessionID)
}

func (s *Service) InsertTranscript(ctx context.Context, segment domain.TranscriptSegment) (domain.TranscriptSegment, error) {
	if s.store == nil {
		return domain.TranscriptSegment{}, errors.New("service: storage is not configured")
	}
	segment, err := s.store.Transcripts().Insert(ctx, segment)
	if err == nil {
		s.emit("transcript.segment", segment)
	}
	return segment, err
}

func (s *Service) ListTranscript(ctx context.Context, lessonID string) ([]domain.TranscriptSegment, error) {
	if s.store == nil {
		return nil, errors.New("service: storage is not configured")
	}
	return s.store.Transcripts().List(ctx, lessonID)
}

func (s *Service) ListObservations(ctx context.Context, lessonID string) ([]domain.Observation, error) {
	if s.store == nil {
		return nil, errors.New("service: storage is not configured")
	}
	return s.store.Observations().List(ctx, lessonID)
}

func (s *Service) ListCandidates(ctx context.Context, limit int) ([]domain.VocabularyEntry, error) {
	if s.store == nil {
		return nil, errors.New("service: storage is not configured")
	}
	return s.store.Vocabulary().ListCandidates(ctx, limit)
}

func (s *Service) ListVocabulary(ctx context.Context, limit int) ([]domain.VocabularyEntry, error) {
	if s.store == nil {
		return nil, errors.New("service: storage is not configured")
	}
	return s.store.Vocabulary().ListVocabulary(ctx, limit)
}

func (s *Service) ConfirmCandidate(ctx context.Context, id string) (domain.VocabularyEntry, error) {
	if s.store == nil {
		return domain.VocabularyEntry{}, errors.New("service: storage is not configured")
	}
	entry, err := s.store.Vocabulary().Confirm(ctx, id)
	if err == nil {
		s.emit("vocabulary.changed", entry)
	}
	return entry, err
}

func (s *Service) EditCandidate(ctx context.Context, id string, edit domain.CandidateEdit) (domain.VocabularyEntry, error) {
	if s.store == nil {
		return domain.VocabularyEntry{}, errors.New("service: storage is not configured")
	}
	if edit.Reading != nil {
		numbered, marked, err := vocabulary.CanonicalPinyin(*edit.Reading)
		if err != nil {
			return domain.VocabularyEntry{}, err
		}
		edit.Reading = &numbered
		if edit.MarkedPinyin == nil {
			edit.MarkedPinyin = &marked
		}
	}
	if edit.ExamplePinyin != nil {
		numbered, _, err := vocabulary.CanonicalPinyin(*edit.ExamplePinyin)
		if err != nil {
			return domain.VocabularyEntry{}, fmt.Errorf("service: example pinyin: %w", err)
		}
		edit.ExamplePinyin = &numbered
	}
	entry, err := s.store.Vocabulary().Edit(ctx, id, edit)
	if err == nil {
		s.emit("vocabulary.changed", entry)
	}
	return entry, err
}

func (s *Service) RejectCandidate(ctx context.Context, id string) (domain.VocabularyEntry, error) {
	if s.store == nil {
		return domain.VocabularyEntry{}, errors.New("service: storage is not configured")
	}
	entry, err := s.store.Vocabulary().Reject(ctx, id)
	if err == nil {
		s.emit("vocabulary.changed", entry)
	}
	return entry, err
}

func (s *Service) MergeCandidates(ctx context.Context, sourceID, targetID string) (domain.VocabularyEntry, error) {
	if s.store == nil {
		return domain.VocabularyEntry{}, errors.New("service: storage is not configured")
	}
	entry, err := s.store.Vocabulary().Merge(ctx, sourceID, targetID)
	if err == nil {
		s.emit("vocabulary.changed", entry)
	}
	return entry, err
}

func (s *Service) DeleteVocabulary(ctx context.Context, id string) error {
	if s.store == nil {
		return errors.New("service: storage is not configured")
	}
	if err := s.store.Vocabulary().Delete(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("service: vocabulary word not found")
		}
		return err
	}
	s.emit("vocabulary.deleted", id)
	return nil
}

type RefreshResult struct {
	Candidates []domain.VocabularyEntry `json:"candidates"`
	Vocabulary []domain.VocabularyEntry `json:"vocabulary"`
}

func (s *Service) Refresh(ctx context.Context, limit int) (RefreshResult, error) {
	candidates, err := s.ListCandidates(ctx, limit)
	if err != nil {
		return RefreshResult{}, err
	}
	vocabulary, err := s.ListVocabulary(ctx, limit)
	if err != nil {
		return RefreshResult{}, err
	}
	return RefreshResult{Candidates: candidates, Vocabulary: vocabulary}, nil
}

func (s *Service) GetSetting(ctx context.Context, key string) (domain.Setting, error) {
	if s.store == nil {
		return domain.Setting{}, errors.New("service: storage is not configured")
	}
	return s.store.Settings().Get(ctx, key)
}

func (s *Service) SetSetting(ctx context.Context, key, valueJSON string) (domain.Setting, error) {
	if s.store == nil {
		return domain.Setting{}, errors.New("service: storage is not configured")
	}
	setting, err := s.store.Settings().Set(ctx, key, valueJSON)
	if err == nil {
		s.emit("settings.changed", setting)
	}
	return setting, err
}

func (s *Service) ListSettings(ctx context.Context) ([]domain.Setting, error) {
	if s.store == nil {
		return nil, errors.New("service: storage is not configured")
	}
	return s.store.Settings().List(ctx)
}

func (s *Service) RecordUsage(ctx context.Context, usage domain.RequestUsage) (domain.RequestUsage, error) {
	if s.store == nil {
		return domain.RequestUsage{}, errors.New("service: storage is not configured")
	}
	return s.store.Usage().Record(ctx, usage)
}

func (s *Service) GetStudyState(ctx context.Context, entryID string) (domain.StudyState, error) {
	if s.store == nil {
		return domain.StudyState{}, errors.New("service: storage is not configured")
	}
	return s.store.Study().GetState(ctx, entryID)
}

func (s *Service) ObservationStats(ctx context.Context, entryID string) (int, time.Time, error) {
	if s.store == nil {
		return 0, time.Time{}, errors.New("service: storage is not configured")
	}
	return s.store.Vocabulary().ObservationStats(ctx, entryID)
}

func (s *Service) RecordReview(ctx context.Context, entryID string, rating int, reviewedAt time.Time, metadata string) (domain.ReviewEvent, domain.StudyState, error) {
	if s.store == nil {
		return domain.ReviewEvent{}, domain.StudyState{}, errors.New("service: storage is not configured")
	}
	event, state, err := s.store.Study().RecordReview(ctx, entryID, rating, reviewedAt, metadata)
	if err == nil {
		s.emit("study.reviewed", event)
	}
	return event, state, err
}
