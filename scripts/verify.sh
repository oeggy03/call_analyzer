#!/usr/bin/env bash
# Run the offline checks. Exits non-zero on the first failure.
# Does not call OpenRouter and does not pass -execute to cmd/evaluate.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

unformatted="$(
  find . -name '*.go' \
    -not -path './frontend/node_modules/*' \
    -not -path './native/macos-capture/.build/*' \
    -not -path './build/*' \
    -exec gofmt -l {} +
)"
if [[ -n "${unformatted}" ]]; then
  printf '%s\n' "${unformatted}" >&2
  printf 'verify: gofmt found unformatted Go files\n' >&2
  exit 1
fi

if [[ ! -d frontend/node_modules ]]; then
  printf 'verify: frontend dependencies are missing. Run ./scripts/setup.sh\n' >&2
  exit 1
fi
(
  cd frontend
  npm run typecheck
  npm test -- --run
  npm run lint
  npm run build
)

# Package main embeds frontend/dist, so the frontend build must come first.
packages="$(go list ./...)"
filtered="$(printf '%s\n' "${packages}" | grep -v '/frontend/node_modules/' || true)"
if [[ -z "${filtered}" ]]; then
  printf 'verify: go list returned no project packages\n' >&2
  exit 1
fi
# Package paths do not contain spaces.
# shellcheck disable=SC2086
go test ${filtered}
# shellcheck disable=SC2086
go vet ${filtered}

go run ./cmd/evaluate

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf 'verify: skipped Swift checks (not macOS)\n'
else
  if ! command -v swift >/dev/null 2>&1; then
    printf 'verify: swift is required on macOS\n' >&2
    exit 1
  fi
  swift build -c release --package-path native/macos-capture
  if xcrun --find xctest >/dev/null 2>&1; then
    (cd native/macos-capture && swift test)
  else
    printf 'verify: skipped swift test (XCTest is not installed; full Xcode is required)\n'
  fi
fi

printf 'verify: ok\n'
