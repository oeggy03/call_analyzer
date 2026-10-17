# Call Analyzer frontend

This directory contains the Vite + React + strict TypeScript desktop UI for
the macOS MVP. The UI uses semantic HTML, keyboard-visible controls, a
responsive CSS layout, and a calm high-contrast palette designed for Chinese
learning workflows.

## Commands

```bash
npm install
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

## Backend contract

`src/lib/api.ts` is the only integration boundary. It detects
`window.go.main.App` and centralizes the expected Wails method names in
`BACKEND_METHODS`. Every method returns a full `AppSnapshot`. Argument shapes
match the Wails contract exactly: `StartLesson` and `SaveSettings` receive one
object; candidate confirmation, editing, and rejection receive positional
IDs; `EditCandidate` receives `(candidateID, patch)` where the patch maps
`pinyin` and/or `meaning`; and `MergeCandidates` receives `(sourceID,
targetID)`.

The current expected methods are:

- `GetAppSnapshot`
- `RequestCapturePermission`
- `StartLesson`
- `StopLesson`
- `MarkMoment`
- `ConfirmCandidate`
- `EditCandidate`
- `RejectCandidate`
- `MergeCandidates`
- `SaveSettings`
- `Refresh`

The runtime event `call_analyzer:snapshot_changed` is subscribed to when
`window.runtime.EventsOn` exists. An event payload may be an `AppSnapshot`; if
it is not, the frontend fetches a fresh snapshot.

Domain types in `src/lib/types.ts` use **camelCase** consistently. The Go
methods should expose JSON with camelCase keys (for example `capturePermission`,
`lastUpdated`, and `openRouterKeyConfigured`). If Go structs use snake_case
instead, add the conversion in the runtime adapter rather than in components.

Settings key input is write-only by design. The frontend sends a new key only
when the user saves it and only receives a boolean
`openRouterKeyConfigured` value.

ZDR describes provider-side retention. Local audio retention is controlled
separately by the selected audio retention setting.
