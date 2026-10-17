# Development

The macOS desktop app requires Go 1.25 or newer, Node.js 22 or newer, npm, macOS 15 or later, and Swift 6. The Go language version in `go.mod` is 1.25.0. Windows builds use the capture stub and do not provide native capture.

Lockfiles to keep: `go.sum` and `frontend/package-lock.json`. The Swift package has no dependencies, so there is no `Package.resolved`.

## Install

```sh
./scripts/setup.sh
```

That checks Go, Node, npm, and Swift, runs `npm ci` and `npm run build` in `frontend`, and installs `github.com/wailsapp/wails/v2/cmd/wails@v2.10.2` when `$(go env GOPATH)/bin/wails` is missing. Override the binary with `WAILS_BIN`. The frontend build creates `frontend/dist`, which `assets.go` embeds.

Full Xcode is only required for `swift test`. Command Line Tools can build the helper and the desktop app.

On Windows, install Go with the official installer from [go.dev/dl](https://go.dev/dl/), open a new PowerShell window, and check `go version` reports 1.25 or newer. The Windows development machine used for this setup has Go 1.27.2 installed system-wide; `go version` reported `go1.27.2 windows/amd64` on 2026-10-17. This records that machine's toolchain, not a project requirement: `go.mod` still requires Go 1.25.0 or newer. Repository scripts are Bash scripts; the Wails app bundle and Swift helper are macOS-only. For frontend checks on Windows, install dependencies and build directly:

```powershell
cd frontend
npm ci
npm run typecheck
npm test -- --run
npm run lint
npm run build
```

The Windows Go test run is currently blocked by embedded migration paths: `go test ./internal/storage` reports that `migrations\001_init.sql` cannot be opened (and similarly fails for migrations 003 and 004). Run the complete Go verification on macOS.

The repository enforces LF line endings for Go and shell sources. Keep those
line endings when editing on Windows; the verification script runs `gofmt`.

## Environment

The app does not load `.env` files. `.env.example` lists the process environment variables. Export the ones you need.

| Variable | Effect |
| --- | --- |
| `OPENROUTER_API_KEY` | Read before Keychain. When set, Settings cannot clear the key. Required only for `cmd/evaluate -execute` and for live model calls if Keychain is empty. |
| `CALL_ANALYZER_CAPTURE_HELPER` | Absolute helper path; tried before `CALL_ANALYZER_CAPTURE_BIN`, the executable-adjacent helper, and `build/bin/call-analyzer-capture`. See `resolveHelperPath`. |
| `CALL_ANALYZER_CAPTURE_BIN` | Alternate helper path. |
| `WAILS_BIN` | Wails CLI used by `scripts/setup.sh`, `scripts/dev.sh`, and `scripts/build-macos-app.sh`. |
| `APPLE_SIGNING_IDENTITY` | Passed to `codesign` by `scripts/build-macos-app.sh`. Default `-` is ad-hoc. |
| `CALL_ANALYZER_CAPTURE_OUTPUT_DIR` | Helper install directory for `scripts/build-macos-capture.sh`. |
| `CALL_ANALYZER_CAPTURE_SWIFT_SCRATCH` | Swift package scratch directory used by `scripts/build-macos-capture.sh`. |

There is no database server, Redis, or other local service. Most tests open SQLite with `storage.Open(ctx, ":memory:")`; persistence and migration tests use temporary database files.

Tests follow their language conventions: Go `_test.go` files stay beside their
package code, Vitest specs and setup live in `frontend/src/test`, and Swift
tests live in `native/macos-capture/Tests`. Synthetic evaluation cases live in
`fixtures/evaluation`.

## Fixtures and offline checks

`fixtures/evaluation/manifest.json` has three synthetic cases and no audio paths. From the repository root:

```sh
go run ./cmd/evaluate
```

This validates the manifest and exits. It does not call OpenRouter.

`go run ./cmd/evaluate -execute` transcribes and extracts with the live API. It spends credits. Do not run it unless the user asks. Consent-cleared WAV paths would be added to the manifest first; none are in the repository.

Go tests that talk to OpenRouter point `openrouter.Client` at an `httptest.Server`.

## Run

Desktop session, after setup:

```sh
./scripts/dev.sh
```

That builds the helper with `scripts/build-macos-capture.sh`, then runs `wails dev`.

UI demo without capture:

```sh
cd frontend
npm run dev
```

`createApi` in `frontend/src/lib/api.ts` uses the demo adapter when `window.go.main.App` is missing and the Vite build is a dev build. A production bundle without Wails shows the unavailable state.

Packaged app: `docs/operations.md`.

## Checks

```sh
./scripts/verify.sh
```

The script exits non-zero on the first failed check. It runs:

1. `gofmt -l` on Go files outside `frontend/node_modules`, `native/macos-capture/.build`, and `build`.
2. In `frontend`: `npm run typecheck`, `npm test -- --run`, `npm run lint`, `npm run build`.
3. `go test` and `go vet` for `go list ./...`, excluding `/frontend/node_modules/`.
4. `go run ./cmd/evaluate` with no `-execute` flag.
5. On macOS, `swift build -c release` for `native/macos-capture`.
6. `swift test` in that package only when `xcrun --find xctest` succeeds.

`assets.go` embeds `frontend/dist`. The frontend build is step 2 so a fresh clone can compile package main. `./scripts/setup.sh` also runs that build.

For focused checks while iterating, use the relevant package or boundary:

| Change | Focused checks |
| --- | --- |
| Go package | `gofmt -l` on edited Go files, then `go test ./path/to/package` and `go vet ./path/to/package` |
| Wails method or snapshot | After building the frontend if `frontend/dist` is absent, run `go test -count=1 -run TestWailsMethodsMatchFrontendContract .`, then `cd frontend && npm run typecheck && npm test -- --run` |
| React UI | `cd frontend && npm run typecheck && npm test -- --run && npm run lint` |
| SQL migration | `go test ./internal/storage` |
| Swift helper or macOS adapter | `swift build -c release --package-path native/macos-capture`; run `swift test` in that directory when full Xcode is installed |

Run `./scripts/verify.sh` before finishing a cross-component change.

`TestWailsMethodsMatchFrontendContract` checks that each name in `BACKEND_METHODS` exists on `App` and validates the result shapes for `GenerateManualVocabulary`, `RecognizeVocabularyPhoto`, `ListLessonTranscripts`, and `GetLessonTranscript`.

GitHub Actions runs the same script on `macos-latest` after `npm ci --prefix frontend` (`.github/workflows/verify.yml`). The Swift test step runs when `xctest` is available; otherwise the script reports that it skipped that step.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| `verify: frontend dependencies are missing` | Run `./scripts/setup.sh`. |
| `go test ./...` mentions `flatted` | Use `./scripts/verify.sh`. The `flatted` npm package contains a `.go` file, and this module otherwise compiles it. |
| `no such module 'XCTest'` | Expected without full Xcode. Install Xcode to run `swift test`, or rely on the script's skip. |
| Helper not found | Build it with `./scripts/build-macos-capture.sh`, or set `CALL_ANALYZER_CAPTURE_HELPER`. |
| Screen Recording or microphone changes do nothing | The root README says to restart the app after changing macOS privacy permissions. |
| Settings will not clear the API key | `OPENROUTER_API_KEY` is set in the environment. |
| `wails: command not found` | The install path is `$(go env GOPATH)/bin/wails`. `./scripts/dev.sh` uses that path, or `WAILS_BIN`. |
| Live lesson spends money | A real capture calls OpenRouter. Tests and `./scripts/verify.sh` do not. |
