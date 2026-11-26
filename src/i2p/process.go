package i2p

import (
	"errors"
	"os/exec"
	"time"
)

// stopI2PD asks the managed router to stop cleanly and bounds shutdown before
// forcibly killing and reaping a process that does not exit.
func stopI2PD(cmd *exec.Cmd, exited <-chan struct{}) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	select {
	case <-exited:
		return nil
	default:
	}

	signalErr := signalI2PD(cmd)
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-exited:
		if isProcessDone(signalErr) {
			return nil
		}
		return signalErr
	case <-timer.C:
	}

	killErr := cmd.Process.Kill()
	// The process waiter owns Wait and closes exited after it has reaped the
	// child. Killing must therefore be followed by waiting for that notification.
	<-exited
	if isProcessDone(killErr) {
		killErr = nil
	}
	return errors.Join(signalErr, killErr)
}
