# Product behavior

These rules are enforced in code, but they are easy to miss because the UI, the service, and the schema do not say the same thing. Cite the linked code before changing them.

## Lessons

- Consent is required both by the disabled Start button in `frontend/src/App.tsx` and by `App.StartLesson`.
- The Live view requests target `system`. On macOS this captures system audio across apps; Zoom does not need to be running and audio is not isolated to one app. Microphone capture defaults on but can be turned off.
- `SaveSettings` returns an error while a lesson is active.
- A failed `StartCapture` deletes the lesson row that was just inserted.
- Startup closes lessons that have no `ended_at` via `LessonRepository.EndOrphaned`.
- A capture failure sets the lesson error and emits `capture.failed`. Nothing restarts the helper.

## Vocabulary

- New automatic suggestions start as `candidate`; when a suggestion matches an existing word, its current status is preserved. Confirmation is a separate `App.ConfirmCandidate` call.
- Add Word generation returns a non-persistent draft and is disabled during capture. Saving a complete manual entry writes `confirmed` without a model request and remains available during capture.
- Photo recognition returns a draft and records usage under `photo-vocabulary`. Reading and saving are available during capture. The UI queues up to eight photos, scales them to at most 1280 pixels on the longest edge, and encodes them as JPEG. The backend accepts JPEGs up to 1.5 MiB and at most 12 suggestions per image.
- Inbox lists status `candidate`. Vocabulary lists status `confirmed`. See `ListCandidates` and `ListVocabulary`.
- The unique key is simplified form + traditional form + reading (`idx_vocabulary_dedup`).
- Evidence text must appear inside the transcript after Chinese normalization (`NormalizeCandidate`).
- If the model supplies no senses, lookup uses the five-entry map in `NewEmbeddedDictionary`. There is no CC-CEDICT import.
- `snapshot.go` labels a confirmed entry `mastered` when study repetitions are at least 5, and `review` when `due_at` is in the past. No Wails method calls `RecordReview`, so normal use leaves saved entries in `learning`.

## Budget and privacy

- Defaults are `qwen/qwen3-asr-1.7b` for transcription, `qwen/qwen3-32b` for analysis, and `qwen/qwen3.7-flash` for photo recognition (`internal/openrouter/client.go`).
- The legacy `hardBudgetUsd` setting defaults to 0.50 USD and remains validated as greater than 0 and at most 0.50. Invalid stored values reset to 0.50 on load. This is not a runtime spending cap: `Budget.Check` returns no error and `HardExceeded` is always false.
- The UI warning is `cost >= 0.35` in `snapshot.go`, independent of the configured budget. OpenRouter requests continue past either threshold.
- Lesson transcription, lesson analysis, and Add Word generation send Zero Data Retention and `data_collection: deny`. Photo vision sends neither flag. `Privacy.ZDREnabled` is constant `true` and does not describe photo requests.
- `OPENROUTER_API_KEY` in the process environment is consulted before Keychain. `ClearAPIKey` cannot remove an environment value.
- Settings accept `sessionOnly`, `oneDay`, and `sevenDays`. The UI disables the latter two. The selected value is copied onto the lesson as `retention_policy`, but WAV files are removed as they are read and the spool is removed when capture stops for every setting.

## Audio

- Helper chunks are 2 seconds at 16 kHz mono. Go assembles up-to-15-second audio windows, usually requiring eight seconds of buffered audio before a silence-triggered flush. Stopping can flush a shorter final window. Adjacent windows overlap by 2 seconds and use 2 seconds of trailing silence (`capture_pipeline.go`).
- Manual-mark audio comes from the remote/system track only. Microphone frames update the mic meter and can be transcribed, but they are not appended to the mark ring.
- Blank lesson tags become `lesson_YYYY-MM-DD_HH-MM` in local time. Lesson tags are deduplicated and limited to eight tags of 64 characters each. Manual vocabulary accepts up to 32 tags of 64 characters; photo recognition returns no tags.
- Session WAV directories default to `os.TempDir()` and are removed on stop. Stale directories are cleaned when the source is initialized and before a new session spool is created if the recorded owner process is no longer running (`cleanupStaleSessionSpools`).

## Sync and study data

- `loadSettings` sets `syncEnabled` to false on every load.
- Outbox rows are still written for vocabulary and lesson mutations. They are not delivered.
- Lesson, manual generation, and photo recognition usage are recorded separately under the lesson id, `manual-vocabulary`, and `photo-vocabulary` respectively.
