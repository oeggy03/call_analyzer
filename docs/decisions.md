# Decisions

These describe what the code does now. They are not a history of why an alternative was rejected, except where a comment in the code states the reason.

## Desktop UI talks to one Wails snapshot API

The React app calls `window.go.main.App` only from `frontend/src/lib/api.ts`. Methods return a full `AppSnapshot`, except `GenerateManualVocabulary`, which returns `ManualVocabularyDraft`. The demo client implements the same interface so `npm test` does not start Wails.

## Capture is a separate Swift process

`native/macos-capture` is the ScreenCaptureKit, microphone, and Vision helper. Go starts it and reads JSON lines (`internal/capture/macos`). The desktop wiring uses bundle `us.zoom.xos` and 2-second chunks (`platform_source_darwin.go`). Go owns utterance windows (`internal/service/capture_pipeline.go`).

## The API key is not a database setting

`NewDefaultSecretStore` on Darwin checks `OPENROUTER_API_KEY`, then Keychain (`internal/service/secret_darwin.go`). `SaveSettings` writes other preferences as JSON rows. The snapshot exposes only `openRouterKeyConfigured`.

## Model suggestions are not trusted vocabulary

`internal/vocabulary` documents that it produces candidates and that confirmation is separate. `UpsertCandidate` stores `candidate`. `SaveManualVocabulary` stores `confirmed` because the person saved the form.

## Provider calls opt out of retention

`internal/openrouter/client.go` sets `zdr` and `data_collection: deny` on transcription and chat. Local audio lifetime is the session spool, not that provider flag.

## Post-lesson extraction is one chat call

`reconcileLesson` still labels 30-second windows with 5 seconds of overlap, then sends them in one prompt. The comment in `internal/service/reconcile.go` says a one-hour lesson would otherwise need about 144 sequential calls and would not finish when the learner presses Stop.

## Sync is an unused outbox

`internal/sync` can POST pending rows, and storage writes those rows in the same transaction as the mutation. `loadSettings` sets `syncEnabled` to false, and `NewApp` does not start a dispatcher. SQLite stays the source of truth.

## SQLite is embedded and migrated in-process

`internal/storage` uses `modernc.org/sqlite` and applies `migrations/*.sql` from the binary. The next change is a new numbered file. Applied files stay as they are.

## Generated frontend output is not source

`frontend/src` does not import `frontend/wailsjs`. `wails dev` and `wails build` regenerate it. `npm run build` writes `frontend/dist`, which `assets.go` embeds. Both directories are gitignored.
