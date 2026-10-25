package sync

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"
	stdsync "sync"
	"time"

	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/storage"
)

const (
	SupabaseSecretName = "SUPABASE_SECRET_KEY"

	SyncStatusNotConfigured = "not_configured"
	SyncStatusWaiting       = "waiting"
	SyncStatusSyncing       = "syncing"
	SyncStatusIdle          = "idle"
	SyncStatusError         = "error"

	PullStatusPulling = "pulling"
	PullStatusIdle    = "idle"
	PullStatusError   = "error"
)

type SecretStore interface {
	Get(context.Context, string) (string, error)
}

type Status struct {
	Configured      bool
	PendingCount    int
	Status          string
	WaitingUntil    *time.Time
	LastSyncAt      *time.Time
	LastSyncError   string
	PullStatus      string
	PullChangeCount int
	LastPullAt      *time.Time
	PullError       string
}

type RunnerOptions struct {
	BaseURL        string
	HTTPClient     *http.Client
	QuietPeriod    time.Duration
	PollInterval   time.Duration
	RequestTimeout time.Duration
	Now            func() time.Time
	OnStatusChange func(Status)
}

type Runner struct {
	store       *storage.Store
	secretStore SecretStore
	options     RunnerOptions

	processMu stdsync.Mutex
	mu        stdsync.RWMutex
	client    *PostgRESTClient
	started   bool
	cancel    context.CancelFunc
	done      chan struct{}
	status    Status
}

func NewRunner(
	store *storage.Store,
	secretStore SecretStore,
	options RunnerOptions,
) *Runner {
	if options.QuietPeriod <= 0 {
		options.QuietPeriod = 10 * time.Minute
	}
	if options.PollInterval <= 0 {
		options.PollInterval = 5 * time.Second
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 20 * time.Second
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Runner{
		store:       store,
		secretStore: secretStore,
		options:     options,
		status: Status{
			Status: SyncStatusNotConfigured,
		},
	}
}

func (r *Runner) Running() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.started
}

func (r *Runner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := r.clientFor(ctx)
	if err != nil {
		r.setError(err)
		return
	}
	events, err := r.pendingEvents(ctx)
	if err != nil {
		r.setError(err)
		return
	}
	r.setPendingCount(countEligible(events))
	if client == nil {
		r.setNotConfigured()
		return
	}

	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	runContext, cancel := context.WithCancel(context.Background())
	r.started = true
	r.cancel = cancel
	r.done = make(chan struct{})
	done := r.done
	r.mu.Unlock()

	go r.loop(runContext, done)
}

func (r *Runner) Stop(ctx context.Context) error {
	if err := r.stopLoop(ctx); err != nil {
		return err
	}
	return r.SyncNow(ctx)
}

// StopWithoutFlush stops the background loop and leaves pending outbox rows
// for the next launch.
func (r *Runner) StopWithoutFlush(ctx context.Context) error {
	return r.stopLoop(ctx)
}

func (r *Runner) stopLoop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.RLock()
	cancel := r.cancel
	done := r.done
	r.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// HasUploadablePending reports whether a flush would send at least one row.
// Candidate, rejected, and study-review rows do not count.
func (r *Runner) HasUploadablePending(ctx context.Context) (bool, error) {
	if r == nil || r.store == nil {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	events, err := r.pendingEvents(ctx)
	if err != nil {
		return false, err
	}
	for _, group := range collapseEvents(eligibleEvents(events)) {
		needed, err := r.groupNeedsUpload(ctx, group)
		if err != nil {
			return false, err
		}
		if needed {
			return true, nil
		}
	}
	return false, nil
}

func (r *Runner) SyncNow(ctx context.Context) error {
	return r.process(ctx, true)
}

// RunOnce runs the normal quiet-period check without starting the background
// timer. It is useful to drive deterministic offline checks.
func (r *Runner) RunOnce(ctx context.Context) error {
	return r.process(ctx, false)
}

func (r *Runner) Status() Status {
	if r == nil {
		return Status{Status: SyncStatusNotConfigured}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneStatus(r.status)
}

func (r *Runner) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	r.pullWithTimeout(ctx)
	r.runWithTimeout(ctx, false)
	ticker := time.NewTicker(r.options.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runWithTimeout(ctx, false)
		}
	}
}

func (r *Runner) pullWithTimeout(ctx context.Context) {
	runContext, cancel := context.WithTimeout(ctx, r.options.RequestTimeout)
	defer cancel()
	_ = r.Pull(runContext)
}

// Pull reads confirmed vocabulary from Supabase into SQLite. SyncNow does not
// call it.
func (r *Runner) Pull(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.processMu.Lock()
	defer r.processMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	client, err := r.clientFor(ctx)
	if err != nil {
		r.setPullError(err)
		return err
	}
	if client == nil {
		return nil
	}
	if r.store == nil {
		err := errors.New("sync: storage is not configured")
		r.setPullError(err)
		return err
	}
	r.setPulling()
	entries, err := client.ListConfirmedVocabulary(ctx)
	if err != nil {
		r.setPullError(err)
		return err
	}
	result, err := r.store.Vocabulary().ApplyCloudVocabulary(ctx, entries)
	if err != nil {
		r.setPullError(err)
		return err
	}
	r.setPullIdle(result.Changes(), r.options.Now().UTC())
	return nil
}

func (r *Runner) runWithTimeout(ctx context.Context, force bool) {
	runContext, cancel := context.WithTimeout(ctx, r.options.RequestTimeout)
	defer cancel()
	_ = r.process(runContext, force)
}

func (r *Runner) process(ctx context.Context, force bool) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.processMu.Lock()
	defer r.processMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	client, err := r.clientFor(ctx)
	if err != nil {
		r.setError(err)
		return err
	}
	events, err := r.pendingEvents(ctx)
	if err != nil {
		r.setError(err)
		return err
	}
	eligible := eligibleEvents(events)
	r.setPendingCount(len(eligible))
	if client == nil {
		r.setNotConfigured()
		return nil
	}
	if len(eligible) == 0 {
		r.setIdle()
		return nil
	}

	newest := eligible[0].CreatedAt
	for _, event := range eligible[1:] {
		if event.CreatedAt.After(newest) {
			newest = event.CreatedAt
		}
	}
	now := r.options.Now().UTC()
	waitingUntil := newest.Add(r.options.QuietPeriod)
	wasError := r.Status().Status == SyncStatusError
	if !force && !wasError && now.Before(waitingUntil) {
		r.setWaiting(len(eligible), waitingUntil)
		return nil
	}

	groups := collapseEvents(eligible)
	r.setSyncing(len(eligible))
	for _, group := range groups {
		if err := r.processGroup(ctx, client, group); err != nil {
			_ = markFailed(ctx, r.store.Outbox(), group.events, err)
			pending, pendingErr := r.pendingEvents(ctx)
			if pendingErr == nil {
				r.setErrorWithPending(err, countEligible(pending))
			} else {
				r.setError(err)
			}
			return err
		}
		if err := markDelivered(ctx, r.store.Outbox(), group.events); err != nil {
			r.setError(err)
			return err
		}
	}

	pending, err := r.pendingEvents(ctx)
	if err != nil {
		r.setError(err)
		return err
	}
	r.setPendingCount(countEligible(pending))
	r.setLastSync(now)
	if countEligible(pending) == 0 {
		r.setIdle()
	} else {
		r.setSyncing(countEligible(pending))
	}
	return nil
}

func (r *Runner) processGroup(
	ctx context.Context,
	client *PostgRESTClient,
	group aggregateEvents,
) error {
	if group.latest.EventType == "vocabulary.entry.deleted" {
		return client.DeleteVocabularyEntry(ctx, group.id)
	}
	entry, err := r.store.Vocabulary().Get(ctx, group.id)
	if errors.Is(err, sql.ErrNoRows) {
		return client.DeleteVocabularyEntry(ctx, group.id)
	}
	if err != nil {
		return err
	}
	switch entry.Status {
	case domain.VocabularyStatusCandidate, domain.VocabularyStatusRejected:
		return nil
	case domain.VocabularyStatusMerged:
		return r.publishMergeTarget(ctx, client, entry.MergedIntoID)
	case domain.VocabularyStatusConfirmed:
		return client.PublishVocabularyEntry(ctx, entry)
	default:
		return errors.New("sync: unknown vocabulary status")
	}
}

func (r *Runner) groupNeedsUpload(ctx context.Context, group aggregateEvents) (bool, error) {
	if group.latest.EventType == "vocabulary.entry.deleted" {
		return true, nil
	}
	entry, err := r.store.Vocabulary().Get(ctx, group.id)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	switch entry.Status {
	case domain.VocabularyStatusConfirmed:
		return true, nil
	case domain.VocabularyStatusMerged:
		targetID := strings.TrimSpace(entry.MergedIntoID)
		if targetID == "" {
			return false, nil
		}
		target, err := r.store.Vocabulary().Get(ctx, targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return target.Status == domain.VocabularyStatusConfirmed || target.Manual, nil
	default:
		return false, nil
	}
}

func (r *Runner) publishMergeTarget(
	ctx context.Context,
	client *PostgRESTClient,
	targetID string,
) error {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil
	}
	target, err := r.store.Vocabulary().Get(ctx, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if target.Status != domain.VocabularyStatusConfirmed && !target.Manual {
		return nil
	}
	return client.PublishVocabularyEntry(ctx, target)
}

func (r *Runner) clientFor(ctx context.Context) (*PostgRESTClient, error) {
	if r.secretStore == nil {
		return nil, nil
	}
	secret, err := r.secretStore.Get(ctx, SupabaseSecretName)
	if err != nil || strings.TrimSpace(secret) == "" {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		r.mu.Lock()
		r.client = nil
		r.mu.Unlock()
		return nil, nil
	}
	secret = strings.TrimSpace(secret)
	r.mu.RLock()
	current := r.client
	r.mu.RUnlock()
	if current != nil && current.secret == secret {
		return current, nil
	}
	client, err := NewPostgRESTClient(r.options.BaseURL, secret, r.options.HTTPClient)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.client = client
	r.mu.Unlock()
	r.setConfigured()
	return client, nil
}

func (r *Runner) pendingEvents(ctx context.Context) ([]domain.OutboxEvent, error) {
	if r.store == nil {
		return nil, errors.New("sync: storage is not configured")
	}
	return r.store.Outbox().ListPendingVocabulary(ctx, 1_000_000)
}

func eligibleEvents(events []domain.OutboxEvent) []domain.OutboxEvent {
	eligible := make([]domain.OutboxEvent, 0, len(events))
	for _, event := range events {
		if event.EventType == "study.review.recorded" {
			continue
		}
		eligible = append(eligible, event)
	}
	return eligible
}

func countEligible(events []domain.OutboxEvent) int {
	return len(eligibleEvents(events))
}

type aggregateEvents struct {
	id     string
	events []domain.OutboxEvent
	latest domain.OutboxEvent
}

func collapseEvents(events []domain.OutboxEvent) []aggregateEvents {
	byID := make(map[string]*aggregateEvents, len(events))
	for _, event := range events {
		group, ok := byID[event.AggregateID]
		if !ok {
			group = &aggregateEvents{id: event.AggregateID, latest: event}
			byID[event.AggregateID] = group
		}
		group.events = append(group.events, event)
		if event.CreatedAt.After(group.latest.CreatedAt) ||
			(event.CreatedAt.Equal(group.latest.CreatedAt) && event.ID > group.latest.ID) {
			group.latest = event
		}
	}
	groups := make([]aggregateEvents, 0, len(byID))
	for _, group := range byID {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].latest.CreatedAt.Equal(groups[j].latest.CreatedAt) {
			return groups[i].id < groups[j].id
		}
		return groups[i].latest.CreatedAt.Before(groups[j].latest.CreatedAt)
	})
	return groups
}

func markDelivered(
	ctx context.Context,
	outbox interface {
		MarkDelivered(context.Context, string) error
	},
	events []domain.OutboxEvent,
) error {
	for _, event := range events {
		if err := outbox.MarkDelivered(ctx, event.ID); err != nil {
			return err
		}
	}
	return nil
}

func markFailed(
	ctx context.Context,
	outbox interface {
		MarkFailed(context.Context, string, error) error
	},
	events []domain.OutboxEvent,
	failure error,
) error {
	var firstErr error
	for _, event := range events {
		if err := outbox.MarkFailed(ctx, event.ID, failure); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *Runner) setNotConfigured() {
	r.updateStatus(func(status *Status) {
		status.Configured = false
		status.Status = SyncStatusNotConfigured
		status.WaitingUntil = nil
		status.LastSyncError = ""
	})
}

func (r *Runner) setConfigured() {
	r.updateStatus(func(status *Status) {
		status.Configured = true
		if status.Status == SyncStatusNotConfigured {
			status.Status = SyncStatusIdle
		}
	})
}

func (r *Runner) setPendingCount(count int) {
	r.updateStatus(func(status *Status) {
		status.PendingCount = count
	})
}

func (r *Runner) setWaiting(count int, waitingUntil time.Time) {
	r.updateStatus(func(status *Status) {
		status.Configured = true
		status.PendingCount = count
		status.Status = SyncStatusWaiting
		status.WaitingUntil = timePtr(waitingUntil)
	})
}

func (r *Runner) setSyncing(count int) {
	r.updateStatus(func(status *Status) {
		status.Configured = true
		status.PendingCount = count
		status.Status = SyncStatusSyncing
		status.WaitingUntil = nil
	})
}

func (r *Runner) setIdle() {
	r.updateStatus(func(status *Status) {
		status.Status = SyncStatusIdle
		status.WaitingUntil = nil
		status.LastSyncError = ""
	})
}

func (r *Runner) setLastSync(at time.Time) {
	r.updateStatus(func(status *Status) {
		status.LastSyncAt = timePtr(at)
		status.LastSyncError = ""
	})
}

func (r *Runner) setPulling() {
	r.updateStatus(func(status *Status) {
		status.Configured = true
		status.PullStatus = PullStatusPulling
		status.PullError = ""
	})
}

func (r *Runner) setPullIdle(changes int, at time.Time) {
	r.updateStatus(func(status *Status) {
		status.Configured = true
		status.PullStatus = PullStatusIdle
		status.PullChangeCount = changes
		status.LastPullAt = timePtr(at)
		status.PullError = ""
	})
}

func (r *Runner) setPullError(err error) {
	r.updateStatus(func(status *Status) {
		status.PullStatus = PullStatusError
		status.PullChangeCount = 0
		status.LastPullAt = timePtr(r.options.Now().UTC())
		if err != nil {
			status.PullError = err.Error()
		}
	})
}

func (r *Runner) setError(err error) {
	r.setErrorWithPending(err, r.Status().PendingCount)
}

func (r *Runner) setErrorWithPending(err error, pending int) {
	r.updateStatus(func(status *Status) {
		status.Status = SyncStatusError
		status.PendingCount = pending
		status.WaitingUntil = nil
		if err != nil {
			status.LastSyncError = err.Error()
		}
	})
}

func (r *Runner) updateStatus(update func(*Status)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	next := cloneStatus(r.status)
	update(&next)
	if next.Status == "" {
		next.Status = SyncStatusNotConfigured
	}
	changed := !sameStatus(r.status, next)
	r.status = next
	callback := r.options.OnStatusChange
	r.mu.Unlock()
	if changed && callback != nil {
		callback(cloneStatus(next))
	}
}

func cloneStatus(status Status) Status {
	return Status{
		Configured:      status.Configured,
		PendingCount:    status.PendingCount,
		Status:          status.Status,
		WaitingUntil:    cloneTime(status.WaitingUntil),
		LastSyncAt:      cloneTime(status.LastSyncAt),
		LastSyncError:   status.LastSyncError,
		PullStatus:      status.PullStatus,
		PullChangeCount: status.PullChangeCount,
		LastPullAt:      cloneTime(status.LastPullAt),
		PullError:       status.PullError,
	}
}

func sameStatus(left, right Status) bool {
	return left.Configured == right.Configured &&
		left.PendingCount == right.PendingCount &&
		left.Status == right.Status &&
		sameTime(left.WaitingUntil, right.WaitingUntil) &&
		sameTime(left.LastSyncAt, right.LastSyncAt) &&
		left.LastSyncError == right.LastSyncError &&
		left.PullStatus == right.PullStatus &&
		left.PullChangeCount == right.PullChangeCount &&
		sameTime(left.LastPullAt, right.LastPullAt) &&
		left.PullError == right.PullError
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func sameTime(left, right *time.Time) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return left.Equal(*right)
	}
}

func timePtr(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}
