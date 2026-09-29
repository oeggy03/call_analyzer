/**
 * Frontend domain types use camelCase. The Wails adapter is the one place that
 * should translate to/from Go JSON if the backend serializes snake_case.
 */

export type ConnectionState = 'connected' | 'connecting' | 'offline' | 'unavailable' | 'demo'
export type CapturePermission = 'unknown' | 'requesting' | 'granted' | 'denied'
export type TargetApp = 'zoom' | 'googleMeet' | 'teams'
export type LessonStatus = 'idle' | 'starting' | 'live' | 'stopping' | 'error'
export type TranscriptSource = 'microphone' | 'remote' | 'system' | 'system/ocr' | 'system/manual'
export type CandidateBucket = 'manual' | 'highConfidence' | 'possibleDuplicate' | 'lowConfidence'
export type CandidateStatus = 'pending' | 'confirmed' | 'rejected'
export type VocabularyStatus = 'learning' | 'review' | 'mastered'

export interface AudioLevels {
  mic: number
  remote: number
}

export interface LessonState {
  id?: string
  status: LessonStatus
  startedAt?: string
  error?: string
}

export interface TranscriptLine {
  id: string
  text: string
  source: TranscriptSource
  timestamp: string
  speaker?: string
  isMoment?: boolean
}

export interface Candidate {
  id: string
  simplified: string
  traditional?: string
  pinyin: string
  meaning: string
  partOfSpeech?: string
  classifier?: string
  example: string
  examplePinyin: string
  exampleTranslation: string
  provenance: string
  confidence: number
  evidence: string
  timestamp: string
  bucket: CandidateBucket
  status: CandidateStatus
  duplicateOf?: string
  tags: string[]
}

export interface VocabularyEntry {
  id: string
  simplified: string
  traditional?: string
  pinyin: string
  meaning: string
  partOfSpeech?: string
  classifier?: string
  example: string
  examplePinyin: string
  exampleTranslation: string
  status: VocabularyStatus
  tags: string[]
  lastSeen: string
  seenCount: number
}

export interface CostSummary {
  currentUsd: number
  projectedUsd: number
  hardBudgetUsd: number
  currency: 'USD'
  warning: boolean
  hardExceeded: boolean
}

export interface PrivacyState {
  zdrEnabled: boolean
  audioRetention: 'sessionOnly' | 'oneDay' | 'sevenDays'
}

export interface Settings {
  openRouterKeyConfigured: boolean
  sttModel: string
  analyzerModel: string
  hardBudgetUsd: number
  audioRetention: PrivacyState['audioRetention']
  ocrEnabled: boolean
  microphoneEnabled: boolean
}

export interface AppSnapshot {
  connection: ConnectionState
  capturePermission: CapturePermission
  target: TargetApp
  lesson: LessonState
  levels: AudioLevels
  transcript: TranscriptLine[]
  candidates: Candidate[]
  vocabulary: VocabularyEntry[]
  cost: CostSummary
  privacy: PrivacyState
  settings: Settings
  lastUpdated: string
}

export interface StartLessonInput {
  target: TargetApp
  consent: boolean
}

export interface CandidateEditPatch {
  simplified?: string
  traditional?: string
  pinyin?: string
  meaning?: string
  partOfSpeech?: string
  classifier?: string
  example?: string
  examplePinyin?: string
  exampleTranslation?: string
  tags?: string[]
}

export interface SettingsPatch {
  openRouterKey?: string
  sttModel?: string
  analyzerModel?: string
  hardBudgetUsd?: number
  audioRetention?: Settings['audioRetention']
  ocrEnabled?: boolean
  microphoneEnabled?: boolean
}
