//go:build linux

package i2p

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParentDeathSignalStopsManagedChild(t *testing.T) {
	if os.Getenv("SEARCH_I2P_PDEATH_HELPER") == "1" {
		pidPath := os.Getenv("SEARCH_I2P_PDEATH_PID_FILE")
		child := exec.Command("sleep", "60")
		setParentDeathSignal(child)
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(child.Process.Pid)), 0600); err != nil {
			_ = child.Process.Kill()
			os.Exit(3)
		}
		os.Exit(0)
	}

	pidPath := filepath.Join(t.TempDir(), "child.pid")
	helper := exec.Command(os.Args[0], "-test.run=^TestParentDeathSignalStopsManagedChild$")
	helper.Env = append(os.Environ(), "SEARCH_I2P_PDEATH_HELPER=1", "SEARCH_I2P_PDEATH_PID_FILE="+pidPath)
	if output, err := helper.CombinedOutput(); err != nil {
		t.Fatalf("helper process failed: %v: %s", err, strings.TrimSpace(string(output)))
	}
	pidBytes, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read helper child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid helper child pid %q: %v", pidBytes, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stopped, err := linuxProcessStopped(pid)
		if err != nil {
			t.Fatalf("check child process %d: %v", pid, err)
		}
		if stopped {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("managed child %d survived parent exit", pid)
}

func linuxProcessStopped(pid int) (bool, error) {
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	commEnd := strings.LastIndexByte(string(stat), ')')
	if commEnd < 0 {
		return false, errors.New("invalid Linux process stat record")
	}
	fields := strings.Fields(string(stat[commEnd+1:]))
	if len(fields) == 0 {
		return false, errors.New("Linux process stat record has no state")
	}
	return fields[0] == "Z" || fields[0] == "X", nil
}
