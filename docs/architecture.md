# Architecture

Package roles are listed in `README.md`. This file is the boundary and flow map. Paths below are the source of truth; do not copy signatures out of them.

## Boundaries

| Boundary | Owner | Crosses into | Contract |
| --- | --- | --- | --- |
| Desktop UI | `frontend/src/App.tsx` | Wails only through `frontend/src/lib/api.ts` | `BACKEND_METHODS`, `BACKEND_EVENTS`, and `AppSnapshot` in `frontend/src/lib/types.ts` |
| Wails app | `app.go`, `app_contract.go`, `snapshot.go` | `internal/service`, `internal/selection` | Exported `App` methods. Input and output structs are in `app_contract.go`. |
| Lesson service | `internal/service/service.go`, `capture_pipeline.go` | capture, OpenRouter, storage, vocabulary, sync | `capture.Source`, `openrouter.Client`, `storage.Store` |
| Capture | `internal/capture/capture.go` | macOS adapter on Darwin, `MockSource` otherwise | `Source`, plus optional `ConfigurableSource`, `EventSource`, `PermissionSource`, `TargetSource` |
| Helper process | `internal/capture/macos` | `native/macos-capture` | JSON-line events. Commands are in `native/macos-capture/Sources/call-analyzer-capture/CLI.swift`. |
| Selection lookup | `internal/selection`, `internal/service/selection_lookup.go` | `native/macos-lookup`, Wails runtime events | JSON-line `lookup`/`add`/`save` requests (`result`/`draft`/`saved`/`error` replies); saves emit `vocabulary.changed` → `call_analyzer:snapshot_changed` (`call_analyzer:selection.add-word` remains wired as a legacy listener) |
| Model HTTP | `internal/openrouter/client.go` | OpenRouter | Audio transcription, text chat, and vision chat |
| Database | `internal/storage/store.go` | embedded SQL | `internal/storage/migrations/*.sql`, applied in filename order |
| Vocabulary rules | `internal/vocabulary/vocabulary.go` | used by `internal/service/vocabulary.go` | `NormalizeCandidate`, `CanonicalPinyin`, `ExtractionSchema` |
| Sync | `internal/sync/runner.go`, `postgrest.go` | service, storage, Supabase PostgREST | `Runner` is started by `Service.Initialize`; the legacy `Dispatcher` and `SyncEnabled` flag remain unused |

`assets.go` embeds `frontend/dist` for the packaged app. Vite writes that directory. It is not hand-edited source.

Domain structs in `internal/domain/domain.go` are the storage and service shapes. `AppSnapshot` in `app_contract.go` is the main UI shape. `snapshot.go` maps domain records into it. Five methods return separate result types: `GenerateManualVocabulary` (`ManualVocabularyDraft`), `RecognizeVocabularyPhoto` (`PhotoVocabularyDraft`), `ListLessonTranscripts` (`[]LessonTranscriptSummary`), `GetLessonTranscript` (`LessonTranscript`), and `ListTags` (`[]string`).

## Startup and shutdown

- `App.startup` initializes `Service` (which closes orphaned lessons and starts `sync.Runner`), starts the optional `selection.Runner` on macOS, emits `backend.ready`, and launches `checkSupabaseAccess` in the background (`PostgRESTClient.ProbeReadWrite`).
- `App.RequestCapturePermission` (**Check setup** on Live Lesson) runs both `checkReadiness` and `checkSupabaseAccess`, updating `capturePermission`, `readiness`, and `supabaseAccessStatus`/`supabaseAccessDetail`. `App.SaveSupabaseKey` writes a replacement cloud key to the app's sync secret store and re-runs `CheckSupabaseAccess`.
- `App.beforeClose` checks `Service.HasUnsyncedVocabulary`. When an uploadable row is pending, it prompts `"You still have unsynced items. Sync now?"`: **Sync now** runs `Service.SyncNow` (blocking close if upload fails), while **Not now** marks `SkipShutdownSync` so `App.shutdown` stops the runner via `StopWithoutFlush`.

## Start a lesson

1. `App.StartLesson` in `app.go` checks consent, target, API key, and `ValidateTarget`, and normalizes comma-separated lesson tags (`service.NormalizeLessonTags`, defaulting to `lesson_YYYY-MM-DD_HH-MM` in local time).
2. `Service.StartLessonWithMetadata` inserts a `lessons` row.
3. `Service.StartCapture` configures microphone and OCR from settings, then `Source.Start`.
4. The Live view requests target `system`. On Darwin, ScreenCaptureKit captures system audio across apps; Zoom does not need to be running and the audio is not isolated to one selected app.
5. The adapter starts `call-analyzer-capture`. Resolution order is in `resolveHelperPath`.

`ValidateTarget` performs the capture readiness check. For the system target it does not require a specific running app. `macos.Source.Configure` passes the system target to the helper.

## Look up a highlighted word

1. After service initialization, `App.startup` starts the optional selection
   runner. `call-analyzer-lookup` registers `Command-Option-5`, reads the
   focused app's selected text, optional surrounding context, and bounds
   through Accessibility, then sends a JSON-line lookup request.
   A missing helper does not block startup, and `App.shutdown` stops the
   runner.
2. `Service.LookupSelection` validates the surface, checks exact local
   confirmed/candidate vocabulary and its session cache, and calls the
   selection model only when the local query returns no rows and the cache
   misses. The helper renders at most three pinyin/gloss rows.
3. Choosing **Add word** sends a JSON-line add request. `Service.AddSelection`
   generates a manual-vocabulary draft, including during a live lesson, and
   the helper shows that card. **Save** calls `Service.SaveSelection`.
   **Details** expands a saved card in the panel. The main window stays where it is.

## Live audio to a candidate

1. The helper writes WAV chunks and emits JSON events. The adapter reads a chunk and deletes the file.
2. `captureSession.handleFrame` levels every frame. Only non-microphone frames go into the ring buffer.
3. `capture_pipeline.go` cuts utterance windows and queues transcription.
4. `openrouter.Client.Transcribe` posts the WAV. Overlap text is reconciled before insert.
5. About every 20 seconds of new transcript, the analyzer returns JSON matching `vocabulary.ExtractionSchema`. Screen OCR events (`system/ocr`) attach up to 90 seconds of recent speech context (`recentSpeechContext`) and run extraction immediately.
6. `VocabularyService.ProcessExtraction` drops candidates whose evidence is not in the transcript, then stores new suggestions as `candidate`. A match to an existing word keeps that row's status. `tagAutomaticEntries` attaches the lesson's `LessonTags` to newly stored `candidate` rows.

Mark Moment is `App.MarkMoment` → `Service.MarkMoment`. It inserts a `system/mark` transcript row and queues up to 25 seconds of pre-roll and 10 seconds of post-roll from the system-audio ring buffer. Microphone frames are not added to that ring buffer. The constants are `DefaultPreRoll` and `DefaultPostRoll` in `internal/audio/ring.go`.

## Add Word

`App.GenerateManualVocabulary` calls the analyzer and records usage under `ManualVocabularySessionID`. It returns a draft and does not insert vocabulary. The draft adds `needsChoice`, `matchedContext`, `appliedVariant`, and `variants`. The UI leaves the form unchanged when `needsChoice` is true and shows those variants for selection. The UI disables generation during a live lesson. `App.SaveManualVocabulary` persists a confirmed entry, and the UI permits saving a complete manual form during capture. Photo recognition returns a draft and records usage under `PhotoVocabularySessionID`; reading and saving photos also remain available during capture.

Lesson transcription, lesson analysis, Add Word generation, and selection lookup/card generation (`Transcribe` and `ChatJSON`) request ZDR routing and deny data collection. Photo vision requests (`ChatVisionJSON`) do not include those routing fields. Photo, manual generation, and selection lookup usage are recorded separately from lesson cost.

## Stop a lesson

`App.StopLesson` → `Service.StopAndEndCurrentLesson`. That stops the helper, waits for queued analysis, then `reconcileLesson` sends one chat request covering the stored transcript when possible. The lesson row is closed with its final recorded cost. There is no enforced runtime hard-budget stop.

## Database

`storage.Open` runs embedded migrations once and records them in `schema_migrations`. Repositories live next to `store.go` (`lessons.go`, `vocabulary.go`, `outbox.go`, `study.go`). Supported vocabulary and lesson mutations write outbox rows in their transaction. `App.DeleteVocabulary` removes a confirmed word and writes `vocabulary.entry.deleted` in that transaction. The runner deletes the hosted row by id and by the Chinese key in that payload. Launch pull skips a word while its delete is still pending. The vocabulary runner consumes only undelivered `vocabulary_entry` events after a ten-minute quiet period, collapses each aggregate to one operation, and reads the current SQLite row. It acknowledges candidate, rejected, and merged-source rows locally, uploads confirmed and manually saved rows through PostgREST, republishes a confirmed merge target, and leaves `study.review.recorded` pending. It does not sync lessons, transcripts, observations, or study-state updates. Launch pulls confirmed vocabulary before the upload loop. `SyncNow` uploads only. Closing asks before uploading pending vocabulary, and shutdown flushes only when that prompt was accepted or there was nothing to upload. The legacy `SyncEnabled` flag and `sync.Dispatcher` remain unused.

The legacy `hardBudgetUsd` setting remains validated as greater than zero and no more than US$0.50. `Budget.Check` does not block requests and the snapshot always reports `hardExceeded: false`; the Settings UI describes the session as uncapped.
