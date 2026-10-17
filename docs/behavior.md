# Product behavior

These rules are enforced in code, but they are easy to miss because the UI, the service, and the schema do not say the same thing. Cite the linked code before changing them.

## Lessons

- Consent is required both by the disabled Start button in `frontend/src/App.tsx` and by `App.StartLesson`.
- Zoom is the only capture target `macos.Source.Configure` accepts. The Live view disables Google Meet and Teams. `validTarget` still returns true for `googleMeet` and `teams`, and `ValidateTarget` does not check those apps.
- `SaveSettings` returns an error while a lesson is active.
- A failed `StartCapture` deletes the lesson row that was just inserted.
- Startup closes lessons that have no `ended_at` via `LessonRepository.EndOrphaned`.
- A capture failure sets the lesson error and emits `capture.failed`. Nothing restarts the helper.

## Vocabulary

- Automatic extraction always inserts `candidate`. Confirmation is a separate `App.ConfirmCandidate` call.
- Add Word generate does not persist the word. Add Word save writes `confirmed` immediately. That save is the user confirmation.
- Inbox lists status `candidate`. Vocabulary lists status `confirmed`. See `ListCandidates` and `ListVocabulary`.
- The unique key is simplified form + traditional form + reading (`idx_vocabulary_dedup`).
- Evidence text must appear inside the transcript after Chinese normalization (`NormalizeCandidate`).
- If the model supplies no senses, lookup uses the five-entry map in `NewEmbeddedDictionary`. There is no CC-CEDICT import.
- `snapshot.go` labels a confirmed entry `mastered` when study repetitions are at least 5, and `review` when `due_at` is in the past. No Wails method calls `RecordReview`, so ordinary use stays `learning`.

## Budget and privacy

- Default models are the constants `DefaultASRModel` and `DefaultChatModel` in `internal/openrouter/client.go`.
- The hard budget must be greater than 0 and at most 0.50 USD (`ApplySettings`). A non-positive or larger stored value is rewritten to 0.50 on load.
- The UI warning is `cost >= 0.35` in `snapshot.go`. It is not a fraction of the configured budget. A budget below 0.35 can hard-stop without that warning.
- At the hard budget, transcription and analysis stop. Capture and mark timestamps continue (`exhaustSessionBudget`).
- Transcription and chat both send Zero Data Retention and `data_collection: deny`. `Privacy.ZDREnabled` on the snapshot is constant `true`.
- `OPENROUTER_API_KEY` in the process environment is consulted before Keychain. `ClearAPIKey` cannot remove an environment value.
- Settings accept `sessionOnly`, `oneDay`, and `sevenDays`. The UI disables the latter two. No code retains WAV files past session-spool cleanup. The selected value is still copied onto the lesson as `retention_policy`.

## Audio

- Helper chunks are 2 seconds. Go utterance windows are 8–15 seconds, with 2 seconds of overlap and 2 seconds of trailing silence (`capture_pipeline.go`).
- Manual-mark audio comes from the remote/system track only. Microphone frames update the mic meter and can be transcribed, but they are not appended to the mark ring.
- Session WAV directories are created under `os.TempDir()` when no spool parent is configured, and removed on stop. The next capture start deletes leftover directories whose owner pid is not alive (`cleanupStaleSessionSpools`).

## Sync and study data

- `loadSettings` sets `syncEnabled` to false on every load.
- Outbox rows are still written for vocabulary and lesson mutations. They are not delivered.
- Manual vocabulary usage is charged to the stable session id `manual-vocabulary`, not to a lesson.
