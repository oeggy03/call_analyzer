#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_DIR="${ROOT_DIR}/native/macos-lookup"
OUTPUT_DIR="${CALL_ANALYZER_LOOKUP_OUTPUT_DIR:-${ROOT_DIR}/build/bin}"
SCRATCH_PATH="${CALL_ANALYZER_LOOKUP_SWIFT_SCRATCH:-${TMPDIR:-/tmp}/call-analyzer-lookup-build}"

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf 'call-analyzer-lookup requires macOS (AppKit is unavailable here)\n' >&2
  exit 1
fi

if ! command -v swift >/dev/null 2>&1; then
  printf 'swift was not found; install Xcode Command Line Tools or Xcode\n' >&2
  exit 1
fi

mkdir -p "${OUTPUT_DIR}"
BIN_PATH="$(
  swift build \
    --package-path "${PACKAGE_DIR}" \
    --scratch-path "${SCRATCH_PATH}" \
    --configuration release \
    --show-bin-path
)"
swift build \
  --package-path "${PACKAGE_DIR}" \
  --scratch-path "${SCRATCH_PATH}" \
  --configuration release \
  --product call-analyzer-lookup
install -m 755 "${BIN_PATH}/call-analyzer-lookup" \
  "${OUTPUT_DIR}/call-analyzer-lookup"
printf 'Built %s\n' "${OUTPUT_DIR}/call-analyzer-lookup"
