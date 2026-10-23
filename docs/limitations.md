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
- Outbox rows accumulate. Nothing in the desktop app delivers them. There is no hosted API in this repository.
- Windows uses a stub `MockSource` and does not capture audio. On Windows, `go test ./internal/storage` currently fails to open embedded migrations 001, 003, and 004 because the migration path uses Windows separators; run complete Go verification on macOS.
- `frontend/wailsjs` and `frontend/dist` are generated. They are not the contract. `frontend/src/lib/api.ts` is.
- `go test ./...` compiles `frontend/node_modules/flatted/golang/pkg/flatted` when npm dependencies are installed. `./scripts/verify.sh` excludes it.
- `swift test` does not run on Command Line Tools alone (`no such module 'XCTest'`).
- Add Word variant choice depends on the model returning distinct senses. There is no local dictionary lookup for Add Word. If the model returns only one variant, the form fills immediately.

## Unresolved

- No server contract accompanies `internal/sync.HTTPSyncer`. The payload is the JSON form of `domain.OutboxEvent`.
- No importer exists for the CC-CEDICT-sized dictionary mentioned in `README.md`.
