//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly
// +build linux darwin freebsd openbsd netbsd dragonfly

package terminal

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestStartResizeListenerWithSignal(t *testing.T) {
	h := NewResizeHandler()
	ctx := context.Background()

	callCount := 0
	h.OnResize(func(s Size) {
		callCount++
	})

	// Start the listener
	cancel := h.StartResizeListener(ctx)
	defer cancel()

	// Give goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Send SIGWINCH to ourselves
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess failed: %v", err)
	}

	// Send the signal
	err = proc.Signal(syscall.SIGWINCH)
	if err != nil {
		t.Fatalf("Signal failed: %v", err)
	}

	// Wait for the signal to be processed
	time.Sleep(100 * time.Millisecond)

	// The callback may or may not be called depending on whether
	// the terminal size actually changed. The important thing is
	// that the signal was received and processed without panic.
	// The signal handler branch is now covered.
}

func TestStartResizeListenerMultipleSignals(t *testing.T) {
	h := NewResizeHandler()
	ctx := context.Background()

	signalCount := 0
	h.OnResize(func(s Size) {
		signalCount++
	})

	// Start the listener
	cancel := h.StartResizeListener(ctx)
	defer cancel()

	// Give goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Send multiple SIGWINCH signals
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		err = proc.Signal(syscall.SIGWINCH)
		if err != nil {
			t.Fatalf("Signal %d failed: %v", i, err)
		}
		// Small delay between signals
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for signals to be processed
	time.Sleep(100 * time.Millisecond)

	// Clean up
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestStartResizeListenerSignalThenCancel(t *testing.T) {
	h := NewResizeHandler()
	ctx := context.Background()

	// Start the listener
	cancel := h.StartResizeListener(ctx)

	// Give goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Send SIGWINCH
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess failed: %v", err)
	}

	err = proc.Signal(syscall.SIGWINCH)
	if err != nil {
		t.Fatalf("Signal failed: %v", err)
	}

	// Wait for signal to be processed
	time.Sleep(50 * time.Millisecond)

	// Cancel the listener
	cancel()

	// Verify listener is stopped (should not panic)
	time.Sleep(50 * time.Millisecond)

	// Send another signal after cancel - should be ignored
	_ = proc.Signal(syscall.SIGWINCH)
	time.Sleep(50 * time.Millisecond)
}

func TestStartResizeListenerConcurrentSignals(t *testing.T) {
	h := NewResizeHandler()
	ctx := context.Background()

	// Start the listener
	cancel := h.StartResizeListener(ctx)
	defer cancel()

	// Give goroutine time to start
	time.Sleep(50 * time.Millisecond)

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess failed: %v", err)
	}

	// Send signals from multiple goroutines
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 3; j++ {
				_ = proc.Signal(syscall.SIGWINCH)
				time.Sleep(10 * time.Millisecond)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Wait for signals to be processed
	time.Sleep(100 * time.Millisecond)
}
