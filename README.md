# Mandarin Lesson Analyzer

A local-first macOS desktop app for consented lessons. It captures system
audio from apps on the Mac, optionally includes the microphone, sends short
speech chunks to OpenRouter for transcription, proposes evidence-backed
vocabulary, and keeps transcripts and confirmed words in SQLite.

The MVP uses:

- React + TypeScript for the Wails desktop UI.
- Go for session orchestration, audio buffering, OpenRouter calls, extraction,
  SQLite, candidate review, and cost accounting.
- A narrow Swift ScreenCaptureKit/Vision helper for APIs macOS does not expose
  to Go. It writes temporary 16 kHz mono WAV chunks which Go consumes and
  deletes.

New automatic suggestions enter the candidate inbox. They become trusted
vocabulary only after confirmation; matching existing entries keep their
current status.

## Documentation

Coding agents start at `AGENTS.md`. Read a supporting doc when the change needs it, not before every edit.

- `docs/architecture.md`: component boundaries and end-to-end flows.
- `docs/behavior.md`: lesson, vocabulary, budget, and privacy rules.
- `docs/development.md`: install, environment, startup, and checks.
- `docs/operations.md`: the local app bundle, permissions, and data files.
- `docs/decisions.md`: architectural choices recorded from the code.
- `docs/limitations.md`: known gaps and unresolved questions.
- `docs/work/README.md`: optional notes for work that spans sessions.
- `frontend/README.md`: the React UI and its Wails boundary.
- `internal/capture/macos/README.md`: the helper process boundary.

## Requirements

- Apple Silicon Mac running macOS 15 or later.
- Go 1.25+, Node.js 22+, npm, Swift 6/Xcode Command Line Tools.
- An OpenRouter API key with access to:
  - `qwen/qwen3-asr-1.7b`
  - `qwen/qwen3-32b`
  - `qwen/qwen3.7-flash` for photo recognition

Full Xcode is only required to execute the Swift XCTest suite. The helper and
desktop app build with the Command Line Tools available on the development Mac.

## Build and run locally

Install frontend dependencies and the pinned Wails CLI:

```sh
./scripts/setup.sh
```

`scripts/setup.sh` installs `github.com/wailsapp/wails/v2/cmd/wails@v2.10.2` when it is not already at `$(go env GOPATH)/bin/wails`.

Build the complete signed local app bundle:

```sh
./scripts/build-macos-app.sh
open build/bin/call_analyzer.app
```

The first capture request opens macOS privacy controls. Allow Screen & System
Audio Recording and, only if microphone capture is enabled in Settings,
Microphone access. Restart the app after changing macOS privacy permissions.

For frontend-only work with deterministic sample data:

```sh
cd frontend
npm install
npm run dev
```

For a Wails development session:

```sh
./scripts/dev.sh
```

That builds the helper, then starts Wails. The same two steps are `scripts/build-macos-capture.sh` and `wails dev`.

The helper can be overridden with
`CALL_ANALYZER_CAPTURE_HELPER=/absolute/path/to/call-analyzer-capture`.

## Using the MVP

1. Open Settings and save an OpenRouter API key. On macOS it is written to
   Keychain, never SQLite.
2. Join a lesson where all participants have consented to capture.
3. Acknowledge consent and start the lesson. The Live view captures system
   audio across apps and does not require Zoom to be running. Microphone
   capture is enabled by default and can be turned off in Settings.
4. Press `M` or click **Mark Moment** for important words.
5. Confirm, edit, merge, or reject suggestions in Candidate Inbox.
6. Confirmed entries appear in Vocabulary immediately. Deleting a saved word removes it locally and queues that delete for cloud sync.

You can also open **Add Word** and enter only the simplified Chinese word.
**Generate missing details** uses the configured analyzer (Qwen 3 32B by
default) to propose pinyin, an English meaning, and an example. When the word
has more than one reading or meaning, the form stays as you typed it until you
pick a variant. A meaning or distinguishing pinyin you already entered selects
the matching variant. Generation does not invent tags. Generation is
unavailable during a live lesson. Review or edit every field before saving.
Fully completed forms can be saved without an API call, including during a
live lesson. The UI displays the model and actual request cost for each
generated draft.

**Read photo** accepts a JPEG, PNG, WebP, HEIC, or HEIF image, resizes it in
the browser to at most 1280 pixels on its longest side, and encodes it as
JPEG. Up to eight photos can be queued at once; the backend accepts images up
to 1.5 MiB and keeps at most 12 suggestions per image. Review and choose
suggestions before saving. Photo reading and saving are available during a
live lesson. Photo requests use the configured vision model and record usage
separately from lesson cost. They do not send Zero Data Retention or
data-collection-denial routing flags. The live transcript stays in a scrolling
pane: scroll up to pause, then choose **Resume live** or return to the bottom.
**Past transcripts** opens earlier lessons without leaving that pane.

Automatic audio is transcribed in utterance-aware windows of up to 15 seconds,
usually requiring eight seconds of buffered audio before a silence-triggered
flush. Stopping can flush a shorter final window. Adjacent windows overlap by
two seconds and use two seconds of trailing silence. Vocabulary extraction
batches about 20 seconds of new transcript. A manual mark uses up to 25 seconds
of pre-roll and 10 seconds of post-roll from system audio.

Lesson transcription, vocabulary analysis, and Add Word generation request
Zero Data Retention routing and deny data collection. Photo recognition does
not send those controls. Local audio retention is controlled separately. The
default is session-only temporary chunks. Chunks are removed after processing
and the session spool is removed when capture stops. A new session removes
leftover spools whose owner process is no longer running.

The cost display warns at US$0.35. The US$0.50 hard-budget setting remains
range-validated for compatibility, but it does not stop OpenRouter requests.
The Settings screen reports that the session has no cap.

## MVP boundaries

- The bundled dictionary is a small starter lexicon with an interface for a
  future full CC-CEDICT import.
- The evaluation manifest contains deterministic synthetic fixtures. Accuracy
  should be recalibrated with consent-cleared real lessons before relying on
  unattended automatic capture.
- OCR is off by default. In system mode it receives screen frames from the
  largest display; there is no user-selected display or crop.
- Windows capture remains a stub. Vocabulary sync is opt-in through the
  cross-platform outbox runner when `SUPABASE_SECRET_KEY` is configured; it
  sends current confirmed and manually saved vocabulary rows only.
- Audio is session-only in this MVP. Evidence is retained as transcript text
  and timestamps, not encrypted replayable clips.
- A native stream failure is surfaced in the lesson UI but does not
  automatically restart ScreenCaptureKit.

## Verification

```sh
./scripts/verify.sh
```

The script is the formatting, Go, frontend, fixture, and Swift check. It exits
non-zero when a check fails. Command details, the XCTest skip, and narrower
commands are in `docs/development.md`.

End-to-end HTTP tests use local fake OpenRouter servers and deterministic PCM;
they do not spend API credits. A real system-audio capture requires macOS
permissions.

`./scripts/verify.sh` includes `go run ./cmd/evaluate`, which validates
`fixtures/evaluation/manifest.json` and does not call OpenRouter. To run the
transcript/vocabulary evaluation against OpenRouter (billable), set
`OPENROUTER_API_KEY` and add consent-cleared WAV paths to the manifest:

```sh
go run ./cmd/evaluate -execute
```

## Architecture

Flows and boundaries are in `docs/architecture.md`. Package roles:

- `app.go`, `snapshot.go`: Wails application contract.
- `internal/service`: live capture workers and end-to-end analysis pipeline.
- `internal/openrouter`: transcription, text and vision requests, and cost accounting.
- `internal/storage`: embedded SQLite migrations and repositories.
- `internal/vocabulary`: pinyin normalization, evidence validation, and
  conservative form+reading deduplication.
- `internal/capture`: cross-platform Go capture boundary.
- `internal/capture/macos`: native helper process adapter.
- `native/macos-capture`: ScreenCaptureKit, microphone capture, and local OCR.
- `internal/sync`: quiet-period vocabulary outbox runner and PostgREST client.
- `frontend`: React/TypeScript desktop experience.

Windows expansion should implement the existing capture interfaces with
process-specific WASAPI loopback and Windows Graphics Capture. The desktop
SQLite file must not be synchronized directly; the current runner sends only
eligible vocabulary rows to the authenticated Supabase PostgREST endpoint.
Lessons, transcripts, observations, and study-state changes remain outside
this sync boundary.
