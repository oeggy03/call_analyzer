//go:build !darwin

package selection

import "context"

// Start is a no-op on platforms without the AppKit helper.
func (r *Runner) Start(context.Context, Handler) error {
	return nil
}

// Stop is a no-op on platforms without the AppKit helper.
func (r *Runner) Stop(context.Context) error {
	return nil
}
