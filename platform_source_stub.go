//go:build !darwin

package main

import "github.com/oeggy03/call_analyzer/internal/capture"

func newPlatformSource() capture.Source {
	return capture.NewMockSource()
}
