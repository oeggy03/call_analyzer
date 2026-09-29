#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_DIR="${ROOT_DIR}/native/macos-capture"
OUTPUT_DIR="${CALL_ANALYZER_CAPTURE_OUTPUT_DIR:-${ROOT_DIR}/build/bin}"
SCRATCH_PATH="${CALL_ANALYZER_CAPTURE_SWIFT_SCRATCH:-${TMPDIR:-/tmp}/call-analyzer-capture-build}"

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf 'call-analyzer-capture requires macOS (ScreenCaptureKit is unavailable here)\n' >&2
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
  --product call-analyzer-capture
install -m 755 "${BIN_PATH}/call-analyzer-capture" \
  "${OUTPUT_DIR}/call-analyzer-capture"
printf 'Built %s\n' "${OUTPUT_DIR}/call-analyzer-capture"
