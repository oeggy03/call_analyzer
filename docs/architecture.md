# Architecture

Package roles are listed in `README.md`. This file is the boundary and flow map. Paths below are the source of truth; do not copy signatures out of them.

## Boundaries

| Boundary | Owner | Crosses into | Contract |
| --- | --- | --- | --- |
| Desktop UI | `frontend/src/App.tsx` | Wails only through `frontend/src/lib/api.ts` | `BACKEND_METHODS` and `AppSnapshot` in `frontend/src/lib/types.ts` |
| Wails app | `app.go`, `snapshot.go` | `internal/service` | Exported `App` methods. Input structs are in `app_contract.go`. |
| Lesson service | `internal/service/service.go`, `capture_pipeline.go` | capture, OpenRouter, storage, vocabulary | `capture.Source`, `openrouter.Client`, `storage.Store` |
| Capture | `internal/capture/capture.go` | macOS adapter on Darwin, `MockSource` otherwise | `Source`, plus optional `ConfigurableSource`, `EventSource`, `TargetSource` |
| Helper process | `internal/capture/macos` | `native/macos-capture` | JSON-line events. Commands are in `native/macos-capture/Sources/call-analyzer-capture/CLI.swift`. |
| Model HTTP | `internal/openrouter/client.go` | OpenRouter | `Transcribe`, `ChatJSON` |
| Database | `internal/storage/store.go` | embedded SQL | `internal/storage/migrations/*.sql`, applied in filename order |
| Vocabulary rules | `internal/vocabulary/vocabulary.go` | used by `internal/service/vocabulary.go` | `NormalizeCandidate`, `ExtractionSchema` |
| Sync | `internal/sync/sync.go` | nothing at runtime | `Dispatcher` exists; the app does not construct it |

`assets.go` embeds `frontend/dist` for the packaged app. Vite writes that directory. It is not hand-edited source.

Domain structs in `internal/domain/domain.go` are the storage and service shapes. `AppSnapshot` in `app_contract.go` is the UI shape. `snapshot.go` maps one to the other.

## Start a lesson

1. `App.StartLesson` in `app.go` checks consent, target, API key, and `ValidateTarget`.
2. `Service.StartLessonWithMetadata` inserts a `lessons` row.
3. `Service.StartCapture` configures microphone and OCR from settings, then `Source.Start`.
4. On Darwin, `platform_source_darwin.go` builds `macos.Source` for bundle `us.zoom.xos`.
5. The adapter starts `call-analyzer-capture`. Resolution order is in `resolveHelperPath`.

`ValidateTarget` only checks that Zoom is shareable. `Source.Configure` rejects any other target id.

## Live audio to a candidate

1. The helper writes WAV chunks and emits JSON events. The adapter reads a chunk and deletes the file.
2. `captureSession.handleFrame` levels every frame. Only non-microphone frames go into the ring buffer.
3. `capture_pipeline.go` cuts utterance windows and queues transcription.
4. `openrouter.Client.Transcribe` posts the WAV. Overlap text is reconciled before insert.
5. About every 20 seconds of new transcript, the analyzer returns JSON matching `vocabulary.ExtractionSchema`.
6. `VocabularyService.ProcessExtraction` drops candidates whose evidence is not in the transcript, then `UpsertCandidate` stores status `candidate`.

Mark Moment is `App.MarkMoment` → `Service.MarkMoment`. It inserts a `system/mark` transcript row and queues the ring-buffer window immediately. Pre-roll and post-roll constants are `DefaultPreRoll` and `DefaultPostRoll` in `internal/audio/ring.go`.

## Add Word

`App.GenerateManualVocabulary` calls the analyzer and records usage under `ManualVocabularySessionID`. It does not insert vocabulary. `App.SaveManualVocabulary` persists a confirmed entry. Both refuse to run during an active lesson.

## Stop a lesson

`App.StopLesson` → `Service.StopAndEndCurrentLesson`. That stops the helper, waits for queued analysis, then `reconcileLesson` sends one chat request covering the stored transcript. The window size used to build that prompt is in `internal/service/reconcile.go`. The lesson row is closed with its final cost.

## Database

`storage.Open` runs embedded migrations once and records them in `schema_migrations`. Repositories live next to `store.go` (`lessons.go`, `vocabulary.go`, `outbox.go`, `study.go`). Vocabulary confirm, reject, merge, manual save, lesson delete, and study review insert `outbox` rows inside the same transaction. No app startup path calls `sync.Dispatcher.Dispatch`.
