# Decisions

These describe what the code does now. They are not a history of why an alternative was rejected, except where a comment in the code states the reason.

## Desktop UI talks to one Wails snapshot API

The React app calls `window.go.main.App` only from `frontend/src/lib/api.ts`. Most methods return `AppSnapshot`; `GenerateManualVocabulary`, `RecognizeVocabularyPhoto`, `ListLessonTranscripts`, and `GetLessonTranscript` return dedicated draft or transcript-history types. The demo client implements the same interface so UI tests do not start Wails.

## Capture is a separate Swift process

`native/macos-capture` is the ScreenCaptureKit, microphone, and Vision helper. Go starts it and reads JSON lines (`internal/capture/macos`). The Live view requests system audio across apps; Zoom does not need to be running. The helper writes 2-second 16 kHz mono chunks, and Go owns utterance windows (`internal/service/capture_pipeline.go`).

## The API key is not a database setting

`NewDefaultSecretStore` on Darwin checks `OPENROUTER_API_KEY`, then Keychain (`internal/service/secret_darwin.go`). `SaveSettings` writes other preferences as JSON rows. The snapshot exposes only `openRouterKeyConfigured`.

## Model suggestions are not trusted vocabulary

New automatic suggestions start as `candidate`, and confirmation is separate. When an automatic suggestion matches an existing word, `UpsertCandidate` preserves that row's current status. `SaveManualVocabulary` stores `confirmed` because the person saved the form.

## Provider privacy flags vary by request

`internal/openrouter/client.go` requests `zdr` and `data_collection: deny` for lesson transcription, lesson analysis, and Add Word generation. Photo vision calls do not include those flags. Local audio lifetime is the session spool, separate from provider routing.

## Cost display has no runtime cap

The UI warns at US$0.35 and labels the session as uncapped. The legacy `hardBudgetUsd` setting remains range-validated from greater than zero through US$0.50, but `Budget.Check` does not block requests and `HardExceeded` remains false.

## Post-lesson extraction is one chat call

`reconcileLesson` still labels 30-second windows with 5 seconds of overlap, then sends them in one prompt. The comment in `internal/service/reconcile.go` says a one-hour lesson would otherwise need about 144 sequential calls and would not finish when the learner presses Stop.

## Vocabulary sync consumes the outbox

Storage writes vocabulary outbox rows in the same transaction as the
mutation. `Service.Initialize` starts the cross-platform vocabulary runner,
which waits ten minutes, reads current SQLite rows, and sends eligible
confirmed and manually saved entries through PostgREST. A merged candidate stays local unless its target is confirmed, in which case that target is republished. `study.review.recorded`
and non-vocabulary data remain local to this boundary. `loadSettings` still
sets the legacy `syncEnabled` flag to false, and `sync.Dispatcher` remains
unused. SQLite stays the source of truth.

## SQLite is embedded and migrated in-process

`internal/storage` uses `modernc.org/sqlite` and applies `migrations/*.sql` from the binary. The next change is a new numbered file. Applied files stay as they are.

## Generated frontend output is not source

`frontend/src` does not import `frontend/wailsjs`. `wails dev` and `wails build` regenerate the Wails bindings. `npm run build` writes generated `frontend/dist` output, which `assets.go` embeds. Treat both as generated build output.
