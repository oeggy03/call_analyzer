# Agent guide

Mandarin Lesson Analyzer is a local-first macOS desktop app. Its window title is `Call Analyzer`, its bundle display name is `Mandarin Lesson Analyzer`, and its Go module is `github.com/oeggy03/call_analyzer`.

## Ecosystem

Three repositories sit next to each other. This one owns the desktop app, its SQLite file, and the vocabulary sync goroutine. `../call-analyzer-lesson-db` owns the shared Postgres migrations. `../call-analyzer-active-recall` is the live Cloud Run service that reads confirmed vocabulary and sends ten words to Telegram at 9:00 `Asia/Singapore`. It does not write reviews yet.

Do not edit either sibling unless the user opened that workspace. A shared column starts as a new migration in `call-analyzer-lesson-db`, then this app's sync mapping, then the recall service if it reads the column. The recall service uses its own Supabase secret in Google Secret Manager. Never copy this Mac's `SUPABASE_SECRET_KEY` into that service or onto a Windows machine.

## Source of truth

Current executable code defines behavior. Use exported Go methods and request types, frontend types and clients, and embedded migrations for exact contracts. Generated files are outputs; inspect their source instead. Documentation explains those sources and gives workflow context. If a claim conflicts with current code, trace the call path and correct the relevant documentation when the task includes that behavior; do not repeat the stale claim as fact. A test can also encode an outdated expectation, so compare it with the implementation.

The budget fields are a legacy settings and display contract. `internal/openrouter/client.go` currently leaves `Budget.Check` and `HardExceeded` inactive; do not describe the displayed `hardBudgetUsd` setting as a runtime cloud-call cap.

## Read by task

| Need | Read |
| --- | --- |
| Product walkthrough and package map | `README.md` |
| UI, Wails, service, capture, or database boundary | `docs/architecture.md` |
| Lesson, vocabulary, privacy, settings, or budget behavior | `docs/behavior.md` |
| Install, environment, development, or checks | `docs/development.md` |
| App bundle, permissions, or local data files | `docs/operations.md` |
| Existing architectural choices | `docs/decisions.md` |
| Verified unfinished behavior or mismatch | `docs/limitations.md` |
| React boundary | `frontend/README.md` |
| Capture helper process | `internal/capture/macos/README.md` |
| Desktop vocabulary sync | `.agents/skills/supabase-sync/SKILL.md` |
| Delegation and role map | `docs/agents.md` |
| Multi-session handoff | `docs/work/README.md` |

Do not read every document for a small single-package change.

## Models and delegation

The repository Codex configuration assigns the orchestrator/planner `gpt-6.1-sol` with high reasoning. The orchestrator plans, reviews, integrates, and validates; it does not implement repository changes. Frontend, backend, native capture, tooling, and documentation implementation roles use `gpt-6-luna` with xhigh reasoning.

These instructions authorize the orchestrator to delegate implementation work within the user's requested repository scope. Assign each task to the matching configured role with bounded, non-overlapping ownership; the orchestrator integrates cross-boundary contracts and reviews the result. If the required role or model is unavailable or not loaded, report that and pause dependent implementation instead of silently substituting a model. Role files do not change the user's approval or sandbox settings.

## Project invariants

- `StartLesson` checks consent, target, and API-key readiness. Settings are locked during an active lesson.
- New automatic suggestions start as `candidate`; when a suggestion matches an existing word, its current status is preserved. Confirmation promotes a candidate to trusted vocabulary; reject and merge resolve it without promotion. `SaveManualVocabulary` creates a confirmed entry, while generation and photo recognition return drafts without inserting vocabulary.
- Never put the OpenRouter key in SQLite, logs, or `AppSnapshot`. The UI receives only `openRouterKeyConfigured`. On macOS the environment value precedes Keychain.
- Transcription and standard chat requests ask for Zero Data Retention and deny data collection. Vision photo-recognition chat intentionally omits those controls; inspect `internal/openrouter/client.go` before making claims about provider privacy flags. Provider flags are separate from local audio retention.
- macOS `Source.Configure` maps system-audio aliases to its system-audio source and accepts a nonblank explicit bundle ID. Wails currently limits lesson targets in `validTarget`; Zoom is a default option, not the only app the capture source can target. Check both boundaries before claiming a target is supported in the UI. Swift emits 2-second chunks; Go assembles utterance windows in `internal/service/capture_pipeline.go`.
- The selection lookup helper is separate from capture. `helperEnvironment` removes `OPENROUTER_API_KEY` and `SUPABASE_SECRET_KEY` before that process starts. Selection text must not be logged or stored in `AppSnapshot`. Non-Darwin builds keep `internal/selection.Runner` as a no-op.
- `loadSettings` forces sync off. The app does not start `internal/sync.Dispatcher`; never sync the SQLite file.
- The vocabulary sync runner is a separate cross-platform goroutine started by
  `Service.Initialize`; it does not use or revive the legacy `SyncEnabled` flag.
  It waits ten minutes after the newest eligible vocabulary outbox row and
  uploads the current confirmed or manually saved SQLite row. Launch pulls
  confirmed vocabulary first. `SyncNow` uploads only. Startup checks that the
  secret can read and write; a replacement is saved through the same secret
  store and is never placed in the snapshot. A merged candidate
  is not uploaded; when its target is confirmed, that target is uploaded with
  the moved senses, examples, and tags. Closing asks before uploading pending
  rows; declining leaves them in the outbox. Study review events stay pending.
  Deleting a saved word removes it locally and enqueues `vocabulary.entry.deleted`.
  Launch pull leaves that word out while the delete is still pending.
- `SUPABASE_SECRET_KEY` is read from the environment first on every OS. On
  macOS only, the fallback is this machine's Keychain item under service
  `github.com/oeggy03.call_analyzer` and account `SUPABASE_SECRET_KEY`.
  Windows and other non-Darwin builds have no Keychain fallback and must use a
  different `SUPABASE_SECRET_KEY` environment value; never reuse the Mac key.
- The sync runner must use the app-provided secret store, not construct a
  default store internally. Never place the Supabase secret in SQLite, logs,
  tests, documentation, or `AppSnapshot`.
- Wails JSON is camelCase. `EditCandidate` sends `pinyin`, mapped in `app.go` to `domain.CandidateEdit.Reading`.
- Keep the non-Darwin capture stubs compiling when changing the capture implementation.
- Tests and fixture evaluation stay offline. Never pass `-execute` to `cmd/evaluate` unless the user explicitly asks for live OpenRouter calls.
- Do not read or write the real Keychain item or `~/Library/Application Support/call_analyzer/call_analyzer.db` unless the user asks.

## Contract and completion

Before a cross-component change, find all callers and consumers, inspect persisted data, and update the Go and frontend contracts together. For a Wails change, inspect `app.go`, `app_contract.go`, `frontend/src/lib/types.ts`, and both clients in `frontend/src/lib/api.ts`. For a schema change, add the next migration; do not edit an applied migration.

If work will span sessions or another agent must resume it, follow `docs/work/README.md`: create a focused `docs/work/<short-slug>.md` handoff note and delete it when the work is done. Do not add a handoff note for routine work.

Preserve unrelated user changes. Run the checks in `docs/development.md` that match the change, inspect their output, and report what could not be verified. Update task-relevant docs when setup, a contract, workflow, architecture, or non-obvious behavior changes.
