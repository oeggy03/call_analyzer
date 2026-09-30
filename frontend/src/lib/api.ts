import type {
  AppSnapshot,
  Candidate,
  CandidateEditPatch,
  SettingsPatch,
  StartLessonInput,
} from './types'
import type {
  CapturePermission,
  CandidateStatus,
  ConnectionState,
  TargetApp,
} from './types'
import type { VocabularyEntry } from './types'
import type { LessonStatus } from './types'
import type { TranscriptLine } from './types'
import type { AudioLevels } from './types'
import type { CostSummary } from './types'
import type { PrivacyState } from './types'
import type { Settings } from './types'
import type { CandidateBucket } from './types'
import type { VocabularyStatus } from './types'
import type { ManualVocabularyDraft, ManualVocabularyInput } from './types'

/**
 * Keep Wails method names in one place. If the generated binding changes,
 * update this map rather than changing UI code.
 */
export const BACKEND_METHODS = {
  getSnapshot: 'GetAppSnapshot',
  generateManualVocabulary: 'GenerateManualVocabulary',
  saveManualVocabulary: 'SaveManualVocabulary',
  requestCapturePermission: 'RequestCapturePermission',
  startLesson: 'StartLesson',
  stopLesson: 'StopLesson',
  markMoment: 'MarkMoment',
  confirmCandidate: 'ConfirmCandidate',
  editCandidate: 'EditCandidate',
  rejectCandidate: 'RejectCandidate',
  mergeCandidates: 'MergeCandidates',
  saveSettings: 'SaveSettings',
  deleteLastLesson: 'DeleteLastLesson',
  refresh: 'Refresh',
} as const

export const BACKEND_EVENTS = {
  snapshotChanged: 'call_analyzer:snapshot_changed',
  captureLevels: 'call_analyzer:capture.levels',
} as const

export type ApiMode = 'runtime' | 'demo' | 'unavailable'
export type ApiModePreference = 'auto' | ApiMode

export interface AppApi {
  readonly mode: ApiMode
  getSnapshot(): Promise<AppSnapshot>
  generateManualVocabulary(input: ManualVocabularyInput): Promise<ManualVocabularyDraft>
  saveManualVocabulary(input: ManualVocabularyInput): Promise<AppSnapshot>
  requestCapturePermission(): Promise<AppSnapshot>
  startLesson(input: StartLessonInput): Promise<AppSnapshot>
  stopLesson(): Promise<AppSnapshot>
  markMoment(): Promise<AppSnapshot>
  confirmCandidate(candidateId: string): Promise<AppSnapshot>
  editCandidate(candidateId: string, patch: CandidateEditPatch): Promise<AppSnapshot>
  rejectCandidate(candidateId: string): Promise<AppSnapshot>
  mergeCandidates(sourceId: string, targetId: string): Promise<AppSnapshot>
  saveSettings(patch: SettingsPatch): Promise<AppSnapshot>
  deleteLastLesson(): Promise<AppSnapshot>
  refresh(): Promise<AppSnapshot>
  subscribe(listener: (snapshot: AppSnapshot) => void): () => void
}

export class ApiUnavailableError extends Error {
  constructor(message = 'The Wails backend is unavailable.') {
    super(message)
    this.name = 'ApiUnavailableError'
  }
}

export class ApiValidationError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'ApiValidationError'
  }
}

type RuntimeApp = Record<string, unknown>
type RuntimeEvents = {
  EventsOn?: (
    eventName: string,
    callback: (...args: unknown[]) => void,
  ) => (() => void) | void
  EventsOff?: (eventName: string) => void
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: RuntimeApp
      }
    }
    runtime?: RuntimeEvents
  }
}

const DEMO_TIMESTAMP = '2026-09-29T14:30:00.000Z'
const DEMO_MOMENT_TIMESTAMP = '2026-09-29T14:30:05.000Z'
const DEMO_MANUAL_VOCABULARY_MODEL = 'qwen/qwen3.8-flash'
const DEMO_MANUAL_VOCABULARY_COST = 0.0002

const demoTranscript: TranscriptLine[] = [
  {
    id: 'line-1',
    text: '我们下周可以把这个方案再讨论一下。',
    source: 'remote',
    timestamp: '2026-09-29T14:29:42.000Z',
    speaker: 'Remote',
  },
  {
    id: 'line-2',
    text: '好的，我先整理一下重点。',
    source: 'microphone',
    timestamp: '2026-09-29T14:29:49.000Z',
    speaker: 'You',
  },
  {
    id: 'line-3',
    text: '这个表达在工作场景里很常见。',
    source: 'remote',
    timestamp: '2026-09-29T14:29:58.000Z',
    speaker: 'Remote',
  },
]

const demoCandidates: Candidate[] = [
  {
    id: 'candidate-manual-1',
    simplified: '重点',
    traditional: '重點',
    pinyin: 'zhòngdiǎn',
    meaning: 'key point; main focus',
    partOfSpeech: 'noun',
    classifier: undefined,
    example: '请先说一下这个方案的重点。',
    examplePinyin: 'Qǐng xiān shuō yíxià zhège fāng’àn de zhòngdiǎn.',
    exampleTranslation: 'Please first explain the key points of this plan.',
    provenance: 'Manual mark · Live Lesson',
    confidence: 1,
    evidence: '整理一下重点',
    timestamp: '2026-09-29T14:29:49.000Z',
    bucket: 'manual',
    status: 'pending',
    tags: ['work'],
  },
  {
    id: 'candidate-high-1',
    simplified: '方案',
    traditional: '方案',
    pinyin: 'fāng’àn',
    meaning: 'plan; proposal; solution',
    partOfSpeech: 'noun',
    classifier: '个',
    example: '我们下周可以把这个方案再讨论一下。',
    examplePinyin: 'Wǒmen xià zhōu kěyǐ bǎ zhège fāng’àn zài tǎolùn yíxià.',
    exampleTranslation: 'We can discuss this proposal again next week.',
    provenance: 'Remote transcript · 00:42',
    confidence: 0.96,
    evidence: '把这个方案再讨论一下',
    timestamp: '2026-09-29T14:29:42.000Z',
    bucket: 'highConfidence',
    status: 'pending',
    tags: ['work', 'meeting'],
  },
  {
    id: 'candidate-duplicate-1',
    simplified: '重点',
    traditional: '重點',
    pinyin: 'zhòngdiǎn',
    meaning: 'key point; main focus',
    partOfSpeech: 'noun',
    example: '今天我们先确认重点。',
    examplePinyin: 'Jīntiān wǒmen xiān quèrèn zhòngdiǎn.',
    exampleTranslation: 'Today we will confirm the key points first.',
    provenance: 'Mic transcript · 00:49',
    confidence: 0.71,
    evidence: '整理一下重点',
    timestamp: '2026-09-29T14:29:49.000Z',
    bucket: 'possibleDuplicate',
    status: 'pending',
    duplicateOf: 'candidate-manual-1',
    tags: ['work'],
  },
  {
    id: 'candidate-low-1',
    simplified: '常见',
    traditional: '常見',
    pinyin: 'chángjiàn',
    meaning: 'common; frequently seen',
    partOfSpeech: 'adjective',
    example: '这个表达在工作场景里很常见。',
    examplePinyin: 'Zhège biǎodá zài gōngzuò chǎngjǐng lǐ hěn chángjiàn.',
    exampleTranslation: 'This expression is common in work settings.',
    provenance: 'Remote transcript · 01:02',
    confidence: 0.54,
    evidence: '表达在工作场景里很常见',
    timestamp: '2026-09-29T14:29:58.000Z',
    bucket: 'lowConfidence',
    status: 'pending',
    tags: ['work'],
  },
]

const demoVocabulary: VocabularyEntry[] = [
  {
    id: 'vocab-1',
    simplified: '重点',
    traditional: '重點',
    pinyin: 'zhòngdiǎn',
    meaning: 'key point; main focus',
    partOfSpeech: 'noun',
    status: 'learning',
    example: '请先说一下这个方案的重点。',
    examplePinyin: 'Qǐng xiān shuō yíxià zhège fāng’àn de zhòngdiǎn.',
    exampleTranslation: 'Please first explain the key points of this plan.',
    tags: ['work'],
    lastSeen: 'Today, 2:30 PM',
    seenCount: 3,
  },
  {
    id: 'vocab-2',
    simplified: '方案',
    pinyin: 'fāng’àn',
    meaning: 'plan; proposal; solution',
    partOfSpeech: 'noun',
    classifier: '个',
    status: 'review',
    example: '我们下周可以把这个方案再讨论一下。',
    examplePinyin: 'Wǒmen xià zhōu kěyǐ bǎ zhège fāng’àn zài tǎolùn yíxià.',
    exampleTranslation: 'We can discuss this proposal again next week.',
    tags: ['work', 'meeting'],
    lastSeen: 'Today, 2:29 PM',
    seenCount: 5,
  },
  {
    id: 'vocab-3',
    simplified: '场景',
    traditional: '場景',
    pinyin: 'chǎngjǐng',
    meaning: 'scene; context; setting',
    partOfSpeech: 'noun',
    status: 'learning',
    example: '这个表达在工作场景里很常见。',
    examplePinyin: 'Zhège biǎodá zài gōngzuò chǎngjǐng lǐ hěn chángjiàn.',
    exampleTranslation: 'This expression is common in work settings.',
    tags: ['work'],
    lastSeen: 'Yesterday',
    seenCount: 2,
  },
  {
    id: 'vocab-4',
    simplified: '确认',
    traditional: '確認',
    pinyin: 'quèrèn',
    meaning: 'to confirm; confirmation',
    partOfSpeech: 'verb',
    status: 'mastered',
    example: '今天我们先确认重点。',
    examplePinyin: 'Jīntiān wǒmen xiān quèrèn zhòngdiǎn.',
    exampleTranslation: 'Today we will confirm the key points first.',
    tags: ['work', 'meeting'],
    lastSeen: 'Sep 26',
    seenCount: 11,
  },
]

function createDemoSnapshot(): AppSnapshot {
  return {
    connection: 'demo',
    capturePermission: 'granted',
    readiness: { checked: true, targetAvailable: true, checkedAt: DEMO_TIMESTAMP },
    target: 'zoom',
    lesson: { id: 'demo-lesson-1', status: 'idle' },
    levels: { mic: 0.18, remote: 0.36 },
    transcript: demoTranscript,
    candidates: demoCandidates,
    vocabulary: demoVocabulary,
    cost: {
      currentUsd: 0.04,
      projectedUsd: 0.18,
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
      openRouterKeyConfigured: true,
      sttModel: 'qwen/qwen3-asr-1.7b',
      analyzerModel: 'qwen/qwen3.8-flash',
      hardBudgetUsd: 0.5,
      audioRetention: 'sessionOnly',
      ocrEnabled: false,
      microphoneEnabled: true,
    },
    lastUpdated: DEMO_TIMESTAMP,
  }
}

function cloneSnapshot(snapshot: AppSnapshot): AppSnapshot {
  return JSON.parse(JSON.stringify(snapshot)) as AppSnapshot
}

function normalizeManualVocabularyInput(input: ManualVocabularyInput): ManualVocabularyInput {
  const simplified = input.simplified.trim()
  if (!simplified) throw new ApiValidationError('Chinese word (simplified) is required.')
  return {
    simplified,
    traditional: input.traditional?.trim() ?? '',
    pinyin: input.pinyin?.trim() ?? '',
    meaning: input.meaning?.trim() ?? '',
    partOfSpeech: input.partOfSpeech?.trim() ?? '',
    classifier: input.classifier?.trim() ?? '',
    example: input.example?.trim() ?? '',
    examplePinyin: input.examplePinyin?.trim() ?? '',
    exampleTranslation: input.exampleTranslation?.trim() ?? '',
    tags: Array.from(new Set((input.tags ?? []).map((tag) => tag.trim()).filter(Boolean))),
    aiGenerated: Boolean(input.aiGenerated),
  }
}

function hasCompleteManualVocabulary(input: ManualVocabularyInput): boolean {
  return Boolean(
    input.pinyin?.trim() &&
      input.meaning?.trim() &&
      input.example?.trim() &&
      input.examplePinyin?.trim() &&
      input.exampleTranslation?.trim(),
  )
}

function isOptionalString(value: unknown): boolean {
  return value === undefined || typeof value === 'string'
}

export function isManualVocabularyDraft(value: unknown): value is ManualVocabularyDraft {
  if (!isRecord(value)) return false
  const requiredStrings = ['simplified', 'pinyin', 'meaning', 'example', 'examplePinyin', 'exampleTranslation', 'model']
  if (requiredStrings.some((field) => typeof value[field] !== 'string' || !value[field].trim())) return false
  if (typeof value.cost !== 'number' || !Number.isFinite(value.cost) || value.cost < 0) return false
  if (!['traditional', 'partOfSpeech', 'classifier'].every((field) => isOptionalString(value[field]))) return false
  if (value.aiGenerated !== undefined && typeof value.aiGenerated !== 'boolean') return false
  return value.tags === undefined || (Array.isArray(value.tags) && value.tags.every((tag) => typeof tag === 'string'))
}

const DEMO_MANUAL_VOCABULARY_EXAMPLES: Record<string, ManualVocabularyInput> = {
  学习: {
    simplified: '学习',
    traditional: '學習',
    pinyin: 'xué xí',
    meaning: 'to study; to learn',
    partOfSpeech: 'verb',
    example: '我每天学习中文。',
    examplePinyin: 'Wǒ měi tiān xué xí Zhōng wén.',
    exampleTranslation: 'I study Chinese every day.',
    tags: ['study'],
  },
  你好: {
    simplified: '你好',
    traditional: '你好',
    pinyin: 'nǐ hǎo',
    meaning: 'hello',
    partOfSpeech: 'greeting',
    example: '你好，很高兴认识你。',
    examplePinyin: 'Nǐ hǎo, hěn gāo xìng rèn shi nǐ.',
    exampleTranslation: 'Hello, nice to meet you.',
    tags: ['conversation'],
  },
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export function isAppSnapshot(value: unknown): value is AppSnapshot {
  if (!isRecord(value)) return false
  return (
    typeof value.connection === 'string' &&
    typeof value.capturePermission === 'string' &&
    typeof value.target === 'string' &&
    isRecord(value.lesson) &&
    Array.isArray(value.transcript) &&
    Array.isArray(value.candidates) &&
    Array.isArray(value.vocabulary) &&
    isRecord(value.cost) &&
    isRecord(value.privacy) &&
    isRecord(value.settings)
  )
}

function isAudioLevels(value: unknown): value is AudioLevels {
  return (
    isRecord(value) &&
    typeof value.mic === 'number' &&
    Number.isFinite(value.mic) &&
    typeof value.remote === 'number' &&
    Number.isFinite(value.remote)
  )
}

class DemoApi implements AppApi {
  readonly mode = 'demo' as const
  private snapshot = createDemoSnapshot()
  private listener?: (snapshot: AppSnapshot) => void
  private nextMoment = 1

  async getSnapshot(): Promise<AppSnapshot> {
    return cloneSnapshot(this.snapshot)
  }

  async generateManualVocabulary(input: ManualVocabularyInput): Promise<ManualVocabularyDraft> {
    if (!this.snapshot.settings.openRouterKeyConfigured) {
      throw new ApiValidationError('Add an OpenRouter API key in Settings before generating details.')
    }
    const normalized = normalizeManualVocabularyInput(input)
    const fallback: ManualVocabularyInput = {
      simplified: normalized.simplified,
      traditional: normalized.simplified,
      pinyin: 'hǎo',
      meaning: 'a useful Mandarin word',
      partOfSpeech: 'word',
      example: '我正在学习这个词。',
      examplePinyin: 'Wǒ zhèng zài xué xí zhè ge cí.',
      exampleTranslation: 'I am learning this word.',
      tags: ['manual'],
    }
    const generated = DEMO_MANUAL_VOCABULARY_EXAMPLES[normalized.simplified] ?? fallback
    const cost = DEMO_MANUAL_VOCABULARY_COST
    const draft: ManualVocabularyDraft = {
      simplified: normalized.simplified,
      traditional: normalized.traditional || generated.traditional || normalized.simplified,
      pinyin: normalized.pinyin || generated.pinyin,
      meaning: normalized.meaning || generated.meaning,
      partOfSpeech: normalized.partOfSpeech || generated.partOfSpeech,
      classifier: normalized.classifier || generated.classifier || '',
      example: normalized.example || generated.example,
      examplePinyin: normalized.examplePinyin || generated.examplePinyin,
      exampleTranslation: normalized.exampleTranslation || generated.exampleTranslation,
      tags: normalized.tags && normalized.tags.length > 0 ? normalized.tags : generated.tags ?? [],
      aiGenerated: Boolean(normalized.aiGenerated) || !hasCompleteManualVocabulary(normalized),
      model: DEMO_MANUAL_VOCABULARY_MODEL,
      cost,
    }
    this.snapshot.cost = {
      ...this.snapshot.cost,
      currentUsd: this.snapshot.cost.currentUsd + cost,
      projectedUsd: Math.max(this.snapshot.cost.projectedUsd, this.snapshot.cost.currentUsd + cost),
    }
    this.publish()
    return { ...draft, tags: [...(draft.tags ?? [])] }
  }

  async saveManualVocabulary(input: ManualVocabularyInput): Promise<AppSnapshot> {
    const normalized = normalizeManualVocabularyInput(input)
    const missingFields = [
      ['pinyin', normalized.pinyin],
      ['English meaning', normalized.meaning],
      ['Chinese sample sentence', normalized.example],
      ['sample pinyin', normalized.examplePinyin],
      ['sample English translation', normalized.exampleTranslation],
    ]
      .filter(([, value]) => !value)
      .map(([label]) => label)
    if (missingFields.length > 0) {
      throw new ApiValidationError(`Complete the required details before saving: ${missingFields.join(', ')}.`)
    }
    const existing = this.snapshot.vocabulary.find((entry) => entry.simplified === normalized.simplified)
    const nextEntry: VocabularyEntry = {
      id: existing?.id ?? `vocab-manual-${normalized.simplified}`,
      simplified: normalized.simplified,
      traditional: normalized.traditional || normalized.simplified,
      pinyin: normalized.pinyin ?? '',
      meaning: normalized.meaning ?? '',
      partOfSpeech: normalized.partOfSpeech || undefined,
      classifier: normalized.classifier || undefined,
      example: normalized.example ?? '',
      examplePinyin: normalized.examplePinyin ?? '',
      exampleTranslation: normalized.exampleTranslation ?? '',
      status: existing?.status ?? 'learning',
      tags: [...(normalized.tags ?? [])],
      lastSeen: 'Just now',
      seenCount: (existing?.seenCount ?? 0) + 1,
    }
    this.snapshot.vocabulary = [
      nextEntry,
      ...this.snapshot.vocabulary.filter((entry) => entry.simplified !== normalized.simplified),
    ]
    this.publish()
    return this.getSnapshot()
  }

  async requestCapturePermission(): Promise<AppSnapshot> {
    this.snapshot.capturePermission = 'granted'
    this.publish()
    return this.getSnapshot()
  }

  async startLesson(input: StartLessonInput): Promise<AppSnapshot> {
    if (!input.consent) {
      throw new ApiValidationError('Consent is required before starting a lesson.')
    }
    this.snapshot.target = input.target
    this.snapshot.capturePermission = 'granted'
    this.snapshot.connection = 'demo'
    this.snapshot.lesson = {
      id: this.snapshot.lesson.id ?? 'demo-lesson-1',
      status: 'live',
      startedAt: DEMO_TIMESTAMP,
    }
    this.snapshot.levels = { mic: 0.42, remote: 0.64 }
    this.publish()
    return this.getSnapshot()
  }

  async stopLesson(): Promise<AppSnapshot> {
    this.snapshot.lesson = { id: this.snapshot.lesson.id ?? 'demo-lesson-1', status: 'idle' }
    this.snapshot.levels = { mic: 0, remote: 0 }
    this.publish()
    return this.getSnapshot()
  }

  async markMoment(): Promise<AppSnapshot> {
    const momentId = `candidate-manual-${this.nextMoment + 1}`
    this.nextMoment += 1
    this.snapshot.transcript = [
      ...this.snapshot.transcript,
      {
        id: `moment-${this.nextMoment}`,
        text: '已标记当前对话片段。',
        source: 'system',
        timestamp: DEMO_MOMENT_TIMESTAMP,
        speaker: 'Moment',
        isMoment: true,
      },
    ]
    this.snapshot.candidates = [
      ...this.snapshot.candidates,
      {
        id: momentId,
        simplified: '对话片段',
        traditional: '對話片段',
        pinyin: 'duìhuà piànduàn',
        meaning: 'conversation snippet',
        partOfSpeech: 'noun',
        example: '我标记了一个对话片段。',
        examplePinyin: 'Wǒ biāojì le yí ge duìhuà piànduàn.',
        exampleTranslation: 'I marked a conversation snippet.',
        provenance: 'Manual mark · just now',
        confidence: 1,
        evidence: '当前对话片段',
        timestamp: DEMO_MOMENT_TIMESTAMP,
        bucket: 'manual',
        status: 'pending',
        tags: [],
      },
    ]
    this.publish()
    return this.getSnapshot()
  }

  async confirmCandidate(candidateId: string): Promise<AppSnapshot> {
    const candidate = this.findCandidate(candidateId)
    candidate.status = 'confirmed'
    if (!this.snapshot.vocabulary.some((entry) => entry.simplified === candidate.simplified)) {
      this.snapshot.vocabulary = [
        {
          id: `vocab-${candidate.id}`,
          simplified: candidate.simplified,
          traditional: candidate.traditional,
          pinyin: candidate.pinyin,
          meaning: candidate.meaning,
          partOfSpeech: candidate.partOfSpeech,
          classifier: candidate.classifier,
          example: candidate.example,
          examplePinyin: candidate.examplePinyin,
          exampleTranslation: candidate.exampleTranslation,
          status: 'learning',
          tags: candidate.tags,
          lastSeen: 'Just now',
          seenCount: 1,
        },
        ...this.snapshot.vocabulary,
      ]
    }
    this.publish()
    return this.getSnapshot()
  }

  async editCandidate(candidateId: string, patch: CandidateEditPatch): Promise<AppSnapshot> {
    const candidate = this.findCandidate(candidateId)
    Object.assign(candidate, patch)
    this.publish()
    return this.getSnapshot()
  }

  async rejectCandidate(candidateId: string): Promise<AppSnapshot> {
    const candidate = this.findCandidate(candidateId)
    candidate.status = 'rejected'
    this.publish()
    return this.getSnapshot()
  }

  async mergeCandidates(sourceId: string, targetId: string): Promise<AppSnapshot> {
    const source = this.findCandidate(sourceId)
    const target = this.findCandidate(targetId)
    source.status = 'rejected'
    source.duplicateOf = target.id
    if (!target.tags.includes('merged')) target.tags = [...target.tags, 'merged']
    this.publish()
    return this.getSnapshot()
  }

  async saveSettings(patch: SettingsPatch): Promise<AppSnapshot> {
    const next = { ...this.snapshot.settings }
    if (patch.openRouterKey !== undefined) {
      next.openRouterKeyConfigured = patch.openRouterKey.trim().length > 0
    }
    if (patch.sttModel !== undefined) next.sttModel = patch.sttModel
    if (patch.analyzerModel !== undefined) next.analyzerModel = patch.analyzerModel
    if (patch.hardBudgetUsd !== undefined) next.hardBudgetUsd = patch.hardBudgetUsd
    if (patch.audioRetention !== undefined) next.audioRetention = patch.audioRetention
    if (patch.ocrEnabled !== undefined) next.ocrEnabled = patch.ocrEnabled
    if (patch.microphoneEnabled !== undefined) next.microphoneEnabled = patch.microphoneEnabled
    this.snapshot.settings = next
    this.snapshot.cost.hardBudgetUsd = next.hardBudgetUsd
    this.snapshot.privacy.audioRetention = next.audioRetention
    this.publish()
    return this.getSnapshot()
  }

  async deleteLastLesson(): Promise<AppSnapshot> {
    if (this.snapshot.lesson.status === 'live') {
      throw new ApiValidationError('Stop the live lesson before deleting its data.')
    }
    this.snapshot.lesson = { status: 'idle' }
    this.snapshot.levels = { mic: 0, remote: 0 }
    this.snapshot.transcript = []
    this.snapshot.candidates = []
    this.snapshot.cost = {
      ...this.snapshot.cost,
      currentUsd: 0,
      projectedUsd: 0,
      warning: false,
      hardExceeded: false,
    }
    this.publish()
    return this.getSnapshot()
  }

  async refresh(): Promise<AppSnapshot> {
    return this.getSnapshot()
  }

  subscribe(listener: (snapshot: AppSnapshot) => void): () => void {
    this.listener = listener
    return () => {
      if (this.listener === listener) this.listener = undefined
    }
  }

  private findCandidate(candidateId: string): Candidate {
    const candidate = this.snapshot.candidates.find((item) => item.id === candidateId)
    if (!candidate) throw new ApiValidationError('Candidate no longer exists.')
    return candidate
  }

  private publish(): void {
    this.listener?.(cloneSnapshot(this.snapshot))
  }
}

class RuntimeApi implements AppApi {
  readonly mode = 'runtime' as const
  private latestSnapshot?: AppSnapshot

  constructor(private readonly app: RuntimeApp) {}

  async getSnapshot(): Promise<AppSnapshot> {
    return this.callSnapshot(BACKEND_METHODS.getSnapshot)
  }

  async generateManualVocabulary(input: ManualVocabularyInput): Promise<ManualVocabularyDraft> {
    const result = await this.call(BACKEND_METHODS.generateManualVocabulary, [input])
    if (!isManualVocabularyDraft(result)) {
      throw new ApiUnavailableError(
        `Backend method ${BACKEND_METHODS.generateManualVocabulary} returned an invalid manual vocabulary draft.`,
      )
    }
    return result
  }

  async saveManualVocabulary(input: ManualVocabularyInput): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.saveManualVocabulary, input)
  }

  async requestCapturePermission(): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.requestCapturePermission)
  }

  async startLesson(input: StartLessonInput): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.startLesson, input)
  }

  async stopLesson(): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.stopLesson)
  }

  async markMoment(): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.markMoment)
  }

  async confirmCandidate(candidateId: string): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.confirmCandidate, candidateId)
  }

  async editCandidate(candidateId: string, patch: CandidateEditPatch): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.editCandidate, candidateId, patch)
  }

  async rejectCandidate(candidateId: string): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.rejectCandidate, candidateId)
  }

  async mergeCandidates(sourceId: string, targetId: string): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.mergeCandidates, sourceId, targetId)
  }

  async saveSettings(patch: SettingsPatch): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.saveSettings, patch)
  }

  async deleteLastLesson(): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.deleteLastLesson)
  }

  async refresh(): Promise<AppSnapshot> {
    return this.callSnapshot(BACKEND_METHODS.refresh)
  }

  subscribe(listener: (snapshot: AppSnapshot) => void): () => void {
    const eventsOn = window.runtime?.EventsOn
    if (!eventsOn) return () => undefined

    const callback = (...args: unknown[]) => {
      const possibleSnapshot = args[0]
      if (isAppSnapshot(possibleSnapshot)) {
        const accepted = this.remember(possibleSnapshot)
        if (accepted === possibleSnapshot) listener(accepted)
        return
      }
      void this.getSnapshot().then(listener).catch(() => undefined)
    }
    const levelsCallback = (...args: unknown[]) => {
      const levels = args[0]
      if (!this.latestSnapshot || !isAudioLevels(levels)) return
      const next = {
        ...this.latestSnapshot,
        levels,
      }
      this.latestSnapshot = next
      listener(next)
    }
    const snapshotCleanup = eventsOn(BACKEND_EVENTS.snapshotChanged, callback)
    const levelsCleanup = eventsOn(BACKEND_EVENTS.captureLevels, levelsCallback)
    return () => {
      if (typeof snapshotCleanup === 'function') snapshotCleanup()
      else window.runtime?.EventsOff?.(BACKEND_EVENTS.snapshotChanged)
      if (typeof levelsCleanup === 'function') levelsCleanup()
      else window.runtime?.EventsOff?.(BACKEND_EVENTS.captureLevels)
    }
  }

  private async callSnapshot(methodName: string, ...args: unknown[]): Promise<AppSnapshot> {
    const result = await this.call(methodName, args)
    if (!isAppSnapshot(result)) {
      throw new ApiUnavailableError(`Backend method ${methodName} returned an invalid snapshot.`)
    }
    return this.remember(result)
  }

  private async callMutation(methodName: string, ...args: unknown[]): Promise<AppSnapshot> {
    const result = await this.call(methodName, args)
    if (!isAppSnapshot(result)) {
      throw new ApiUnavailableError(`Backend method ${methodName} returned an invalid snapshot.`)
    }
    return this.remember(result)
  }

  private remember(snapshot: AppSnapshot): AppSnapshot {
    const currentTime = Date.parse(this.latestSnapshot?.lastUpdated ?? '')
    const nextTime = Date.parse(snapshot.lastUpdated)
    if (
      this.latestSnapshot &&
      Number.isFinite(currentTime) &&
      Number.isFinite(nextTime) &&
      nextTime < currentTime
    ) {
      return this.latestSnapshot
    }
    this.latestSnapshot = snapshot
    return snapshot
  }

  private async call(methodName: string, args: unknown[]): Promise<unknown> {
    const method = this.app[methodName]
    if (typeof method !== 'function') {
      throw new ApiUnavailableError(`Backend method ${methodName} is not available.`)
    }
    return (method as (...parameters: unknown[]) => unknown).apply(this.app, args)
  }
}

class UnavailableApi implements AppApi {
  readonly mode = 'unavailable' as const

  getSnapshot(): Promise<AppSnapshot> {
    return Promise.reject(new ApiUnavailableError())
  }

  generateManualVocabulary(): Promise<ManualVocabularyDraft> {
    return Promise.reject(new ApiUnavailableError())
  }

  saveManualVocabulary(): Promise<AppSnapshot> {
    return this.fail()
  }

  requestCapturePermission(): Promise<AppSnapshot> {
    return this.fail()
  }

  startLesson(): Promise<AppSnapshot> {
    return this.fail()
  }

  stopLesson(): Promise<AppSnapshot> {
    return this.fail()
  }

  markMoment(): Promise<AppSnapshot> {
    return this.fail()
  }

  confirmCandidate(): Promise<AppSnapshot> {
    return this.fail()
  }

  editCandidate(): Promise<AppSnapshot> {
    return this.fail()
  }

  rejectCandidate(): Promise<AppSnapshot> {
    return this.fail()
  }

  mergeCandidates(): Promise<AppSnapshot> {
    return this.fail()
  }

  saveSettings(): Promise<AppSnapshot> {
    return this.fail()
  }

  deleteLastLesson(): Promise<AppSnapshot> {
    return this.fail()
  }

  refresh(): Promise<AppSnapshot> {
    return this.fail()
  }

  subscribe(): () => void {
    return () => undefined
  }

  private fail(): Promise<AppSnapshot> {
    return Promise.reject(new ApiUnavailableError())
  }
}

function getRuntimeApp(): RuntimeApp | undefined {
  if (typeof window === 'undefined') return undefined
  const app = window.go?.main?.App
  return app && typeof app === 'object' ? app : undefined
}

function isDevBuild(): boolean {
  return typeof import.meta !== 'undefined' && Boolean(import.meta.env?.DEV)
}

export function createDemoApi(): AppApi {
  return new DemoApi()
}

export function createUnavailableApi(): AppApi {
  return new UnavailableApi()
}

export function createApi(options: { mode?: ApiModePreference } = {}): AppApi {
  const preference = options.mode ?? 'auto'
  if (preference === 'demo') return createDemoApi()
  if (preference === 'unavailable') return createUnavailableApi()

  const runtimeApp = getRuntimeApp()
  if (runtimeApp) return new RuntimeApi(runtimeApp)
  if (preference === 'runtime') return createUnavailableApi()
  return isDevBuild() ? createDemoApi() : createUnavailableApi()
}

export const api = createApi()

// Keep these imports type-visible in generated declaration output for backend
// implementers reading this contract.
export type BackendSnapshotContract = {
  connection: ConnectionState
  capturePermission: CapturePermission
  readiness: AppSnapshot['readiness']
  target: TargetApp
  lesson: { id?: string; status: LessonStatus }
  levels: AudioLevels
  cost: CostSummary
  privacy: PrivacyState
  settings: Settings
  candidates: Array<{
    bucket: CandidateBucket
    status: CandidateStatus
  }>
  vocabulary: Array<{
    status: VocabularyStatus
  }>
}
