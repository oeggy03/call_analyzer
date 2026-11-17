---
name: verify-call-analyzer
description: >-
  Run and interpret the repository's offline verification workflow. Use when
  checking a change, diagnosing a verification failure, or explaining a known
  platform skip.
---

# Verify Call Analyzer

Use this skill when a task calls for the full repository check or when a failure needs interpretation. For a narrow package change, follow the focused check matrix in `docs/development.md`.

## Workflow

1. Read `docs/development.md` for current tool requirements and troubleshooting.
2. From the repository root, run `./scripts/verify.sh` when a full check is required.
3. Inspect the failing command and its output. Fix the reported issue in the relevant files and rerun the required check; do not claim success from a partial run.
4. Keep checks offline. Do not pass `-execute` to `go run ./cmd/evaluate` or submit live audio to OpenRouter unless the user explicitly asks.

The script runs `gofmt -l`, frontend typecheck/tests/lint/build, Go tests and vet, offline fixture evaluation, and macOS Swift checks across both `native/macos-capture` and `native/macos-lookup`. It skips all Swift checks off macOS. On macOS it builds both Swift packages and runs `swift test` in both packages only when XCTest is available; the skip is expected without full Xcode. The script exits nonzero when a required check fails.

SQLite tests use in-memory databases for most repository cases and temporary directories when persistence across opens is under test. Keep them isolated from the real user database. OpenRouter tests use local HTTP fakes; never require a live key for ordinary tests.

Do not run blanket `gofmt -w` over the repository. If `gofmt -l` identifies files, format only the relevant changed files, then rerun the applicable checks.

## Reference

See [reference.md](reference.md) for the script's exact check order and the contract smoke test location.
