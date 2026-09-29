//go:build !darwin

package macos

import "context"

// Adapter keeps the same API on non-macOS systems so higher-level capture
// wiring can be compiled and tested without platform-specific build tags.
type Adapter struct {
	config Config
}

func NewAdapter(config Config) *Adapter {
	return &Adapter{config: config}
}

func (a *Adapter) ListTargets(context.Context) ([]Target, error) {
	return nil, ErrUnsupportedPlatform
}

func (a *Adapter) Start(context.Context, CaptureOptions) (EventStream, error) {
	return nil, ErrUnsupportedPlatform
}

func (a *Adapter) Stop(context.Context) error {
	return ErrUnsupportedPlatform
}
