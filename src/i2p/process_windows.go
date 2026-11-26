//go:build windows

package i2p

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func signalI2PD(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func isProcessDone(err error) bool {
	return err == nil || errors.Is(err, os.ErrProcessDone)
}

func processMatchesI2PD(pid int, binary string) (bool, error) {
	output, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	line := strings.TrimSpace(string(output))
	if line == "" || strings.HasPrefix(line, "INFO:") {
		return false, nil
	}
	fields := strings.SplitN(line, ",", 2)
	if len(fields) == 0 {
		return false, nil
	}
	name := strings.Trim(fields[0], "\" \r\n")
	return strings.EqualFold(name, filepath.Base(binary)), nil
}

func terminateStaleI2PD(pid int, binary string) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Kill(); err != nil && !isProcessDone(err) {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		matches, err := processMatchesI2PD(pid, binary)
		if err != nil || !matches {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("stale i2pd process did not exit after kill")
}
