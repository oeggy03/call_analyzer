# Limitations

Verified against the current tree. Product boundaries also listed in `README.md` are not repeated unless the code is more specific.

- Google Meet and Teams are disabled in the Live view and labeled "coming later". `validTarget` accepts them, and `ValidateTarget` skips the running-app check for anything other than Zoom. Capture still uses `us.zoom.xos`.
- `oneDay` and `sevenDays` can be stored by `ApplySettings`, and the UI marks them "coming later". No code keeps audio after the session spool is deleted.
- The cost warning compares against 0.35 USD, not against the configured hard budget (`snapshot.go`).
- Study labels `review` and `mastered` are computed in `toVocabularySnapshot`, but no screen calls `RecordReview`.
- The embedded dictionary is the five entries in `NewEmbeddedDictionary`.
- `fixtures/evaluation/manifest.json` is synthetic text. It has no WAV files. The root README says accuracy should be recalibrated with consent-cleared lessons before unattended capture.
- OCR follows the display that contains the active Zoom window (`CaptureDisplaySelector` in the Swift package). There is no user crop. OCR defaults off (`defaultRuntimeSettings`, `platform_source_darwin.go`).
- A native stream failure is shown on the lesson and does not restart capture.
- Outbox rows accumulate. Nothing in the desktop app delivers them. There is no hosted API in this repository.
- Windows is a stub `MockSource`. It does not capture audio.
- `frontend/wailsjs` and `frontend/dist` are generated. They are not the contract. `frontend/src/lib/api.ts` is.
- `go test ./...` compiles `frontend/node_modules/flatted/golang/pkg/flatted` when npm dependencies are installed. `./scripts/verify.sh` excludes it.
- `swift test` does not run on Command Line Tools alone (`no such module 'XCTest'`).

## Unresolved

- No server contract accompanies `internal/sync.HTTPSyncer`. The payload is the JSON form of `domain.OutboxEvent`.
- No importer exists for the CC-CEDICT-sized dictionary mentioned in `README.md`.
- The repository does not say whether `googleMeet` and `teams` should be removed from `validTarget` until a capture backend exists, or left as reserved ids.
