# Call Analyzer frontend

This directory contains the Vite + React + strict TypeScript desktop UI for
the macOS MVP. The UI uses semantic HTML, keyboard-visible controls, a
responsive CSS layout, and a calm high-contrast palette designed for Chinese
learning workflows.

## Commands

```bash
# From the repository root, ./scripts/setup.sh runs npm ci in this directory.
npm ci
npm run dev
npm run typecheck
npm test
npm run build
npm run lint
```

`npm run dev` uses a deterministic in-memory demo adapter when the Wails
runtime is not present. This makes the UI useful in a browser and keeps tests
independent of a running desktop backend. A production build without a Wails
runtime shows the backend-unavailable state instead of pretending to capture.
The demo defaults are `qwen/qwen3-asr-1.7b` for STT and
`qwen/qwen3-32b` for analysis.

Vitest specs and shared setup live in `src/test`.

## Backend contract

`src/lib/api.ts` is the only integration boundary. It detects
`window.go.main.App` and centralizes the expected Wails method names in
`BACKEND_METHODS`. That object is the method list; do not keep a second copy
here. `contract_test.go` fails when a listed name is missing on `App`.

Most methods return `AppSnapshot`. Five methods return dedicated result types:
`GenerateManualVocabulary` returns `ManualVocabularyDraft`,
`RecognizeVocabularyPhoto` returns `PhotoVocabularyDraft`,
`ListLessonTranscripts` returns `[]LessonTranscriptSummary`,
`GetLessonTranscript` returns `LessonTranscript`, and `ListTags` returns
`[]string`. Argument shapes are the
structs in `app_contract.go` and `src/lib/types.ts`. `StartLesson` and
`SaveSettings` receive one object.
Confirm, reject, merge, and vocabulary delete receive positional IDs. `EditCandidate` receives
`(candidateID, patch)`; `app.go` maps patch `pinyin` onto `reading`.

The runtime events `call_analyzer:snapshot_changed`,
`call_analyzer:capture.levels`, and `call_analyzer:selection.add-word` are
subscribed to when `window.runtime.EventsOn` exists. A snapshot event payload
may be an `AppSnapshot`; if it is not, the frontend fetches a fresh snapshot.
`capture.levels` updates the live audio meters in place. `selection.add-word`
remains wired for routing into Vocabulary, Candidate Inbox, or Add Word, though
the native lookup helper currently edits and saves cards inside its own panel
and triggers `snapshot_changed`. The event names are `BACKEND_EVENTS`.

Domain types in `src/lib/types.ts` use **camelCase** consistently. The Go
methods should expose JSON with camelCase keys (for example `capturePermission`,
`lastUpdated`, and `openRouterKeyConfigured`). If Go structs use snake_case
instead, add the conversion in the runtime adapter rather than in components.

Secret inputs are write-only by design. Settings sends a new OpenRouter key only
when the user saves it and only receives `openRouterKeyConfigured`; Live Lesson
sends a replacement Supabase secret through `SaveSupabaseKey` and receives only
`supabaseSyncConfigured`, `supabaseAccessStatus`, and `supabaseAccessDetail`.

The Live view requests system audio across apps and does not require Zoom.
Add Word generation is disabled during capture, while saving a complete
manual entry remains available. Photo reading and saving are also available
during capture.

`privacy.zdrEnabled` reports whether Zero Data Retention routing is active (`false`
by default; lesson transcription, lesson analysis, Add Word generation, and
selection lookup send `data_collection: deny` without restricting `zdr` routing).
Photo vision requests do not include ZDR or
data-collection-denial fields. Local audio retention is separate; the helper
currently deletes audio after capture regardless of the selected retention
label.
