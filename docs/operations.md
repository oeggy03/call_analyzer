# Operations

This repository ships a local macOS app. There is no hosted service, container, or production database in the tree.

## Build the app

On macOS, after `./scripts/setup.sh`:

```sh
./scripts/build-macos-app.sh
open build/bin/call_analyzer.app
```

The script builds both Swift helpers, runs `wails build -clean -platform darwin/arm64`, copies `call-analyzer-capture` and `call-analyzer-lookup` into `Contents/MacOS` next to each other, and codesigns both nested helpers and the bundle. `APPLE_SIGNING_IDENTITY` defaults to `-` (ad-hoc). The bundle id and usage strings are in `build/darwin/Info.plist`.

`build/bin/` is gitignored.

## Permissions

The first capture request opens macOS privacy controls. Screen & System Audio Recording is required. Microphone access is required only when microphone capture is enabled in Settings. Restart the app after changing those permissions.

Selection lookup requires Accessibility permission for `call-analyzer-lookup`
in System Settings → Privacy & Security → Accessibility; granting it only to
the main bundle is insufficient. This feature does not use Screen Recording.
Ad-hoc signing can make macOS ask for Accessibility again after a rebuild, so
grant the helper permission again when prompted.

The helper's plist is `native/macos-capture/Sources/call-analyzer-capture/Info.plist`.

## Local data

| Data | Location | Notes |
| --- | --- | --- |
| SQLite | `os.UserConfigDir()/call_analyzer/call_analyzer.db` | On macOS, `~/Library/Application Support/call_analyzer/call_analyzer.db`. Opened in `NewApp`. Mode `0600`. |
| API key | Keychain service `github.com/oeggy03.call_analyzer`, account `OPENROUTER_API_KEY` | Not in SQLite. Environment variable wins. |
| Supabase secret | `SUPABASE_SECRET_KEY` environment variable, or on macOS only the Keychain service `github.com/oeggy03.call_analyzer`, account `SUPABASE_SECRET_KEY` | Not in SQLite, logs, tests, docs, or snapshots. The fallback item belongs only to this Mac. Startup checks read and write access, and a replacement entered in the app is saved to that same Keychain item. The environment value still wins. |
| Supabase URL | `SUPABASE_URL`, defaulting to `https://zoiwsytnyrdifggwleru.supabase.co` | Used only by the vocabulary PostgREST client. |
| Capture audio | A temp directory created by `createSessionSpool` | 16 kHz mono chunks are deleted as the adapter reads them. The session spool is removed on stop; stale directories with no live owner are removed during source initialization and before a new session. |
| Settings | `settings` table | JSON values. Keys are the constants in `internal/service/settings.go`. |
| Model usage | `request_usage` | Per lesson id, plus `manual-vocabulary` for Add Word generation, `photo-vocabulary` for photo recognition, and `selection-lookup` for highlighted-word lookup. |

The selected audio-retention label is stored on lesson metadata, but the helper currently removes audio at the end of capture regardless of that setting.

Do not copy the SQLite file to another machine as a sync mechanism. The
desktop vocabulary runner pulls confirmed vocabulary on launch, then consumes
its outbox and sends current eligible vocabulary rows to Supabase through
PostgREST. A saved-word delete is one of those outbox rows and is sent as a
hosted vocabulary delete. Sync now uploads only. It never sends lessons,
transcripts, observations, or study-state updates. The legacy `SyncEnabled`
flag remains false.

## What is not deployed

- No PostgreSQL service is provisioned by this repository; the app uses the
  configured hosted Supabase PostgREST endpoint.
- The sync goroutine is cross-platform. macOS can use its own Keychain item;
  Windows and other non-Darwin builds have no Keychain and must use a different
  `SUPABASE_SECRET_KEY` environment value. Never copy the Mac key to Windows.
- Windows capture is the `capture.Source` interface only. `newPlatformSource` returns `MockSource` off Darwin.
