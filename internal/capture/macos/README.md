# macOS capture adapter

This package owns the process boundary for `native/macos-capture` and exposes
`Source`, which implements the shared `capture.Source` contract plus its
optional permission, target, and rich-event interfaces.

```go
source := macos.NewSource(macos.Config{
    HelperPath: os.Getenv("CALL_ANALYZER_CAPTURE_HELPER"),
    BundleID:   "us.zoom.xos",
    SpoolDir:   spoolDir,
    Microphone: true,
    OCR:        true,
    ChunkSeconds: 2,
})

err := source.Start(ctx, onAudioFrame)
```

The desktop app sets `ChunkSeconds` to 2 in `platform_source_darwin.go`.
Utterance windows are assembled later in `internal/service/capture_pipeline.go`.

`Config.HelperPath` takes precedence over
`CALL_ANALYZER_CAPTURE_HELPER`, then `CALL_ANALYZER_CAPTURE_BIN`, then
`build/bin/call-analyzer-capture`. `Source.SetEventHandler` maps OCR, state,
and error events to the shared event type; `SetNativeEventHandler` is
available when helper-specific fields such as chunk paths or confidence are
needed. Call `Stop` with a deadline to request a graceful SIGTERM, drain
final chunks, and remove the private session spool.
