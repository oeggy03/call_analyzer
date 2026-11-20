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
  LessonTranscript,
  LessonTranscriptSummary,
  ManualVocabularyDraft,
  ManualVocabularyInput,
  ManualVocabularyVariant,
  PhotoVocabularyDraft,
  RegenerateManualVocabularyInput,
  SelectionAddWordEvent,
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

export function defaultLessonTag(now = new Date()): string {
  const pad = (value: number) => String(value).padStart(2, '0')
  return `lesson_${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}_${pad(now.getHours())}-${pad(now.getMinutes())}`
}

function storedLessonTags(raw?: string): string {
  const seen = new Set<string>()
  const tags: string[] = []
  for (const part of (raw ?? '').split(',')) {
    const tag = part.trim().replace(/\s+/g, ' ')
    if (!tag || seen.has(tag)) continue
    seen.add(tag)
    tags.push(Array.from(tag).slice(0, 64).join(''))
    if (tags.length === 8) break
  }
  return tags.join(',') || defaultLessonTag()
}

/**
 * Keep Wails method names in one place. If the generated binding changes,
 * update this map rather than changing UI code.
 */
export const BACKEND_METHODS = {
  getSnapshot: 'GetAppSnapshot',
  generateManualVocabulary: 'GenerateManualVocabulary',
  regenerateManualVocabulary: 'RegenerateManualVocabulary',
  recognizeVocabularyPhoto: 'RecognizeVocabularyPhoto',
  saveManualVocabulary: 'SaveManualVocabulary',
  listLessonTranscripts: 'ListLessonTranscripts',
  listTags: 'ListTags',
  getLessonTranscript: 'GetLessonTranscript',
  requestCapturePermission: 'RequestCapturePermission',
  startLesson: 'StartLesson',
  stopLesson: 'StopLesson',
  markMoment: 'MarkMoment',
  confirmCandidate: 'ConfirmCandidate',
  editCandidate: 'EditCandidate',
  rejectCandidate: 'RejectCandidate',
  mergeCandidates: 'MergeCandidates',
  deleteVocabulary: 'DeleteVocabulary',
  saveSettings: 'SaveSettings',
  syncNow: 'SyncNow',
  saveSupabaseKey: 'SaveSupabaseKey',
  deleteLastLesson: 'DeleteLastLesson',
  refresh: 'Refresh',
} as const

export const BACKEND_EVENTS = {
  snapshotChanged: 'call_analyzer:snapshot_changed',
  captureLevels: 'call_analyzer:capture.levels',
  selectionAddWord: 'call_analyzer:selection.add-word',
} as const

export type ApiMode = 'runtime' | 'demo' | 'unavailable'
export type ApiModePreference = 'auto' | ApiMode

export interface AppApi {
  readonly mode: ApiMode
  getSnapshot(): Promise<AppSnapshot>
  generateManualVocabulary(input: ManualVocabularyInput): Promise<ManualVocabularyDraft>
  regenerateManualVocabulary(input: RegenerateManualVocabularyInput): Promise<ManualVocabularyDraft>
  recognizeVocabularyPhoto(imageBase64: string): Promise<PhotoVocabularyDraft>
  saveManualVocabulary(input: ManualVocabularyInput): Promise<AppSnapshot>
  listLessonTranscripts(limit: number): Promise<LessonTranscriptSummary[]>
  listTags(): Promise<string[]>
  getLessonTranscript(lessonId: string): Promise<LessonTranscript>
  requestCapturePermission(): Promise<AppSnapshot>
  startLesson(input: StartLessonInput): Promise<AppSnapshot>
  stopLesson(): Promise<AppSnapshot>
  markMoment(): Promise<AppSnapshot>
  confirmCandidate(candidateId: string): Promise<AppSnapshot>
  editCandidate(candidateId: string, patch: CandidateEditPatch): Promise<AppSnapshot>
  rejectCandidate(candidateId: string): Promise<AppSnapshot>
  mergeCandidates(sourceId: string, targetId: string): Promise<AppSnapshot>
  deleteVocabulary(entryId: string): Promise<AppSnapshot>
  saveSettings(patch: SettingsPatch): Promise<AppSnapshot>
  syncNow(): Promise<AppSnapshot>
  saveSupabaseKey(key: string): Promise<AppSnapshot>
  deleteLastLesson(): Promise<AppSnapshot>
  refresh(): Promise<AppSnapshot>
  subscribe(listener: (snapshot: AppSnapshot) => void): () => void
  subscribeSelectionAddWord(listener: (event: SelectionAddWordEvent) => void): () => void
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
const DEMO_MANUAL_VOCABULARY_MODEL = 'qwen/qwen3-32b'
const DEMO_MANUAL_VOCABULARY_COST = 0.0002
const DEMO_PHOTO_VOCABULARY_COST = 0.0001

const demoTranscript: TranscriptLine[] = [
  {
    id: 'line-1',
    text: '我们下周可以把这个方案再讨论一下。',
    pinyin: 'wǒ men xià zhōu kě yǐ bǎ zhè gè fāng àn zài tǎo lùn yī xià。',
    source: 'remote',
    timestamp: '2026-09-29T14:29:42.000Z',
    speaker: 'Remote',
  },
  {
    id: 'line-2',
    text: '好的，我先整理一下重点。',
    pinyin: 'hǎo de， wǒ xiān zhěng lǐ yī xià zhòng diǎn。',
    source: 'microphone',
    timestamp: '2026-09-29T14:29:49.000Z',
    speaker: 'You',
  },
  {
    id: 'line-3',
    text: '这个表达在工作场景里很常见。',
    pinyin: 'zhè gè biǎo dá zài gōng zuò chǎng jǐng lǐ hěn cháng jiàn。',
    source: 'remote',
    timestamp: '2026-09-29T14:29:58.000Z',
    speaker: 'Remote',
  },
]

const demoPastLessons: LessonTranscript[] = [
  {
    lesson: {
      id: 'demo-lesson-2',
      title: 'zoom',
      startedAt: '2026-09-28T09:15:00.000Z',
      endedAt: '2026-09-28T09:42:00.000Z',
      segmentCount: 2,
      finalCost: 0.0031,
    },
    lines: [
      {
        id: 'past-line-1',
        text: '上次我们确认了发布日期。',
        pinyin: 'shàng cì wǒ men què rèn le fā bù rì qī。',
        source: 'remote',
        timestamp: '2026-09-28T09:27:00.000Z',
        speaker: 'Remote',
      },
      {
        id: 'past-line-2',
        text: '我会把更新后的计划发给大家。',
        pinyin: 'wǒ huì bǎ gēng xīn hòu de jì huà fā gěi dà jiā。',
        source: 'microphone',
        timestamp: '2026-09-28T09:28:12.000Z',
        speaker: 'You',
      },
    ],
  },
  {
    lesson: {
      id: 'demo-lesson-1',
      title: 'Weekly planning',
      startedAt: '2026-09-27T14:30:00.000Z',
      endedAt: '2026-09-27T14:51:00.000Z',
      segmentCount: 3,
      finalCost: 0,
    },
    lines: demoTranscript,
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
      zdrEnabled: false,
      audioRetention: 'sessionOnly',
    },
    settings: {
      openRouterKeyConfigured: true,
      supabaseSyncConfigured: true,
      syncPendingCount: 0,
      syncStatus: 'idle',
      lastSyncAt: DEMO_TIMESTAMP,
      supabaseAccessStatus: 'ready',
      supabaseAccessDetail: 'Cloud vocabulary can be read and written.',
      sttModel: 'qwen/qwen3-asr-1.7b',
      analyzerModel: 'qwen/qwen3-32b',
      visionModel: 'qwen/qwen3.7-flash',
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

function manualVocabularyInputToVariant(input: ManualVocabularyInput): ManualVocabularyVariant {
  return {
    traditional: input.traditional,
    pinyin: input.pinyin?.trim() ?? '',
    meaning: input.meaning?.trim() ?? '',
    partOfSpeech: input.partOfSpeech,
    classifier: input.classifier,
    example: input.example?.trim() ?? '',
    examplePinyin: input.examplePinyin?.trim() ?? '',
    exampleTranslation: input.exampleTranslation?.trim() ?? '',
  }
}

function hasManualVocabularyContext(input: ManualVocabularyInput): boolean {
  return Boolean(
    input.meaning?.trim() ||
      input.pinyin?.trim() ||
      input.example?.trim() ||
      input.exampleTranslation?.trim() ||
      input.partOfSpeech?.trim() ||
      input.classifier?.trim() ||
      input.tags?.length,
  )
}

function mergeManualVocabularyVariant(
  input: ManualVocabularyInput,
  variant: ManualVocabularyVariant,
): ManualVocabularyInput {
  return {
    simplified: input.simplified,
    traditional: input.traditional || variant.traditional || input.simplified,
    pinyin: input.pinyin || variant.pinyin,
    meaning: input.meaning || variant.meaning,
    partOfSpeech: input.partOfSpeech || variant.partOfSpeech || '',
    classifier: input.classifier || variant.classifier || '',
    example: input.example || variant.example,
    examplePinyin: input.examplePinyin || variant.examplePinyin,
    exampleTranslation: input.exampleTranslation || variant.exampleTranslation,
    tags: [...(input.tags ?? [])],
    aiGenerated: Boolean(input.aiGenerated) || !hasCompleteManualVocabulary(input),
  }
}

function loosePinyin(value: string): string {
  return value
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLocaleLowerCase()
    .replace(/[0-9\s'’·]/g, '')
}

function meaningsMatch(left: string, right: string): boolean {
  const normalizedLeft = left.trim().toLocaleLowerCase()
  const normalizedRight = right.trim().toLocaleLowerCase()
  if (!normalizedLeft || !normalizedRight) return false
  return (
    normalizedLeft === normalizedRight ||
    (normalizedLeft.length >= 4 && normalizedRight.includes(normalizedLeft)) ||
    (normalizedRight.length >= 4 && normalizedLeft.includes(normalizedRight))
  )
}

function chooseDemoVariant(input: ManualVocabularyInput, variants: ManualVocabularyVariant[]): number | undefined {
  if (variants.length === 1) return 0

  const meaningMatches = input.meaning
    ? variants.flatMap((variant, index) => (meaningsMatch(input.meaning ?? '', variant.meaning) ? [index] : []))
    : []
  if (meaningMatches.length === 1) return meaningMatches[0]

  const inputPinyin = loosePinyin(input.pinyin ?? '')
  if (inputPinyin) {
    const pinyinMatches = variants.flatMap((variant, index) => (
      loosePinyin(variant.pinyin) === inputPinyin ? [index] : []
    ))
    const allVariantsSharePinyin = variants.every((variant) => loosePinyin(variant.pinyin) === inputPinyin)
    if (pinyinMatches.length === 1 && !allVariantsSharePinyin) return pinyinMatches[0]
  }

  return undefined
}

function isOptionalString(value: unknown): boolean {
  return value === undefined || typeof value === 'string'
}

function isManualVocabularyVariant(value: unknown): value is ManualVocabularyVariant {
  if (!isRecord(value)) return false
  const requiredStrings = ['pinyin', 'meaning', 'example', 'examplePinyin', 'exampleTranslation']
  if (requiredStrings.some((field) => typeof value[field] !== 'string' || !value[field].trim())) return false
  return ['traditional', 'partOfSpeech', 'classifier'].every((field) => isOptionalString(value[field]))
}

export function isManualVocabularyDraft(value: unknown): value is ManualVocabularyDraft {
  if (!isRecord(value)) return false
  if (typeof value.simplified !== 'string' || !value.simplified.trim()) return false
  if (typeof value.model !== 'string' || !value.model.trim()) return false
  if (typeof value.cost !== 'number' || !Number.isFinite(value.cost) || value.cost < 0) return false
  if (
    ![
      'traditional',
      'pinyin',
      'meaning',
      'partOfSpeech',
      'classifier',
      'example',
      'examplePinyin',
      'exampleTranslation',
    ].every((field) => isOptionalString(value[field]))
  ) return false
  if (value.aiGenerated !== undefined && typeof value.aiGenerated !== 'boolean') return false
  if (value.tags !== undefined && (!Array.isArray(value.tags) || !value.tags.every((tag) => typeof tag === 'string'))) {
    return false
  }
  if (value.needsChoice !== undefined && typeof value.needsChoice !== 'boolean') return false
  if (value.matchedContext !== undefined && typeof value.matchedContext !== 'boolean') return false
  if (
    value.appliedVariant !== undefined &&
    (typeof value.appliedVariant !== 'number' ||
      !Number.isFinite(value.appliedVariant) ||
      !Number.isInteger(value.appliedVariant) ||
      value.appliedVariant < -1)
  ) {
    return false
  }
  if (value.variants !== undefined) {
    if (!Array.isArray(value.variants) || !value.variants.every(isManualVocabularyVariant)) {
      return false
    }
  }

  if (value.needsChoice === true) {
    return Array.isArray(value.variants) && value.variants.length >= 2
  }

  const requiredStrings = ['pinyin', 'meaning', 'example', 'examplePinyin', 'exampleTranslation']
  return requiredStrings.every((field) => typeof value[field] === 'string' && value[field].trim())
}

function isManualVocabularyInputValue(value: unknown, requireAiGenerated = false): value is ManualVocabularyInput {
  if (!isRecord(value) || typeof value.simplified !== 'string' || !value.simplified.trim()) return false
  const stringFields = [
    'traditional',
    'pinyin',
    'meaning',
    'partOfSpeech',
    'classifier',
    'example',
    'examplePinyin',
    'exampleTranslation',
  ]
  if (!stringFields.every((field) => isOptionalString(value[field]))) return false
  if (value.tags !== undefined && (!Array.isArray(value.tags) || !value.tags.every((tag) => typeof tag === 'string'))) return false
  if (value.aiGenerated !== undefined && typeof value.aiGenerated !== 'boolean') return false
  return !requireAiGenerated || value.aiGenerated === true
}

export function isPhotoVocabularyDraft(value: unknown): value is PhotoVocabularyDraft {
  if (!isRecord(value)) return false
  return (
    Array.isArray(value.entries) &&
    value.entries.every((entry) => isManualVocabularyInputValue(entry, true)) &&
    typeof value.model === 'string' &&
    Boolean(value.model.trim()) &&
    typeof value.cost === 'number' &&
    Number.isFinite(value.cost) &&
    value.cost >= 0
  )
}

function isTranscriptLineValue(value: unknown): value is TranscriptLine {
  if (!isRecord(value)) return false
  const validSources = ['microphone', 'remote', 'system', 'system/ocr', 'system/manual']
  return (
    typeof value.id === 'string' &&
    typeof value.text === 'string' &&
    typeof value.timestamp === 'string' &&
    typeof value.source === 'string' &&
    validSources.includes(value.source) &&
    isOptionalString(value.speaker) &&
    (value.isMoment === undefined || typeof value.isMoment === 'boolean')
  )
}

export function isLessonTranscriptSummary(value: unknown): value is LessonTranscriptSummary {
  if (!isRecord(value)) return false
  return (
    typeof value.id === 'string' &&
    Boolean(value.id.trim()) &&
    typeof value.title === 'string' &&
    Boolean(value.title.trim()) &&
    isOptionalString(value.startedAt) &&
    isOptionalString(value.endedAt) &&
    typeof value.segmentCount === 'number' &&
    Number.isInteger(value.segmentCount) &&
    value.segmentCount >= 0 &&
    typeof value.finalCost === 'number' &&
    Number.isFinite(value.finalCost) &&
    value.finalCost >= 0
  )
}

export function isLessonTranscript(value: unknown): value is LessonTranscript {
  if (!isRecord(value)) return false
  return isLessonTranscriptSummary(value.lesson) && Array.isArray(value.lines) && value.lines.every(isTranscriptLineValue)
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

const DEMO_MANUAL_VOCABULARY_VARIANTS: Record<string, ManualVocabularyVariant[]> = {
  吞吐: [
    {
      traditional: '吞吐',
      pinyin: 'tūn tǔ',
      meaning: 'throughput',
      partOfSpeech: 'noun',
      example: '这个接口的吞吐很高。',
      examplePinyin: 'Zhè ge jiē kǒu de tūn tǔ hěn gāo.',
      exampleTranslation: 'This interface has high throughput.',
    },
    {
      traditional: '吞吐',
      pinyin: 'tūn tǔ',
      meaning: 'to take in and send out',
      partOfSpeech: 'verb',
      example: '江河在入海口不断吞吐。',
      examplePinyin: 'Jiāng hé zài rù hǎi kǒu bú duàn tūn tǔ.',
      exampleTranslation: 'Rivers keep taking in and sending out water where they meet the sea.',
    },
  ],
  行: [
    {
      traditional: '行',
      pinyin: 'xíng',
      meaning: 'to walk; to be OK',
      partOfSpeech: 'verb',
      example: '这样行不行？',
      examplePinyin: 'Zhè yàng xíng bu xíng?',
      exampleTranslation: 'Is this OK?',
    },
    {
      traditional: '行',
      pinyin: 'háng',
      meaning: 'row; profession',
      partOfSpeech: 'noun',
      classifier: '个',
      example: '他已经在这一行工作了十年。',
      examplePinyin: 'Tā yǐ jīng zài zhè yì háng gōng zuò le shí nián.',
      exampleTranslation: 'He has worked in this profession for ten years.',
    },
  ],
}

const DEMO_PHOTO_VOCABULARY_ENTRIES: ManualVocabularyInput[] = [
  {
    simplified: '学习',
    traditional: '學習',
    pinyin: 'xué xí',
    meaning: 'to study; to learn',
    partOfSpeech: 'verb',
    example: '我每天学习中文。',
    examplePinyin: 'Wǒ měi tiān xué xí Zhōng wén.',
    exampleTranslation: 'I study Chinese every day.',
    tags: ['study'],
    aiGenerated: true,
  },
  {
    simplified: '重点',
    traditional: '重點',
    pinyin: 'zhòngdiǎn',
    meaning: 'key point; main focus',
    partOfSpeech: 'noun',
    classifier: '个',
    example: '请先说一下这个方案的重点。',
    examplePinyin: 'Qǐng xiān shuō yíxià zhège fāng’àn de zhòngdiǎn.',
    exampleTranslation: 'Please first explain the key points of this plan.',
    tags: ['work'],
    aiGenerated: true,
  },
]

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
  private selectionAddWordListeners = new Set<(event: SelectionAddWordEvent) => void>()
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
    const variants = (DEMO_MANUAL_VOCABULARY_VARIANTS[normalized.simplified] ?? [
      manualVocabularyInputToVariant(generated),
    ]).map((variant) => ({ ...variant }))
    const appliedVariant = chooseDemoVariant(normalized, variants)
    const cost = DEMO_MANUAL_VOCABULARY_COST
    const draft: ManualVocabularyDraft = appliedVariant === undefined
      ? {
          ...normalized,
          model: DEMO_MANUAL_VOCABULARY_MODEL,
          cost,
          needsChoice: true,
          matchedContext: false,
          appliedVariant: -1,
          variants,
        }
      : {
          ...mergeManualVocabularyVariant(normalized, variants[appliedVariant]),
          model: DEMO_MANUAL_VOCABULARY_MODEL,
          cost,
          needsChoice: false,
          matchedContext: hasManualVocabularyContext(normalized),
          appliedVariant,
          variants,
        }
    this.snapshot.cost = {
      ...this.snapshot.cost,
      currentUsd: this.snapshot.cost.currentUsd + cost,
      projectedUsd: Math.max(this.snapshot.cost.projectedUsd, this.snapshot.cost.currentUsd + cost),
    }
    this.publish()
    return { ...draft, tags: [...(draft.tags ?? [])] }
  }

  async regenerateManualVocabulary(input: RegenerateManualVocabularyInput): Promise<ManualVocabularyDraft> {
    if (this.snapshot.lesson.status === 'live' || this.snapshot.lesson.status === 'starting') {
      throw new ApiValidationError('Finish the active lesson before regenerating word details.')
    }
    if (!this.snapshot.settings.openRouterKeyConfigured) {
      throw new ApiValidationError('Add an OpenRouter API key in Settings before regenerating details.')
    }
    const normalized = normalizeManualVocabularyInput(input)
    const instruction = (input.instruction ?? '').trim()
    const base = DEMO_MANUAL_VOCABULARY_EXAMPLES[normalized.simplified]
    const cost = DEMO_MANUAL_VOCABULARY_COST
    const nextExample = instruction.toLowerCase().includes('casual')
      ? `我平时也常常用到「${normalized.simplified}」。`
      : instruction.toLowerCase().includes('simple')
        ? `我懂「${normalized.simplified}」。`
        : `今天我们聊到了「${normalized.simplified}」。`
    const nextExamplePinyin = instruction.toLowerCase().includes('casual')
      ? 'Wǒ píng shí yě cháng cháng yòng dào zhè ge cí.'
      : instruction.toLowerCase().includes('simple')
        ? 'Wǒ dǒng zhè ge cí.'
        : 'Jīn tiān wǒ men liáo dào le zhè ge cí.'
    const nextExampleTranslation = instruction
      ? `Example revised (${instruction}): We talked about "${normalized.simplified}".`
      : `Today we talked about "${normalized.simplified}".`
    const draft: ManualVocabularyDraft = {
      simplified: normalized.simplified,
      traditional: normalized.traditional || base?.traditional || normalized.simplified,
      pinyin: normalized.pinyin || base?.pinyin || 'hǎo',
      meaning: normalized.meaning || base?.meaning || 'a useful Mandarin word',
      partOfSpeech: normalized.partOfSpeech || base?.partOfSpeech || 'word',
      classifier: normalized.classifier || base?.classifier || '',
      example: nextExample,
      examplePinyin: nextExamplePinyin,
      exampleTranslation: nextExampleTranslation,
      tags: [...(normalized.tags ?? [])],
      aiGenerated: true,
      model: DEMO_MANUAL_VOCABULARY_MODEL,
      cost,
      needsChoice: false,
      matchedContext: instruction.length > 0,
      appliedVariant: -1,
    }
    this.snapshot.cost = {
      ...this.snapshot.cost,
      currentUsd: this.snapshot.cost.currentUsd + cost,
      projectedUsd: Math.max(this.snapshot.cost.projectedUsd, this.snapshot.cost.currentUsd + cost),
    }
    this.publish()
    return { ...draft, tags: [...(draft.tags ?? [])] }
  }

  async recognizeVocabularyPhoto(imageBase64: string): Promise<PhotoVocabularyDraft> {
    if (imageBase64.includes('fail-photo')) {
      throw new ApiValidationError('This photo could not be read.')
    }
    if (!this.snapshot.settings.openRouterKeyConfigured) {
      throw new ApiValidationError('Add an OpenRouter API key in Settings before reading a photo.')
    }
    const entries = imageBase64.includes('single')
      ? DEMO_PHOTO_VOCABULARY_ENTRIES.slice(0, 1)
      : imageBase64.includes('photo-b')
        ? [
            DEMO_PHOTO_VOCABULARY_ENTRIES[1],
            {
              simplified: '方案',
              traditional: '方案',
              pinyin: 'fāng àn',
              meaning: 'plan; proposal',
              partOfSpeech: 'noun',
              classifier: '个',
              example: '我们需要讨论这个方案。',
              examplePinyin: 'Wǒmen xūyào tǎolùn zhège fāng àn.',
              exampleTranslation: 'We need to discuss this plan.',
              tags: [],
              aiGenerated: true,
            },
          ]
        : DEMO_PHOTO_VOCABULARY_ENTRIES
    const cost = DEMO_PHOTO_VOCABULARY_COST
    this.snapshot.cost = {
      ...this.snapshot.cost,
      currentUsd: this.snapshot.cost.currentUsd + cost,
      projectedUsd: Math.max(this.snapshot.cost.projectedUsd, this.snapshot.cost.currentUsd + cost),
    }
    this.publish()
    return {
      entries: entries.map((entry) => ({ ...entry, tags: [], aiGenerated: true })),
      model: this.snapshot.settings.visionModel,
      cost,
    }
  }

  async listTags(): Promise<string[]> {
    const names = new Set<string>()
    for (const entry of [...this.snapshot.vocabulary, ...this.snapshot.candidates]) {
      for (const tag of entry.tags) {
        const name = tag.trim()
        if (name) names.add(name)
      }
    }
    return [...names].sort((left, right) => left.localeCompare(right))
  }

  async listLessonTranscripts(limit: number): Promise<LessonTranscriptSummary[]> {
    const safeLimit = Number.isFinite(limit) ? Math.max(0, Math.floor(limit)) : 0
    return demoPastLessons.slice(0, safeLimit).map(({ lesson }) => ({ ...lesson }))
  }

  async getLessonTranscript(lessonId: string): Promise<LessonTranscript> {
    const lesson = demoPastLessons.find((item) => item.lesson.id === lessonId)
    if (!lesson) throw new ApiValidationError('Past lesson transcript no longer exists.')
    return {
      lesson: { ...lesson.lesson },
      lines: lesson.lines.map((line) => ({ ...line })),
    }
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
    const tags = storedLessonTags(input.tags)
    this.snapshot.lesson = {
      id: this.snapshot.lesson.id ?? 'demo-lesson-1',
      status: 'live',
      startedAt: DEMO_TIMESTAMP,
      tags,
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

  async deleteVocabulary(entryId: string): Promise<AppSnapshot> {
    if (!this.snapshot.vocabulary.some((entry) => entry.id === entryId)) {
      throw new ApiValidationError('That word is no longer in Vocabulary.')
    }
    this.snapshot.vocabulary = this.snapshot.vocabulary.filter((entry) => entry.id !== entryId)
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
    if (patch.visionModel !== undefined) next.visionModel = patch.visionModel
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

  async syncNow(): Promise<AppSnapshot> {
    return this.getSnapshot()
  }

  async saveSupabaseKey(key: string): Promise<AppSnapshot> {
    if (!key.trim()) throw new ApiValidationError('Cloud vocabulary key is required.')
    this.snapshot.settings = {
      ...this.snapshot.settings,
      supabaseSyncConfigured: true,
      supabaseAccessStatus: 'ready',
      supabaseAccessDetail: 'Cloud vocabulary can be read and written.',
    }
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

  subscribeSelectionAddWord(listener: (event: SelectionAddWordEvent) => void): () => void {
    this.selectionAddWordListeners.add(listener)
    return () => this.selectionAddWordListeners.delete(listener)
  }

  emitSelectionAddWord(event: SelectionAddWordEvent): void {
    for (const listener of this.selectionAddWordListeners) listener(event)
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

  async regenerateManualVocabulary(input: RegenerateManualVocabularyInput): Promise<ManualVocabularyDraft> {
    const result = await this.call(BACKEND_METHODS.regenerateManualVocabulary, [input])
    if (!isManualVocabularyDraft(result)) {
      throw new ApiUnavailableError(
        `Backend method ${BACKEND_METHODS.regenerateManualVocabulary} returned an invalid manual vocabulary draft.`,
      )
    }
    return result
  }

  async recognizeVocabularyPhoto(imageBase64: string): Promise<PhotoVocabularyDraft> {
    const result = await this.call(BACKEND_METHODS.recognizeVocabularyPhoto, [imageBase64])
    if (!isPhotoVocabularyDraft(result)) {
      throw new ApiUnavailableError(
        `Backend method ${BACKEND_METHODS.recognizeVocabularyPhoto} returned an invalid photo vocabulary draft.`,
      )
    }
    return result
  }

  async saveManualVocabulary(input: ManualVocabularyInput): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.saveManualVocabulary, input)
  }

  async listTags(): Promise<string[]> {
    const result = await this.call(BACKEND_METHODS.listTags, [])
    if (!Array.isArray(result) || !result.every((tag) => typeof tag === 'string')) {
      throw new ApiUnavailableError(`Backend method ${BACKEND_METHODS.listTags} returned invalid tags.`)
    }
    return result
  }

  async listLessonTranscripts(limit: number): Promise<LessonTranscriptSummary[]> {
    const result = await this.call(BACKEND_METHODS.listLessonTranscripts, [limit])
    if (!Array.isArray(result) || !result.every(isLessonTranscriptSummary)) {
      throw new ApiUnavailableError(
        `Backend method ${BACKEND_METHODS.listLessonTranscripts} returned invalid lesson transcripts.`,
      )
    }
    return result
  }

  async getLessonTranscript(lessonId: string): Promise<LessonTranscript> {
    const result = await this.call(BACKEND_METHODS.getLessonTranscript, [lessonId])
    if (!isLessonTranscript(result)) {
      throw new ApiUnavailableError(
        `Backend method ${BACKEND_METHODS.getLessonTranscript} returned an invalid lesson transcript.`,
      )
    }
    return result
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

  async deleteVocabulary(entryId: string): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.deleteVocabulary, entryId)
  }

  async saveSettings(patch: SettingsPatch): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.saveSettings, patch)
  }

  async syncNow(): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.syncNow)
  }

  async saveSupabaseKey(key: string): Promise<AppSnapshot> {
    return this.callMutation(BACKEND_METHODS.saveSupabaseKey, key)
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

  subscribeSelectionAddWord(listener: (event: SelectionAddWordEvent) => void): () => void {
    const eventsOn = window.runtime?.EventsOn
    if (!eventsOn) return () => undefined

    const callback = (...args: unknown[]) => {
      const possibleEvent = args[0]
      if (isRecord(possibleEvent) && typeof possibleEvent.destination === 'string') {
        listener(possibleEvent as unknown as SelectionAddWordEvent)
      }
    }
    const cleanup = eventsOn(BACKEND_EVENTS.selectionAddWord, callback)
    return () => {
      if (typeof cleanup === 'function') cleanup()
      else window.runtime?.EventsOff?.(BACKEND_EVENTS.selectionAddWord)
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

  regenerateManualVocabulary(): Promise<ManualVocabularyDraft> {
    return this.fail()
  }

  recognizeVocabularyPhoto(): Promise<PhotoVocabularyDraft> {
    return this.fail()
  }

  saveManualVocabulary(): Promise<AppSnapshot> {
    return this.fail()
  }

  listTags(): Promise<string[]> {
    return this.fail()
  }

  listLessonTranscripts(): Promise<LessonTranscriptSummary[]> {
    return this.fail()
  }

  getLessonTranscript(): Promise<LessonTranscript> {
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

  deleteVocabulary(): Promise<AppSnapshot> {
    return this.fail()
  }

  saveSettings(): Promise<AppSnapshot> {
    return this.fail()
  }

  syncNow(): Promise<AppSnapshot> {
    return this.fail()
  }

  saveSupabaseKey(): Promise<AppSnapshot> {
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

  subscribeSelectionAddWord(): () => void {
    return () => undefined
  }

  private fail<T>(): Promise<T> {
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

export function createDemoApi(): AppApi & {
  emitSelectionAddWord(event: SelectionAddWordEvent): void
} {
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
