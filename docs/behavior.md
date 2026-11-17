# Product behavior

These rules are enforced in code, but they are easy to miss because the UI, the service, and the schema do not say the same thing. Cite the linked code before changing them.

## Lessons

- Consent is required both by the disabled Start button in `frontend/src/App.tsx` and by `App.StartLesson`. The UI Start button also requires `openRouterKeyConfigured` and `capturePermission === 'granted'` (**Check setup** / `App.RequestCapturePermission`), while `App.StartLesson` re-checks consent, target, API key, and `ValidateTarget`. Cloud vocabulary access (`supabaseAccessStatus`) is shown in Startup checks—with an inline Keychain key form when `missing` or `invalid`—without blocking lesson capture when cloud sync is unconfigured.
- The Live view requests target `system`. On macOS this captures system audio across apps; Zoom does not need to be running and audio is not isolated to one app. Microphone capture defaults on but can be turned off.
- `SaveSettings` returns an error while a lesson is active.
- A failed `StartCapture` deletes the lesson row that was just inserted.
- Startup closes lessons that have no `ended_at` via `LessonRepository.EndOrphaned`.
- A capture failure sets the lesson error and emits `capture.failed`. Nothing restarts the helper.
- `App.DeleteLastLesson` is disabled during an active lesson and deletes the latest completed lesson (`EndedAt != nil`). Its transaction purges pending `vocabulary_entry` outbox rows for entries observed in that lesson so deleted transcript evidence does not linger in outbox payloads, deletes `candidate` and `rejected` entries that have no remaining observations (queueing `vocabulary.entry.deleted`), and preserves `confirmed` entries while queueing an evidence-free `vocabulary.entry.evidence_redacted` event.

## Vocabulary

- New automatic suggestions start as `candidate`; when a suggestion matches an existing word, its current status is preserved. Confirmation is a separate `App.ConfirmCandidate` call. Newly created automatic `candidate` rows receive the lesson's normalized `LessonTags`.
- Pinyin normalization (`CanonicalPinyin` in `internal/vocabulary/vocabulary.go`) accepts tone marks, tone numbers `1`–`5` (`u:` or `v` for `ü`), and joined syllables (`segmentSyllable` splits e.g. `wǒmen` into `wǒ men` / `wo3 men5` by longest valid prefix).
- Add Word generation returns a non-persistent draft and is disabled during capture. The draft can include up to four variants (`internal/service/manual_vocabulary.go`). When more than one reading or meaning is still possible, `needsChoice` is true and the Add Word form stays as typed until the user picks a variant. Returned variants stay listed above the form actions, including after a typed detail selects one of them. Clear form empties that manual form and its choices; photo suggestions stay. Replacing the simplified word does the same for the other fields and the previous choices, so the next word is not generated from the previous card. A typed meaning selects a variant when it uniquely matches that gloss: exact text, or a containment where the shorter side is at least four characters. A typed pinyin selects a variant only when that reading is unique among the returned variants. If the model marks the result `selected` and the form already has a meaning, example, example translation, part of speech, classifier, or tag, that entry fills the remaining fields. Non-empty typed fields are kept. Photo recognition and lesson suggestions are unchanged.
- Saving a complete manual entry writes `confirmed` without a model request and remains available during capture. Saving over an existing `(simplified, traditional, reading)` key restores a `candidate` or `rejected` row to `confirmed`, rejects restoring a `merged` row, and skips writing a duplicate outbox event when an existing `confirmed` row already has identical fields, senses, examples, and tags (`manualEntriesEqual`).
- Photo recognition returns a draft and records usage under `photo-vocabulary`. Reading and saving are available during capture. The UI queues up to eight photos (chosen, dropped, or pasted), scales them to at most 1280 pixels on the longest edge, and encodes them as JPEG. The backend accepts JPEGs up to 1.5 MiB and at most 12 suggestions per image.
- Inbox lists status `candidate`. Vocabulary lists status `confirmed`, paginated at 9 words per page with search, status, and tag filters. See `ListCandidates` and `ListVocabulary`.
- Deleting a word from Vocabulary asks inside an in-app confirmation modal before calling `App.DeleteVocabulary`, which removes the confirmed word and its senses, examples, evidence, tags, and study rows. The same transaction drops that word's pending outbox rows and writes `vocabulary.entry.deleted`.
- The unique key is simplified form + traditional form + reading (`idx_vocabulary_dedup`).
- Evidence text must appear inside the transcript after Chinese normalization (`NormalizeCandidate`).
- Lesson candidate extraction uses the five-entry map in `NewEmbeddedDictionary` when the model supplies no senses. There is no CC-CEDICT import.
- `snapshot.go` labels a confirmed entry `mastered` when study repetitions are at least 5, and `review` when `due_at` is in the past. No Wails method calls `RecordReview`, so normal use leaves saved entries in `learning`.

### Highlighted-word lookup

- While the app process is running, `Command-Option-5` asks the separate
  `call-analyzer-lookup` helper to read the highlighted text through the
  macOS Accessibility API. It checks SQLite first for exact simplified or
  traditional matches with status `confirmed` or `candidate`; the five-entry
  embedded dictionary is not used. Saved readings appear as separate rows,
  with at most three rows, and a candidate is hidden when a confirmed row has
  the same reading.
- When the local query returns no rows and the session cache misses,
  `google/gemini-3.1-flash-lite` receives the word and, when Accessibility
  provides it, a short surrounding sentence (`±40` UTF-16 code units in
  `LookupAgent.swift`, trimmed to at most 80 runes in `normalizeSelectionContext`
  and omitted when identical to the selection or lacking Han text). The request
  allows 400 tokens (`selectionLookupMaxTokens`), sets temperature `0` and a
  12-second timeout, disables reasoning, and uses the same Zero Data Retention
  and data-collection-denial controls as other text chat. Successful glosses are
  cached in memory for the app session, up to 64 entries, and usage is stored
  under `selection-lookup`. The surrounding sentence is not shown in the
  panel. The selected text is not logged or added to `AppSnapshot`.
- The panel shows pinyin and a short English gloss. It stays open until Close is pressed. A local entry that already has at least one example shows **Details**, expanding a read-only card in the panel; a model lookup row or an example-free local row shows **Add word**, which generates the example and the other card fields in the panel (including during a live lesson), and **Save** writes the confirmed entry from there. The main window is not brought forward. Highlights without Han text show a notice and do not call the model. A phrase or sentence is allowed.
- Tooltip generation uses the same manual-vocabulary model, parser, and `manual-vocabulary` usage session ID as Add Word (`qwen/qwen3-32b` by default). The placeholder `No English gloss saved` is left blank on the card. Saved tag names can be toggled on the tooltip card and on the Add Word, photo review, and candidate edit forms. `ListTags` returns those names.

## Budget and privacy

- Defaults are `qwen/qwen3-asr-1.7b` for transcription, `qwen/qwen3-32b` for analysis, `qwen/qwen3.7-flash` for photo recognition (`qwen/qwen3-vl-30b-a3b-instruct` is the other allowed vision setting), and `google/gemini-3.1-flash-lite` for highlighted-word lookup (`internal/openrouter/client.go`, `internal/service/settings.go`). Legacy non-ZDR analyzer model settings (`qwen/qwen3.8-flash`, `qwen/qwen3.8-max-0902`) reset to `qwen/qwen3-32b` on load.
- The legacy `hardBudgetUsd` setting defaults to 0.50 USD and remains validated as greater than 0 and at most 0.50. Invalid stored values reset to 0.50 on load. This is not a runtime spending cap: `Budget.Check` returns no error and `HardExceeded` is always false.
- The UI warning is `cost >= 0.35` in `snapshot.go`, independent of the configured budget. OpenRouter requests continue past either threshold.
- Lesson transcription, lesson analysis, Add Word generation, and highlighted-word lookup/card generation send Zero Data Retention and `data_collection: deny`. Photo vision sends neither flag. `Privacy.ZDREnabled` is constant `true` and does not describe photo requests.
- `OPENROUTER_API_KEY` in the process environment is consulted before Keychain. `ClearAPIKey` cannot remove an environment value.
- Settings accept `sessionOnly`, `oneDay`, and `sevenDays`. The UI disables the latter two. The selected value is copied onto the lesson as `retention_policy`, but WAV files are removed as they are read and the spool is removed when capture stops for every setting.

## Audio

- Helper chunks are 2 seconds at 16 kHz mono. Go assembles up-to-15-second audio windows, usually requiring eight seconds of buffered audio before a silence-triggered flush. Stopping can flush a shorter final window. Adjacent windows overlap by 2 seconds and use 2 seconds of trailing silence (`capture_pipeline.go`).
- Manual-mark audio comes from the remote/system track only. Microphone frames update the mic meter and can be transcribed, but they are not appended to the mark ring.
- Blank lesson tags become `lesson_YYYY-MM-DD_HH-MM` in local time. Lesson tags are deduplicated and limited to eight tags of 64 characters each. Manual vocabulary accepts up to 32 tags of 64 characters; photo recognition returns no tags.
- Session WAV directories default to `os.TempDir()` and are removed on stop. Stale directories are cleaned when the source is initialized and before a new session spool is created if the recorded owner process is no longer running (`cleanupStaleSessionSpools`).

## Sync and study data

- `loadSettings` sets `syncEnabled` to false on every load.
- Vocabulary outbox rows are delivered by a cross-platform background runner
  after a ten-minute quiet period from the newest undelivered eligible row.
  On launch the runner reads confirmed cloud vocabulary in paged 1000-row
  requests (up to 20,000 rows) and applies it locally. A newer local `updated_at` is kept. The same
  simplified, traditional, and reading under a different id keeps the local
  id. A confirmed local word with no pending outbox row is removed when that
  id is gone from the cloud. A pending `vocabulary.entry.deleted` row keeps
  that word out of the launch pull, including a cloud row that uses another
  id for the same simplified, traditional, and reading. `SyncNow` uploads only and does not pull.
  Startup checks probe that the saved Supabase secret can read vocabulary and
  write a tag, then delete that tag. A missing or rejected key asks for a
  replacement. On macOS the replacement is stored in the Keychain. An
  environment value still wins over the Keychain.
  Closing the window asks "You still have unsynced items. Sync now?" when a flush would upload a row. Sync now sends those rows and then closes. Not now closes and leaves them in the outbox.
- The runner collapses events by vocabulary id and reads the current SQLite
  row. Confirmed and manually saved rows are uploaded. A merged candidate is
  not uploaded; a confirmed merge target is uploaded with its moved senses,
  examples, and tags. Candidate and rejected
  rows are acknowledged locally without HTTP. A missing local row sends an
  upstream DELETE by id. A `vocabulary.entry.deleted` event sends that DELETE
  and, when the payload includes simplified, traditional, and reading, a
  second DELETE for that key.
- Child senses, examples, and entry tags are replaced from SQLite, while tags
  are upserted by name. The first confirmed/manual publish initializes
  `study_state` with ignore-duplicates resolution.
- `study.review.recorded` remains pending and does not block the vocabulary
  quiet timer. Lessons, transcripts, observations, and later study-state
  changes are not synced.
- Lesson, manual/tooltip card generation, photo recognition, and highlighted-word lookup usage are recorded separately under the lesson id, `manual-vocabulary`, `photo-vocabulary`, and `selection-lookup` respectively.
