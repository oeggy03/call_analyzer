# Verify reference

| Check | Command inside `scripts/verify.sh` |
| --- | --- |
| Format | `gofmt -l` via `find`, excluding `frontend/node_modules`, `native/macos-capture/.build`, and `build` |
| UI | `npm run typecheck`, `npm test -- --run`, `npm run lint`, `npm run build` in `frontend` |
| Go | `go test` and `go vet` on `go list ./...` minus `/frontend/node_modules/` |
| Fixtures | `go run ./cmd/evaluate` from the repository root |
| Helper | `swift build -c release --package-path native/macos-capture` on Darwin |
| Helper tests | `swift test` in `native/macos-capture` only when `xcrun --find xctest` succeeds |

The UI build runs before `go test` because `assets.go` embeds `frontend/dist`.

`frontend/node_modules/flatted/golang/pkg/flatted/flatted.go` is why the package filter exists.

Contract smoke test: `TestWailsMethodsMatchFrontendContract` in `contract_test.go`.

Versions and troubleshooting: `docs/development.md`.
