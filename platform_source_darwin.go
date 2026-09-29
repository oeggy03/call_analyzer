//go:build darwin

package main

import (
	"github.com/oeggy03/call_analyzer/internal/capture"
	"github.com/oeggy03/call_analyzer/internal/capture/macos"
)

func newPlatformSource() capture.Source {
	return macos.NewSource(macos.Config{
		BundleID:     "us.zoom.xos",
		Microphone:   true,
		OCR:          false,
		ChunkSeconds: 10,
	})
}
