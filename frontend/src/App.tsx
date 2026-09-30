import { useCallback, useEffect, useMemo, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { api as defaultApi } from './lib/api'
import type { AppApi } from './lib/api'
import type {
  AppSnapshot,
  Candidate,
  CandidateBucket,
  CandidateEditPatch,
  ManualVocabularyDraft,
  ManualVocabularyInput,
  SettingsPatch,
  TargetApp,
  TranscriptSource,
  VocabularyStatus,
} from './lib/types'
import './styles.css'

type View = 'live' | 'inbox' | 'add-word' | 'vocabulary' | 'settings'

interface AppProps {
  apiClient?: AppApi
}

const EMPTY_SNAPSHOT: AppSnapshot = {
  connection: 'unavailable',
  capturePermission: 'unknown',
  readiness: { checked: false, targetAvailable: false },
  target: 'zoom',
  lesson: { status: 'idle' },
  levels: { mic: 0, remote: 0 },
  transcript: [],
  candidates: [],
  vocabulary: [],
  cost: {
    currentUsd: 0,
    projectedUsd: 0,
    hardBudgetUsd: 0.5,
    currency: 'USD',
    warning: false,
    hardExceeded: false,
  },
  privacy: {
    zdrEnabled: true,
    audioRetention: 'sessionOnly',
  },
  settings: {
    openRouterKeyConfigured: false,
    sttModel: 'qwen/qwen3-asr-1.7b',
    analyzerModel: 'qwen/qwen3.8-flash',
    hardBudgetUsd: 0.5,
    audioRetention: 'sessionOnly',
    ocrEnabled: false,
    microphoneEnabled: true,
  },
  lastUpdated: '',
}

const NAV_ITEMS: Array<{ id: View; label: string; description: string; icon: IconName }> = [
  { id: 'live', label: 'Live Lesson', description: 'Capture and review a live call', icon: 'waveform' },
  { id: 'inbox', label: 'Candidate Inbox', description: 'Review new vocabulary', icon: 'inbox' },
  { id: 'add-word', label: 'Add Word', description: 'Create a vocabulary entry', icon: 'spark' },
  { id: 'vocabulary', label: 'Vocabulary', description: 'Browse your saved words', icon: 'book' },
  { id: 'settings', label: 'Settings', description: 'Models, privacy, and access', icon: 'settings' },
]

const CANDIDATE_SECTIONS: Array<{
  bucket: CandidateBucket
  title: string
  description: string
}> = [
  { bucket: 'manual', title: 'Manual marks', description: 'Moments you flagged during a lesson.' },
  { bucket: 'highConfidence', title: 'High confidence', description: 'Strong candidates ready for a quick review.' },
  { bucket: 'possibleDuplicate', title: 'Possible duplicates', description: 'Candidates that may already be in your list.' },
  { bucket: 'lowConfidence', title: 'Low confidence', description: 'Useful leads that need a closer look.' },
]

const STATUS_OPTIONS: Array<{ value: VocabularyStatus | 'all'; label: string }> = [
  { value: 'all', label: 'All statuses' },
  { value: 'learning', label: 'Learning' },
  { value: 'review', label: 'Review' },
  { value: 'mastered', label: 'Mastered' },
]

const STT_MODEL_OPTIONS = [
  { value: 'qwen/qwen3-asr-1.7b', label: 'Qwen 3 ASR 1.7B' },
  { value: 'qwen/qwen3-asr-0.6b', label: 'Qwen 3 ASR 0.6B' },
  { value: 'openai/whisper-large-v3-turbo', label: 'OpenAI Whisper Large v3 Turbo' },
  { value: 'microsoft/mai-transcribe-2', label: 'Microsoft MAI Transcribe 2' },
  { value: 'mistralai/voxtral-mini-3b-2507', label: 'Mistral Voxtral Mini 3B' },
] as const

const ANALYZER_MODEL_OPTIONS = [
  { value: 'qwen/qwen3.8-flash', label: 'Qwen 3.8 Flash' },
  { value: 'qwen/qwen3.8-max-0902', label: 'Qwen 3.8 Max' },
] as const

const DEFAULT_STT_MODEL = STT_MODEL_OPTIONS[0].value
const DEFAULT_ANALYZER_MODEL = ANALYZER_MODEL_OPTIONS[0].value

type CandidateEditDraft = {
  simplified: string
  traditional: string
  pinyin: string
  meaning: string
  partOfSpeech: string
  classifier: string
  example: string
  examplePinyin: string
  exampleTranslation: string
  tags: string
}

type ManualVocabularyForm = {
  simplified: string
  traditional: string
  pinyin: string
  meaning: string
  partOfSpeech: string
  classifier: string
  example: string
  examplePinyin: string
  exampleTranslation: string
  tags: string
  aiGenerated: boolean
}

const EMPTY_MANUAL_VOCABULARY_FORM: ManualVocabularyForm = {
  simplified: '',
  traditional: '',
  pinyin: '',
  meaning: '',
  partOfSpeech: '',
  classifier: '',
  example: '',
  examplePinyin: '',
  exampleTranslation: '',
  tags: '',
  aiGenerated: false,
}

export function App({ apiClient = defaultApi }: AppProps) {
  const [activeView, setActiveView] = useState<View>('live')
  const [snapshot, setSnapshot] = useState<AppSnapshot>()
  const [isLoading, setIsLoading] = useState(true)
  const [loadError, setLoadError] = useState<string>()
  const [actionError, setActionError] = useState<string>()
  const [actionMessage, setActionMessage] = useState<string>()
  const [busyAction, setBusyAction] = useState<string>()
  const [consent, setConsent] = useState(false)
  const [selectedTarget, setSelectedTarget] = useState<TargetApp>('zoom')
  const [editingCandidate, setEditingCandidate] = useState<Candidate>()
  const [mergeCandidate, setMergeCandidate] = useState<Candidate>()
  const [editDraft, setEditDraft] = useState<CandidateEditDraft>({
    simplified: '',
    traditional: '',
    pinyin: '',
    meaning: '',
    partOfSpeech: '',
    classifier: '',
    example: '',
    examplePinyin: '',
    exampleTranslation: '',
    tags: '',
  })
  const [editError, setEditError] = useState<string>()
  const [manualForm, setManualForm] = useState<ManualVocabularyForm>(EMPTY_MANUAL_VOCABULARY_FORM)
  const [manualGeneration, setManualGeneration] = useState<ManualVocabularyDraft>()

  const current = snapshot ?? EMPTY_SNAPSHOT
  const backendUnavailable = apiClient.mode === 'unavailable' || Boolean(loadError)

  useEffect(() => {
    let active = true
    setIsLoading(true)
    setLoadError(undefined)

    const unsubscribe = apiClient.subscribe((nextSnapshot) => {
      if (active) {
        setSnapshot(nextSnapshot)
        setSelectedTarget(nextSnapshot.target)
      }
    })

    void apiClient
      .getSnapshot()
      .then((nextSnapshot) => {
        if (!active) return
        setSnapshot(nextSnapshot)
        setSelectedTarget(nextSnapshot.target)
      })
      .catch((error: unknown) => {
        if (active) setLoadError(getErrorMessage(error))
      })
      .finally(() => {
        if (active) setIsLoading(false)
      })

    return () => {
      active = false
      unsubscribe()
    }
  }, [apiClient])

  const runAction = useCallback(
    async (name: string, action: () => Promise<AppSnapshot>, successMessage: string) => {
      setBusyAction(name)
      setActionError(undefined)
      setActionMessage(undefined)
      try {
        const nextSnapshot = await action()
        setSnapshot(nextSnapshot)
        setSelectedTarget(nextSnapshot.target)
        setActionMessage(successMessage)
        return true
      } catch (error: unknown) {
        setActionError(getErrorMessage(error))
        return false
      } finally {
        setBusyAction(undefined)
      }
    },
    [],
  )

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      const isTyping = target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target?.isContentEditable
      if (isTyping || event.defaultPrevented || event.key.toLowerCase() !== 'm') return
      if (current.lesson.status !== 'live' || busyAction) return
      event.preventDefault()
      void runAction('mark-moment', () => apiClient.markMoment(), 'Moment marked. It is waiting in Candidate Inbox.')
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [apiClient, busyAction, current.lesson.status, runAction])

  useEffect(() => {
    if (!editingCandidate && !mergeCandidate) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || busyAction) return
      event.preventDefault()
      setEditingCandidate(undefined)
      setMergeCandidate(undefined)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [busyAction, editingCandidate, mergeCandidate])

  const handleStart = () => {
    void runAction(
      'start-lesson',
      () => apiClient.startLesson({ target: selectedTarget, consent }),
      'Lesson started. Your transcript will appear below.',
    )
  }

  const handleStop = () => {
    void runAction(
      'stop-lesson',
      () => apiClient.stopLesson(),
      'Lesson stopped. Reconciliation and finalization completed.',
    )
  }

  const handleRefresh = () => {
    void runAction('refresh', () => apiClient.refresh(), 'Data refreshed.')
  }

  const handlePermission = () => {
    void runAction('permission', () => apiClient.requestCapturePermission(), 'Capture permission status updated.')
  }

  const openEdit = (candidate: Candidate) => {
    setEditingCandidate(candidate)
    setEditDraft({
      simplified: candidate.simplified,
      traditional: candidate.traditional ?? '',
      pinyin: candidate.pinyin,
      meaning: candidate.meaning,
      partOfSpeech: candidate.partOfSpeech ?? '',
      classifier: candidate.classifier ?? '',
      example: candidate.example,
      examplePinyin: candidate.examplePinyin ?? '',
      exampleTranslation: candidate.exampleTranslation,
      tags: candidate.tags.join(', '),
    })
    setEditError(undefined)
    setActionError(undefined)
  }

  const saveEdit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!editingCandidate) return
    const requiredFields: Array<[keyof CandidateEditDraft, string]> = [
      ['simplified', 'Hanzi'],
      ['pinyin', 'pinyin'],
      ['meaning', 'meaning'],
      ['example', 'sample sentence'],
      ['examplePinyin', 'sample pinyin'],
      ['exampleTranslation', 'translation'],
    ]
    const missingFields = requiredFields
      .filter(([field]) => !editDraft[field].trim())
      .map(([, label]) => label)
    if (missingFields.length > 0) {
      setEditError(`Complete the required fields: ${missingFields.join(', ')}.`)
      return
    }
    setEditError(undefined)
    const patch: CandidateEditPatch = {
      simplified: editDraft.simplified.trim(),
      traditional: editDraft.traditional.trim() || undefined,
      pinyin: editDraft.pinyin.trim(),
      meaning: editDraft.meaning.trim(),
      partOfSpeech: editDraft.partOfSpeech.trim(),
      classifier: editDraft.classifier.trim(),
      example: editDraft.example.trim(),
      examplePinyin: editDraft.examplePinyin.trim(),
      exampleTranslation: editDraft.exampleTranslation.trim(),
      tags: parseTags(editDraft.tags),
    }
    void runAction(
      `edit-${editingCandidate.id}`,
      () => apiClient.editCandidate(editingCandidate.id, patch),
      `${editingCandidate.simplified} updated.`,
    ).then((didSave) => {
      if (didSave) setEditingCandidate(undefined)
    })
  }

  const confirmCandidate = (candidate: Candidate) => {
    void runAction(
      `confirm-${candidate.id}`,
      () => apiClient.confirmCandidate(candidate.id),
      `${candidate.simplified} added to Vocabulary.`,
    )
  }

  const rejectCandidate = (candidate: Candidate) => {
    void runAction(
      `reject-${candidate.id}`,
      () => apiClient.rejectCandidate(candidate.id),
      `${candidate.simplified} rejected.`,
    )
  }

  const mergeCandidates = () => {
    if (!mergeCandidate?.duplicateOf) return
    const target = current.candidates.find((candidate) => candidate.id === mergeCandidate.duplicateOf)
    if (!target) {
      setActionError('The original candidate is no longer available.')
      setMergeCandidate(undefined)
      return
    }
    void runAction(
      `merge-${mergeCandidate.id}`,
      () => apiClient.mergeCandidates(mergeCandidate.id, target.id),
      `${mergeCandidate.simplified} merged with ${target.simplified}.`,
    ).then(() => setMergeCandidate(undefined))
  }

  const saveSettings = (patch: SettingsPatch) => {
    const successMessage = patch.openRouterKey === ''
      ? 'API key cleared. Settings saved securely.'
      : 'Settings saved securely.'
    void runAction('save-settings', () => apiClient.saveSettings(patch), successMessage)
  }

  const updateManualForm = (nextForm: ManualVocabularyForm) => {
    setManualForm(nextForm)
    setActionError(undefined)
  }

  const handleGenerateManualVocabulary = () => {
    const input = manualVocabularyFormToInput(manualForm)
    if (!input.simplified.trim()) {
      setActionError('Enter a Chinese word (simplified) before generating details.')
      setActionMessage(undefined)
      return
    }
    if (!current.settings.openRouterKeyConfigured) {
      setActionError('Add an OpenRouter API key in Settings before generating missing details.')
      setActionMessage(undefined)
      return
    }
    if (hasCompleteManualVocabularyForm(manualForm)) {
      setActionError(undefined)
      setActionMessage('All learning details are already complete; generation is not needed.')
      return
    }
    setBusyAction('generate-manual-vocabulary')
    setActionError(undefined)
    setActionMessage(undefined)
    void apiClient
      .generateManualVocabulary(input)
      .then((draft) => {
        setManualForm(manualVocabularyDraftToForm(draft))
        setManualGeneration(draft)
        setActionMessage('Missing details generated. Review every field before saving.')
      })
      .catch((error: unknown) => {
        setActionError(getErrorMessage(error))
      })
      .finally(() => {
        setBusyAction(undefined)
      })
  }

  const handleSaveManualVocabulary = (event?: FormEvent<HTMLFormElement>) => {
    event?.preventDefault()
    const missingFields = manualVocabularyMissingFields(manualForm)
    if (missingFields.length > 0) {
      setActionError(`Complete the required details before saving: ${missingFields.join(', ')}.`)
      setActionMessage(undefined)
      return
    }
    const input = manualVocabularyFormToInput(manualForm)
    void runAction(
      'save-manual-vocabulary',
      () => apiClient.saveManualVocabulary(input),
      `${input.simplified} saved to Vocabulary.`,
    ).then((didSave) => {
      if (!didSave) return
      setManualForm({ ...EMPTY_MANUAL_VOCABULARY_FORM })
      setManualGeneration(undefined)
      setActiveView('vocabulary')
    })
  }

  const handleDeleteLastLesson = () => {
    if (['live', 'starting', 'stopping'].includes(current.lesson.status) || !current.lesson.id || busyAction) return
    if (
      !window.confirm(
        'Delete the last lesson data? This removes its transcript, evidence, and any unconfirmed suggestions supported only by that lesson. Confirmed vocabulary is kept without the deleted evidence.',
      )
    ) {
      return
    }
    void runAction(
      'delete-last-lesson',
      () => apiClient.deleteLastLesson(),
      'Last lesson data deleted. Transcript, linked evidence, and orphan suggestions were removed; confirmed vocabulary was kept.',
    )
  }

  return (
    <div className="app-shell">
      <Sidebar
        activeView={activeView}
        onNavigate={setActiveView}
        mode={apiClient.mode}
        zdrEnabled={current.privacy.zdrEnabled}
      />
      <main className="main-content">
        <TopBar
          activeView={activeView}
          connection={current.connection}
          mode={apiClient.mode}
          isRefreshing={busyAction === 'refresh'}
          onRefresh={handleRefresh}
        />
        <div className="page-content">
          {isLoading && !snapshot ? (
            <LoadingState />
          ) : (
            <>
              {(backendUnavailable || apiClient.mode === 'demo') && (
                <BackendUnavailable mode={apiClient.mode} onRetry={handleRefresh} />
              )}
              {actionError && (
                <InlineAlert tone="error" onDismiss={() => setActionError(undefined)}>
                  {actionError}
                </InlineAlert>
              )}
              {actionMessage && (
                <InlineAlert tone="success" onDismiss={() => setActionMessage(undefined)}>
                  {actionMessage}
                </InlineAlert>
              )}
              {activeView === 'live' && (
                <LiveLessonView
                  snapshot={current}
                  consent={consent}
                  selectedTarget={selectedTarget}
                  busyAction={busyAction}
                  onConsentChange={setConsent}
                  onTargetChange={setSelectedTarget}
                  onStart={handleStart}
                  onStop={handleStop}
                  onOpenSettings={() => setActiveView('settings')}
                  onMarkMoment={() =>
                    void runAction(
                      'mark-moment',
                      () => apiClient.markMoment(),
                      'Moment marked. It is waiting in Candidate Inbox.',
                    )
                  }
                  onPermission={handlePermission}
                />
              )}
              {activeView === 'inbox' && (
                <CandidateInboxView
                  candidates={current.candidates}
                  busyAction={busyAction}
                  onConfirm={confirmCandidate}
                  onEdit={openEdit}
                  onReject={rejectCandidate}
                  onMerge={setMergeCandidate}
                />
              )}
              {activeView === 'add-word' && (
                <ManualVocabularyView
                  form={manualForm}
                  generatedDraft={manualGeneration}
                  hasApiKey={current.settings.openRouterKeyConfigured}
                  lessonActive={['live', 'starting', 'stopping'].includes(current.lesson.status)}
                  busyAction={busyAction}
                  onFormChange={updateManualForm}
                  onGenerate={handleGenerateManualVocabulary}
                  onSave={handleSaveManualVocabulary}
                  onOpenSettings={() => setActiveView('settings')}
                />
              )}
              {activeView === 'vocabulary' && (
                <VocabularyView
                  entries={current.vocabulary}
                  onRefresh={handleRefresh}
                  isRefreshing={busyAction === 'refresh'}
                />
              )}
              {activeView === 'settings' && (
                <SettingsView
                  snapshot={current}
                  apiMode={apiClient.mode}
                  busyAction={busyAction}
                  onSave={saveSettings}
                  onDeleteLastLesson={handleDeleteLastLesson}
                />
              )}
            </>
          )}
        </div>
      </main>
      {editingCandidate && (
        <EditCandidateDialog
          candidate={editingCandidate}
          draft={editDraft}
          validationError={editError}
          isSaving={busyAction === `edit-${editingCandidate.id}`}
          onDraftChange={(draft) => {
            setEditDraft(draft)
            setEditError(undefined)
          }}
          onCancel={() => setEditingCandidate(undefined)}
          onSubmit={saveEdit}
        />
      )}
      {mergeCandidate && (
        <MergeDialog
          source={mergeCandidate}
          target={current.candidates.find((candidate) => candidate.id === mergeCandidate.duplicateOf)}
          isMerging={busyAction === `merge-${mergeCandidate.id}`}
          onCancel={() => setMergeCandidate(undefined)}
          onConfirm={mergeCandidates}
        />
      )}
    </div>
  )
}

function Sidebar({
  activeView,
  onNavigate,
  mode,
  zdrEnabled,
}: {
  activeView: View
  onNavigate: (view: View) => void
  mode: AppApi['mode']
  zdrEnabled: boolean
}) {
  return (
    <aside className="sidebar">
      <div className="brand-lockup">
        <div className="brand-mark" aria-hidden="true">
          <span>文</span>
        </div>
        <div>
          <strong>Call Analyzer</strong>
          <span>Chinese learning desk</span>
        </div>
      </div>
      <nav className="primary-nav" aria-label="Primary navigation">
        <p className="eyebrow nav-eyebrow">Workspace</p>
        {NAV_ITEMS.map((item) => (
          <button
            className={`nav-item ${activeView === item.id ? 'is-active' : ''}`}
            key={item.id}
            type="button"
            aria-label={item.label}
            aria-current={activeView === item.id ? 'page' : undefined}
            onClick={() => onNavigate(item.id)}
          >
            <Icon name={item.icon} />
            <span>
              <strong>{item.label}</strong>
              <small>{item.description}</small>
            </span>
          </button>
        ))}
      </nav>
      <div className="sidebar-footer">
        <div className="privacy-mini">
          <span className="status-dot status-dot-green" aria-hidden="true" />
          <span>
            <strong>{zdrEnabled ? 'Zero data retention' : 'Retention enabled'}</strong>
            <small>{mode === 'demo' ? 'Demo adapter active' : 'Privacy-first capture'}</small>
          </span>
        </div>
        <div className="sidebar-version">Local desktop workspace</div>
      </div>
    </aside>
  )
}

function TopBar({
  activeView,
  connection,
  mode,
  isRefreshing,
  onRefresh,
}: {
  activeView: View
  connection: AppSnapshot['connection']
  mode: AppApi['mode']
  isRefreshing: boolean
  onRefresh: () => void
}) {
  const page = NAV_ITEMS.find((item) => item.id === activeView) ?? NAV_ITEMS[0]
  const status = getConnectionStatus(connection, mode)
  return (
    <header className="topbar">
      <div className="breadcrumb">
        <span>Workspace</span>
        <span aria-hidden="true">/</span>
        <strong>{page.label}</strong>
      </div>
      <div className="topbar-actions">
        <span className={`connection-status connection-${status.tone}`}>
          <span className="status-dot" aria-hidden="true" />
          {status.label}
        </span>
        <button className="icon-button" type="button" aria-label="Refresh data" onClick={onRefresh} disabled={isRefreshing}>
          <Icon name="refresh" />
        </button>
        <div className="avatar" aria-label="Local workspace">LW</div>
      </div>
    </header>
  )
}

function LiveLessonView({
  snapshot,
  consent,
  selectedTarget,
  busyAction,
  onConsentChange,
  onTargetChange,
  onStart,
  onStop,
  onOpenSettings,
  onMarkMoment,
  onPermission,
}: {
  snapshot: AppSnapshot
  consent: boolean
  selectedTarget: TargetApp
  busyAction?: string
  onConsentChange: (value: boolean) => void
  onTargetChange: (value: TargetApp) => void
  onStart: () => void
  onStop: () => void
  onOpenSettings: () => void
  onMarkMoment: () => void
  onPermission: () => void
}) {
  const isLive = snapshot.lesson.status === 'live'
  const isInterrupted = snapshot.lesson.status === 'error' && Boolean(snapshot.lesson.id)
  const hasActiveLesson = isLive || isInterrupted
  const isStarting = busyAction === 'start-lesson'
  const isStopping = busyAction === 'stop-lesson'
  const canInteract = snapshot.connection !== 'unavailable'
  const targetSupported = selectedTarget === 'zoom'
  const hasApiKey = snapshot.settings.openRouterKeyConfigured
  const captureReady = snapshot.capturePermission === 'granted'
  const zoomReady = snapshot.readiness.checked && snapshot.readiness.targetAvailable
  const setupReady = hasApiKey && captureReady && zoomReady
  const pendingCandidates = snapshot.candidates.filter((candidate) => candidate.status === 'pending').length

  return (
    <div className="view-stack">
      <section className="page-heading">
        <div>
          <p className="eyebrow">Capture room</p>
          <h1>Live Lesson</h1>
          <p className="page-intro">
            Turn a real conversation into useful Chinese vocabulary without losing the thread.
          </p>
        </div>
        <div className="heading-actions">
          <PrivacyBadge />
          <span className="soft-badge">
            <span className="status-dot status-dot-blue" aria-hidden="true" />
            {pendingCandidates} candidates
          </span>
        </div>
      </section>

      {(snapshot.cost.warning || snapshot.cost.hardExceeded) && <CostStatusAlert cost={snapshot.cost} />}

      <div className="live-grid">
        <section className="panel capture-panel">
          <PanelHeading
            eyebrow="Capture setup"
            title="Choose your conversation"
            description="Provider retention follows ZDR. Local audio follows the retention policy below."
          />
          <div className="field-group">
            <label className="field-label" htmlFor="target-app">
              Target app
            </label>
            <div className="select-with-icon">
              <select
                id="target-app"
                value={selectedTarget}
                onChange={(event) => onTargetChange(event.target.value as TargetApp)}
                disabled={hasActiveLesson || !canInteract}
              >
                <option value="zoom">Zoom</option>
                <option value="googleMeet" disabled>Google Meet (coming later)</option>
                <option value="teams" disabled>Microsoft Teams (coming later)</option>
              </select>
            </div>
          </div>
          <div className="permission-row">
            <div className="permission-icon">
              <Icon name={snapshot.capturePermission === 'granted' ? 'check' : 'mic'} />
            </div>
            <div>
              <strong>Screen and system audio permission</strong>
              <p>
                {snapshot.capturePermission === 'granted'
                  ? 'Screen and system audio access is granted. This does not grant microphone access; microphone permission may be requested at lesson start when enabled.'
                  : snapshot.capturePermission === 'denied'
                    ? 'Screen and system audio permission was denied. You can try again in System Settings.'
                    : 'Allow screen and system audio access. Microphone permission is separate and may be requested at lesson start when enabled.'}
              </p>
            </div>
          </div>
          <div className="level-grid">
            <AudioMeter label="Your microphone" value={snapshot.levels.mic} color="blue" />
            <AudioMeter label="Remote audio" value={snapshot.levels.remote} color="purple" />
          </div>
          <div className="capture-note">
            <Icon name="shield" />
            <span>ZDR controls provider retention. Local audio follows the selected retention policy.</span>
          </div>
        </section>

        <section className={`panel lesson-panel ${hasActiveLesson ? 'is-live' : ''}`}>
          <div className="lesson-panel-top">
            <div>
              <p className="eyebrow">Lesson control</p>
              <h2>{isLive ? 'Lesson in progress' : isInterrupted ? 'Capture interrupted' : 'Ready when you are'}</h2>
            </div>
            <LessonStatus status={snapshot.lesson.status} />
          </div>
          {snapshot.lesson.error && <InlineAlert tone="error">{snapshot.lesson.error}</InlineAlert>}
          {!hasActiveLesson ? (
            <>
              <div className="consent-box">
                <input
                  id="lesson-consent"
                  type="checkbox"
                  checked={consent}
                  onChange={(event) => onConsentChange(event.target.checked)}
                  disabled={!canInteract}
                />
                <label htmlFor="lesson-consent">
                  I confirm that all participants have consented to capturing this conversation for this lesson and understand that the transcript is processed for vocabulary suggestions.
                </label>
              </div>
              <div className="setup-checklist" aria-label="Startup checks">
                <div className="setup-checklist-heading">
                  <div>
                    <strong>Startup checks</strong>
                    <p>Run these checks again after opening Zoom or changing macOS permissions.</p>
                  </div>
                  <button className="button button-secondary button-small" type="button" onClick={onPermission} disabled={!canInteract || Boolean(busyAction)}>
                    {busyAction === 'permission' ? 'Checking…' : 'Check setup'}
                  </button>
                </div>
                <SetupCheck label="OpenRouter API key" detail={hasApiKey ? 'Saved securely' : 'Add a key in Settings'} status={hasApiKey ? 'ready' : 'blocked'} />
                <SetupCheck
                  label="Screen and system audio"
                  detail={captureReady ? 'Permission granted' : snapshot.capturePermission === 'denied' ? 'Permission denied' : 'Not checked'}
                  status={captureReady ? 'ready' : snapshot.capturePermission === 'denied' ? 'blocked' : 'pending'}
                />
                <SetupCheck
                  label="Zoom"
                  detail={zoomReady
                    ? 'Open and available to capture'
                    : !snapshot.readiness.checked
                    ? 'Not checked'
                    : !captureReady
                    ? 'Waiting for capture access'
                    : 'Zoom is not open'}
                  status={zoomReady ? 'ready' : snapshot.readiness.checked && captureReady ? 'blocked' : 'pending'}
                />
                <SetupCheck label="Participant consent" detail={consent ? 'Confirmed' : 'Confirmation required'} status={consent ? 'ready' : 'pending'} />
                {snapshot.readiness.error && (
                  <div className="setup-check-error" role="alert">
                    <Icon name="warning" />
                    <span>{snapshot.readiness.error}</span>
                  </div>
                )}
                {!hasApiKey && (
                  <button className="button button-secondary button-small setup-settings-button" type="button" onClick={onOpenSettings}>
                    Open Settings
                  </button>
                )}
              </div>
              <button className="button button-primary start-button" type="button" onClick={onStart} disabled={!consent || !canInteract || !targetSupported || !setupReady || isStarting}>
                <Icon name="play" />
                {isStarting ? 'Starting lesson…' : 'Start lesson'}
              </button>
              {!hasApiKey
                ? <p className="helper-text">Configure an API key in Settings to enable lesson capture.</p>
                : !targetSupported
                ? <p className="helper-text">Zoom is the only supported target in this macOS MVP.</p>
                : !setupReady
                ? <p className="helper-text">Complete the startup checks before beginning capture.</p>
                : !consent && <p className="helper-text">Consent is required before capture can begin.</p>}
            </>
          ) : (
            <>
              <div className="live-clock">
                <span className="live-pulse" aria-hidden="true" />
                <span>{isInterrupted ? 'Capture stopped unexpectedly' : `Capturing ${formatStartedAt(snapshot.lesson.startedAt)}`}</span>
              </div>
              <button className="button button-danger start-button" type="button" onClick={onStop} disabled={isStopping}>
                <Icon name="stop" />
                {isStopping ? 'Finishing lesson…' : isInterrupted ? 'Finish lesson' : 'Stop lesson'}
              </button>
            </>
          )}
          <div className="shortcut-row">
            <span>Quick mark</span>
            <span className="shortcut-hint"><kbd>M</kbd> Mark a moment</span>
          </div>
        </section>
      </div>

      <section className="panel transcript-panel">
        <div className="panel-heading-row">
          <PanelHeading
            eyebrow="Live transcript"
            title="Conversation thread"
            description="Source labels help you notice what to revisit."
          />
          <button className="button button-mark" type="button" onClick={onMarkMoment} disabled={!isLive || Boolean(busyAction)}>
            <Icon name="bookmark" />
            Mark Moment
            <kbd>M</kbd>
          </button>
        </div>
        {snapshot.transcript.length > 0 ? (
          <div className="transcript-list" aria-live="polite">
            {snapshot.transcript.map((line) => (
              <TranscriptRow key={line.id} line={line} />
            ))}
          </div>
        ) : (
          <EmptyState icon="waveform" title="Your transcript will appear here" description="Start a lesson to see microphone and remote audio in one rolling thread." />
        )}
      </section>

      <div className="metric-grid">
        <MetricCard
          label="Candidate queue"
          value={String(pendingCandidates)}
          detail="Waiting for review"
          icon="inbox"
          tone="blue"
        />
        <MetricCard
          label="Current cost"
          value={formatUsd(snapshot.cost.currentUsd)}
          detail={`Projected ${formatUsd(snapshot.cost.projectedUsd)}`}
          icon="coin"
          tone="purple"
        />
        <MetricCard
          label="Hard budget"
          value={formatUsd(snapshot.cost.hardBudgetUsd)}
          detail="Session cap"
          icon="shield"
          tone="green"
        />
        <MetricCard
          label="Privacy mode"
          value={snapshot.privacy.zdrEnabled ? 'ZDR on' : 'Retention on'}
          detail={retentionLabel(snapshot.privacy.audioRetention)}
          icon="lock"
          tone="orange"
        />
      </div>
    </div>
  )
}

function CandidateInboxView({
  candidates,
  busyAction,
  onConfirm,
  onEdit,
  onReject,
  onMerge,
}: {
  candidates: Candidate[]
  busyAction?: string
  onConfirm: (candidate: Candidate) => void
  onEdit: (candidate: Candidate) => void
  onReject: (candidate: Candidate) => void
  onMerge: (candidate: Candidate) => void
}) {
  const pendingCount = candidates.filter((candidate) => candidate.status === 'pending').length
  return (
    <div className="view-stack">
      <section className="page-heading">
        <div>
          <p className="eyebrow">Review workspace</p>
          <h1>Candidate Inbox</h1>
          <p className="page-intro">A calm review pass keeps your vocabulary focused and useful.</p>
        </div>
        <div className="heading-summary">
          <strong>{pendingCount}</strong>
          <span>pending review</span>
        </div>
      </section>
      {pendingCount === 0 ? (
        <section className="panel">
          <EmptyState icon="check" title="Inbox is clear" description="New candidates from your next lesson will appear here." />
        </section>
      ) : (
        <div className="candidate-sections">
          {CANDIDATE_SECTIONS.map((section) => {
            const sectionCandidates = candidates.filter(
              (candidate) => candidate.bucket === section.bucket && candidate.status === 'pending',
            )
            if (sectionCandidates.length === 0) return null
            return (
              <section className="candidate-section" key={section.bucket}>
                <div className="section-heading">
                  <div>
                    <h2>{section.title}</h2>
                    <p>{section.description}</p>
                  </div>
                  <span className="count-badge">{sectionCandidates.length}</span>
                </div>
                <div className="candidate-list">
                  {sectionCandidates.map((candidate) => (
                    <CandidateCard
                      key={candidate.id}
                      candidate={candidate}
                      isBusy={busyAction?.includes(candidate.id) ?? false}
                      onConfirm={onConfirm}
                      onEdit={onEdit}
                      onReject={onReject}
                      onMerge={onMerge}
                    />
                  ))}
                </div>
              </section>
            )
          })}
        </div>
      )}
    </div>
  )
}

function CandidateCard({
  candidate,
  isBusy,
  onConfirm,
  onEdit,
  onReject,
  onMerge,
}: {
  candidate: Candidate
  isBusy: boolean
  onConfirm: (candidate: Candidate) => void
  onEdit: (candidate: Candidate) => void
  onReject: (candidate: Candidate) => void
  onMerge: (candidate: Candidate) => void
}) {
  return (
    <article className="candidate-card">
      <div className="candidate-card-main">
        <div className="candidate-word-row">
          <div>
            <span className="candidate-word">{candidate.simplified}</span>
            {candidate.traditional && candidate.traditional !== candidate.simplified && (
              <span className="traditional"> / {candidate.traditional}</span>
            )}
          </div>
          <span className={`confidence-badge ${confidenceTone(candidate.confidence)}`}>
            {Math.round(candidate.confidence * 100)}% confidence
          </span>
        </div>
        <p className="candidate-pinyin">{candidate.pinyin}</p>
        <p className="candidate-meaning">
          {candidate.meaning}
          {candidate.partOfSpeech && <span> · {candidate.partOfSpeech}</span>}
          {candidate.classifier && <span> · classifier {candidate.classifier}</span>}
        </p>
        <div className="example-block">
          <p>{candidate.example}</p>
          <p className="example-pinyin">{candidate.examplePinyin}</p>
          <p className="translation">{candidate.exampleTranslation}</p>
        </div>
        <div className="evidence-row">
          <span className="evidence-label">Evidence</span>
          <mark>{candidate.evidence}</mark>
          <span className="timestamp">{formatTimestamp(candidate.timestamp)}</span>
        </div>
        <div className="provenance-row">
          <Icon name="spark" />
          <span>{candidate.provenance}</span>
          {candidate.tags.map((tag) => <span className="tag" key={tag}>{tag}</span>)}
        </div>
      </div>
      <div className="candidate-actions">
        {candidate.bucket === 'possibleDuplicate' && candidate.duplicateOf && (
          <button className="button button-secondary button-small" type="button" onClick={() => onMerge(candidate)} disabled={isBusy}>
            <Icon name="merge" />
            Merge
          </button>
        )}
        <button className="button button-ghost button-small" type="button" onClick={() => onEdit(candidate)} disabled={isBusy}>
          Edit
        </button>
        <button className="button button-ghost button-small button-reject" type="button" onClick={() => onReject(candidate)} disabled={isBusy}>
          Reject
        </button>
        <button className="button button-primary button-small" type="button" onClick={() => onConfirm(candidate)} disabled={isBusy}>
          {isBusy ? 'Saving…' : 'Confirm'}
        </button>
      </div>
    </article>
  )
}

function ManualVocabularyView({
  form,
  generatedDraft,
  hasApiKey,
  lessonActive,
  busyAction,
  onFormChange,
  onGenerate,
  onSave,
  onOpenSettings,
}: {
  form: ManualVocabularyForm
  generatedDraft?: ManualVocabularyDraft
  hasApiKey: boolean
  lessonActive: boolean
  busyAction?: string
  onFormChange: (form: ManualVocabularyForm) => void
  onGenerate: () => void
  onSave: (event: FormEvent<HTMLFormElement>) => void
  onOpenSettings: () => void
}) {
  const isGenerating = busyAction === 'generate-manual-vocabulary'
  const isSaving = busyAction === 'save-manual-vocabulary'
  const isBusy = Boolean(busyAction)
  const missingFields = manualVocabularyMissingFields(form)
  const hasCompleteDetails = missingFields.length === 0
  const canGenerate = Boolean(form.simplified.trim()) && hasApiKey && !lessonActive && !hasCompleteDetails && !isBusy

  return (
    <div className="view-stack">
      <section className="page-heading">
        <div>
          <p className="eyebrow">Vocabulary builder</p>
          <h1>Add Word</h1>
          <p className="page-intro">
            Start with one Chinese word. Add anything you already know, then let AI fill only the missing details.
          </p>
        </div>
        <div className="heading-actions">
          <span className="soft-badge"><Icon name="spark" /> Editable draft</span>
        </div>
      </section>

      <div className="manual-vocabulary-layout">
        <form className="panel manual-vocabulary-form" onSubmit={onSave}>
          <div className="panel-heading">
            <p className="eyebrow">Word details</p>
            <h2>Build your vocabulary card</h2>
            <p>Only the simplified Chinese word is required to begin. Every other field is optional until you save.</p>
          </div>

          <div className="manual-form-grid manual-form-grid-top">
            <div className="field-group">
              <label className="field-label field-label-required" htmlFor="manual-simplified">
                Chinese word (simplified)
              </label>
              <input
                id="manual-simplified"
                className="text-input"
                value={form.simplified}
                onChange={(event) => onFormChange({ ...form, simplified: event.target.value })}
                aria-required="true"
                required
                autoFocus
                placeholder="例如：学习"
              />
            </div>
            <div className="field-group">
              <label className="field-label field-label-optional" htmlFor="manual-traditional">
                Traditional
              </label>
              <input
                id="manual-traditional"
                className="text-input"
                value={form.traditional}
                onChange={(event) => onFormChange({ ...form, traditional: event.target.value })}
                placeholder="例如：學習"
              />
            </div>
          </div>

          <div className="manual-form-grid">
            <div className="field-group">
              <label className="field-label field-label-optional" htmlFor="manual-pinyin">
                Pinyin
              </label>
              <input
                id="manual-pinyin"
                className="text-input"
                value={form.pinyin}
                onChange={(event) => onFormChange({ ...form, pinyin: event.target.value })}
                placeholder="例如：xué xí"
              />
            </div>
            <div className="field-group">
              <label className="field-label field-label-optional" htmlFor="manual-meaning">
                English meaning
              </label>
              <input
                id="manual-meaning"
                className="text-input"
                value={form.meaning}
                onChange={(event) => onFormChange({ ...form, meaning: event.target.value })}
                placeholder="例如：to study; to learn"
              />
            </div>
          </div>

          <div className="manual-form-grid">
            <div className="field-group">
              <label className="field-label field-label-optional" htmlFor="manual-part-of-speech">
                Part of speech
              </label>
              <input
                id="manual-part-of-speech"
                className="text-input"
                value={form.partOfSpeech}
                onChange={(event) => onFormChange({ ...form, partOfSpeech: event.target.value })}
                placeholder="例如：verb"
              />
            </div>
            <div className="field-group">
              <label className="field-label field-label-optional" htmlFor="manual-classifier">
                Classifier
              </label>
              <input
                id="manual-classifier"
                className="text-input"
                value={form.classifier}
                onChange={(event) => onFormChange({ ...form, classifier: event.target.value })}
                placeholder="例如：本"
              />
            </div>
          </div>

          <div className="field-group manual-form-wide">
            <label className="field-label field-label-optional" htmlFor="manual-example">
              Chinese sample sentence
            </label>
            <textarea
              id="manual-example"
              className="text-input textarea"
              value={form.example}
              onChange={(event) => onFormChange({ ...form, example: event.target.value })}
              rows={2}
              placeholder="例如：我每天学习中文。"
            />
          </div>

          <div className="manual-form-grid">
            <div className="field-group">
              <label className="field-label field-label-optional" htmlFor="manual-example-pinyin">
                Sample pinyin
              </label>
              <input
                id="manual-example-pinyin"
                className="text-input"
                value={form.examplePinyin}
                onChange={(event) => onFormChange({ ...form, examplePinyin: event.target.value })}
                placeholder="例如：Wǒ měi tiān xué xí Zhōng wén."
              />
            </div>
            <div className="field-group">
              <label className="field-label field-label-optional" htmlFor="manual-example-translation">
                Sample English translation
              </label>
              <input
                id="manual-example-translation"
                className="text-input"
                value={form.exampleTranslation}
                onChange={(event) => onFormChange({ ...form, exampleTranslation: event.target.value })}
                placeholder="例如：I study Chinese every day."
              />
            </div>
          </div>

          <div className="field-group manual-form-wide">
            <label className="field-label field-label-optional field-label-tags" htmlFor="manual-tags">
              Tags
            </label>
            <input
              id="manual-tags"
              className="text-input"
              value={form.tags}
              onChange={(event) => onFormChange({ ...form, tags: event.target.value })}
              placeholder="例如：study, work"
            />
          </div>

          {!hasApiKey && (
            <div className="manual-key-alert" role="status">
              <div className="manual-key-alert-icon"><Icon name="spark" /></div>
              <div>
                <strong>AI generation needs an API key</strong>
                <p>Add an OpenRouter key in Settings to fill missing details. A complete manual form can still be saved.</p>
              </div>
              <button className="button button-secondary button-small" type="button" onClick={onOpenSettings} disabled={isBusy}>
                Open Settings
              </button>
            </div>
          )}

          {lessonActive && (
            <div className="manual-key-alert" role="status">
              <div className="manual-key-alert-icon"><Icon name="waveform" /></div>
              <div>
                <strong>Generation pauses during live lessons</strong>
                <p>Finish the lesson before using AI here so capture keeps the full processing budget.</p>
              </div>
            </div>
          )}

          {generatedDraft && (
            <div className="manual-generation-meta" role="status" aria-live="polite">
              <div className="manual-generation-meta-icon"><Icon name="spark" /></div>
              <div>
                <strong>Generated with {generatedDraft.model}</strong>
                <p>Actual request cost: {formatUsd(generatedDraft.cost)}. Qwen 3.8 Flash is the inexpensive default for this workflow.</p>
              </div>
            </div>
          )}

          {hasCompleteDetails && (
            <div className="manual-complete-note" role="status">
              <Icon name="check" />
              <span>All learning-critical details are complete. You can save directly; generation is not needed.</span>
            </div>
          )}

          <div className="manual-action-row">
            <button className="button button-secondary" type="button" onClick={onGenerate} disabled={!canGenerate}>
              <Icon name="spark" />
              {isGenerating ? 'Generating…' : 'Generate missing details'}
            </button>
            <button className="button button-primary" type="submit" disabled={isBusy || !form.simplified.trim() || !hasCompleteDetails}>
              <Icon name="check" />
              {isSaving ? 'Saving…' : 'Save to Vocabulary'}
            </button>
          </div>
          {lessonActive
            ? <p className="helper-text">AI generation is available again after the active lesson finishes.</p>
            : !hasApiKey
            ? <p className="helper-text">Generation is unavailable without a key, but saving does not use AI.</p>
            : hasCompleteDetails
            ? <p className="helper-text">Your form is ready to save without another AI request.</p>
            : <p className="helper-text">Missing pinyin, meaning, or sample details must be completed before saving.</p>}
        </form>

        <aside className="manual-vocabulary-side">
          <section className="panel manual-guide-card">
            <PanelHeading
              eyebrow="Simple workflow"
              title="Review before you save"
              description="Generation creates an editable draft. Your final review is always in control."
            />
            <ol className="manual-workflow-list">
              <li><span>1</span><p>Enter the Chinese word and anything you already know.</p></li>
              <li><span>2</span><p>Generate missing details when needed, then edit every value.</p></li>
              <li><span>3</span><p>Save the confirmed card to your Vocabulary collection.</p></li>
            </ol>
          </section>
          <section className="panel manual-guide-card">
            <PanelHeading
              eyebrow="Save requirements"
              title="Useful context matters"
              description="Traditional, part of speech, classifier, and tags are optional. A saved card needs the learning-critical details."
            />
            <div className="manual-requirement-list">
              {['Pinyin', 'English meaning', 'Chinese sample sentence', 'Sample pinyin', 'Sample English translation'].map((label) => (
                <span key={label}><Icon name="check" />{label}</span>
              ))}
            </div>
          </section>
        </aside>
      </div>
    </div>
  )
}

function VocabularyView({
  entries,
  onRefresh,
  isRefreshing,
}: {
  entries: AppSnapshot['vocabulary']
  onRefresh: () => void
  isRefreshing: boolean
}) {
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<VocabularyStatus | 'all'>('all')
  const [tag, setTag] = useState('all')
  const tags = useMemo(() => Array.from(new Set(entries.flatMap((entry) => entry.tags))).sort(), [entries])
  const filteredEntries = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    return entries.filter((entry) => {
      const matchesQuery =
        !normalizedQuery ||
        [entry.simplified, entry.traditional, entry.pinyin, entry.meaning]
          .filter(Boolean)
          .some((value) => value?.toLowerCase().includes(normalizedQuery))
      const matchesStatus = status === 'all' || entry.status === status
      const matchesTag = tag === 'all' || entry.tags.includes(tag)
      return matchesQuery && matchesStatus && matchesTag
    })
  }, [entries, query, status, tag])

  return (
    <div className="view-stack">
      <section className="page-heading">
        <div>
          <p className="eyebrow">Your language bank</p>
          <h1>Vocabulary</h1>
          <p className="page-intro">Small, contextual collections beat a giant word list.</p>
        </div>
        <button className="button button-secondary" type="button" onClick={onRefresh} disabled={isRefreshing}>
          <Icon name="refresh" />
          {isRefreshing ? 'Refreshing…' : 'Refresh'}
        </button>
      </section>
      <section className="panel vocabulary-toolbar" aria-label="Vocabulary filters">
        <div className="search-field">
          <Icon name="search" />
          <label className="visually-hidden" htmlFor="vocabulary-search">Search vocabulary</label>
          <input
            id="vocabulary-search"
            type="search"
            placeholder="Search words, pinyin, or meanings"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>
        <label className="filter-select">
          <span>Status</span>
          <select aria-label="Vocabulary status filter" value={status} onChange={(event) => setStatus(event.target.value as VocabularyStatus | 'all')}>
            {STATUS_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
          </select>
        </label>
        <label className="filter-select">
          <span>Tag</span>
          <select aria-label="Vocabulary tag filter" value={tag} onChange={(event) => setTag(event.target.value)}>
            <option value="all">All tags</option>
            {tags.map((item) => <option key={item} value={item}>{item}</option>)}
          </select>
        </label>
      </section>
      {filteredEntries.length === 0 ? (
        <section className="panel">
          <EmptyState
            icon="book"
            title={entries.length === 0 ? 'No vocabulary yet' : 'No words match those filters'}
            description={entries.length === 0 ? 'Confirm a candidate after your first lesson to start your collection.' : 'Try a broader search or clear one of the filters.'}
          />
        </section>
      ) : (
        <section className="vocabulary-grid" aria-live="polite">
          {filteredEntries.map((entry) => <VocabularyCard entry={entry} key={entry.id} />)}
        </section>
      )}
    </div>
  )
}

function VocabularyCard({ entry }: { entry: AppSnapshot['vocabulary'][number] }) {
  return (
    <article className="vocabulary-card">
      <div className="vocabulary-card-top">
        <div>
          <span className="vocabulary-word">{entry.simplified}</span>
          {entry.traditional && entry.traditional !== entry.simplified && <span className="traditional"> / {entry.traditional}</span>}
        </div>
        <span className={`status-pill status-${entry.status}`}>{entry.status}</span>
      </div>
      <p className="pinyin-marked">{entry.pinyin}</p>
      <p className="vocabulary-meaning">{entry.meaning}</p>
      <p className="vocabulary-meta">
        {entry.partOfSpeech ?? 'phrase'}
        {entry.classifier && ` · ${entry.classifier}`}
      </p>
      <div className="example-block vocabulary-example">
        <p>{entry.example}</p>
        <p className="example-pinyin">{entry.examplePinyin}</p>
        <p className="translation">{entry.exampleTranslation}</p>
      </div>
      <div className="vocabulary-card-footer">
        <span>Last seen {formatTimestamp(entry.lastSeen)}</span>
        <span>{entry.seenCount} {entry.seenCount === 1 ? 'appearance' : 'appearances'}</span>
      </div>
      <div className="tag-row">
        {entry.tags.map((tag) => <span className="tag" key={tag}>{tag}</span>)}
      </div>
    </article>
  )
}

function SettingsView({
  snapshot,
  apiMode,
  busyAction,
  onSave,
  onDeleteLastLesson,
}: {
  snapshot: AppSnapshot
  apiMode: AppApi['mode']
  busyAction?: string
  onSave: (patch: SettingsPatch) => void
  onDeleteLastLesson: () => void
}) {
  const [keyDraft, setKeyDraft] = useState('')
  const [sttModel, setSttModel] = useState(normalizeModel(snapshot.settings.sttModel, STT_MODEL_OPTIONS, DEFAULT_STT_MODEL))
  const [analyzerModel, setAnalyzerModel] = useState(normalizeModel(snapshot.settings.analyzerModel, ANALYZER_MODEL_OPTIONS, DEFAULT_ANALYZER_MODEL))
  const [retention, setRetention] = useState(snapshot.settings.audioRetention)
  const [ocrEnabled, setOcrEnabled] = useState(snapshot.settings.ocrEnabled)
  const [microphoneEnabled, setMicrophoneEnabled] = useState(snapshot.settings.microphoneEnabled)

  useEffect(() => {
    setSttModel(normalizeModel(snapshot.settings.sttModel, STT_MODEL_OPTIONS, DEFAULT_STT_MODEL))
    setAnalyzerModel(normalizeModel(snapshot.settings.analyzerModel, ANALYZER_MODEL_OPTIONS, DEFAULT_ANALYZER_MODEL))
    setRetention(snapshot.settings.audioRetention)
    setOcrEnabled(snapshot.settings.ocrEnabled)
    setMicrophoneEnabled(snapshot.settings.microphoneEnabled)
  }, [snapshot.settings])

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const patch: SettingsPatch = {
      sttModel,
      analyzerModel,
      audioRetention: retention,
      ocrEnabled,
      microphoneEnabled,
      hardBudgetUsd: 0.5,
    }
    if (keyDraft.trim()) patch.openRouterKey = keyDraft
    onSave(patch)
    setKeyDraft('')
  }

  const clearApiKey = () => {
    if (!snapshot.settings.openRouterKeyConfigured || busyAction === 'save-settings') return
    if (!window.confirm('Clear the saved OpenRouter API key? You will need to add it again before starting a lesson.')) return
    onSave({ openRouterKey: '' })
  }

  const lessonIsActive = ['live', 'starting', 'stopping', 'error'].includes(snapshot.lesson.status)
  const canDeleteLastLesson = Boolean(snapshot.lesson.id) && !lessonIsActive && !busyAction

  return (
    <div className="view-stack">
      <section className="page-heading">
        <div>
          <p className="eyebrow">Workspace preferences</p>
          <h1>Settings</h1>
          <p className="page-intro">Keep credentials private and make the capture experience fit your calls.</p>
        </div>
      </section>
      <form className="settings-layout" onSubmit={submit}>
        <div className="settings-main">
          <section className="panel settings-panel">
            <PanelHeading eyebrow="Provider access" title="OpenRouter" description="The key is stored securely by the local desktop backend and is never shown here after saving." />
            <div className="field-group">
              <label className="field-label" htmlFor="openrouter-key">API key</label>
              <input
                id="openrouter-key"
                className="text-input"
                type="password"
                value={keyDraft}
                onChange={(event) => setKeyDraft(event.target.value)}
                placeholder={snapshot.settings.openRouterKeyConfigured ? 'Saved securely · enter a new key to replace it' : 'sk-or-v1-…'}
                autoComplete="new-password"
              />
              <p className="helper-text">Write-only field. Existing credentials are intentionally not displayed.</p>
              {snapshot.settings.openRouterKeyConfigured && (
                <div className="key-actions">
                  <span className="key-status" role="status">A key is saved securely.</span>
                  <button className="button button-danger-outline button-small" type="button" onClick={clearApiKey} disabled={lessonIsActive || busyAction === 'save-settings'}>
                    {busyAction === 'save-settings' ? 'Clearing…' : 'Clear API key'}
                  </button>
                </div>
              )}
            </div>
          </section>
          <section className="panel settings-panel">
            <PanelHeading eyebrow="Model routing" title="Speech and analysis" description="Choose the models that power transcription and candidate suggestions." />
            <div className="form-grid">
              <div className="field-group">
                <label className="field-label" htmlFor="stt-model">STT model</label>
                <select id="stt-model" className="text-input" value={sttModel} onChange={(event) => setSttModel(event.target.value)}>
                  {STT_MODEL_OPTIONS.map((option) => <option value={option.value} key={option.value}>{option.label}</option>)}
                </select>
              </div>
              <div className="field-group">
                <label className="field-label" htmlFor="analyzer-model">Analyzer model</label>
                <select id="analyzer-model" className="text-input" value={analyzerModel} onChange={(event) => setAnalyzerModel(event.target.value)}>
                  {ANALYZER_MODEL_OPTIONS.map((option) => <option value={option.value} key={option.value}>{option.label}</option>)}
                </select>
              </div>
            </div>
            <div className="budget-row">
              <div>
                <strong>Hard budget</strong>
                <p>Pauses cloud processing when the session reaches its cap.</p>
              </div>
              <span className="budget-value">{formatUsd(snapshot.settings.hardBudgetUsd)} <small>USD</small></span>
            </div>
          </section>
          <section className="panel settings-panel">
            <PanelHeading eyebrow="Capture and retention" title="Privacy controls" description="These controls apply to new lessons." />
            <div className="field-group">
              <label className="field-label" htmlFor="audio-retention">Audio retention</label>
              <select id="audio-retention" className="text-input" value={retention} onChange={(event) => setRetention(event.target.value as SettingsViewProps['retention'])}>
                <option value="sessionOnly">Session only (recommended)</option>
                <option value="oneDay" disabled>Keep for 1 day (coming later)</option>
                <option value="sevenDays" disabled>Keep for 7 days (coming later)</option>
              </select>
            </div>
            <ToggleRow label="Microphone capture" description="Include your microphone in the live transcript." checked={microphoneEnabled} onChange={setMicrophoneEnabled} />
            <ToggleRow label="OCR for shared screens" description="Look for visible Chinese text when a supported window is shared." checked={ocrEnabled} onChange={setOcrEnabled} />
            <div className="privacy-explanation">
              <Icon name="shield" />
              <div>
                <strong>Zero data retention (ZDR)</strong>
                <p>ZDR controls provider-side retention. Local audio follows the selected retention policy; confirmed vocabulary remains in your local collection.</p>
              </div>
              <span className="status-pill status-mastered">Enabled</span>
            </div>
          </section>
          <section className="panel settings-panel danger-zone">
            <PanelHeading
              eyebrow="Data deletion"
              title="Delete last lesson data"
              description="Remove the last lesson's transcript and linked evidence from this desktop workspace."
            />
            <p className="danger-copy">
              Unconfirmed suggestions supported only by this lesson are removed. Confirmed vocabulary remains as user-owned study data, with the deleted lesson evidence stripped.
            </p>
            <button className="button button-danger-outline" type="button" onClick={onDeleteLastLesson} disabled={!canDeleteLastLesson}>
              <Icon name="trash" />
              {busyAction === 'delete-last-lesson' ? 'Deleting…' : 'Delete last lesson data'}
            </button>
            {lessonIsActive
              ? <p className="helper-text">Stop the active lesson before deleting its data.</p>
              : !snapshot.lesson.id && <p className="helper-text">There is no lesson data available to delete.</p>}
          </section>
        </div>
        <aside className="settings-side">
          <section className="panel diagnostics-card">
            <PanelHeading eyebrow="Diagnostics" title="Helper status" description="Useful context when capture needs attention." />
            <DiagnosticRow label="Backend" value={apiMode === 'runtime' ? 'Wails runtime' : apiMode === 'demo' ? 'Demo adapter' : 'Unavailable'} tone={apiMode === 'unavailable' ? 'warning' : 'good'} />
            <DiagnosticRow label="Capture target" value={labelForTarget(snapshot.target)} />
            <DiagnosticRow label="Permission" value={snapshot.capturePermission === 'granted' ? 'Granted' : 'Needs attention'} tone={snapshot.capturePermission === 'granted' ? 'good' : 'warning'} />
            <DiagnosticRow label="Snapshot updated" value={snapshot.lastUpdated ? formatTimestamp(snapshot.lastUpdated) : 'Not connected'} />
          </section>
          <button className="button button-primary save-settings" type="submit" disabled={lessonIsActive || busyAction === 'save-settings'}>
            <Icon name="check" />
            {busyAction === 'save-settings' ? 'Saving…' : 'Save settings'}
          </button>
          {lessonIsActive && <p className="settings-footnote">Finish the active lesson before changing provider or capture settings.</p>}
          <p className="settings-footnote">Your key is sent only to the local Wails backend when you save and is stored securely by the desktop runtime.</p>
        </aside>
      </form>
    </div>
  )
}

type SettingsViewProps = {
  retention: AppSnapshot['settings']['audioRetention']
}

function ToggleRow({
  label,
  description,
  checked,
  onChange,
}: {
  label: string
  description: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <label className="toggle-row">
      <span>
        <strong>{label}</strong>
        <small>{description}</small>
      </span>
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span className="toggle-track" aria-hidden="true"><span /></span>
    </label>
  )
}

function DiagnosticRow({ label, value, tone }: { label: string; value: string; tone?: 'good' | 'warning' }) {
  return (
    <div className="diagnostic-row">
      <span>{label}</span>
      <strong className={tone ? `diagnostic-${tone}` : ''}>{value}</strong>
    </div>
  )
}

function EditCandidateDialog({
  candidate,
  draft,
  validationError,
  isSaving,
  onDraftChange,
  onCancel,
  onSubmit,
}: {
  candidate: Candidate
  draft: CandidateEditDraft
  validationError?: string
  isSaving: boolean
  onDraftChange: (draft: CandidateEditDraft) => void
  onCancel: () => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
}) {
  return (
    <div className="dialog-backdrop" role="presentation">
      <form
        className="dialog-card dialog-card-wide"
        role="dialog"
        aria-modal="true"
        aria-labelledby="edit-dialog-title"
        aria-describedby={validationError ? 'edit-dialog-error' : undefined}
        onSubmit={onSubmit}
      >
        <div className="dialog-heading">
          <div>
            <p className="eyebrow">Candidate edit</p>
            <h2 id="edit-dialog-title">{candidate.simplified}</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close edit dialog" onClick={onCancel}><Icon name="close" /></button>
        </div>
        {validationError && <div className="dialog-error" id="edit-dialog-error" role="alert">{validationError}</div>}
        <div className="dialog-form-grid">
          <div className="field-group">
            <label className="field-label" htmlFor="edit-simplified">Hanzi</label>
            <input
              id="edit-simplified"
              className="text-input"
              value={draft.simplified}
              onChange={(event) => onDraftChange({ ...draft, simplified: event.target.value })}
              aria-required="true"
              aria-invalid={Boolean(validationError && !draft.simplified.trim())}
            />
          </div>
          <div className="field-group">
            <label className="field-label" htmlFor="edit-traditional">Traditional</label>
            <input id="edit-traditional" className="text-input" value={draft.traditional} onChange={(event) => onDraftChange({ ...draft, traditional: event.target.value })} />
          </div>
        </div>
        <div className="field-group">
          <label className="field-label" htmlFor="edit-pinyin">Pinyin</label>
          <input
            id="edit-pinyin"
            className="text-input"
            value={draft.pinyin}
            onChange={(event) => onDraftChange({ ...draft, pinyin: event.target.value })}
            aria-required="true"
            aria-invalid={Boolean(validationError && !draft.pinyin.trim())}
            autoFocus
          />
        </div>
        <div className="field-group">
          <label className="field-label" htmlFor="edit-meaning">Meaning</label>
          <textarea
            id="edit-meaning"
            className="text-input textarea"
            value={draft.meaning}
            onChange={(event) => onDraftChange({ ...draft, meaning: event.target.value })}
            aria-required="true"
            aria-invalid={Boolean(validationError && !draft.meaning.trim())}
            rows={3}
          />
        </div>
        <div className="dialog-form-grid">
          <div className="field-group">
            <label className="field-label" htmlFor="edit-pos">Part of speech</label>
            <input id="edit-pos" className="text-input" value={draft.partOfSpeech} onChange={(event) => onDraftChange({ ...draft, partOfSpeech: event.target.value })} />
          </div>
          <div className="field-group">
            <label className="field-label" htmlFor="edit-classifier">Classifier</label>
            <input id="edit-classifier" className="text-input" value={draft.classifier} onChange={(event) => onDraftChange({ ...draft, classifier: event.target.value })} />
          </div>
        </div>
        <div className="field-group">
          <label className="field-label" htmlFor="edit-example">Sample sentence</label>
          <textarea
            id="edit-example"
            className="text-input textarea"
            value={draft.example}
            onChange={(event) => onDraftChange({ ...draft, example: event.target.value })}
            aria-required="true"
            aria-invalid={Boolean(validationError && !draft.example.trim())}
            rows={2}
          />
        </div>
        <div className="field-group">
          <label className="field-label" htmlFor="edit-example-pinyin">Sample pinyin</label>
          <input id="edit-example-pinyin" className="text-input" value={draft.examplePinyin} onChange={(event) => onDraftChange({ ...draft, examplePinyin: event.target.value })} />
        </div>
        <div className="field-group">
          <label className="field-label" htmlFor="edit-example-translation">Translation</label>
          <textarea
            id="edit-example-translation"
            className="text-input textarea"
            value={draft.exampleTranslation}
            onChange={(event) => onDraftChange({ ...draft, exampleTranslation: event.target.value })}
            aria-required="true"
            aria-invalid={Boolean(validationError && !draft.exampleTranslation.trim())}
            rows={2}
          />
        </div>
        <div className="field-group">
          <label className="field-label" htmlFor="edit-tags">Tags <span className="field-hint">(comma-separated)</span></label>
          <input id="edit-tags" className="text-input" value={draft.tags} onChange={(event) => onDraftChange({ ...draft, tags: event.target.value })} />
        </div>
        <div className="dialog-actions">
          <button className="button button-ghost" type="button" onClick={onCancel}>Cancel</button>
          <button className="button button-primary" type="submit" disabled={isSaving}>{isSaving ? 'Saving…' : 'Save changes'}</button>
        </div>
      </form>
    </div>
  )
}

function MergeDialog({
  source,
  target,
  isMerging,
  onCancel,
  onConfirm,
}: {
  source: Candidate
  target?: Candidate
  isMerging: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <div className="dialog-backdrop" role="presentation">
      <div className="dialog-card" role="dialog" aria-modal="true" aria-labelledby="merge-dialog-title">
        <div className="dialog-heading">
          <div>
            <p className="eyebrow">Possible duplicate</p>
            <h2 id="merge-dialog-title">Merge candidates?</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close merge dialog" onClick={onCancel} autoFocus><Icon name="close" /></button>
        </div>
        <p className="dialog-copy">
          Keep the stronger original entry <strong>{target?.simplified ?? 'original'}</strong> and remove the duplicate <strong>{source.simplified}</strong> from review.
        </p>
        <div className="dialog-actions">
          <button className="button button-ghost" type="button" onClick={onCancel}>Keep separate</button>
          <button className="button button-primary" type="button" onClick={onConfirm} disabled={!target || isMerging}>{isMerging ? 'Merging…' : 'Confirm merge'}</button>
        </div>
      </div>
    </div>
  )
}

function TranscriptRow({ line }: { line: AppSnapshot['transcript'][number] }) {
  return (
    <div className={`transcript-row ${line.isMoment ? 'is-moment' : ''}`}>
      <div className="transcript-time">{formatTimestamp(line.timestamp)}</div>
      <div className="transcript-source">
        <span className={`source-dot source-${sourceClass(line.source)}`} aria-hidden="true" />
        <span>{sourceLabel(line.source)}</span>
      </div>
      <div className="transcript-text">
        <p>{line.text}</p>
        {line.speaker && <span>{line.speaker}</span>}
      </div>
      {line.isMoment && <span className="moment-label"><Icon name="bookmark" /> Marked</span>}
    </div>
  )
}

function AudioMeter({ label, value, color }: { label: string; value: number; color: 'blue' | 'purple' }) {
  const safeValue = Math.max(0, Math.min(1, value))
  return (
    <div className="meter">
      <div className="meter-label"><span>{label}</span><strong>{Math.round(safeValue * 100)}%</strong></div>
      <div className="meter-track" role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(safeValue * 100)}>
        <span className={`meter-fill meter-${color}`} style={{ width: `${Math.max(4, safeValue * 100)}%` }} />
      </div>
    </div>
  )
}

function LessonStatus({ status }: { status: AppSnapshot['lesson']['status'] }) {
  const labels = { idle: 'Ready', starting: 'Starting', live: 'Live', stopping: 'Stopping', error: 'Needs attention' }
  return <span className={`lesson-status lesson-status-${status}`}><span className="status-dot" aria-hidden="true" />{labels[status]}</span>
}

function MetricCard({
  label,
  value,
  detail,
  icon,
  tone,
}: {
  label: string
  value: string
  detail: string
  icon: IconName
  tone: string
}) {
  return (
    <article className="metric-card">
      <div className={`metric-icon metric-${tone}`}><Icon name={icon} /></div>
      <div>
        <span>{label}</span>
        <strong>{value}</strong>
        <small>{detail}</small>
      </div>
    </article>
  )
}

function PanelHeading({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) {
  return (
    <div className="panel-heading">
      <p className="eyebrow">{eyebrow}</p>
      <h2>{title}</h2>
      <p>{description}</p>
    </div>
  )
}

function PrivacyBadge() {
  return <span className="privacy-badge"><Icon name="shield" /> ZDR enabled</span>
}

function CostStatusAlert({ cost }: { cost: AppSnapshot['cost'] }) {
  const hardExceeded = Boolean(cost.hardExceeded)
  return (
    <div className={`cost-alert ${hardExceeded ? 'cost-alert-hard' : 'cost-alert-warning'}`} role="alert">
      <Icon name="warning" />
      <div>
        <strong>{hardExceeded ? 'Hard cost limit reached' : 'Cost warning: approaching the hard cost limit'}</strong>
        <p>
          {hardExceeded
            ? 'Cloud transcription and extraction are paused. Local capture and mark timestamps continue; a new lesson starts with a fresh budget.'
            : `Projected cost is ${formatUsd(cost.projectedUsd)} against the ${formatUsd(cost.hardBudgetUsd)} session cap.`}
        </p>
        <span>Current {formatUsd(cost.currentUsd)} · Cap {formatUsd(cost.hardBudgetUsd)}</span>
      </div>
    </div>
  )
}

function SetupCheck({
  label,
  detail,
  status,
}: {
  label: string
  detail: string
  status: 'ready' | 'blocked' | 'pending'
}) {
  return (
    <div className={`setup-check setup-check-${status}`}>
      <span className="setup-check-icon"><Icon name={status === 'ready' ? 'check' : status === 'blocked' ? 'warning' : 'refresh'} /></span>
      <strong>{label}</strong>
      <span>{detail}</span>
    </div>
  )
}

function InlineAlert({ tone, children, onDismiss }: { tone: 'error' | 'success'; children: string; onDismiss?: () => void }) {
  return (
    <div className={`inline-alert alert-${tone}`} role={tone === 'error' ? 'alert' : 'status'}>
      <Icon name={tone === 'error' ? 'warning' : 'check'} />
      <span>{children}</span>
      {onDismiss && <button className="alert-dismiss" type="button" aria-label="Dismiss message" onClick={onDismiss}><Icon name="close" /></button>}
    </div>
  )
}

function BackendUnavailable({ mode, onRetry }: { mode: AppApi['mode']; onRetry: () => void }) {
  return (
    <div className="backend-alert" role="status">
      <div className="backend-alert-icon"><Icon name="plug" /></div>
      <div>
        <strong>{mode === 'demo' ? 'Demo mode is active' : 'Backend unavailable'}</strong>
        <p>
          {mode === 'demo'
            ? 'The Wails runtime is not connected, so you are viewing deterministic sample data.'
            : 'Start the local desktop runtime to capture calls and update your vocabulary.'}
        </p>
      </div>
      {mode !== 'demo' && (
        <button className="button button-secondary button-small" type="button" onClick={onRetry}>Retry connection</button>
      )}
    </div>
  )
}

function LoadingState() {
  return (
    <div className="loading-state" role="status" aria-live="polite">
      <span className="loading-spinner" aria-hidden="true" />
      <strong>Connecting to your workspace…</strong>
      <span>Loading capture state and vocabulary.</span>
    </div>
  )
}

function EmptyState({ icon, title, description }: { icon: IconName; title: string; description: string }) {
  return (
    <div className="empty-state">
      <div className="empty-icon"><Icon name={icon} /></div>
      <h3>{title}</h3>
      <p>{description}</p>
    </div>
  )
}

function getConnectionStatus(connection: AppSnapshot['connection'], mode: AppApi['mode']) {
  if (mode === 'demo') return { label: 'Demo adapter', tone: 'demo' }
  if (mode === 'unavailable' || connection === 'unavailable') return { label: 'Backend unavailable', tone: 'offline' }
  if (connection === 'offline') return { label: 'Offline', tone: 'offline' }
  if (connection === 'connecting') return { label: 'Connecting', tone: 'connecting' }
  return { label: 'Connected', tone: 'connected' }
}

function getErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim()) return error.message
  if (typeof error === 'string' && error.trim()) return error
  if (error && typeof error === 'object') {
    for (const key of ['message', 'error', 'detail']) {
      const value = (error as Record<string, unknown>)[key]
      if (typeof value === 'string' && value.trim()) return value
    }
    try {
      const serialized = JSON.stringify(error)
      if (serialized && serialized !== '{}') return serialized
    } catch {
      // Fall through to the stable generic message for non-serializable values.
    }
  }
  return 'Something went wrong. Please try again.'
}

function formatUsd(value: number): string {
  if (value !== 0 && Math.abs(value) < 0.001) return '<$0.001'
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: value !== 0 && Math.abs(value) < 0.01 ? 4 : 2,
  }).format(value)
}

function formatTimestamp(value: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('en-US', { hour: 'numeric', minute: '2-digit' }).format(date)
}

function formatStartedAt(value?: string): string {
  return value ? `since ${formatTimestamp(value)}` : 'just now'
}

function retentionLabel(value: AppSnapshot['privacy']['audioRetention']): string {
  return value === 'sessionOnly' ? 'Session only' : value === 'oneDay' ? '1 day' : '7 days'
}

function labelForTarget(target: TargetApp): string {
  return target === 'googleMeet' ? 'Google Meet' : target === 'teams' ? 'Microsoft Teams' : 'Zoom'
}

function sourceLabel(source: TranscriptSource): string {
  if (source === 'microphone') return 'Microphone'
  if (source === 'remote') return 'Remote'
  if (source === 'system/ocr') return 'Screen OCR'
  if (source === 'system/manual') return 'Marked audio'
  return 'System'
}

function sourceClass(source: TranscriptSource): 'microphone' | 'remote' | 'system' {
  return source === 'microphone' || source === 'remote' ? source : 'system'
}

function confidenceTone(confidence: number): string {
  if (confidence >= 0.9) return 'confidence-high'
  if (confidence >= 0.7) return 'confidence-medium'
  return 'confidence-low'
}

function normalizeModel(value: string, options: readonly { value: string }[], fallback: string): string {
  return options.some((option) => option.value === value) ? value : fallback
}

function parseTags(value: string): string[] {
  return Array.from(new Set(value.split(',').map((tag) => tag.trim()).filter(Boolean)))
}

function manualVocabularyFormToInput(form: ManualVocabularyForm): ManualVocabularyInput {
  return {
    simplified: form.simplified.trim(),
    traditional: form.traditional.trim(),
    pinyin: form.pinyin.trim(),
    meaning: form.meaning.trim(),
    partOfSpeech: form.partOfSpeech.trim(),
    classifier: form.classifier.trim(),
    example: form.example.trim(),
    examplePinyin: form.examplePinyin.trim(),
    exampleTranslation: form.exampleTranslation.trim(),
    tags: parseTags(form.tags),
    aiGenerated: form.aiGenerated,
  }
}

function manualVocabularyDraftToForm(draft: ManualVocabularyDraft): ManualVocabularyForm {
  return {
    simplified: draft.simplified,
    traditional: draft.traditional ?? '',
    pinyin: draft.pinyin ?? '',
    meaning: draft.meaning ?? '',
    partOfSpeech: draft.partOfSpeech ?? '',
    classifier: draft.classifier ?? '',
    example: draft.example ?? '',
    examplePinyin: draft.examplePinyin ?? '',
    exampleTranslation: draft.exampleTranslation ?? '',
    tags: (draft.tags ?? []).join(', '),
    aiGenerated: Boolean(draft.aiGenerated),
  }
}

function manualVocabularyMissingFields(form: ManualVocabularyForm): string[] {
  return [
    ['Chinese word (simplified)', form.simplified],
    ['pinyin', form.pinyin],
    ['English meaning', form.meaning],
    ['Chinese sample sentence', form.example],
    ['sample pinyin', form.examplePinyin],
    ['sample English translation', form.exampleTranslation],
  ]
    .filter(([, value]) => !value.trim())
    .map(([label]) => label)
}

function hasCompleteManualVocabularyForm(form: ManualVocabularyForm): boolean {
  return manualVocabularyMissingFields(form).length === 0
}

type IconName =
  | 'waveform'
  | 'inbox'
  | 'book'
  | 'settings'
  | 'refresh'
  | 'video'
  | 'check'
  | 'mic'
  | 'shield'
  | 'play'
  | 'stop'
  | 'bookmark'
  | 'coin'
  | 'lock'
  | 'spark'
  | 'merge'
  | 'search'
  | 'close'
  | 'warning'
  | 'trash'
  | 'plug'

function Icon({ name }: { name: IconName }) {
  const paths: Record<IconName, ReactNode> = {
    waveform: <><path d="M3 12h3l2-7 4 14 2-9 2 5h5" /><path d="M3 19h18" /></>,
    inbox: <><path d="M4 4h16v15H4z" /><path d="M4 13h4l1.5 2h5L16 13h4" /></>,
    book: <><path d="M5 4.5A2.5 2.5 0 0 1 7.5 2H20v17H7.5A2.5 2.5 0 0 0 5 21.5z" /><path d="M5 4.5v17M9 6h7M9 10h7" /></>,
    settings: <><path d="M12 8.5A3.5 3.5 0 1 0 12 15.5 3.5 3.5 0 0 0 12 8.5Z" /><path d="m19.4 15 .1.1a1.8 1.8 0 0 1-2.5 2.5l-.1-.1a1.8 1.8 0 0 0-3 .9v.2a1.8 1.8 0 0 1-3.6 0v-.2a1.8 1.8 0 0 0-3-.9l-.1.1a1.8 1.8 0 0 1-2.5-2.5l.1-.1a1.8 1.8 0 0 0-.9-3H3.7a1.8 1.8 0 0 1 0-3.6h.2a1.8 1.8 0 0 0 .9-3l-.1-.1a1.8 1.8 0 0 1 2.5-2.5l.1.1a1.8 1.8 0 0 0 3-.9V1.8a1.8 1.8 0 0 1 3.6 0V2a1.8 1.8 0 0 0 3 .9l.1-.1a1.8 1.8 0 0 1 2.5 2.5l-.1.1a1.8 1.8 0 0 0 .9 3h.2a1.8 1.8 0 0 1 0 3.6h-.2a1.8 1.8 0 0 0-.9 3Z" /></>,
    refresh: <><path d="M20 11a8 8 0 0 0-14.7-4L3 10" /><path d="M3 4v6h6M4 13a8 8 0 0 0 14.7 4L21 14" /><path d="M21 20v-6h-6" /></>,
    video: <><rect x="3" y="5" width="13" height="14" rx="2" /><path d="m16 10 5-3v10l-5-3z" /></>,
    check: <><path d="m5 12 4 4L19 6" /></>,
    mic: <><rect x="9" y="3" width="6" height="11" rx="3" /><path d="M5 10a7 7 0 0 0 14 0M12 17v4M8 21h8" /></>,
    shield: <><path d="M12 3 20 6v5c0 5-3.4 8.5-8 10-4.6-1.5-8-5-8-10V6z" /><path d="m8.5 12 2.2 2.2 4.8-5" /></>,
    play: <><path d="m8 5 11 7-11 7z" /></>,
    stop: <><rect x="6" y="6" width="12" height="12" rx="2" /></>,
    bookmark: <><path d="M6 4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v18l-6-4-6 4z" /></>,
    coin: <><circle cx="12" cy="12" r="8" /><path d="M12 7v10M15 9.5c-.7-.7-1.6-1-3-1-1.5 0-2.5.7-2.5 1.8 0 3 5.5 1.2 5.5 4.2 0 1-.9 1.8-2.7 1.8-1.4 0-2.4-.4-3.2-1.2" /></>,
    lock: <><rect x="5" y="10" width="14" height="11" rx="2" /><path d="M8 10V7a4 4 0 0 1 8 0v3" /></>,
    spark: <><path d="m12 2 1.3 6.7L20 10l-6.7 1.3L12 18l-1.3-6.7L4 10l6.7-1.3z" /><path d="m19 16 .5 2.5L22 19l-2.5.5L19 22l-.5-2.5L16 19l2.5-.5z" /></>,
    merge: <><path d="M7 5v4a3 3 0 0 0 3 3h4a3 3 0 0 1 3 3v4" /><path d="m14 16 3 3 3-3M7 5l-3 3 3 3" /></>,
    search: <><circle cx="10.8" cy="10.8" r="6.8" /><path d="m16 16 5 5" /></>,
    close: <><path d="m5 5 14 14M19 5 5 19" /></>,
    warning: <><path d="m12 3 9 17H3z" /><path d="M12 9v5M12 17.5v.1" /></>,
    trash: <><path d="M4 7h16M10 11v6M14 11v6M6 7l1 14h10l1-14M9 7V4h6v3" /></>,
    plug: <><path d="M9 7V3M15 7V3M7 7h10v3a5 5 0 0 1-10 0zM12 15v6M8 21h8" /></>,
  }
  return (
    <svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {paths[name]}
    </svg>
  )
}
