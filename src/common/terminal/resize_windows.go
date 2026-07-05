//go:build windows
// +build windows

package terminal

import (
	"context"
	"time"
)

// StartResizeListener starts polling for terminal size changes on Windows
// Windows doesn't have SIGWINCH, so we poll instead
// Returns a cancel function to stop listening
func (h *ResizeHandler) StartResizeListener(ctx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.Refresh()
			}
		}
	}()

	return cancel
}
