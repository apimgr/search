//go:build linux

package i2p

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func setParentDeathSignal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}

func processMatchesI2PD(pid int, binary string) (bool, error) {
	procPath := filepath.Join("/proc", strconv.Itoa(pid), "exe")
	processPath, err := os.Readlink(procPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	want, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return false, err
	}
	got, err := filepath.EvalSymlinks(processPath)
	if err != nil {
		return false, err
	}
	return got == want, nil
}
