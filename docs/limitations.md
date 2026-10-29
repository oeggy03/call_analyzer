# Limitations

Verified against the current tree. Product boundaries also listed in `README.md` are not repeated unless the code is more specific.

- The Live view captures system audio across apps and does not isolate audio to one selected application.
- `oneDay` and `sevenDays` are disabled in the UI. Although settings code accepts them, audio is still removed when capture stops.
- The `hardBudgetUsd` setting remains range-validated, but it is not a runtime spending cap. Requests continue past the configured value; the warning threshold is US$0.35.
- Photo vision requests do not include ZDR or data-collection-denial routing flags.
- Study labels `review` and `mastered` are computed in `toVocabularySnapshot`, but no Wails method or screen calls `RecordReview`.
- The embedded dictionary is the five entries in `NewEmbeddedDictionary`.
- `fixtures/evaluation/manifest.json` is synthetic text. It has no WAV files. The root README says accuracy should be recalibrated with consent-cleared lessons before unattended capture.
- OCR is off by default. In system mode the helper captures screen frames from the largest display; there is no user-selected display or crop (`defaultRuntimeSettings`).
- A native stream failure is shown on the lesson and does not restart capture.
- Highlighted-word lookup needs the Call Analyzer process running; closing the
  app window quits it and stops the `Command-Option-5` helper. There is no
  menu-bar-only mode.
- Lookup reads selected text through Accessibility, not screen pixels. A selection without Han text is refused. There is no character-count cap on the highlight.
- Vocabulary outbox delivery requires a configured per-machine Supabase
  secret. Without it, rows remain local. The runner does not sync lessons,
  transcripts, observations, study-state changes, or review events.
- Launch pulls confirmed vocabulary and keeps the row with the newer
  `updated_at`. There is no field-by-field conflict screen. `SyncNow` uploads
  the current SQLite row and can replace an older cloud copy.
- macOS stores the Supabase secret in this Mac's Keychain when the environment
  variable is absent. Windows and other non-Darwin builds have no Keychain and
  must use their own `SUPABASE_SECRET_KEY`; the Mac value is not portable.
- Windows uses a stub `MockSource` and does not capture audio. On Windows, `go test ./internal/storage` currently fails to open embedded migrations 001, 003, and 004 because the migration path uses Windows separators; run complete Go verification on macOS.
- `frontend/wailsjs` and `frontend/dist` are generated. They are not the contract. `frontend/src/lib/api.ts` is.
- `go test ./...` compiles `frontend/node_modules/flatted/golang/pkg/flatted` when npm dependencies are installed. `./scripts/verify.sh` excludes it.
- `swift test` does not run on Command Line Tools alone (`no such module 'XCTest'`).
- Add Word variant choice depends on the model returning distinct senses. There is no local dictionary lookup for Add Word. If the model returns only one variant, the form fills immediately.

## Unresolved

- The legacy `internal/sync.HTTPSyncer` remains a generic event POST helper
  without a server contract; the active vocabulary runner uses the explicit
  PostgREST table contract instead.
- No importer exists for the CC-CEDICT-sized dictionary mentioned in `README.md`.
