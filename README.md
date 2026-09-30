# Mandarin Lesson Analyzer

A local-first macOS desktop app that captures a consented Zoom lesson, sends
short speech chunks to OpenRouter for Mandarin/English transcription, proposes
evidence-backed vocabulary, and keeps confirmed words in SQLite.

The MVP uses:

- React + TypeScript for the Wails desktop UI.
- Go for session orchestration, audio buffering, OpenRouter calls, extraction,
  SQLite, candidate review, budget controls, and future sync.
- A narrow Swift ScreenCaptureKit/Vision helper for APIs macOS does not expose
  to Go. It writes temporary 16 kHz mono WAV chunks which Go consumes and
  deletes.

Automatic suggestions always enter a candidate inbox. They are never promoted
to trusted vocabulary without confirmation.

## Requirements

- Apple Silicon Mac running macOS 15 or later.
- Go 1.25+, Node.js 22+, npm, Swift 6/Xcode Command Line Tools.
- Zoom desktop app.
- An OpenRouter API key with access to:
  - `qwen/qwen3-asr-1.7b`
  - `qwen/qwen3-32b`

Full Xcode is only required to execute the Swift XCTest suite. The helper and
desktop app build with the Command Line Tools available on the development Mac.

## Build and run locally

Install the pinned Wails CLI once:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
```

Build the complete signed local app bundle:

```sh
./scripts/build-macos-app.sh
open build/bin/call_analyzer.app
```

The first capture request opens macOS privacy controls. Allow Screen & System
Audio Recording and, only if enabled in Settings, Microphone access. Restart
the app after changing macOS privacy permissions.

For frontend-only work with deterministic sample data:

```sh
cd frontend
npm install
npm run dev
```

For a Wails development session, first build the helper, then run Wails:

```sh
./scripts/build-macos-capture.sh
$(go env GOPATH)/bin/wails dev
```

The helper can be overridden with
`CALL_ANALYZER_CAPTURE_HELPER=/absolute/path/to/call-analyzer-capture`.

## Using the MVP

1. Open Settings and save an OpenRouter API key. On macOS it is written to
   Keychain, never SQLite.
2. Keep the default Qwen models and the US$0.50 hard lesson budget.
3. Join the consented Zoom lesson on the same Mac, preferably with headphones.
4. Select Zoom, acknowledge consent, and start the lesson.
5. Press `M` or click **Mark Moment** for important words.
6. Confirm, edit, merge, or reject suggestions in Candidate Inbox.
7. Confirmed entries appear in Vocabulary immediately.

You can also open **Add Word** outside a live lesson and enter only the
simplified Chinese word. **Generate missing details** uses the configured
analyzer (Qwen 3 32B by default) to propose pinyin, meanings, usage,
examples, and tags. Review or edit every field before saving. Fully completed
forms can be saved without an API call. The UI displays the model and actual
request cost for each generated draft.

Automatic audio is transcribed in utterance-aware 8–15 second windows, with
two seconds of overlap. Vocabulary extraction batches roughly 20 seconds of
new transcript to control cost. A manual mark analyzes the complete 25-second
pre-roll and 10-second post-roll immediately. At the configured hard budget,
cloud work pauses while local capture and mark timestamps continue.

Provider requests enforce Zero Data Retention routing and deny endpoints marked
for data collection. Local audio retention is controlled separately; the
default is session-only temporary chunks. Session spools are deleted after
processing, and orphaned spools from a crashed process are purged at next
startup.

## MVP boundaries

- The bundled dictionary is a small starter lexicon with an interface for a
  future full CC-CEDICT import.
- The evaluation manifest contains deterministic synthetic fixtures. Accuracy
  should be recalibrated with consent-cleared real lessons before relying on
  unattended automatic capture.
- OCR follows the display containing the active Zoom window, but there is no
  user-selectable crop or image-model escalation yet.
- Windows capture and hosted sync are extension boundaries only; this release
  implements macOS ScreenCaptureKit, local SQLite, and a disabled outbox API.
- Audio is session-only in this MVP. Evidence is retained as transcript text
  and timestamps, not encrypted replayable clips.
- A native stream failure is surfaced in the lesson UI but does not
  automatically restart ScreenCaptureKit.

## Verification

```sh
gofmt -w .
go test ./...
go vet ./...

cd frontend
npm run typecheck
npm test -- --run
npm run lint
npm run build

cd ../native/macos-capture
swift build -c release
```

End-to-end HTTP tests use local fake OpenRouter servers and deterministic PCM;
they do not spend API credits. A real Zoom capture still requires the macOS
permissions and a running Zoom process.

Validate the model evaluation fixture manifest without spending credits:

```sh
go run ./cmd/evaluate
```

To run the transcript/vocabulary evaluation against OpenRouter (billable), set
`OPENROUTER_API_KEY` and add consent-cleared WAV paths to the manifest:

```sh
go run ./cmd/evaluate -execute
```

## Architecture

- `app.go`, `snapshot.go`: Wails application contract.
- `internal/service`: live capture workers and end-to-end analysis pipeline.
- `internal/openrouter`: privacy-constrained STT and strict structured output.
- `internal/storage`: embedded SQLite migrations and repositories.
- `internal/vocabulary`: pinyin normalization, evidence validation, and
  conservative form+reading deduplication.
- `internal/capture`: cross-platform Go capture boundary.
- `internal/capture/macos`: native helper process adapter.
- `native/macos-capture`: ScreenCaptureKit, microphone capture, and local OCR.
- `internal/sync`: disabled-by-default idempotent outbox HTTP boundary.
- `frontend`: React/TypeScript desktop experience.

Windows expansion should implement the existing capture interfaces with
process-specific WASAPI loopback and Windows Graphics Capture. Future online
access should dispatch the existing outbox to an authenticated Go/PostgreSQL
service; the desktop SQLite file must not be synchronized directly.
