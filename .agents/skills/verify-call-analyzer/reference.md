# Verification reference

`./scripts/verify.sh` is the source of truth for the full local check. It runs these checks in order and stops on the first failure:

| Stage | Check |
| --- | --- |
| Go formatting | `gofmt -l` over Go files, excluding `frontend/node_modules`, `native/macos-capture/.build`, and `build` |
| Frontend | In `frontend`: `npm run typecheck`, `npm test -- --run`, `npm run lint`, then `npm run build` |
| Go | `go test` and `go vet` on `go list ./...`, filtered to exclude `/frontend/node_modules/` |
| Fixtures | `go run ./cmd/evaluate`, without `-execute` |
| Swift build | On macOS, `swift build -c release --package-path native/macos-capture` |
| Swift tests | On macOS only when `xcrun --find xctest` succeeds |

The frontend build runs before Go checks because `assets.go` embeds `frontend/dist`. Off macOS, all Swift checks are skipped. On macOS without XCTest (full Xcode), the script reports a Swift test skip; a build failure or an available test failure is still an error.

The Wails contract test is `TestWailsMethodsMatchFrontendContract` in `contract_test.go`. Read its current result-type map together with methods in `app.go`; draft and transcript query methods do not all return `AppSnapshot`.

For setup and tool versions, read `docs/development.md`. The script itself is authoritative if this reference becomes stale.
