//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly
// +build linux darwin freebsd openbsd netbsd dragonfly

package terminal

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// StartResizeListener starts listening for SIGWINCH signals
// Returns a cancel function to stop listening
func (h *ResizeHandler) StartResizeListener(ctx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(sigChan)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigChan:
				h.Refresh()
			}
		}
	}()

	return cancel
}
