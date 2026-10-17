#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WAILS_BIN="${WAILS_BIN:-$(go env GOPATH)/bin/wails}"
APP_PATH="${ROOT_DIR}/build/bin/call_analyzer.app"
HELPER_DIR="${ROOT_DIR}/build/native"
HELPER_PATH="${HELPER_DIR}/call-analyzer-capture"
SIGNING_IDENTITY="${APPLE_SIGNING_IDENTITY:--}"

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf 'The macOS app can only be built on macOS.\n' >&2
  exit 1
fi

if [[ ! -x "${WAILS_BIN}" ]]; then
  printf 'Wails was not found at %s. Run: go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2\n' "${WAILS_BIN}" >&2
  exit 1
fi

CALL_ANALYZER_CAPTURE_OUTPUT_DIR="${HELPER_DIR}" \
  "${ROOT_DIR}/scripts/build-macos-capture.sh"
(
  cd "${ROOT_DIR}"
  "${WAILS_BIN}" build -clean -platform darwin/arm64
)

if [[ ! -d "${APP_PATH}" ]]; then
  printf 'Expected app bundle was not produced at %s\n' "${APP_PATH}" >&2
  exit 1
fi

install -m 755 "${HELPER_PATH}" "${APP_PATH}/Contents/MacOS/call-analyzer-capture"

# Sign the nested executable first, then the app. "-" performs ad-hoc signing
# for local development; set APPLE_SIGNING_IDENTITY for stable distribution.
codesign --force --sign "${SIGNING_IDENTITY}" \
  "${APP_PATH}/Contents/MacOS/call-analyzer-capture"
codesign --force --deep --sign "${SIGNING_IDENTITY}" "${APP_PATH}"
codesign --verify --deep --strict "${APP_PATH}"

printf 'Built %s\n' "${APP_PATH}"
