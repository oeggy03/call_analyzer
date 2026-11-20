# Mandarin Lesson Analyzer

A local-first macOS desktop app for consented lessons. It captures system
audio from apps on the Mac, optionally includes the microphone, sends short
speech chunks to OpenRouter for transcription, proposes evidence-backed
vocabulary, and keeps transcripts and confirmed words in SQLite.

The MVP uses:

- React + TypeScript for the Wails desktop UI.
- Go for session orchestration, audio buffering, OpenRouter calls, extraction,
  SQLite, candidate review, cost accounting, selection lookup, and
  cross-platform Supabase vocabulary outbox sync.
- Two narrow Swift macOS helpers for APIs macOS does not expose cleanly to Go:
  `call-analyzer-capture` (ScreenCaptureKit audio/screen frames, microphone, and
  Vision OCR, writing temporary 16 kHz mono WAV chunks that Go consumes and
  deletes) and `call-analyzer-lookup` (an `LSUIElement` AppKit agent for the
  `Command-Option-5` global shortcut, Accessibility text/bounds reads, and the
  floating lookup and card-saving panel).

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
- `docs/agents.md`: agent roles, harness configs, and package ownership.
- `docs/work/README.md`: optional notes for work that spans sessions.
- `frontend/README.md`: the React UI and its Wails boundary.
- `internal/capture/macos/README.md`: the helper process boundary.

## Requirements

- Apple Silicon Mac running macOS 15 or later.
- Go 1.25+, Node.js 22+, npm, Swift 6/Xcode Command Line Tools.
- An OpenRouter API key with access to:
  - `qwen/qwen3-asr-1.7b` (default transcription; configurable in Settings)
  - `qwen/qwen3-32b` (default lesson analysis and card generation; configurable in Settings)
  - `qwen/qwen3.7-flash` for photo recognition (or `qwen/qwen3-vl-30b-a3b-instruct` in Settings)
  - `google/gemini-3.1-flash-lite` for `Command-Option-5` highlighted-word lookup

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
Selection lookup separately requires Accessibility permission for
`call-analyzer-lookup`; it does not use Screen Recording.

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

That builds the capture helper and the lookup helper, then starts Wails. The
underlying commands are `scripts/build-macos-capture.sh`,
`scripts/build-macos-lookup.sh`, and `wails dev`.

The helper can be overridden with
`CALL_ANALYZER_CAPTURE_HELPER=/absolute/path/to/call-analyzer-capture`.
The lookup helper can be overridden with
`CALL_ANALYZER_LOOKUP_HELPER=/absolute/path/to/call-analyzer-lookup`.

## Using the MVP

1. Open Settings and save an OpenRouter API key. On macOS it is written to
   Keychain, never SQLite.
2. Join a lesson where all participants have consented to capture.
3. Run **Check setup** on Live Lesson if needed (checks the OpenRouter key,
   cloud vocabulary read/write access with an inline Keychain key prompt when
   missing or rejected, and Screen & System Audio permission), set optional
   comma-separated lesson tags (defaulting to `lesson_YYYY-MM-DD_HH-MM` in local
   time and applied to new automatic candidates), acknowledge consent, and start
   the lesson. The Live view captures system audio across apps and does not
   require Zoom to be running. Microphone capture is enabled by default and can
   be turned off in Settings.
4. Press `M` or click **Mark Moment** for important words.
5. Confirm, edit, merge, or reject suggestions in Candidate Inbox.
6. Confirmed entries appear in Vocabulary immediately (9 words per page, with
   search, status, and tag filters). Deleting a saved word asks for confirmation
   in-app, removes it locally, and queues that delete for cloud sync.
7. While Call Analyzer is running, highlight Chinese text in any Accessibility-enabled
   app and press `Command-Option-5` to review its pinyin and gloss. **Add word**
   generates the rest of the card inside the tooltip (including during a live
   lesson), lets you pick a variant or toggle existing tags, and **Save** stores
   it without bringing the main window forward. **Details** opens a read-only
   card for an entry that already has a saved example.

You can also open **Add Word** and enter only the simplified Chinese word.
**Generate missing details** uses the configured analyzer (Qwen 3 32B by
default) to propose pinyin, an English meaning, and an example. When the word
has more than one reading or meaning, the form stays as you typed it until you
pick a variant. A meaning or distinguishing pinyin you already entered selects
the matching variant. Generation does not invent tags, while existing saved tags
can be toggled on Add Word, Photo review, Candidate edit, and the lookup
tooltip. Tone-marked, numbered, and joined pinyin syllables (such as `wǒmen`)
are accepted. Generation on the Add Word screen is unavailable during a live
lesson. Review or edit every field before saving. Fully completed forms can be
saved without an API call, including during a live lesson. The UI displays the
model and actual request cost for each generated draft.

**Read photo** accepts a JPEG, PNG, WebP, HEIC, or HEIF image (dropped, chosen,
or pasted), resizes it in the browser to at most 1280 pixels on its longest
side, and encodes it as JPEG. Up to eight photos can be queued at once; the
backend accepts images up to 1.5 MiB and keeps at most 12 suggestions per image.
Review and choose suggestions before saving. Photo reading and saving are
available during a live lesson. Photo requests use the configured vision model
and record usage separately from lesson cost. They do not send Zero Data
Retention or data-collection-denial routing flags. The live transcript stays in
a scrolling pane: scroll up to pause, then choose **Resume live** or return to
the bottom. The **Past transcripts** sidebar view opens earlier saved lessons in
a read-only reader.

Automatic audio is transcribed in utterance-aware windows of up to 15 seconds,
usually requiring eight seconds of buffered audio before a silence-triggered
flush. Stopping can flush a shorter final window. Adjacent windows overlap by
two seconds and use two seconds of trailing silence. Audio transcription runs on
a concurrent worker pool with ordered per-track commit, while vocabulary
extraction runs on a separate background queue that batches about 30 seconds of
new transcript. A manual mark uses up to 25 seconds of pre-roll and 10 seconds
of post-roll from system audio.

Lesson transcription, vocabulary analysis, Add Word generation, and
`Command-Option-5` selection lookup/card generation deny provider data
collection (`data_collection: deny`) without restricting Zero Data Retention
routing. Photo recognition does not send those
controls. Local audio retention is controlled separately. The default is
session-only temporary chunks. Chunks are removed after processing and the
session spool is removed when capture stops. A new session removes leftover
spools whose owner process is no longer running.

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
  cross-platform outbox runner when `SUPABASE_SECRET_KEY` is configured (from
  the environment, or on macOS its Keychain item): startup probes read/write
  access, launch pulls confirmed cloud vocabulary, and the runner uploads
  current confirmed/manual rows and saved-word deletes after a 10-minute quiet
  period, on **Sync now** (upload-only), or when accepted on window close.
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

- `app.go`, `app_contract.go`, `snapshot.go`: Wails application contract.
- `internal/service`: live capture workers, manual/photo/selection vocabulary flows, and end-to-end analysis pipeline.
- `internal/openrouter`: transcription, text and vision requests, and cost accounting.
- `internal/storage`: embedded SQLite migrations, repositories, and cloud pull application.
- `internal/vocabulary`: pinyin normalization (including joined syllables), evidence validation, and conservative form+reading deduplication.
- `internal/chinese`, `internal/audio`, `internal/transcript`: simplified-character/sentence-pinyin helpers, PCM/WAV ring buffering and VAD energy, and overlap reconciliation.
- `internal/capture`: cross-platform Go capture boundary.
- `internal/capture/macos`: native capture helper process adapter.
- `native/macos-capture`: ScreenCaptureKit, microphone capture, and local Vision OCR.
- `internal/selection`: JSON-line runner for the highlighted-word lookup helper (no-op off Darwin).
- `native/macos-lookup`: `LSUIElement` AppKit agent for `Command-Option-5`, Accessibility selection reads, and the lookup/card-saving tooltip panel.
- `internal/sync`: launch pull, quiet-period vocabulary outbox runner, and Supabase PostgREST client.
- `cmd/evaluate`, `internal/evaluation`: offline fixture manifest validator and optional live evaluation harness.
- `frontend`: React/TypeScript desktop experience.

Windows expansion should implement the existing capture interfaces with
process-specific WASAPI loopback and Windows Graphics Capture. The desktop
SQLite file must not be synchronized directly; the current runner pulls
confirmed vocabulary on launch and sends only eligible vocabulary rows and
saved-word deletes to the authenticated Supabase PostgREST endpoint.
Lessons, transcripts, observations, and study-state changes remain outside
this sync boundary.
