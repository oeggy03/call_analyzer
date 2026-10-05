# Development

Requirements and the commands below were checked on this Apple Silicon Mac: Go 1.25.14, Node.js 22.23.1, npm 10.9.8, Swift 6.3.2, Wails 2.10.2, macOS 26.5.2. Minimums are lower. The language version in `go.mod` is Go 1.25.0. The root `README.md` requires Node.js 22+, Swift 6, and macOS 15 or later.

Lockfiles to keep: `go.sum` and `frontend/package-lock.json`. The Swift package has no dependencies, so there is no `Package.resolved`.

## Install

```sh
./scripts/setup.sh
```

That checks Go, Node, npm, and Swift, runs `npm ci` and `npm run build` in `frontend`, and installs `github.com/wailsapp/wails/v2/cmd/wails@v2.10.2` when `$(go env GOPATH)/bin/wails` is missing. Override the binary with `WAILS_BIN`. The frontend build creates `frontend/dist`, which `assets.go` embeds.

Full Xcode is only required for `swift test`. Command Line Tools can build the helper and the desktop app.

## Environment

The app does not load `.env` files. `.env.example` lists the process environment variables. Export the ones you need.

| Variable | Effect |
| --- | --- |
| `OPENROUTER_API_KEY` | Read before Keychain. When set, Settings cannot clear the key. Required only for `cmd/evaluate -execute` and for live model calls if Keychain is empty. |
| `CALL_ANALYZER_CAPTURE_HELPER` | Absolute path to `call-analyzer-capture`. Wins over `CALL_ANALYZER_CAPTURE_BIN` and `build/bin/call-analyzer-capture`. See `resolveHelperPath`. |
| `CALL_ANALYZER_CAPTURE_BIN` | Alternate helper path. |
| `WAILS_BIN` | Wails CLI used by `scripts/setup.sh`, `scripts/dev.sh`, and `scripts/build-macos-app.sh`. |
| `APPLE_SIGNING_IDENTITY` | Passed to `codesign` by `scripts/build-macos-app.sh`. Default `-` is ad-hoc. |
| `CALL_ANALYZER_CAPTURE_OUTPUT_DIR` | Helper install directory for `scripts/build-macos-capture.sh`. |

There is no database server, Redis, or other local service. Tests open SQLite with `storage.Open(ctx, ":memory:")`.

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

Use a narrower command from `AGENTS.md` while iterating. Run the script before finishing a cross-component change.

`TestWailsMethodsMatchFrontendContract` fails when a name in `BACKEND_METHODS` is missing on `App`, or when a method other than `GenerateManualVocabulary` does not return `(AppSnapshot, error)`.

GitHub Actions runs the same script on `macos-latest` after `npm ci --prefix frontend` (`.github/workflows/verify.yml`). That runner includes Xcode, so `swift test` runs there. A laptop with only Command Line Tools skips `swift test` and still prints `verify: ok` when everything else passes.

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
