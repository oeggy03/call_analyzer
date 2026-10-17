# Operations

This repository ships a local macOS app. There is no hosted service, container, or production database in the tree.

## Build the app

On macOS, after `./scripts/setup.sh`:

```sh
./scripts/build-macos-app.sh
open build/bin/call_analyzer.app
```

The script builds the Swift helper, runs `wails build -clean -platform darwin/arm64`, copies `call-analyzer-capture` into `Contents/MacOS`, and codesigns the helper and the bundle. `APPLE_SIGNING_IDENTITY` defaults to `-` (ad-hoc). The bundle id and usage strings are in `build/darwin/Info.plist`.

`build/bin/` is gitignored.

## Permissions

The first capture request opens macOS privacy controls. Screen & System Audio Recording is required. Microphone access is required only when microphone capture is enabled in Settings. Restart the app after changing those permissions.

The helper's plist is `native/macos-capture/Sources/call-analyzer-capture/Info.plist`.

## Local data

| Data | Location | Notes |
| --- | --- | --- |
| SQLite | `os.UserConfigDir()/call_analyzer/call_analyzer.db` | On macOS, `~/Library/Application Support/call_analyzer/call_analyzer.db`. Opened in `NewApp`. Mode `0600`. |
| API key | Keychain service `github.com/oeggy03.call_analyzer`, account `OPENROUTER_API_KEY` | Not in SQLite. Environment variable wins. |
| Capture audio | A temp directory created by `createSessionSpool` | Removed when the session stops. Leftover directories with a dead owner pid are removed on the next capture start. |
| Settings | `settings` table | JSON values. Keys are the constants in `internal/service/settings.go`. |
| Model spend | `request_usage` | Per lesson id, plus `manual-vocabulary` for Add Word generation. |

Do not copy the SQLite file to another machine as a sync mechanism. `README.md` says a future hosted API should consume the outbox, and that the desktop database must not be synchronized directly. The outbox dispatcher is not started.

## What is not deployed

- No remote API, PostgreSQL service, or sync worker is included.
- `internal/sync.HTTPSyncer` posts JSON events to an endpoint the desktop app never configures.
- Windows capture is the `capture.Source` interface only. `newPlatformSource` returns `MockSource` off Darwin.
