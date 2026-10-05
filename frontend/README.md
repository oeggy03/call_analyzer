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

## Backend contract

`src/lib/api.ts` is the only integration boundary. It detects
`window.go.main.App` and centralizes the expected Wails method names in
`BACKEND_METHODS`. That object is the method list; do not keep a second copy
here. `contract_test.go` fails when a listed name is missing on `App`.

`GenerateManualVocabulary` returns `ManualVocabularyDraft`. The other methods
return `AppSnapshot`. Argument shapes are the structs in `app_contract.go` and
`src/lib/types.ts`. `StartLesson` and `SaveSettings` receive one object.
Confirm, reject, and merge receive positional IDs. `EditCandidate` receives
`(candidateID, patch)`; `app.go` maps patch `pinyin` onto `reading`.

The runtime event `call_analyzer:snapshot_changed` is subscribed to when
`window.runtime.EventsOn` exists. An event payload may be an `AppSnapshot`; if
it is not, the frontend fetches a fresh snapshot. The event names are
`BACKEND_EVENTS`.

Domain types in `src/lib/types.ts` use **camelCase** consistently. The Go
methods should expose JSON with camelCase keys (for example `capturePermission`,
`lastUpdated`, and `openRouterKeyConfigured`). If Go structs use snake_case
instead, add the conversion in the runtime adapter rather than in components.

Settings key input is write-only by design. The frontend sends a new key only
when the user saves it and only receives a boolean
`openRouterKeyConfigured` value.

ZDR describes provider-side retention. Local audio retention is controlled
separately by the selected audio retention setting.
