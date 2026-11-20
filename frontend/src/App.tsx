import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { DragEvent, FormEvent, ReactNode } from 'react'
import { api as defaultApi, defaultLessonTag } from './lib/api'
import type { AppApi } from './lib/api'
import type {
  AppSnapshot,
  Candidate,
  CandidateBucket,
  CandidateEditPatch,
  LessonTranscript,
  LessonTranscriptSummary,
  ManualVocabularyDraft,
  ManualVocabularyInput,
  ManualVocabularyVariant,
  SelectionAddWordEvent,
  SettingsPatch,
  TargetApp,
  TranscriptSource,
  VocabularyStatus,
} from './lib/types'
import './styles.css'

type View = 'live' | 'transcripts' | 'inbox' | 'add-word' | 'vocabulary' | 'settings'

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
    zdrEnabled: false,
    audioRetention: 'sessionOnly',
  },
  settings: {
    openRouterKeyConfigured: false,
    supabaseSyncConfigured: false,
    syncPendingCount: 0,
    syncStatus: 'not_configured',
    sttModel: 'qwen/qwen3-asr-1.7b',
    analyzerModel: 'qwen/qwen3-32b',
    visionModel: 'qwen/qwen3.7-flash',
    hardBudgetUsd: 0.5,
    audioRetention: 'sessionOnly',
    ocrEnabled: false,
    microphoneEnabled: true,
  },
  lastUpdated: '',
}

const NAV_ITEMS: Array<{ id: View; label: string; description: string; icon: IconName }> = [
  { id: 'live', label: 'Live Lesson', description: 'Capture and review a live call', icon: 'waveform' },
  { id: 'transcripts', label: 'Past transcripts', description: 'Read a saved lesson', icon: 'history' },
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
  { value: 'qwen/qwen3-32b', label: 'Qwen 3 32B' },
  { value: 'qwen/qwen3.5-9b', label: 'Qwen 3.5 9B' },
] as const

const VISION_MODEL_OPTIONS = [
  { value: 'qwen/qwen3.7-flash', label: 'Qwen 3.7 Flash' },
  { value: 'qwen/qwen3-vl-30b-a3b-instruct', label: 'Qwen3-VL 30B' },
] as const

const DEFAULT_STT_MODEL = STT_MODEL_OPTIONS[0].value
const DEFAULT_ANALYZER_MODEL = ANALYZER_MODEL_OPTIONS[0].value
const DEFAULT_VISION_MODEL = VISION_MODEL_OPTIONS[0].value

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

type PreparedPhoto = {
  imageBase64: string
  previewDataUrl: string
}

type PhotoQueueStatus = 'preparing' | 'ready' | 'reading' | 'done' | 'no-words' | 'failed'

type PhotoQueueItem = {
  id: string
  fileName: string
  status: PhotoQueueStatus
  previewDataUrl?: string
  imageBase64?: string
  error?: string
}

type PhotoSuggestion = {
  id: string
  input: ManualVocabularyInput
  photoIds: string[]
  selected: boolean
  expanded: boolean
}

type ManualVariantPrompt = {
  headword: string
  origin: ManualVocabularyForm
  variants: ManualVocabularyVariant[]
  appliedIndex: number
}

type VocabularyFocusQuery = {
  query: string
  token: number
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
  const [lessonTag, setLessonTag] = useState(defaultLessonTag)
  const lessonTagTouched = useRef(false)
  const [editingCandidate, setEditingCandidate] = useState<Candidate>()
  const [mergeCandidate, setMergeCandidate] = useState<Candidate>()
  const [wordPendingDelete, setWordPendingDelete] = useState<AppSnapshot['vocabulary'][number]>()
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
  const [editRegenerateError, setEditRegenerateError] = useState<string>()
  const [editRegenerateNotice, setEditRegenerateNotice] = useState<string>()
  const [manualForm, setManualForm] = useState<ManualVocabularyForm>(EMPTY_MANUAL_VOCABULARY_FORM)
  const [manualGeneration, setManualGeneration] = useState<ManualVocabularyDraft>()
  const [manualVariantPrompt, setManualVariantPrompt] = useState<ManualVariantPrompt>()
  const [vocabularyFocusQuery, setVocabularyFocusQuery] = useState<VocabularyFocusQuery>({
    query: '',
    token: 0,
  })
  const [existingTags, setExistingTags] = useState<string[]>([])
  const [photoQueue, setPhotoQueue] = useState<PhotoQueueItem[]>([])
  const [photoSuggestions, setPhotoSuggestions] = useState<PhotoSuggestion[]>([])
  const [photoInputError, setPhotoInputError] = useState<string>()
  const [photoCost, setPhotoCost] = useState(0)
  const [photoModel, setPhotoModel] = useState<string>()
  const [photoReadProgress, setPhotoReadProgress] = useState<{ current: number; total: number }>()
  const photoId = useRef(0)
  const manualFormRef = useRef<ManualVocabularyForm>({ ...EMPTY_MANUAL_VOCABULARY_FORM })
  const manualGenerationRequest = useRef(0)

  const current = snapshot ?? EMPTY_SNAPSHOT
  const backendUnavailable = apiClient.mode === 'unavailable' || Boolean(loadError)

  useEffect(() => {
    let active = true
    setIsLoading(true)
    setLoadError(undefined)

    const unsubscribe = apiClient.subscribe((nextSnapshot) => {
      if (active) {
        setSnapshot(nextSnapshot)
      }
    })

    void apiClient
      .getSnapshot()
      .then((nextSnapshot) => {
        if (!active) return
        setSnapshot(nextSnapshot)
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

  const tagCatalogKey = useMemo(() => {
    return [...current.vocabulary, ...current.candidates]
      .flatMap((entry) => entry.tags)
      .join('\n')
  }, [current.vocabulary, current.candidates])

  useEffect(() => {
    let active = true
    void apiClient.listTags()
      .then((tags) => {
        if (active) setExistingTags(tags)
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [apiClient, tagCatalogKey])

  const runAction = useCallback(
    async (name: string, action: () => Promise<AppSnapshot>, successMessage: string) => {
      setBusyAction(name)
      setActionError(undefined)
      setActionMessage(undefined)
      try {
        const nextSnapshot = await action()
        setSnapshot(nextSnapshot)
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
    if (!editingCandidate && !mergeCandidate && !wordPendingDelete) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || busyAction) return
      event.preventDefault()
      setEditingCandidate(undefined)
      setMergeCandidate(undefined)
      setWordPendingDelete(undefined)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [busyAction, editingCandidate, mergeCandidate, wordPendingDelete])

  useEffect(() => {
    if (current.lesson.status === 'live' || current.lesson.status === 'error') return
    const timer = window.setInterval(() => {
      if (!lessonTagTouched.current) setLessonTag(defaultLessonTag())
    }, 30_000)
    return () => window.clearInterval(timer)
  }, [current.lesson.status])

  const handleStart = () => {
    const tags = lessonTag.trim() || defaultLessonTag()
    if (tags !== lessonTag) {
      lessonTagTouched.current = false
      setLessonTag(tags)
    }
    void runAction(
      'start-lesson',
      () => apiClient.startLesson({ target: 'system', consent, tags }),
      'Lesson started. Your transcript will appear below.',
    )
  }

  const handleStop = () => {
    void runAction(
      'stop-lesson',
      () => apiClient.stopLesson(),
      'Lesson stopped. Reconciliation and finalization completed.',
    ).then((stopped) => {
      if (!stopped) return
      lessonTagTouched.current = false
      setLessonTag(defaultLessonTag())
    })
  }

  const handleRefresh = () => {
    void runAction('refresh', () => apiClient.refresh(), 'Data refreshed.')
  }

  const handlePermission = () => {
    void runAction('permission', () => apiClient.requestCapturePermission(), 'Capture permission status updated.')
  }

  const saveCloudKey = async (key: string) => {
    setBusyAction('save-cloud-key')
    setActionError(undefined)
    setActionMessage(undefined)
    try {
      const next = await apiClient.saveSupabaseKey(key)
      setSnapshot(next)
      if (next.settings.supabaseAccessStatus === 'ready') {
        setActionMessage('Cloud vocabulary key saved.')
        return true
      }
      setActionError(next.settings.supabaseAccessDetail || 'This key cannot read and write cloud vocabulary.')
      return false
    } catch (error: unknown) {
      setActionError(getErrorMessage(error))
      return false
    } finally {
      setBusyAction(undefined)
    }
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
    setEditRegenerateError(undefined)
    setEditRegenerateNotice(undefined)
    setActionError(undefined)
  }

  const handleRegenerateCandidate = async (instruction: string): Promise<boolean> => {
    if (!editingCandidate) return false
    if (!editDraft.simplified.trim()) {
      setEditRegenerateError('Enter a Chinese word before regenerating.')
      return false
    }
    if (!current.settings.openRouterKeyConfigured) {
      setEditRegenerateError('Add an OpenRouter API key in Settings before regenerating.')
      return false
    }
    const candidateId = editingCandidate.id
    setBusyAction(`regenerate-candidate-${candidateId}`)
    setEditRegenerateError(undefined)
    setEditRegenerateNotice(undefined)
    try {
      const draft = await apiClient.regenerateManualVocabulary({
        simplified: editDraft.simplified.trim(),
        traditional: editDraft.traditional.trim() || undefined,
        pinyin: editDraft.pinyin.trim() || undefined,
        meaning: editDraft.meaning.trim() || undefined,
        partOfSpeech: editDraft.partOfSpeech.trim() || undefined,
        classifier: editDraft.classifier.trim() || undefined,
        example: editDraft.example.trim() || undefined,
        examplePinyin: editDraft.examplePinyin.trim() || undefined,
        exampleTranslation: editDraft.exampleTranslation.trim() || undefined,
        tags: parseTags(editDraft.tags),
        instruction: instruction.trim() || undefined,
      })
      setEditDraft((prev) => ({
        simplified: draft.simplified || prev.simplified,
        traditional: draft.traditional ?? '',
        pinyin: draft.pinyin ?? '',
        meaning: draft.meaning ?? '',
        partOfSpeech: draft.partOfSpeech ?? '',
        classifier: draft.classifier ?? '',
        example: draft.example ?? '',
        examplePinyin: draft.examplePinyin ?? '',
        exampleTranslation: draft.exampleTranslation ?? '',
        tags: prev.tags,
      }))
      setEditError(undefined)
      setEditRegenerateNotice(`Card regenerated with ${draft.model} (${formatUsd(draft.cost)}). Review before saving.`)
      return true
    } catch (error: unknown) {
      setEditRegenerateError(getErrorMessage(error))
      return false
    } finally {
      setBusyAction((active) => (active === `regenerate-candidate-${candidateId}` ? undefined : active))
    }
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

  const handleDeleteVocabulary = (entry: AppSnapshot['vocabulary'][number]) => {
    if (busyAction) return
    setWordPendingDelete(entry)
  }

  const confirmDeleteVocabulary = () => {
    if (!wordPendingDelete) return
    const entry = wordPendingDelete
    void runAction(
      `delete-vocabulary-${entry.id}`,
      () => apiClient.deleteVocabulary(entry.id),
      `${entry.simplified} deleted. Cloud vocabulary updates on the next sync.`,
    ).finally(() => setWordPendingDelete(undefined))
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

  const syncNow = () => {
    if (!current.settings.supabaseSyncConfigured || busyAction === 'sync-now') return
    void runAction('sync-now', () => apiClient.syncNow(), 'Cloud vocabulary sync requested.')
  }

  const setManualFormValue = (nextForm: ManualVocabularyForm) => {
    manualFormRef.current = nextForm
    setManualForm(nextForm)
  }

  const updateManualForm = (nextForm: ManualVocabularyForm) => {
    const previousForm = manualFormRef.current
    const headwordChanged = nextForm.simplified.trim() !== previousForm.simplified.trim()
    const formToStore = headwordChanged
      ? { ...EMPTY_MANUAL_VOCABULARY_FORM, simplified: nextForm.simplified }
      : nextForm
    manualFormRef.current = formToStore
    setManualForm(formToStore)
    setActionError(undefined)
    if (headwordChanged) {
      manualGenerationRequest.current += 1
      setManualGeneration(undefined)
      setManualVariantPrompt(undefined)
      setActionMessage(undefined)
      return
    }
    setManualVariantPrompt((prompt) => {
      if (!prompt || prompt.appliedIndex >= 0) return prompt
      return { ...prompt, origin: { ...formToStore } }
    })
  }

  const handleGenerateManualVocabulary = () => {
    const formAtRequest = manualFormRef.current
    const input = manualVocabularyFormToInput(formAtRequest)
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
    if (hasCompleteManualVocabularyForm(formAtRequest)) {
      setActionError(undefined)
      setActionMessage('All learning details are already complete; generation is not needed.')
      return
    }
    const requestedHeadword = input.simplified.trim()
    const requestId = manualGenerationRequest.current + 1
    manualGenerationRequest.current = requestId
    setBusyAction('generate-manual-vocabulary')
    setActionError(undefined)
    setActionMessage(undefined)
    setManualGeneration(undefined)
    setManualVariantPrompt(undefined)
    void apiClient
      .generateManualVocabulary(input)
      .then((draft) => {
        if (requestId !== manualGenerationRequest.current) return
        if (manualFormRef.current.simplified.trim() !== requestedHeadword) return
        const origin = { ...manualFormRef.current }
        setManualGeneration(draft)
        if (draft.needsChoice) {
          setManualVariantPrompt({
            headword: origin.simplified.trim(),
            origin,
            variants: draft.variants ?? [],
            appliedIndex: -1,
          })
          setActionMessage('Choose a meaning to fill the empty fields.')
          return
        }

        setManualFormValue(manualVocabularyDraftToForm(draft))
        const variants = draft.variants ?? []
        if (variants.length >= 2) {
          const appliedIndex = draft.appliedVariant >= 0 && draft.appliedVariant < variants.length
            ? draft.appliedVariant
            : -1
          setManualVariantPrompt({
            headword: origin.simplified.trim(),
            origin,
            variants,
            appliedIndex,
          })
        } else {
          setManualVariantPrompt(undefined)
        }
        setActionMessage('Missing details generated. Review every field before saving.')
      })
      .catch((error: unknown) => {
        setActionError(getErrorMessage(error))
      })
      .finally(() => {
        if (requestId === manualGenerationRequest.current) setBusyAction(undefined)
      })
  }

  const handleRegenerateManualVocabulary = async (instruction: string): Promise<boolean> => {
    const formAtRequest = manualFormRef.current
    const input = manualVocabularyFormToInput(formAtRequest)
    if (!input.simplified.trim()) {
      setActionError('Enter a Chinese word (simplified) before regenerating.')
      setActionMessage(undefined)
      return false
    }
    if (!current.settings.openRouterKeyConfigured) {
      setActionError('Add an OpenRouter API key in Settings before regenerating.')
      setActionMessage(undefined)
      return false
    }
    const requestedHeadword = input.simplified.trim()
    const requestId = manualGenerationRequest.current + 1
    manualGenerationRequest.current = requestId
    setBusyAction('regenerate-manual-vocabulary')
    setActionError(undefined)
    setActionMessage(undefined)
    try {
      const draft = await apiClient.regenerateManualVocabulary({
        ...input,
        instruction: instruction.trim() || undefined,
      })
      if (requestId !== manualGenerationRequest.current) return false
      if (manualFormRef.current.simplified.trim() !== requestedHeadword) return false
      setManualGeneration(draft)
      setManualVariantPrompt(undefined)
      setManualFormValue(manualVocabularyDraftToForm(draft))
      setActionMessage('Card regenerated. Review every field before saving.')
      return true
    } catch (error: unknown) {
      setActionError(getErrorMessage(error))
      return false
    } finally {
      if (requestId === manualGenerationRequest.current) setBusyAction(undefined)
    }
  }

  const applyManualVariant = (index: number) => {
    const prompt = manualVariantPrompt
    const variant = prompt?.variants[index]
    if (!prompt || !variant) return
    setManualFormValue(manualVocabularyVariantToForm(prompt.origin, variant))
    setManualGeneration((draft) => draft
      ? { ...draft, needsChoice: false, matchedContext: false, appliedVariant: index }
      : draft)
    setManualVariantPrompt({ ...prompt, appliedIndex: index })
    setActionError(undefined)
  }

  const clearManualForm = () => {
    if (busyAction) return
    setManualFormValue({ ...EMPTY_MANUAL_VOCABULARY_FORM })
    setManualVariantPrompt(undefined)
    setManualGeneration(undefined)
    setActionError(undefined)
    setActionMessage(undefined)
  }

  const handlePhotoFiles = useCallback((files: File[]) => {
    if (files.length === 0) return

    const acceptedFiles = files.filter(isAcceptedPhotoFile)
    const rejectedCount = files.length - acceptedFiles.length
    const availableSlots = Math.max(0, 8 - photoQueue.length)
    const filesToAdd = acceptedFiles.slice(0, availableSlots)
    const messages: string[] = []

    if (rejectedCount > 0) {
      messages.push('Some files were not added. Choose JPEG, PNG, WebP, HEIC, or HEIF photos.')
    }
    if (acceptedFiles.length > availableSlots) {
      messages.push('Only 8 photos can be read at once. Extra photos were not added.')
    }
    setPhotoInputError(messages.length > 0 ? messages.join(' ') : undefined)

    if (filesToAdd.length === 0) return

    const nextItems = filesToAdd.map((file) => {
      photoId.current += 1
      return {
        id: `photo-${photoId.current}`,
        fileName: file.name || 'Photo',
        status: 'preparing' as const,
      }
    })
    setPhotoQueue((items) => [...items, ...nextItems])

    filesToAdd.forEach((file, index) => {
      const item = nextItems[index]
      void preparePhoto(file)
        .then((prepared) => {
          setPhotoQueue((items) => items.map((photo) => (
            photo.id === item.id
              ? { ...photo, status: 'ready', previewDataUrl: prepared.previewDataUrl, imageBase64: prepared.imageBase64, error: undefined }
              : photo
          )))
        })
        .catch((error: unknown) => {
          setPhotoQueue((items) => items.map((photo) => (
            photo.id === item.id
              ? { ...photo, status: 'failed', error: getErrorMessage(error) }
              : photo
          )))
        })
    })
  }, [photoQueue.length])

  const handleRemovePhoto = useCallback((photoIdToRemove: string) => {
    setPhotoQueue((items) => items.filter((photo) => photo.id !== photoIdToRemove))
    setPhotoSuggestions((suggestions) => suggestions.flatMap((suggestion) => {
      const photoIds = suggestion.photoIds.filter((id) => id !== photoIdToRemove)
      return photoIds.length > 0 ? [{ ...suggestion, photoIds }] : []
    }))
    setPhotoInputError(undefined)
  }, [])

  const handleReadPhotos = useCallback(async () => {
    if (busyAction || !current.settings.openRouterKeyConfigured) return
    const photosToRead = photoQueue.filter((photo) => photo.status === 'ready' && photo.imageBase64)
    if (photosToRead.length === 0) return

    setBusyAction('recognize-vocabulary-photo')
    setActionError(undefined)
    setActionMessage(undefined)
    setPhotoReadProgress({ current: 1, total: photosToRead.length })

    try {
      for (const [index, photo] of photosToRead.entries()) {
        setPhotoReadProgress({ current: index + 1, total: photosToRead.length })
        setPhotoQueue((items) => items.map((item) => (
          item.id === photo.id ? { ...item, status: 'reading', error: undefined } : item
        )))

        try {
          const draft = await apiClient.recognizeVocabularyPhoto(photo.imageBase64 ?? '')
          const entries = draft.entries.map(normalizePhotoReviewInput)
          setPhotoQueue((items) => items.map((item) => (
            item.id === photo.id
              ? { ...item, status: entries.length > 0 ? 'done' : 'no-words', error: undefined }
              : item
          )))
          setPhotoCost((cost) => cost + draft.cost)
          setPhotoModel(draft.model)
          setPhotoSuggestions((suggestions) => mergePhotoSuggestions(
            suggestions,
            entries,
            photo.id,
            current.vocabulary,
          ))
        } catch (error: unknown) {
          setPhotoQueue((items) => items.map((item) => (
            item.id === photo.id
              ? { ...item, status: 'failed', error: getErrorMessage(error) }
              : item
          )))
        }
      }
    } finally {
      setPhotoReadProgress(undefined)
      setBusyAction(undefined)
    }
  }, [apiClient, busyAction, current.settings.openRouterKeyConfigured, current.vocabulary, photoQueue])

  const updatePhotoSuggestion = useCallback((suggestionId: string, patch: Partial<ManualVocabularyInput>) => {
    setPhotoSuggestions((suggestions) => suggestions.map((suggestion) => {
      if (suggestion.id !== suggestionId) return suggestion
      const input = { ...suggestion.input, ...patch }
      return {
        ...suggestion,
        input,
        selected: isCompleteManualVocabularyInput(input) ? suggestion.selected : false,
      }
    }))
  }, [])

  const togglePhotoSuggestion = useCallback((suggestionId: string, selected: boolean) => {
    setPhotoSuggestions((suggestions) => suggestions.map((suggestion) => (
      suggestion.id === suggestionId ? { ...suggestion, selected } : suggestion
    )))
  }, [])

  const togglePhotoSuggestionDetails = useCallback((suggestionId: string) => {
    setPhotoSuggestions((suggestions) => suggestions.map((suggestion) => (
      suggestion.id === suggestionId ? { ...suggestion, expanded: !suggestion.expanded } : suggestion
    )))
  }, [])

  const handleSavePhotoEntries = async (entries: ManualVocabularyInput[]) => {
    if (entries.length === 0 || busyAction) return
    setBusyAction('save-photo-vocabulary')
    setActionError(undefined)
    setActionMessage(undefined)
    let savedCount = 0
    try {
      for (const entry of entries) {
        const nextSnapshot = await apiClient.saveManualVocabulary(entry)
        setSnapshot(nextSnapshot)
        savedCount += 1
      }
      setPhotoQueue([])
      setPhotoSuggestions([])
      setPhotoInputError(undefined)
      setPhotoCost(0)
      setPhotoModel(undefined)
      setPhotoReadProgress(undefined)
      setManualFormValue({ ...EMPTY_MANUAL_VOCABULARY_FORM })
      setManualGeneration(undefined)
      setManualVariantPrompt(undefined)
      setActionMessage(`${savedCount} words saved to Vocabulary.`)
      setActiveView('vocabulary')
    } catch (error: unknown) {
      const savedNote = savedCount > 0
        ? `${savedCount} ${savedCount === 1 ? 'word was' : 'words were'} saved before this error. `
        : ''
      setActionError(`${savedNote}${getErrorMessage(error)}`)
    } finally {
      setBusyAction(undefined)
    }
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
      setManualFormValue({ ...EMPTY_MANUAL_VOCABULARY_FORM })
      setManualGeneration(undefined)
      setManualVariantPrompt(undefined)
      setPhotoQueue([])
      setPhotoSuggestions([])
      setPhotoInputError(undefined)
      setPhotoCost(0)
      setPhotoModel(undefined)
      setPhotoReadProgress(undefined)
      setActiveView('vocabulary')
    })
  }

  const handleSelectionAddWord = useCallback((event: SelectionAddWordEvent) => {
    if (!event || typeof event.destination !== 'string') return

    setActionError(undefined)
    if (event.destination === 'vocabulary') {
      setVocabularyFocusQuery((previous) => ({
        query: event.simplified || event.traditional || '',
        token: previous.token + 1,
      }))
      setActionMessage(undefined)
      setActiveView('vocabulary')
      return
    }

    if (event.destination === 'inbox') {
      const word = event.simplified || event.traditional || event.pinyin
      setActiveView('inbox')
      setActionMessage(`Opened the candidate for ${word}.`)
      return
    }

    if (event.destination !== 'add-word') return

    manualGenerationRequest.current += 1
    setActiveView('add-word')
    const selectionForm: ManualVocabularyForm = {
      ...EMPTY_MANUAL_VOCABULARY_FORM,
      simplified: event.simplified,
      traditional: event.traditional ?? '',
      pinyin: event.pinyin,
      meaning: event.meaning === 'No English gloss saved' ? '' : event.meaning,
    }

    if (event.draft) {
      const draft = event.draft
      const variants = draft.variants ?? []
      setManualGeneration(draft)
      if (draft.needsChoice) {
        setManualFormValue(selectionForm)
        setManualVariantPrompt({
          headword: selectionForm.simplified.trim(),
          origin: selectionForm,
          variants,
          appliedIndex: -1,
        })
        setActionMessage('Choose a meaning to fill the empty fields.')
        return
      }

      setManualFormValue(manualVocabularyDraftToForm(draft))
      if (variants.length >= 2) {
        const appliedIndex = draft.appliedVariant >= 0 && draft.appliedVariant < variants.length
          ? draft.appliedVariant
          : -1
        setManualVariantPrompt({
          headword: selectionForm.simplified.trim(),
          origin: selectionForm,
          variants,
          appliedIndex,
        })
      } else {
        setManualVariantPrompt(undefined)
      }
      setActionMessage('Missing details generated. Review every field before saving.')
      return
    }

    setManualFormValue(selectionForm)
    setManualGeneration(undefined)
    setManualVariantPrompt(undefined)
    setActionMessage(
      event.lessonBlocked
        ? 'Missing-detail generation is available again after the active lesson finishes.'
        : undefined,
    )
  }, [])

  useEffect(() => apiClient.subscribeSelectionAddWord(handleSelectionAddWord), [apiClient, handleSelectionAddWord])

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

  const pullBanner = cloudPullBanner(current.settings)

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
        <div className={`page-content ${activeView === 'live' ? 'live-page-content' : ''}`}>
          {isLoading && !snapshot ? (
            <LoadingState />
          ) : (
            <>
              {(backendUnavailable || apiClient.mode === 'demo') && (
                <BackendUnavailable mode={apiClient.mode} onRetry={handleRefresh} />
              )}
              {pullBanner && (
                <InlineAlert tone={pullBanner.tone}>
                  {pullBanner.text}
                </InlineAlert>
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
                  lessonTag={lessonTag}
                  busyAction={busyAction}
                  onConsentChange={setConsent}
                  onLessonTagChange={(value) => {
                    lessonTagTouched.current = true
                    setLessonTag(value)
                  }}
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
                  onSaveCloudKey={saveCloudKey}
                />
              )}
              {activeView === 'transcripts' && <PastTranscriptsView apiClient={apiClient} />}
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
                  variantPrompt={manualVariantPrompt}
                  photoQueue={photoQueue}
                  photoSuggestions={photoSuggestions}
                  photoInputError={photoInputError}
                  photoCost={photoCost}
                  photoModel={photoModel}
                  photoReadProgress={photoReadProgress}
                  vocabulary={current.vocabulary}
                  existingTags={existingTags}
                  hasApiKey={current.settings.openRouterKeyConfigured}
                  lessonActive={['live', 'starting', 'stopping'].includes(current.lesson.status)}
                  busyAction={busyAction}
                  onFormChange={updateManualForm}
                  onGenerate={handleGenerateManualVocabulary}
                  onRegenerate={handleRegenerateManualVocabulary}
                  onApplyVariant={applyManualVariant}
                  onClear={clearManualForm}
                  onPhotoFiles={handlePhotoFiles}
                  onRemovePhoto={handleRemovePhoto}
                  onReadPhotos={handleReadPhotos}
                  onTogglePhotoSuggestion={togglePhotoSuggestion}
                  onUpdatePhotoSuggestion={updatePhotoSuggestion}
                  onTogglePhotoSuggestionDetails={togglePhotoSuggestionDetails}
                  onSavePhotoEntries={handleSavePhotoEntries}
                  onSave={handleSaveManualVocabulary}
                  onOpenSettings={() => setActiveView('settings')}
                />
              )}
              {activeView === 'vocabulary' && (
                <VocabularyView
                  entries={current.vocabulary}
                  onRefresh={handleRefresh}
                  isRefreshing={busyAction === 'refresh'}
                  deletingEntryId={
                    busyAction?.startsWith('delete-vocabulary-')
                      ? busyAction.slice('delete-vocabulary-'.length)
                      : undefined
                  }
                  onDelete={handleDeleteVocabulary}
                  focusQuery={vocabularyFocusQuery}
                />
              )}
              {activeView === 'settings' && (
                <SettingsView
                  snapshot={current}
                  apiMode={apiClient.mode}
                  busyAction={busyAction}
                  onSave={saveSettings}
                  onSyncNow={syncNow}
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
          existingTags={existingTags}
          validationError={editError}
          regenerateError={editRegenerateError}
          regenerateNotice={editRegenerateNotice}
          hasApiKey={current.settings.openRouterKeyConfigured}
          lessonActive={['live', 'starting', 'stopping'].includes(current.lesson.status)}
          isSaving={busyAction === `edit-${editingCandidate.id}`}
          isRegenerating={busyAction === `regenerate-candidate-${editingCandidate.id}`}
          onDraftChange={(draft) => {
            setEditDraft(draft)
            setEditError(undefined)
          }}
          onRegenerate={handleRegenerateCandidate}
          onCancel={() => {
            setEditingCandidate(undefined)
            setEditRegenerateError(undefined)
            setEditRegenerateNotice(undefined)
          }}
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
      {wordPendingDelete && (
        <DeleteVocabularyDialog
          entry={wordPendingDelete}
          isDeleting={busyAction === `delete-vocabulary-${wordPendingDelete.id}`}
          onCancel={() => setWordPendingDelete(undefined)}
          onConfirm={confirmDeleteVocabulary}
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
            <strong>{zdrEnabled ? 'Zero data retention' : 'No model training'}</strong>
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
  lessonTag,
  busyAction,
  onConsentChange,
  onLessonTagChange,
  onStart,
  onStop,
  onOpenSettings,
  onMarkMoment,
  onPermission,
  onSaveCloudKey,
}: {
  snapshot: AppSnapshot
  consent: boolean
  lessonTag: string
  busyAction?: string
  onConsentChange: (value: boolean) => void
  onLessonTagChange: (value: string) => void
  onStart: () => void
  onStop: () => void
  onOpenSettings: () => void
  onMarkMoment: () => void
  onPermission: () => void
  onSaveCloudKey: (key: string) => Promise<boolean>
}) {
  const isLive = snapshot.lesson.status === 'live'
  const isInterrupted = snapshot.lesson.status === 'error' && Boolean(snapshot.lesson.id)
  const hasActiveLesson = isLive || isInterrupted
  const isStarting = busyAction === 'start-lesson'
  const isStopping = busyAction === 'stop-lesson'
  const canInteract = snapshot.connection !== 'unavailable'
  const hasApiKey = snapshot.settings.openRouterKeyConfigured
  const captureReady = snapshot.capturePermission === 'granted'
  const setupReady = hasApiKey && captureReady
  const cloudAccess = snapshot.settings.supabaseAccessStatus ?? 'unchecked'
  const cloudReady = cloudAccess === 'ready'
  const cloudNeedsKey = cloudAccess === 'missing' || cloudAccess === 'invalid'
  const [cloudKeyDraft, setCloudKeyDraft] = useState('')
  const pendingCandidates = snapshot.candidates.filter((candidate) => candidate.status === 'pending').length

  return (
    <div className="view-stack live-view-stack">
      <section className="page-heading live-page-heading">
        <div>
          <p className="eyebrow">Capture room</p>
          <h1>Live Lesson</h1>
          <p className="page-intro">Follow the conversation here. Setup stays beside the transcript.</p>
        </div>
        <div className="heading-actions">
          <PrivacyBadge zdrEnabled={snapshot.privacy.zdrEnabled} />
          <span className="soft-badge">
            <span className="status-dot status-dot-blue" aria-hidden="true" />
            {pendingCandidates} candidates
          </span>
          <span className="soft-badge">{formatUsd(snapshot.cost.currentUsd)}</span>
        </div>
      </section>

      {(snapshot.cost.warning || snapshot.cost.hardExceeded) && <CostStatusAlert cost={snapshot.cost} />}

      <div className="live-workspace">
        <div className="live-controls">
        <section className="panel capture-panel">
          <PanelHeading
            eyebrow="Capture setup"
            title="Listen to any app"
            description="System audio is transcribed when the speech is Chinese. Your microphone is included when it is enabled in Settings."
          />
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
            <AudioMeter label="System audio" value={snapshot.levels.remote} color="purple" />
          </div>
          <div className="capture-note">
            <Icon name="shield" />
            <span>
              {snapshot.privacy.zdrEnabled
                ? 'ZDR controls provider retention. Local audio follows the selected retention policy.'
                : 'Provider data collection is denied. Local audio follows the selected retention policy.'}
            </span>
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
                    <p>Run these checks again after changing macOS permissions or the cloud key.</p>
                  </div>
                  <button className="button button-secondary button-small" type="button" onClick={onPermission} disabled={!canInteract || Boolean(busyAction)}>
                    {busyAction === 'permission' ? 'Checking…' : 'Check setup'}
                  </button>
                </div>
                <SetupCheck label="OpenRouter API key" detail={hasApiKey ? 'Saved securely' : 'Add a key in Settings'} status={hasApiKey ? 'ready' : 'blocked'} />
                <SetupCheck
                  label="Cloud vocabulary key"
                  detail={snapshot.settings.supabaseAccessDetail || (cloudAccess === 'checking' ? 'Checking cloud vocabulary access.' : 'Not checked')}
                  status={cloudReady ? 'ready' : cloudAccess === 'unchecked' || cloudAccess === 'checking' ? 'pending' : 'blocked'}
                />
                <SetupCheck
                  label="Screen and system audio"
                  detail={captureReady ? 'Permission granted' : snapshot.capturePermission === 'denied' ? 'Permission denied' : 'Not checked'}
                  status={captureReady ? 'ready' : snapshot.capturePermission === 'denied' ? 'blocked' : 'pending'}
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
                {cloudNeedsKey && (
                  <form
                    className="cloud-key-form"
                    onSubmit={(event) => {
                      event.preventDefault()
                      if (!cloudKeyDraft.trim() || busyAction === 'save-cloud-key') return
                      void onSaveCloudKey(cloudKeyDraft.trim()).then((saved) => {
                        if (saved) setCloudKeyDraft('')
                      })
                    }}
                  >
                    <label className="field-label" htmlFor="cloud-vocabulary-key">Cloud vocabulary key</label>
                    <input
                      id="cloud-vocabulary-key"
                      className="text-input"
                      type="password"
                      value={cloudKeyDraft}
                      onChange={(event) => setCloudKeyDraft(event.target.value)}
                      autoComplete="off"
                      spellCheck={false}
                      placeholder="Supabase secret key"
                    />
                    <p className="helper-text">
                      {cloudAccess === 'invalid'
                        ? 'This key cannot read and write cloud vocabulary. Enter a replacement. On this Mac it is saved in the Keychain.'
                        : 'Add the Supabase secret key for this computer. On this Mac it is saved in the Keychain.'}
                    </p>
                    <button className="button button-secondary button-small" type="submit" disabled={!cloudKeyDraft.trim() || busyAction === 'save-cloud-key'}>
                      {busyAction === 'save-cloud-key' ? 'Saving…' : 'Save key'}
                    </button>
                  </form>
                )}
              </div>
              <div className="lesson-tag-field">
                <label className="field-label" htmlFor="lesson-tag">Lesson tag</label>
                <input
                  id="lesson-tag"
                  className="text-input"
                  value={lessonTag}
                  onChange={(event) => onLessonTagChange(event.target.value)}
                  disabled={!canInteract || isStarting}
                  spellCheck={false}
                  autoComplete="off"
                />
                <p className="helper-text">New words extracted from this lesson get this tag. Separate extra tags with commas.</p>
              </div>
              <button className="button button-primary start-button" type="button" onClick={onStart} disabled={!consent || !canInteract || !setupReady || isStarting}>
                <Icon name="play" />
                {isStarting ? 'Starting lesson…' : 'Start lesson'}
              </button>
              {!hasApiKey
                ? <p className="helper-text">Configure an API key in Settings to enable lesson capture.</p>
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
              {formatLessonTags(snapshot.lesson.tags) && (
                <p className="helper-text">New words from this lesson are tagged {formatLessonTags(snapshot.lesson.tags)}.</p>
              )}
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
        <TranscriptPanel
          snapshot={snapshot}
          isLive={isLive}
          busyAction={busyAction}
          onMarkMoment={onMarkMoment}
        />
      </div>
    </div>
  )
}

function PastTranscriptsView({ apiClient }: { apiClient: AppApi }) {
  const [lessons, setLessons] = useState<LessonTranscriptSummary[]>()
  const [listError, setListError] = useState<string>()
  const [listLoading, setListLoading] = useState(true)
  const [selected, setSelected] = useState<LessonTranscript>()
  const [selectedId, setSelectedId] = useState<string>()
  const [transcriptLoading, setTranscriptLoading] = useState(false)
  const [transcriptError, setTranscriptError] = useState<string>()

  const loadLessons = useCallback(() => {
    setListLoading(true)
    setListError(undefined)
    void apiClient
      .listLessonTranscripts(30)
      .then((nextLessons) => setLessons(nextLessons))
      .catch((error: unknown) => setListError(getErrorMessage(error)))
      .finally(() => setListLoading(false))
  }, [apiClient])

  useEffect(() => {
    loadLessons()
  }, [loadLessons])

  const chooseLesson = (lesson: LessonTranscriptSummary) => {
    setSelectedId(lesson.id)
    setTranscriptLoading(true)
    setTranscriptError(undefined)
    void apiClient
      .getLessonTranscript(lesson.id)
      .then((nextLesson) => setSelected(nextLesson))
      .catch((error: unknown) => {
        setSelected(undefined)
        setTranscriptError(getErrorMessage(error))
      })
      .finally(() => setTranscriptLoading(false))
  }

  return (
    <div className="view-stack">
      <section className="page-heading">
        <div>
          <p className="eyebrow">Lesson history</p>
          <h1>Past transcripts</h1>
          <p className="page-intro">Choose a saved lesson and read its transcript. Nothing here is changed or deleted.</p>
        </div>
      </section>
      <div className="past-page-grid">
        <section className="panel">
          <PanelHeading eyebrow="Saved lessons" title="Choose a transcript" description="Recent lessons are listed by the time they started." />
          {listError && <div className="past-transcripts-error" role="alert">{listError}</div>}
          {listLoading && <p className="transcript-loading" role="status">Loading lessons…</p>}
          {!listLoading && !listError && lessons?.length === 0 && (
            <p className="past-transcripts-empty">No past lessons yet.</p>
          )}
          {!listLoading && !listError && lessons && lessons.length > 0 && (
            <div className="past-lesson-list" role="listbox" aria-label="Past transcripts">
              {lessons.map((lesson) => (
                <button
                  className="past-lesson-option"
                  type="button"
                  role="option"
                  aria-selected={selectedId === lesson.id}
                  key={lesson.id}
                  onClick={() => chooseLesson(lesson)}
                >
                  <span className="past-lesson-date">{formatLessonDate(lesson.startedAt)}</span>
                  <span className="past-lesson-title">{lesson.title || 'Lesson'}</span>
                  <span className="past-lesson-meta">
                    {lesson.segmentCount} {lesson.segmentCount === 1 ? 'segment' : 'segments'}
                    {lesson.finalCost > 0 && ` · ${formatUsd(lesson.finalCost)}`}
                  </span>
                </button>
              ))}
            </div>
          )}
        </section>
        <section className="panel transcript-panel past-transcript-panel">
          <PanelHeading
            eyebrow="Transcript"
            title={selected ? formatLessonDate(selected.lesson.startedAt) : 'No lesson selected'}
            description={selected ? `${selected.lesson.title || 'Lesson'} · read only` : 'Select a lesson on the left.'}
          />
          {transcriptLoading && <p className="transcript-loading" role="status">Loading transcript…</p>}
          {transcriptError && <div className="transcript-history-error" role="alert">{transcriptError}</div>}
          {selected && selected.lines.length > 0 ? (
            <TranscriptList lines={selected.lines} isHistory />
          ) : !transcriptLoading && !transcriptError && (
            <EmptyState
              icon="history"
              title={selected ? 'This lesson has no transcript' : 'Choose a past lesson'}
              description={selected ? 'No captured lines were saved for this lesson.' : 'The transcript opens here and stays in simplified Chinese, with pinyin.'}
            />
          )}
        </section>
      </div>
    </div>
  )
}

function TranscriptPanel({
  snapshot,
  isLive,
  busyAction,
  onMarkMoment,
}: {
  snapshot: AppSnapshot
  isLive: boolean
  busyAction?: string
  onMarkMoment: () => void
}) {
  return (
    <section className="panel transcript-panel">
      <div className="panel-heading-row">
        <PanelHeading
          eyebrow="Live transcript"
          title="Conversation thread"
          description="Source labels help you notice what to revisit."
        />
        <div className="transcript-panel-actions">
          <button className="button button-mark" type="button" onClick={onMarkMoment} disabled={!isLive || Boolean(busyAction)}>
            <Icon name="bookmark" />
            Mark Moment
            <kbd>M</kbd>
          </button>
        </div>
      </div>
      {snapshot.transcript.length > 0 ? (
        <TranscriptList lines={snapshot.transcript} isHistory={false} />
      ) : (
        <EmptyState
          icon="waveform"
          title="Your transcript will appear here"
          description="Start a lesson to see microphone and remote audio in one rolling thread."
        />
      )}
    </section>
  )
}

function TranscriptList({ lines, isHistory }: { lines: AppSnapshot['transcript']; isHistory: boolean }) {
  const listRef = useRef<HTMLDivElement>(null)
  const [isFollowing, setIsFollowing] = useState(!isHistory)
  const [announcement, setAnnouncement] = useState('')
  const latestLineId = useRef<string | undefined>(undefined)
  const hasRendered = useRef(false)
  const scrollIntent = useRef<'start' | 'end' | null>(null)

  const setScrollPosition = (position: 'start' | 'end') => {
    const node = listRef.current
    if (!node) return
    scrollIntent.current = position
    node.scrollTop = position === 'start' ? 0 : node.scrollHeight
    queueMicrotask(() => {
      if (scrollIntent.current === position) scrollIntent.current = null
    })
  }

  useEffect(() => {
    latestLineId.current = lines.at(-1)?.id
    hasRendered.current = true
    setIsFollowing(!isHistory)
    setScrollPosition(isHistory ? 'start' : 'end')
  }, [isHistory])

  useEffect(() => {
    const latest = lines.at(-1)
    if (!latest) return
    if (hasRendered.current && !isHistory && latest.id !== latestLineId.current) {
      setAnnouncement(`${latest.speaker ?? sourceLabel(latest.source)}: ${latest.text}${latest.pinyin ? `，${latest.pinyin}` : ''}`)
    }
    latestLineId.current = latest.id
    hasRendered.current = true
    if (isHistory) {
      setScrollPosition('start')
    } else if (isFollowing) {
      setScrollPosition('end')
    }
  }, [isFollowing, isHistory, lines])

  const handleScroll = () => {
    const node = listRef.current
    if (!node || isHistory || scrollIntent.current) return
    const distanceFromBottom = node.scrollHeight - node.scrollTop - node.clientHeight
    setIsFollowing(distanceFromBottom <= 24)
  }

  const resumeFollowing = () => {
    setScrollPosition('end')
    setIsFollowing(true)
  }

  return (
    <>
      <div className="transcript-list" ref={listRef} onScroll={handleScroll} tabIndex={0} aria-label="Transcript lines">
        {lines.map((line) => <TranscriptRow key={line.id} line={line} />)}
      </div>
      {!isHistory && !isFollowing && (
        <button className="button button-secondary resume-live-button" type="button" onClick={resumeFollowing}>
          Resume live
        </button>
      )}
      <div className="visually-hidden" aria-live="polite" aria-atomic="true">{announcement}</div>
    </>
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
  variantPrompt,
  photoQueue,
  photoSuggestions,
  photoInputError,
  photoCost,
  photoModel,
  photoReadProgress,
  vocabulary,
  existingTags,
  hasApiKey,
  lessonActive,
  busyAction,
  onFormChange,
  onGenerate,
  onRegenerate,
  onApplyVariant,
  onClear,
  onPhotoFiles,
  onRemovePhoto,
  onReadPhotos,
  onTogglePhotoSuggestion,
  onUpdatePhotoSuggestion,
  onTogglePhotoSuggestionDetails,
  onSavePhotoEntries,
  onSave,
  onOpenSettings,
}: {
  form: ManualVocabularyForm
  generatedDraft?: ManualVocabularyDraft
  variantPrompt?: ManualVariantPrompt
  photoQueue: PhotoQueueItem[]
  photoSuggestions: PhotoSuggestion[]
  photoInputError?: string
  photoCost: number
  photoModel?: string
  photoReadProgress?: { current: number; total: number }
  vocabulary: AppSnapshot['vocabulary']
  existingTags: string[]
  hasApiKey: boolean
  lessonActive: boolean
  busyAction?: string
  onFormChange: (form: ManualVocabularyForm) => void
  onGenerate: () => void
  onRegenerate: (instruction: string) => Promise<boolean>
  onApplyVariant: (index: number) => void
  onClear: () => void
  onPhotoFiles: (files: File[]) => void
  onRemovePhoto: (photoId: string) => void
  onReadPhotos: () => void
  onTogglePhotoSuggestion: (suggestionId: string, selected: boolean) => void
  onUpdatePhotoSuggestion: (suggestionId: string, patch: Partial<ManualVocabularyInput>) => void
  onTogglePhotoSuggestionDetails: (suggestionId: string) => void
  onSavePhotoEntries: (entries: ManualVocabularyInput[]) => void
  onSave: (event: FormEvent<HTMLFormElement>) => void
  onOpenSettings: () => void
}) {
  const isGenerating = busyAction === 'generate-manual-vocabulary'
  const isRegenerating = busyAction === 'regenerate-manual-vocabulary'
  const isSaving = busyAction === 'save-manual-vocabulary'
  const isBusy = Boolean(busyAction)
  const missingFields = manualVocabularyMissingFields(form)
  const hasCompleteDetails = missingFields.length === 0
  const canGenerate = Boolean(form.simplified.trim()) && hasApiKey && !lessonActive && !hasCompleteDetails && !isBusy
  const showRegenerateOption = Boolean(form.simplified.trim()) && (hasCompleteDetails || Boolean(generatedDraft))
  const canRegenerate = showRegenerateOption && hasApiKey && !lessonActive && !isBusy
  const [regenerateOpen, setRegenerateOpen] = useState(false)
  const [regeneratePrompt, setRegeneratePrompt] = useState('')
  const canClear = !isBusy && (
    Boolean(variantPrompt || generatedDraft) ||
    Object.entries(form).some(([, value]) => typeof value === 'string' && value.trim())
  )
  const headword = form.simplified.trim()
  useEffect(() => {
    setRegenerateOpen(false)
    setRegeneratePrompt('')
  }, [headword])
  const variantPickerRef = useRef<HTMLFieldSetElement>(null)
  const variantKey = variantPrompt
    ? `${variantPrompt.headword}:${variantPrompt.variants.map((variant) => `${variant.pinyin}:${variant.meaning}`).join('|')}`
    : ''
  useEffect(() => {
    if (!variantKey || typeof variantPickerRef.current?.scrollIntoView !== 'function') return
    variantPickerRef.current.scrollIntoView({ block: 'center' })
  }, [variantKey])

  return (
    <div className="view-stack">
      <section className="page-heading">
        <div>
          <p className="eyebrow">Vocabulary builder</p>
          <h1>Add Word</h1>
          <p className="page-intro">
            Drop photos of notes, slides, or a textbook page. Suggested words stay here until you choose what to save.
          </p>
        </div>
        <div className="heading-actions">
          <span className="soft-badge"><Icon name="spark" /> Editable draft</span>
        </div>
      </section>

      <div className="manual-vocabulary-layout">
        <div className="manual-vocabulary-main">
          <PhotoDropZone
            photoQueue={photoQueue}
            photoInputError={photoInputError}
            hasApiKey={hasApiKey}
            isPreparing={photoQueue.some((photo) => photo.status === 'preparing')}
            isReading={busyAction === 'recognize-vocabulary-photo'}
            photoCost={photoCost}
            photoModel={photoModel}
            photoReadProgress={photoReadProgress}
            onPhotoFiles={onPhotoFiles}
            onRemovePhoto={onRemovePhoto}
            onReadPhotos={onReadPhotos}
            onOpenSettings={onOpenSettings}
          />
          {photoSuggestions.length > 0 && (
            <PhotoVocabularyReview
              suggestions={photoSuggestions}
              photoQueue={photoQueue}
              vocabulary={vocabulary}
              existingTags={existingTags}
              isSaving={busyAction === 'save-photo-vocabulary'}
              onToggle={onTogglePhotoSuggestion}
              onUpdate={onUpdatePhotoSuggestion}
              onToggleDetails={onTogglePhotoSuggestionDetails}
              onSave={onSavePhotoEntries}
            />
          )}
          <form className="panel manual-vocabulary-form" onSubmit={onSave}>
          <div className="panel-heading">
            <p className="eyebrow">Type a word</p>
            <h2>Or add one word yourself</h2>
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
            <ExistingTagPicker
              tags={existingTags}
              value={form.tags}
              onChange={(tags) => onFormChange({ ...form, tags })}
            />
          </div>

          {lessonActive && (
            <KeyBanner icon="waveform" title="Generation pauses during live lessons">
              Finish the lesson before generating missing details; photo reading remains available during capture.
            </KeyBanner>
          )}

          {generatedDraft && (
            <div className="manual-generation-meta" role="status" aria-live="polite">
              <div className="manual-generation-meta-icon"><Icon name="spark" /></div>
              <div>
                <strong>Generated with {generatedDraft.model}</strong>
                <p>Actual request cost: {formatUsd(generatedDraft.cost)}. The default analyzer uses an OpenRouter zero-data-retention endpoint.</p>
                {generatedDraft.matchedContext && !generatedDraft.needsChoice && (
                  <p>Used the details you already entered.</p>
                )}
              </div>
            </div>
          )}

          {hasCompleteDetails && (
            <div className="manual-complete-note" role="status">
              <Icon name="check" />
              <span>All learning-critical details are complete. You can save directly; generation is not needed.</span>
            </div>
          )}

          {variantPrompt && variantPrompt.variants.length > 1 && (
            <fieldset className="manual-variant-picker" ref={variantPickerRef}>
              <legend>Which meaning do you want?</legend>
              <p className="manual-variant-explanation">
                This word has more than one reading or meaning. Pick one to fill the empty fields. Nothing is saved yet.
              </p>
              <div className="manual-variant-options" role="radiogroup" aria-label="Meaning choices">
                {variantPrompt.variants.map((variant, index) => {
                  const radioId = `manual-variant-${index}`
                  const isSelected = variantPrompt.appliedIndex === index
                  return (
                    <label
                      className={`manual-variant-card${isSelected ? ' is-selected' : ''}`}
                      htmlFor={radioId}
                      key={`${variant.meaning}-${variant.pinyin}-${index}`}
                    >
                      <input
                        id={radioId}
                        type="radio"
                        name="manual-vocabulary-variant"
                        checked={isSelected}
                        onChange={() => onApplyVariant(index)}
                        aria-label={`${variant.meaning}, ${variant.pinyin}`}
                      />
                      <span className="manual-variant-card-content">
                        <strong>{variant.meaning}</strong>
                        <span className="manual-variant-detail">
                          {[variant.pinyin, variant.partOfSpeech, variant.classifier].filter(Boolean).join(' · ')}
                        </span>
                        <span className="manual-variant-example">{variant.example}</span>
                        <span className="manual-variant-translation">{variant.exampleTranslation}</span>
                      </span>
                    </label>
                  )
                })}
              </div>
            </fieldset>
          )}

          {regenerateOpen && showRegenerateOption && (
            <RegeneratePanel
              idPrefix="manual"
              prompt={regeneratePrompt}
              onPromptChange={setRegeneratePrompt}
              hasApiKey={hasApiKey}
              lessonActive={lessonActive}
              isRegenerating={isRegenerating}
              disabled={!canRegenerate}
              onClose={() => setRegenerateOpen(false)}
              onSubmit={() => {
                void onRegenerate(regeneratePrompt).then((didRegenerate) => {
                  if (didRegenerate) {
                    setRegenerateOpen(false)
                    setRegeneratePrompt('')
                  }
                })
              }}
            />
          )}

          <div className="manual-action-row">
            <button className="button button-secondary manual-clear-button" type="button" onClick={onClear} disabled={!canClear}>
              Clear form
            </button>
            <button className="button button-secondary" type="button" onClick={onGenerate} disabled={!canGenerate}>
              <Icon name="spark" />
              {isGenerating ? 'Generating…' : 'Generate missing details'}
            </button>
            {showRegenerateOption && (
              <button
                className={`button button-secondary${regenerateOpen ? ' is-active' : ''}`}
                type="button"
                aria-expanded={regenerateOpen}
                onClick={() => setRegenerateOpen((open) => !open)}
                disabled={isBusy}
              >
                <Icon name="spark" />
                {isRegenerating ? 'Regenerating…' : 'Regenerate'}
              </button>
            )}
            <button className="button button-primary" type="submit" disabled={isBusy || !form.simplified.trim() || !hasCompleteDetails}>
              <Icon name="check" />
              {isSaving ? 'Saving…' : 'Save to Vocabulary'}
            </button>
          </div>
          {variantPrompt && variantPrompt.appliedIndex < 0
            ? <p className="helper-text">The form stays as you typed it until you pick a meaning.</p>
            : lessonActive
            ? <p className="helper-text">Missing-detail generation is available again after the active lesson finishes. Photo reading remains available.</p>
            : !hasApiKey
            ? <p className="helper-text">Generation is unavailable without a key, but saving does not use AI.</p>
            : hasCompleteDetails
            ? <p className="helper-text">Your form is ready to save without another AI request.</p>
            : <p className="helper-text">Missing pinyin, meaning, or sample details must be completed before saving.</p>}
          </form>
        </div>

        <aside className="manual-vocabulary-side">
          <section className="panel manual-guide-card">
            <PanelHeading
              eyebrow="Simple workflow"
              title="Review before you save"
              description="Photos create editable suggestions. Your final review is always in control."
            />
            <ol className="manual-workflow-list">
              <li><span>1</span><p>Drop notes, slides, or textbook photos to find vocabulary.</p></li>
              <li><span>2</span><p>Review the combined suggestions and edit details when needed.</p></li>
              <li><span>3</span><p>Save only the words you choose to your Vocabulary collection.</p></li>
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

function PhotoDropZone({
  photoQueue,
  photoInputError,
  hasApiKey,
  isPreparing,
  isReading,
  photoCost,
  photoModel,
  photoReadProgress,
  onPhotoFiles,
  onRemovePhoto,
  onReadPhotos,
  onOpenSettings,
}: {
  photoQueue: PhotoQueueItem[]
  photoInputError?: string
  hasApiKey: boolean
  isPreparing: boolean
  isReading: boolean
  photoCost: number
  photoModel?: string
  photoReadProgress?: { current: number; total: number }
  onPhotoFiles: (files: File[]) => void
  onRemovePhoto: (photoId: string) => void
  onReadPhotos: () => void
  onOpenSettings: () => void
}) {
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleFiles = useCallback((files: File[]) => {
    if (files.length === 0 || isReading) return
    onPhotoFiles(files)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }, [isReading, onPhotoFiles])

  useEffect(() => {
    const handlePaste = (event: globalThis.ClipboardEvent) => {
      const clipboardFiles = Array.from(event.clipboardData?.files ?? [])
      const itemFiles = clipboardFiles.length > 0
        ? clipboardFiles
        : Array.from(event.clipboardData?.items ?? [])
          .filter((item) => item.kind === 'file')
          .map((item) => item.getAsFile())
          .filter((file): file is File => Boolean(file))
      if (itemFiles.length === 0) return
      event.preventDefault()
      handleFiles(itemFiles)
    }
    window.addEventListener('paste', handlePaste)
    return () => window.removeEventListener('paste', handlePaste)
  }, [handleFiles])

  const choosePhoto = () => fileInputRef.current?.click()
  const handleDrop = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    handleFiles(Array.from(event.dataTransfer.files))
  }

  return (
    <section className="panel photo-reader-panel">
      <div className="panel-heading">
        <p className="eyebrow">Photo reader</p>
        <h2>Find words in photos</h2>
        <p>Suggested words stay separate from the manual form until you choose what to save.</p>
      </div>
      <div
        className={`photo-drop-zone ${photoQueue.length > 0 ? 'has-photos' : ''}`}
        onDragOver={(event) => event.preventDefault()}
        onDrop={handleDrop}
      >
        {photoQueue.length === 0 ? (
          <div className="photo-empty-state">
            <div className="photo-empty-icon"><Icon name="spark" /></div>
            <div>
              <strong>Drop photos here</strong>
              <span>JPEG, PNG, WebP, HEIC, or HEIF. You can add up to 8.</span>
            </div>
            <button className="button button-secondary button-small" type="button" onClick={choosePhoto} disabled={isPreparing || isReading}>Choose photos</button>
          </div>
        ) : (
          <>
            <div className="photo-queue" aria-label="Queued photos">
              {photoQueue.map((photo, index) => (
                <article className="photo-queue-item" key={photo.id}>
                  {photo.previewDataUrl ? (
                    <img className="photo-preview" src={photo.previewDataUrl} alt={`Photo ${index + 1} preview`} />
                  ) : (
                    <div className="photo-preview photo-preview-placeholder" aria-hidden="true"><Icon name="spark" /></div>
                  )}
                  <div className="photo-queue-copy">
                    <strong>Photo {index + 1}</strong>
                    <span className={`photo-queue-status photo-queue-status-${photo.status}`}>
                      {photoStatusLabel(photo.status)}
                    </span>
                    {photo.error && <span className="photo-reader-error" role="alert">{photo.error}</span>}
                  </div>
                  <button
                    className="button button-ghost button-small photo-remove-button"
                    type="button"
                    onClick={() => onRemovePhoto(photo.id)}
                    disabled={isReading}
                    aria-label={`Remove photo ${index + 1}`}
                  >
                    Remove
                  </button>
                </article>
              ))}
            </div>
            <button className="button button-secondary button-small photo-add-button" type="button" onClick={choosePhoto} disabled={isPreparing || isReading}>
              Add more photos
            </button>
          </>
        )}
        <input
          ref={fileInputRef}
          className="visually-hidden"
          type="file"
          aria-label="Choose a vocabulary photo"
          multiple
          accept="image/jpeg,image/png,image/webp,image/heic,image/heif,.jpg,.jpeg,.png,.webp,.heic,.heif"
          onChange={(event) => handleFiles(Array.from(event.target.files ?? []))}
        />
      </div>
      {isPreparing && <p className="photo-reader-status" role="status">Preparing photo previews…</p>}
      {photoInputError && <div className="photo-reader-error" role="alert">{photoInputError}</div>}
      {!hasApiKey && (
        <KeyBanner
          className="photo-key-alert"
          icon="spark"
          title="AI generation needs an API key"
          action={(
            <button className="button button-secondary button-small" type="button" onClick={onOpenSettings} disabled={isReading}>
              Open Settings
            </button>
          )}
        >
          Add an OpenRouter key in Settings to find words in photos. A complete manual form can still be saved.
        </KeyBanner>
      )}
      <div className="photo-reader-action-row">
        <div>
          {photoReadProgress && <p className="photo-reader-status" role="status">Reading {photoReadProgress.current} of {photoReadProgress.total}</p>}
          {!photoReadProgress && <p className="helper-text">Learning details stay editable before saving.</p>}
        </div>
        <button
          className="button button-primary"
          type="button"
          onClick={onReadPhotos}
          aria-label="Find words"
          disabled={!photoQueue.some((photo) => photo.status === 'ready') || !hasApiKey || isPreparing || isReading}
        >
          <Icon name="spark" />
          {isReading ? 'Reading…' : 'Find words'}
        </button>
      </div>
      {photoModel && (
        <div className="manual-generation-meta photo-generation-meta" role="status" aria-live="polite">
          <div className="manual-generation-meta-icon"><Icon name="spark" /></div>
          <div>
            <strong>Read with {photoModel}</strong>
            <p>Actual request cost: {formatPhotoCost(photoCost)}.</p>
          </div>
        </div>
      )}
    </section>
  )
}

function PhotoVocabularyReview({
  suggestions,
  photoQueue,
  vocabulary,
  existingTags,
  isSaving,
  onToggle,
  onUpdate,
  onToggleDetails,
  onSave,
}: {
  suggestions: PhotoSuggestion[]
  photoQueue: PhotoQueueItem[]
  vocabulary: AppSnapshot['vocabulary']
  existingTags: string[]
  isSaving: boolean
  onToggle: (suggestionId: string, selected: boolean) => void
  onUpdate: (suggestionId: string, patch: Partial<ManualVocabularyInput>) => void
  onToggleDetails: (suggestionId: string) => void
  onSave: (entries: ManualVocabularyInput[]) => void
}) {
  const selectedEntries = suggestions
    .filter((suggestion) => suggestion.selected && isCompleteManualVocabularyInput(suggestion.input))
    .map((suggestion) => normalizePhotoReviewInput(suggestion.input))

  return (
    <section className="panel photo-review-panel">
      <div className="panel-heading-row">
        <div className="panel-heading">
          <p className="eyebrow">Photo review</p>
          <h2>Suggested words</h2>
          <p>Review the combined suggestions, then choose which complete words to save.</p>
        </div>
        <span className="soft-badge">{selectedEntries.length} selected</span>
      </div>
      <div className="photo-review-list">
        {suggestions.map((suggestion, index) => {
          const missingFields = manualVocabularyInputMissingFields(suggestion.input)
          const isComplete = missingFields.length === 0
          const alreadySaved = vocabulary.some((entry) => entry.simplified.trim() === suggestion.input.simplified.trim())
          const sourceLabels = suggestion.photoIds
            .map((photoId) => photoQueue.findIndex((photo) => photo.id === photoId) + 1)
            .filter((photoNumber) => photoNumber > 0)
            .map((photoNumber) => `Photo ${photoNumber}`)
          const fieldId = (field: string) => `photo-${index}-${field}`
          return (
            <article className={`photo-review-card ${isComplete ? '' : 'is-incomplete'}`} key={suggestion.id}>
              <div className="photo-suggestion-row">
                <label className="photo-review-select">
                  <input
                    type="checkbox"
                    aria-label={suggestion.input.simplified}
                    checked={suggestion.selected}
                    disabled={!isComplete || isSaving}
                    onChange={(event) => onToggle(suggestion.id, event.target.checked)}
                  />
                  <span className="photo-review-word">{suggestion.input.simplified}</span>
                </label>
                <span className="photo-suggestion-pinyin">{suggestion.input.pinyin || '—'}</span>
                <span className="photo-suggestion-meaning">{suggestion.input.meaning || '—'}</span>
                <span className="photo-suggestion-sources">
                  {sourceLabels.length > 0
                    ? sourceLabels.map((label) => <span key={label}>{label}</span>)
                    : 'Photo'}
                </span>
                {alreadySaved && <span className="soft-badge photo-already-saved">Already saved</span>}
                <button
                  className="button button-ghost button-small photo-edit-button"
                  type="button"
                  aria-label={`Edit ${suggestion.input.simplified}`}
                  aria-expanded={suggestion.expanded}
                  onClick={() => onToggleDetails(suggestion.id)}
                  disabled={isSaving}
                >
                  Edit
                </button>
              </div>
              {!isComplete && (
                <p className="photo-review-incomplete" role="status">
                  Missing: {missingFields.join(', ')}
                </p>
              )}
              {suggestion.expanded && (
                <div className="photo-review-fields">
                  <PhotoReviewField id={fieldId('pinyin')} label="Pinyin" value={suggestion.input.pinyin ?? ''} onChange={(value) => onUpdate(suggestion.id, { pinyin: value })} />
                  <PhotoReviewField id={fieldId('meaning')} label="Meaning" value={suggestion.input.meaning ?? ''} onChange={(value) => onUpdate(suggestion.id, { meaning: value })} />
                  <PhotoReviewField id={fieldId('traditional')} label="Traditional" value={suggestion.input.traditional ?? ''} onChange={(value) => onUpdate(suggestion.id, { traditional: value })} />
                  <PhotoReviewField id={fieldId('part-of-speech')} label="Part of speech" value={suggestion.input.partOfSpeech ?? ''} onChange={(value) => onUpdate(suggestion.id, { partOfSpeech: value })} />
                  <PhotoReviewField id={fieldId('classifier')} label="Classifier" value={suggestion.input.classifier ?? ''} onChange={(value) => onUpdate(suggestion.id, { classifier: value })} />
                  <PhotoReviewField id={fieldId('example')} label="Example" value={suggestion.input.example ?? ''} onChange={(value) => onUpdate(suggestion.id, { example: value })} wide />
                  <PhotoReviewField id={fieldId('example-pinyin')} label="Example pinyin" value={suggestion.input.examplePinyin ?? ''} onChange={(value) => onUpdate(suggestion.id, { examplePinyin: value })} />
                  <PhotoReviewField id={fieldId('example-translation')} label="Example translation" value={suggestion.input.exampleTranslation ?? ''} onChange={(value) => onUpdate(suggestion.id, { exampleTranslation: value })} />
                  <PhotoReviewField id={fieldId('tags')} label="Tags" value={(suggestion.input.tags ?? []).join(', ')} onChange={(value) => onUpdate(suggestion.id, { tags: parseTags(value) })} wide />
                  <ExistingTagPicker
                    tags={existingTags}
                    value={(suggestion.input.tags ?? []).join(', ')}
                    onChange={(value) => onUpdate(suggestion.id, { tags: parseTags(value) })}
                  />
                </div>
              )}
            </article>
          )
        })}
      </div>
      <div className="photo-review-actions">
        <p className="helper-text">Unchecked words are not saved.</p>
        <button className="button button-primary" type="button" onClick={() => onSave(selectedEntries)} disabled={isSaving || selectedEntries.length === 0}>
          <Icon name="check" />
          {isSaving ? 'Saving selected…' : 'Save selected'}
        </button>
      </div>
    </section>
  )
}

function PhotoReviewField({
  id,
  label,
  value,
  onChange,
  wide = false,
}: {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  wide?: boolean
}) {
  return (
    <div className={`field-group photo-review-field ${wide ? 'photo-review-field-wide' : ''}`}>
      <label className="field-label" htmlFor={id}>{label}</label>
      {wide || label === 'Example'
        ? <textarea id={id} className="text-input textarea" value={value} onChange={(event) => onChange(event.target.value)} rows={2} />
        : <input id={id} className="text-input" value={value} onChange={(event) => onChange(event.target.value)} />}
    </div>
  )
}

function VocabularyView({
  entries,
  onRefresh,
  isRefreshing,
  deletingEntryId,
  onDelete,
  focusQuery,
}: {
  entries: AppSnapshot['vocabulary']
  onRefresh: () => void
  isRefreshing: boolean
  deletingEntryId?: string
  onDelete: (entry: AppSnapshot['vocabulary'][number]) => void
  focusQuery?: VocabularyFocusQuery
}) {
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<VocabularyStatus | 'all'>('all')
  const [tag, setTag] = useState('all')
  const [page, setPage] = useState(1)
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
  const pageSize = 9
  const pageCount = Math.max(1, Math.ceil(filteredEntries.length / pageSize))
  const currentPage = Math.min(page, pageCount)
  const pageEntries = filteredEntries.slice((currentPage - 1) * pageSize, currentPage * pageSize)

  useEffect(() => {
    setPage(1)
  }, [query, status, tag])

  useEffect(() => {
    if (!focusQuery?.query) return
    setQuery(focusQuery.query)
    setPage(1)
  }, [focusQuery])

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
        <>
          <section className="vocabulary-grid" aria-live="polite">
            {pageEntries.map((entry) => (
              <VocabularyCard
                entry={entry}
                key={entry.id}
                isDeleting={deletingEntryId === entry.id}
                onDelete={onDelete}
              />
            ))}
          </section>
          {pageCount > 1 && (
            <nav className="vocabulary-pager" aria-label="Vocabulary pages">
              <button
                className="button button-secondary button-small"
                type="button"
                onClick={() => setPage((current) => Math.max(1, current - 1))}
                disabled={currentPage === 1}
              >
                Previous page
              </button>
              <span>Page {currentPage} of {pageCount}</span>
              <button
                className="button button-secondary button-small"
                type="button"
                onClick={() => setPage((current) => Math.min(pageCount, current + 1))}
                disabled={currentPage === pageCount}
              >
                Next page
              </button>
            </nav>
          )}
        </>
      )}
    </div>
  )
}

function VocabularyCard({
  entry,
  isDeleting,
  onDelete,
}: {
  entry: AppSnapshot['vocabulary'][number]
  isDeleting: boolean
  onDelete: (entry: AppSnapshot['vocabulary'][number]) => void
}) {
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
      <div className="vocabulary-card-actions">
        <button
          className="button button-danger-outline button-small"
          type="button"
          aria-label={`Delete ${entry.simplified}`}
          onClick={() => onDelete(entry)}
          disabled={isDeleting}
        >
          {isDeleting ? 'Deleting…' : 'Delete'}
        </button>
      </div>
    </article>
  )
}

function SettingsView({
  snapshot,
  apiMode,
  busyAction,
  onSave,
  onSyncNow,
  onDeleteLastLesson,
}: {
  snapshot: AppSnapshot
  apiMode: AppApi['mode']
  busyAction?: string
  onSave: (patch: SettingsPatch) => void
  onSyncNow: () => void
  onDeleteLastLesson: () => void
}) {
  const [keyDraft, setKeyDraft] = useState('')
  const [sttModel, setSttModel] = useState(normalizeModel(snapshot.settings.sttModel, STT_MODEL_OPTIONS, DEFAULT_STT_MODEL))
  const [analyzerModel, setAnalyzerModel] = useState(normalizeModel(snapshot.settings.analyzerModel, ANALYZER_MODEL_OPTIONS, DEFAULT_ANALYZER_MODEL))
  const [visionModel, setVisionModel] = useState(normalizeModel(snapshot.settings.visionModel, VISION_MODEL_OPTIONS, DEFAULT_VISION_MODEL))
  const [retention, setRetention] = useState(snapshot.settings.audioRetention)
  const [ocrEnabled, setOcrEnabled] = useState(snapshot.settings.ocrEnabled)
  const [microphoneEnabled, setMicrophoneEnabled] = useState(snapshot.settings.microphoneEnabled)

  useEffect(() => {
    setSttModel(normalizeModel(snapshot.settings.sttModel, STT_MODEL_OPTIONS, DEFAULT_STT_MODEL))
    setAnalyzerModel(normalizeModel(snapshot.settings.analyzerModel, ANALYZER_MODEL_OPTIONS, DEFAULT_ANALYZER_MODEL))
    setVisionModel(normalizeModel(snapshot.settings.visionModel, VISION_MODEL_OPTIONS, DEFAULT_VISION_MODEL))
    setRetention(snapshot.settings.audioRetention)
    setOcrEnabled(snapshot.settings.ocrEnabled)
    setMicrophoneEnabled(snapshot.settings.microphoneEnabled)
  }, [snapshot.settings])

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const patch: SettingsPatch = {
      sttModel,
      analyzerModel,
      visionModel,
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
              <div className="field-group">
                <label className="field-label" htmlFor="vision-model">Vision model</label>
                <select id="vision-model" className="text-input" value={visionModel} onChange={(event) => setVisionModel(event.target.value)}>
                  {VISION_MODEL_OPTIONS.map((option) => <option value={option.value} key={option.value}>{option.label}</option>)}
                </select>
                <p className="helper-text">Flash is the inexpensive default; 30B is the handwriting fallback.</p>
              </div>
            </div>
            <div className="budget-row">
              <div>
                <strong>Session cost</strong>
                <p>Cost is recorded for each lesson. Reaching $0.50 does not stop capture or analysis.</p>
              </div>
              <span className="budget-value">No cap</span>
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
                <strong>{snapshot.privacy.zdrEnabled ? 'Zero data retention (ZDR)' : 'Provider training opt-out'}</strong>
                <p>
                  {snapshot.privacy.zdrEnabled
                    ? 'ZDR controls provider-side retention. Local audio follows the selected retention policy; confirmed vocabulary remains in your local collection.'
                    : 'Lesson transcription and text requests set data_collection: deny without restricting ZDR routing. Local audio follows the selected retention policy.'}
                </p>
              </div>
              <span className="status-pill status-mastered">{snapshot.privacy.zdrEnabled ? 'Enabled' : 'Active'}</span>
            </div>
          </section>
          <section className="panel settings-panel">
            <PanelHeading
              eyebrow="Cloud sync"
              title="Cloud vocabulary"
              description="Confirmed words are downloaded when the app opens. Uploads wait about 10 minutes after the latest change. Sync now uploads only."
            />
            <div className="budget-row">
              <div>
                <strong>Sync status</strong>
                {snapshot.settings.syncStatus === 'not_configured' && (
                  <p>No Supabase key is configured on this computer.</p>
                )}
                {snapshot.settings.syncStatus === 'waiting' && (
                  <>
                    <p>
                      {snapshot.settings.syncWaitingUntil
                        ? `Waiting until ${formatLessonDate(snapshot.settings.syncWaitingUntil)}.`
                        : 'Waiting for the next sync window.'}
                    </p>
                    <p>
                      {snapshot.settings.syncPendingCount} pending {snapshot.settings.syncPendingCount === 1 ? 'word' : 'words'}.
                    </p>
                  </>
                )}
                {snapshot.settings.syncStatus === 'syncing' && <p>Syncing.</p>}
                {snapshot.settings.syncStatus === 'idle' && (
                  <p>
                    {snapshot.settings.lastSyncAt
                      ? `Last synced ${formatLessonDate(snapshot.settings.lastSyncAt)}.`
                      : 'No sync completed yet.'}
                  </p>
                )}
                {snapshot.settings.syncStatus === 'error' && (
                  <p>{snapshot.settings.lastSyncError || 'Sync failed.'}</p>
                )}
              </div>
              <button
                className="button button-secondary button-small"
                type="button"
                onClick={onSyncNow}
                disabled={!snapshot.settings.supabaseSyncConfigured || busyAction === 'sync-now'}
              >
                <Icon name="refresh" />
                {busyAction === 'sync-now' ? 'Syncing…' : 'Sync now'}
              </button>
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

const REGENERATE_PRESETS = [
  { label: 'Simpler example', prompt: 'Use a simpler, everyday example sentence.' },
  { label: 'More casual', prompt: 'Make the example sentence more casual and conversational.' },
  { label: 'Formal / business', prompt: 'Use a formal or workplace context for the example.' },
  { label: 'Different meaning', prompt: 'Use a different common meaning or sense of this word.' },
] as const

function RegeneratePanel({
  idPrefix,
  prompt,
  onPromptChange,
  hasApiKey,
  lessonActive,
  isRegenerating,
  disabled,
  error,
  onClose,
  onSubmit,
}: {
  idPrefix: string
  prompt: string
  onPromptChange: (value: string) => void
  hasApiKey: boolean
  lessonActive: boolean
  isRegenerating: boolean
  disabled: boolean
  error?: string
  onClose: () => void
  onSubmit: () => void
}) {
  const inputRef = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const trimmedPrompt = prompt.trim().toLowerCase()
  const inputId = `${idPrefix}-regenerate-prompt`

  return (
    <section className="regenerate-panel" aria-label="Regenerate word options">
      <div className="regenerate-panel-header">
        <div className="regenerate-panel-heading">
          <span className="regenerate-panel-title">
            <Icon name="spark" />
            Regenerate card
          </span>
          <span className="regenerate-panel-badge">Optional instructions</span>
        </div>
        <button
          className="icon-button regenerate-panel-close"
          type="button"
          aria-label="Close regenerate options"
          onClick={onClose}
          disabled={isRegenerating}
        >
          <Icon name="close" />
        </button>
      </div>
      <p className="regenerate-panel-copy">
        Describe how to revise the meaning, reading, or example—or leave blank for a fresh alternative.
      </p>
      <textarea
        ref={inputRef}
        id={inputId}
        className="text-input textarea regenerate-prompt-input"
        aria-label="Regeneration instructions"
        placeholder="e.g. casual spoken example, simpler sentence, business sense…"
        value={prompt}
        onChange={(event) => onPromptChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && !event.shiftKey) {
            event.preventDefault()
            if (!disabled && !isRegenerating) onSubmit()
          } else if (event.key === 'Escape') {
            event.preventDefault()
            event.stopPropagation()
            if (!isRegenerating) onClose()
          }
        }}
        disabled={isRegenerating}
        rows={2}
      />
      <div className="regenerate-presets" role="group" aria-label="Quick regeneration prompts">
        {REGENERATE_PRESETS.map((preset) => {
          const isSelected = trimmedPrompt === preset.prompt.toLowerCase()
          return (
            <button
              key={preset.label}
              type="button"
              className={`tag-picker-chip regenerate-preset-chip${isSelected ? ' is-selected' : ''}`}
              aria-pressed={isSelected}
              onClick={() => onPromptChange(isSelected ? '' : preset.prompt)}
              disabled={isRegenerating}
            >
              {preset.label}
            </button>
          )
        })}
      </div>
      {error && (
        <div className="dialog-error regenerate-panel-error" role="alert">
          {error}
        </div>
      )}
      {!hasApiKey ? (
        <p className="helper-text regenerate-panel-hint">
          Add an OpenRouter API key in Settings to regenerate this word.
        </p>
      ) : lessonActive ? (
        <p className="helper-text regenerate-panel-hint">
          Finish the active lesson before regenerating word details here.
        </p>
      ) : null}
      <div className="regenerate-panel-actions">
        <button
          className="button button-ghost button-small"
          type="button"
          onClick={onClose}
          disabled={isRegenerating}
        >
          Cancel
        </button>
        <button
          className="button button-primary button-small"
          type="button"
          onClick={onSubmit}
          disabled={disabled || isRegenerating}
        >
          <Icon name="spark" />
          {isRegenerating ? 'Regenerating…' : 'Regenerate now'}
        </button>
      </div>
    </section>
  )
}

function EditCandidateDialog({
  candidate,
  draft,
  existingTags,
  validationError,
  regenerateError,
  regenerateNotice,
  hasApiKey,
  lessonActive,
  isSaving,
  isRegenerating,
  onDraftChange,
  onRegenerate,
  onCancel,
  onSubmit,
}: {
  candidate: Candidate
  draft: CandidateEditDraft
  existingTags: string[]
  validationError?: string
  regenerateError?: string
  regenerateNotice?: string
  hasApiKey: boolean
  lessonActive: boolean
  isSaving: boolean
  isRegenerating: boolean
  onDraftChange: (draft: CandidateEditDraft) => void
  onRegenerate: (instruction: string) => Promise<boolean>
  onCancel: () => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
}) {
  const [regenerateOpen, setRegenerateOpen] = useState(false)
  const [regeneratePrompt, setRegeneratePrompt] = useState('')
  const canRegenerate = Boolean(draft.simplified.trim()) && hasApiKey && !lessonActive && !isSaving && !isRegenerating

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
        {regenerateNotice && (
          <div className="dialog-success" role="status" aria-live="polite">
            <Icon name="check" />
            <span>{regenerateNotice}</span>
          </div>
        )}
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
          <ExistingTagPicker
            tags={existingTags}
            value={draft.tags}
            onChange={(tags) => onDraftChange({ ...draft, tags })}
          />
        </div>
        {regenerateOpen && (
          <RegeneratePanel
            idPrefix="edit"
            prompt={regeneratePrompt}
            onPromptChange={setRegeneratePrompt}
            hasApiKey={hasApiKey}
            lessonActive={lessonActive}
            isRegenerating={isRegenerating}
            disabled={!canRegenerate}
            error={regenerateError}
            onClose={() => setRegenerateOpen(false)}
            onSubmit={() => {
              void onRegenerate(regeneratePrompt).then((didRegenerate) => {
                if (didRegenerate) {
                  setRegenerateOpen(false)
                  setRegeneratePrompt('')
                }
              })
            }}
          />
        )}
        <div className="dialog-actions dialog-actions-split">
          <button
            className={`button button-secondary dialog-regenerate-toggle${regenerateOpen ? ' is-active' : ''}`}
            type="button"
            aria-expanded={regenerateOpen}
            onClick={() => setRegenerateOpen((open) => !open)}
            disabled={isSaving || isRegenerating}
          >
            <Icon name="spark" />
            {isRegenerating ? 'Regenerating…' : 'Regenerate'}
          </button>
          <div className="dialog-actions-primary">
            <button className="button button-ghost" type="button" onClick={onCancel} disabled={isSaving || isRegenerating}>Cancel</button>
            <button className="button button-primary" type="submit" disabled={isSaving || isRegenerating}>{isSaving ? 'Saving…' : 'Save changes'}</button>
          </div>
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

function DeleteVocabularyDialog({
  entry,
  isDeleting,
  onCancel,
  onConfirm,
}: {
  entry: AppSnapshot['vocabulary'][number]
  isDeleting: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <div className="dialog-backdrop" role="presentation">
      <div className="dialog-card" role="dialog" aria-modal="true" aria-labelledby="delete-vocabulary-title">
        <div className="dialog-heading">
          <div>
            <p className="eyebrow">Saved word</p>
            <h2 id="delete-vocabulary-title">Delete {entry.simplified}?</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close delete dialog" onClick={onCancel} disabled={isDeleting} autoFocus><Icon name="close" /></button>
        </div>
        <p className="dialog-copy">
          This removes <strong>{entry.simplified}</strong> from this computer and queues that delete for cloud vocabulary sync.
        </p>
        <div className="dialog-actions">
          <button className="button button-ghost" type="button" onClick={onCancel} disabled={isDeleting}>Cancel</button>
          <button className="button button-danger" type="button" onClick={onConfirm} disabled={isDeleting}>
            {isDeleting ? 'Deleting…' : 'Delete word'}
          </button>
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
        {line.pinyin && <p className="transcript-pinyin">{line.pinyin}</p>}
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

function PanelHeading({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) {
  return (
    <div className="panel-heading">
      <p className="eyebrow">{eyebrow}</p>
      <h2>{title}</h2>
      <p>{description}</p>
    </div>
  )
}

function PrivacyBadge({ zdrEnabled }: { zdrEnabled: boolean }) {
  return (
    <span className="privacy-badge">
      <Icon name="shield" /> {zdrEnabled ? 'ZDR enabled' : 'No model training'}
    </span>
  )
}

function CostStatusAlert({ cost }: { cost: AppSnapshot['cost'] }) {
  const [dismissed, setDismissed] = useState(false)
  if (dismissed) return null
  const hardExceeded = Boolean(cost.hardExceeded)
  return (
    <div className={`cost-alert ${hardExceeded ? 'cost-alert-hard' : 'cost-alert-warning'}`} role="alert">
      <Icon name="warning" />
      <div>
        <strong>This lesson has spent {formatUsd(cost.currentUsd)}</strong>
        <p>Capture and analysis continue. There is no automatic cost stop.</p>
        <span>Current {formatUsd(cost.currentUsd)} · Projected {formatUsd(cost.projectedUsd)}</span>
      </div>
      <DismissButton onClick={() => setDismissed(true)} />
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

function InlineAlert({ tone, children, onDismiss }: { tone: 'error' | 'success' | 'info'; children: string; onDismiss?: () => void }) {
  const [dismissedText, setDismissedText] = useState<string>()
  if (dismissedText === children) return null
  return (
    <div className={`inline-alert alert-${tone}`} role={tone === 'error' ? 'alert' : 'status'}>
      <Icon name={tone === 'error' ? 'warning' : tone === 'info' ? 'refresh' : 'check'} />
      <span>{children}</span>
      <DismissButton onClick={() => {
        setDismissedText(children)
        onDismiss?.()
      }} />
    </div>
  )
}

function KeyBanner({
  icon,
  title,
  children,
  action,
  className,
}: {
  icon: IconName
  title: string
  children: string
  action?: ReactNode
  className?: string
}) {
  const [dismissed, setDismissed] = useState(false)
  if (dismissed) return null
  return (
    <div className={`manual-key-alert ${className ?? ''}`.trim()} role="status">
      <div className="manual-key-alert-icon"><Icon name={icon} /></div>
      <div>
        <strong>{title}</strong>
        <p>{children}</p>
      </div>
      {action}
      <DismissButton onClick={() => setDismissed(true)} />
    </div>
  )
}

function DismissButton({ onClick }: { onClick: () => void }) {
  return (
    <button className="alert-dismiss" type="button" aria-label="Dismiss message" onClick={onClick}>
      <Icon name="close" />
    </button>
  )
}

function BackendUnavailable({ mode, onRetry }: { mode: AppApi['mode']; onRetry: () => void }) {
  const [dismissed, setDismissed] = useState(false)
  if (dismissed) return null
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
      <DismissButton onClick={() => setDismissed(true)} />
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

function cloudPullBanner(settings: AppSnapshot['settings']): { tone: 'info' | 'success' | 'error'; text: string } | undefined {
  if (settings.pullStatus === 'pulling') {
    return { tone: 'info', text: 'Checking cloud vocabulary…' }
  }
  if (settings.pullStatus === 'error') {
    return { tone: 'error', text: settings.pullError || 'Cloud vocabulary could not be checked.' }
  }
  if (settings.pullStatus === 'idle' && settings.lastPullAt) {
    const count = settings.pullChangeCount ?? 0
    if (count === 0) return { tone: 'info', text: 'Cloud vocabulary is up to date.' }
    const noun = count === 1 ? 'change' : 'changes'
    return { tone: 'success', text: `Brought in ${count} vocabulary ${noun} from the cloud.` }
  }
  return undefined
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

function formatLessonDate(value?: string): string {
  if (!value) return 'Undated lesson'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

function formatPhotoCost(value: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: value > 0 && value < 0.01 ? 4 : 2,
    maximumFractionDigits: 4,
  }).format(value)
}

function formatStartedAt(value?: string): string {
  return value ? `since ${formatTimestamp(value)}` : 'just now'
}

function formatLessonTags(tags?: string): string {
  return (tags ?? '')
    .split(',')
    .map((tag) => tag.trim())
    .filter(Boolean)
    .join(', ')
}

function labelForTarget(target: TargetApp): string {
  return target === 'system' || target === 'zoom' || target === 'googleMeet' || target === 'teams' ? 'Any app' : target
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

function toggleTagValue(value: string, tag: string): string {
  const names = parseTags(value)
  const key = tag.trim().toLowerCase()
  if (!key) return value
  const without = names.filter((name) => name.toLowerCase() !== key)
  const next = without.length === names.length ? [...names, tag.trim()] : without
  return next.join(', ')
}

function ExistingTagPicker({
  tags,
  value,
  onChange,
}: {
  tags: string[]
  value: string
  onChange: (value: string) => void
}) {
  const unique = Array.from(new Set(tags.map((tag) => tag.trim()).filter(Boolean)))
  if (unique.length === 0) return null
  const selected = new Set(parseTags(value).map((tag) => tag.toLowerCase()))
  return (
    <div className="existing-tags">
      <span className="field-hint">Select from existing tags</span>
      <div className="existing-tag-list">
        {unique.map((tag) => {
          const pressed = selected.has(tag.toLowerCase())
          return (
            <button
              key={tag}
              className={pressed ? 'tag-toggle tag-toggle-on' : 'tag-toggle'}
              type="button"
              aria-pressed={pressed}
              onClick={() => onChange(toggleTagValue(value, tag))}
            >
              {tag}
            </button>
          )
        })}
      </div>
    </div>
  )
}

function parseTags(value: string): string[] {
  return Array.from(new Set(value.split(',').map((tag) => tag.trim()).filter(Boolean)))
}

function photoStatusLabel(status: PhotoQueueStatus): string {
  if (status === 'preparing') return 'Preparing'
  if (status === 'ready') return 'Ready'
  if (status === 'reading') return 'Reading'
  if (status === 'done') return 'Done'
  if (status === 'no-words') return 'No words'
  return 'Failed'
}

const ACCEPTED_PHOTO_MIME_TYPES = new Set(['image/jpeg', 'image/png', 'image/webp', 'image/heic', 'image/heif'])

function isAcceptedPhotoFile(file: File): boolean {
  const type = file.type.toLowerCase()
  const name = file.name.toLowerCase()
  return ACCEPTED_PHOTO_MIME_TYPES.has(type) || /\.(?:jpe?g|png|webp|heic|heif)$/.test(name)
}

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => typeof reader.result === 'string' ? resolve(reader.result) : reject(new Error('This browser could not read the selected photo.'))
    reader.onerror = () => reject(new Error('This browser could not read the selected photo.'))
    reader.readAsDataURL(file)
  })
}

async function preparePhoto(file: File): Promise<PreparedPhoto> {
  if (!isAcceptedPhotoFile(file)) {
    throw new Error('Choose a JPEG, PNG, WebP, HEIC, or HEIF photo.')
  }
  let objectUrl: string | undefined
  try {
    const source = typeof URL.createObjectURL === 'function'
      ? (objectUrl = URL.createObjectURL(file))
      : await readFileAsDataUrl(file)
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const element = new Image()
      element.onload = () => resolve(element)
      element.onerror = () => reject(new Error('This browser could not decode this photo. Try a JPEG, PNG, or WebP image.'))
      element.src = source
    })
    const sourceWidth = image.naturalWidth || image.width
    const sourceHeight = image.naturalHeight || image.height
    if (!sourceWidth || !sourceHeight) {
      throw new Error('This browser could not decode this photo. Try a JPEG, PNG, or WebP image.')
    }
    const scale = Math.min(1, 1280 / Math.max(sourceWidth, sourceHeight))
    const width = Math.max(1, Math.round(sourceWidth * scale))
    const height = Math.max(1, Math.round(sourceHeight * scale))
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const context = canvas.getContext('2d')
    if (!context) throw new Error('This browser could not prepare this photo for reading.')
    context.drawImage(image, 0, 0, width, height)
    const previewDataUrl = canvas.toDataURL('image/jpeg', 0.82)
    const comma = previewDataUrl.indexOf(',')
    if (comma < 0 || !previewDataUrl.slice(comma + 1)) {
      throw new Error('This browser could not prepare this photo for reading.')
    }
    return {
      previewDataUrl,
      imageBase64: previewDataUrl.slice(comma + 1),
    }
  } catch (error: unknown) {
    if (error instanceof Error && error.message.trim()) throw error
    throw new Error('This browser could not decode this photo. Try a JPEG, PNG, or WebP image.')
  } finally {
    if (objectUrl) URL.revokeObjectURL(objectUrl)
  }
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

function manualVocabularyInputMissingFields(input: ManualVocabularyInput): string[] {
  return [
    ['Chinese word (simplified)', input.simplified],
    ['pinyin', input.pinyin ?? ''],
    ['English meaning', input.meaning ?? ''],
    ['Chinese sample sentence', input.example ?? ''],
    ['sample pinyin', input.examplePinyin ?? ''],
    ['sample English translation', input.exampleTranslation ?? ''],
  ]
    .filter(([, value]) => !value.trim())
    .map(([label]) => label)
}

function isCompleteManualVocabularyInput(input: ManualVocabularyInput): boolean {
  return manualVocabularyInputMissingFields(input).length === 0
}

function mergePhotoSuggestions(
  suggestions: PhotoSuggestion[],
  entries: ManualVocabularyInput[],
  photoIdToAdd: string,
  vocabulary: AppSnapshot['vocabulary'],
): PhotoSuggestion[] {
  const next = suggestions.map((suggestion) => ({ ...suggestion, photoIds: [...suggestion.photoIds] }))
  entries.forEach((entry, index) => {
    const normalized = normalizePhotoReviewInput(entry)
    const simplified = normalized.simplified.trim()
    const existing = next.find((suggestion) => suggestion.input.simplified.trim() === simplified)
    if (existing) {
      if (!existing.photoIds.includes(photoIdToAdd)) existing.photoIds.push(photoIdToAdd)
      return
    }
    const alreadySaved = vocabulary.some((item) => item.simplified.trim() === simplified)
    next.push({
      id: `${photoIdToAdd}-${index}`,
      input: normalized,
      photoIds: [photoIdToAdd],
      selected: isCompleteManualVocabularyInput(normalized) && !alreadySaved,
      expanded: false,
    })
  })
  return next
}

function normalizePhotoReviewInput(input: ManualVocabularyInput): ManualVocabularyInput {
  return {
    simplified: input.simplified.trim(),
    traditional: input.traditional?.trim() ?? '',
    pinyin: input.pinyin?.trim() ?? '',
    meaning: input.meaning?.trim() ?? '',
    partOfSpeech: input.partOfSpeech?.trim() ?? '',
    classifier: input.classifier?.trim() ?? '',
    example: input.example?.trim() ?? '',
    examplePinyin: input.examplePinyin?.trim() ?? '',
    exampleTranslation: input.exampleTranslation?.trim() ?? '',
    tags: [...(input.tags ?? [])],
    aiGenerated: true,
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

function manualVocabularyVariantToForm(
  origin: ManualVocabularyForm,
  variant: ManualVocabularyVariant,
): ManualVocabularyForm {
  const useOrigin = (value: string, replacement?: string) => value.trim() ? value : replacement ?? ''
  return {
    simplified: origin.simplified,
    traditional: useOrigin(origin.traditional, variant.traditional),
    pinyin: useOrigin(origin.pinyin, variant.pinyin),
    meaning: useOrigin(origin.meaning, variant.meaning),
    partOfSpeech: useOrigin(origin.partOfSpeech, variant.partOfSpeech),
    classifier: useOrigin(origin.classifier, variant.classifier),
    example: useOrigin(origin.example, variant.example),
    examplePinyin: useOrigin(origin.examplePinyin, variant.examplePinyin),
    exampleTranslation: useOrigin(origin.exampleTranslation, variant.exampleTranslation),
    tags: origin.tags,
    aiGenerated: !origin.example.trim() || !origin.examplePinyin.trim() || !origin.exampleTranslation.trim()
      ? true
      : origin.aiGenerated,
  }
}

function manualVocabularyMissingFields(form: ManualVocabularyForm): string[] {
  return manualVocabularyInputMissingFields(manualVocabularyFormToInput(form))
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
  | 'history'
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
    history: <><path d="M4 6h16M4 12h16M4 18h10" /><circle cx="18" cy="18" r="3" /></>,
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
