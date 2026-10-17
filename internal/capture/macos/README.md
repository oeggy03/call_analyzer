# macOS capture adapter

This package owns the process boundary for `native/macos-capture` and exposes
`Source`, which implements the shared `capture.Source` contract plus its
optional permission, target, and rich-event interfaces.

```go
source := macos.NewSource(macos.Config{
    HelperPath: os.Getenv("CALL_ANALYZER_CAPTURE_HELPER"),
    BundleID:   "system",
    SpoolDir:   spoolDir,
    Microphone: true,
    OCR:        false,
    ChunkSeconds: 2,
})

err := source.Start(ctx, onAudioFrame)
```

The desktop Live view configures the system target. ScreenCaptureKit captures
system audio across apps; Zoom does not need to be running. In this mode OCR
receives screen frames from the largest display. The desktop uses
`ChunkSeconds` 2; WAV output is 16 kHz mono. Go assembles utterance windows
later in `internal/service/capture_pipeline.go`.

`Config.HelperPath` takes precedence over
`CALL_ANALYZER_CAPTURE_HELPER`, then `CALL_ANALYZER_CAPTURE_BIN`, then
the helper beside the running executable, then
`build/bin/call-analyzer-capture`. These candidates are tried in order.
`Source.SetEventHandler` maps OCR, state, and error events to the shared event
type; `SetNativeEventHandler` is available when helper-specific fields such as
chunk paths or confidence are needed. The helper commands are `list`,
`capture`, and `stop`; `capture`
requires `--bundle-id` and `--spool`. Call `Stop` with a deadline to request a
graceful SIGTERM and drain final chunks. The adapter deletes chunk files after
reading and removes the private session spool when capture stops. Stale spools
are checked at source initialization and before a new session is created.
