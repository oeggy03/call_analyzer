---
name: verify-call-analyzer
description: >-
  Runs the Call Analyzer offline verification script and interprets its known
  skips. Use when verifying a change, before claiming work is done, when
  tests fail, or when the user mentions verify.sh, gofmt, XCTest, or CI.
---

# Verify Call Analyzer

## When

Use this after a cross-component change, or when the user asks to run checks. For a one-package edit, the narrower commands in `AGENTS.md` are enough until the end of the task.

## Inputs

- A clean enough tree that unrelated user edits must not be reformatted or reverted.
- Go 1.25+, Node.js 22+, npm, and on macOS the Swift toolchain.
- `frontend/node_modules` from `./scripts/setup.sh` or `npm ci --prefix frontend`.

## Procedure

1. From the repository root, run `./scripts/verify.sh`.
2. Do not run `gofmt -w .`. The script uses `gofmt -l` and exits 1 when it prints files.
3. Do not add `-execute` to `go run ./cmd/evaluate`.
4. If the script fails, fix the reported check and run it again. Do not ignore a non-zero exit.

## Verification

- Success prints `verify: ok` and exits 0.
- On Command Line Tools without Xcode, the script prints `verify: skipped swift test` and can still exit 0.
- On a machine with `xctest`, `swift test` must pass. CI uses that stricter path (`.github/workflows/verify.yml`).

## Failure cases

- `frontend dependencies are missing`: run `./scripts/setup.sh`.
- `gofmt found unformatted Go files`: format only those files, then rerun.
- A Go test failed: stay offline. Tests must keep using `httptest` and `:memory:` SQLite.
- `no such module 'XCTest'` from a hand-run `swift test`: expected without Xcode. The script skips this; a direct `swift test` will fail.

## References

- [reference.md](reference.md)
