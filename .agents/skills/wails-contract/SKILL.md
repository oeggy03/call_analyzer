---
name: wails-contract
description: >-
  Change the Call Analyzer Go-to-React Wails contract. Use for App methods,
  snapshot fields, request types, frontend backend methods, or runtime/demo
  client shapes; skip for visual-only UI changes.
---

# Wails contract

Use this when a change alters what the UI can call or the data returned across Wails. Read `docs/architecture.md` and `frontend/README.md`, then inspect the current Go and TypeScript declarations before editing.

## Keep both sides aligned

1. Update the exported method and any input/output structs in `app.go` and `app_contract.go`; update `snapshot.go` only when snapshot mapping changes. Keep JSON fields camelCase.
2. Update `BACKEND_METHODS`, the `AppApi` interface, and both `RuntimeApi` and `DemoApi` in `frontend/src/lib/api.ts`. Update `frontend/src/lib/types.ts` for changed data shapes. UI code calls through `api.ts`.
3. Preserve special mappings such as frontend `pinyin` to `domain.CandidateEdit.Reading` in `app.go`.
4. Match the method's actual result shape. Snapshot-returning mutations use `AppSnapshot`. Current exceptions return `ManualVocabularyDraft` from `GenerateManualVocabulary`, `PhotoVocabularyDraft` from `RecognizeVocabularyPhoto`, `[]LessonTranscriptSummary` from `ListLessonTranscripts`, `LessonTranscript` from `GetLessonTranscript`, and `[]string` from `ListTags`. Check `app.go` and `contract_test.go` when methods change.
5. Keep secrets and local process data out of the boundary: do not expose `OPENROUTER_API_KEY`, `SUPABASE_SECRET_KEY`, selection lookup text, audio bytes, or local filesystem paths in `AppSnapshot`. Keep runtime event names synchronized between `app.go` and `BACKEND_EVENTS` in `frontend/src/lib/api.ts` (`call_analyzer:snapshot_changed`, `call_analyzer:capture.levels`, `call_analyzer:selection.add-word`).
6. Treat generated Wails bindings as build outputs. Change the source contract and let the Wails build regenerate outputs when needed.

Update or add a boundary test when the contract changes. `TestWailsMethodsMatchFrontendContract` in `contract_test.go` should check the result shapes above; its expectation map is part of the contract and can become stale.

## Verification

Run the Wails method/snapshot row in the focused check matrix in `docs/development.md`. For behavior changes, also run the relevant Go package checks listed there. The demo client must accept the same arguments and return the same shapes as the runtime client without a network call.

See [reference.md](reference.md) for source paths and test entry points.
