# Agent instructions

Mandarin Lesson Analyzer is a local-first macOS desktop app. It captures a consented Zoom lesson, sends short audio windows to OpenRouter, and stores Mandarin vocabulary in SQLite only after an explicit confirm or Add Word save.

The window title is "Call Analyzer". The bundle display name is "Mandarin Lesson Analyzer". The Go module is `github.com/oeggy03/call_analyzer`.

## Read this, not everything

| Document | Read it when |
| --- | --- |
| `README.md` | You need the product walkthrough or the package list. |
| `docs/architecture.md` | A change crosses the UI, Wails app, Go service, capture helper, or database. |
| `docs/behavior.md` | The change touches lessons, vocabulary, budget, privacy, or settings. |
| `docs/development.md` | You are installing, starting, configuring, or choosing checks. |
| `docs/operations.md` | You are building the app bundle, permissions, or local data files. |
| `docs/decisions.md` | You are tempted to replace the desktop shell, database, capture process, or sync model. |
| `docs/limitations.md` | Behavior looks unfinished or the UI and service disagree. |
| `frontend/README.md` | You are changing the React UI or its backend boundary. |
| `internal/capture/macos/README.md` | You are changing the helper process boundary. |
| `docs/work/README.md` | The task will span sessions or another agent must resume it. |

Do not read every document before a small edit inside one package.

## Components

| Area | Entry |
| --- | --- |
| Desktop shell | `main.go`, `app.go`, `snapshot.go`, `wails.json` |
| UI boundary | `frontend/src/lib/api.ts`, `frontend/src/lib/types.ts`, `frontend/src/App.tsx` |
| Lesson pipeline | `internal/service` |
| OpenRouter | `internal/openrouter/client.go` |
| SQLite | `internal/storage`, `internal/storage/migrations` |
| Capture interface | `internal/capture/capture.go` |
| macOS helper adapter | `internal/capture/macos` |
| Swift helper | `native/macos-capture` |
| Vocabulary rules | `internal/vocabulary` |
| Sync boundary, not started by the app | `internal/sync` |
| Fixture check | `cmd/evaluate`, `fixtures/evaluation/manifest.json` |

Non-macOS builds use `platform_source_stub.go` and `internal/capture/macos/adapter_stub.go`. Keep those stubs compiling.

## Commands

Run these from the repository root. Details and troubleshooting are in `docs/development.md`.

```sh
./scripts/setup.sh
./scripts/dev.sh
./scripts/verify.sh
```

`./scripts/dev.sh` starts the Wails desktop session on macOS. For the UI only, run `npm run dev` in `frontend`. That uses the in-memory demo when Wails is absent.

Cursor discovers project skills in `.cursor/skills`:

- `verify-call-analyzer` when running checks or interpreting a `verify.sh` failure.
- `wails-contract` when an `App` method or snapshot field changes.
- `sqlite-migration` when a table or embedded SQL file changes.

`./scripts/verify.sh` is the full local check. It must exit non-zero when a check fails. Do not replace it with `gofmt -w`. Do not pass `-execute` to `cmd/evaluate` unless the user explicitly wants billable OpenRouter calls.

## Invariants

- `StartLesson` requires consent, a supported target, and an OpenRouter key. Settings cannot change while a lesson is active.
- Audio and OCR suggestions are stored as `candidate`. They become trusted vocabulary only through confirm, or through an explicit Add Word save. Generate returns a draft and does not insert a row.
- The API key is never written to SQLite, logs, or `AppSnapshot`. The UI receives `openRouterKeyConfigured` only. On macOS the key comes from `OPENROUTER_API_KEY`, then Keychain.
- Provider requests ask for Zero Data Retention and deny data collection. That is separate from the local audio-retention setting.
- The hard lesson budget is greater than 0 and at most 0.50 USD. At the hard budget, cloud calls stop; capture and mark timestamps continue.
- The desktop capture target is Zoom bundle `us.zoom.xos`. The helper emits 2-second chunks. Go assembles the 8–15 second utterance windows.
- `loadSettings` forces sync off. The app never starts `internal/sync.Dispatcher`. Do not sync the SQLite file.
- JSON at the Wails boundary is camelCase. `EditCandidate` sends `pinyin`; `app.go` maps it to `domain.CandidateEdit.Reading`.

## Checks for a change

| Change | Run |
| --- | --- |
| Go only | `gofmt -l` on the edited files, `go test` and `go vet` for those packages. The root package embeds `frontend/dist`, so run `npm run build` in `frontend` first on a fresh clone. |
| Snapshot or Wails methods | The Go tests above, plus `cd frontend && npm run typecheck && npm test -- --run` |
| React UI | `cd frontend && npm run typecheck && npm test -- --run && npm run lint` |
| SQL migration | `go test ./internal/storage` |
| Swift helper or macOS adapter | `swift build -c release --package-path native/macos-capture`, and `swift test` in that directory when Xcode is installed |
| Setup, scripts, or docs that describe commands | `./scripts/verify.sh` |
| Before finishing a cross-component change | `./scripts/verify.sh` |

`go test ./...` from this module also sees `frontend/node_modules` after npm install. `./scripts/verify.sh` excludes that tree. Follow the script.

## Cross-component changes

Before editing across the UI, `app.go`, `internal/service`, capture, or SQLite:

1. Find callers and consumers of the type, method, or table you are changing.
2. Update the contract together: `app.go`, `app_contract.go`, `frontend/src/lib/types.ts`, and both the runtime and demo clients in `frontend/src/lib/api.ts`.
3. Check what is persisted. Vocabulary status, lesson metadata, settings JSON, and outbox rows are not the same lifetime.
4. Add or adjust a test at the boundary you changed. `TestWailsMethodsMatchFrontendContract` already fails when `BACKEND_METHODS` names a method `App` does not have.

## macOS capture

Apply this only when editing `native/macos-capture` or `internal/capture/macos`.

- Change chunk duration in both `platform_source_darwin.go` and `CaptureDefaults.chunkSeconds`, or leave both at 2 seconds. Do not "fix" utterance length in Swift; that policy is in `internal/service/capture_pipeline.go`.
- The helper's commands are `list`, `capture`, and `stop`. `capture` requires `--bundle-id` and `--spool`.
- `swift test` needs full Xcode. Command Line Tools can `swift build`. `./scripts/verify.sh` skips `swift test` when `xctest` is missing and fails when `xctest` is present and the tests fail.

## Working rules

- Preserve unrelated user changes. Do not revert or reformat files you were not asked to change.
- Do not read or write the real Keychain item or `~/Library/Application Support/call_analyzer/call_analyzer.db` unless the user asks.
- Tests and `go run ./cmd/evaluate` without `-execute` must stay offline.

## Completion

A change is complete when the requested behavior is implemented, the checks for that change have been run, and you have inspected the result that those checks can actually see. Report commands, outcomes, and anything you could not run.

Update the relevant document in the table above when you change setup, a contract, a workflow, or architecture. A behavior change that is not obvious from the code belongs in `docs/behavior.md`. A new verified limitation belongs in `docs/limitations.md`.
