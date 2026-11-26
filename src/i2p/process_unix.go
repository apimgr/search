//go:build !windows

package i2p

import (
	"errors"
	"os"
	"os/exec"
	"time"
	"syscall"
)

func signalI2PD(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGTERM)
}

func isProcessDone(err error) bool {
	return err == nil || errors.Is(err, os.ErrProcessDone)
}

func terminateStaleI2PD(pid int, binary string) error {
	matches, err := processMatchesI2PD(pid, binary)
	if err != nil {
		return err
	}
	if !matches {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && !isProcessDone(err) {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		matches, err := processMatchesI2PD(pid, binary)
		if err != nil {
			return err
		}
		if !matches {
			return nil
		}
		if err = syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		} else if err != nil && !errors.Is(err, syscall.EPERM) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	matches, err = processMatchesI2PD(pid, binary)
	if err != nil {
		return err
	}
	if !matches {
		return nil
	}
	if err := process.Kill(); err != nil && !isProcessDone(err) {
		return err
	}
	for i := 0; i < 20; i++ {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		} else if err != nil && !errors.Is(err, syscall.EPERM) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("stale i2pd process did not exit after kill")
}
