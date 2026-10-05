#!/usr/bin/env bash
# Install frontend dependencies and the pinned Wails CLI. Does not start the app.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'setup: missing %s. %s\n' "$1" "$2" >&2
    exit 1
  fi
}

need go "Install Go 1.25 or newer."
need node "Install Node.js 22 or newer."
need npm "npm is required to install frontend dependencies."
if [[ "$(uname -s)" == "Darwin" ]]; then
  need swift "Install Xcode Command Line Tools. Full Xcode is required only for swift test."
fi

go_release="$(go env GOVERSION | sed 's/^go//')"
go_major="$(printf '%s' "${go_release}" | cut -d. -f1)"
go_minor="$(printf '%s' "${go_release}" | cut -d. -f2)"
if [[ "${go_major}" -lt 1 || ( "${go_major}" -eq 1 && "${go_minor}" -lt 25 ) ]]; then
  printf 'setup: Go 1.25 or newer is required (found %s)\n' "${go_release}" >&2
  exit 1
fi

node_major="$(node -p "process.versions.node.split('.')[0]")"
if [[ "${node_major}" -lt 22 ]]; then
  printf 'setup: Node.js 22 or newer is required (found %s)\n' "$(node --version)" >&2
  exit 1
fi

npm ci --prefix frontend
npm run build --prefix frontend

wails_bin="${WAILS_BIN:-$(go env GOPATH)/bin/wails}"
if [[ ! -x "${wails_bin}" ]]; then
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
  wails_bin="${WAILS_BIN:-$(go env GOPATH)/bin/wails}"
fi
if [[ ! -x "${wails_bin}" ]]; then
  printf 'setup: Wails CLI was not installed at %s\n' "${wails_bin}" >&2
  exit 1
fi

printf 'setup: ok\n'
printf 'Next: ./scripts/dev.sh for the desktop app, or cd frontend && npm run dev for the UI demo.\n'
printf 'Checks: ./scripts/verify.sh\n'
