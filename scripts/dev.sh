#!/usr/bin/env bash
# Build the capture helper and start the Wails desktop session.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf 'dev: the desktop app runs on macOS. For the UI demo, run: cd frontend && npm run dev\n' >&2
  exit 1
fi

wails_bin="${WAILS_BIN:-$(go env GOPATH)/bin/wails}"
if [[ ! -x "${wails_bin}" ]]; then
  printf 'dev: Wails CLI was not found at %s. Run ./scripts/setup.sh\n' "${wails_bin}" >&2
  exit 1
fi

"${ROOT_DIR}/scripts/build-macos-capture.sh"
exec "${wails_bin}" dev
