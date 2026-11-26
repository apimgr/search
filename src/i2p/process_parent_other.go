//go:build !linux && !windows

package i2p

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func setParentDeathSignal(*exec.Cmd) {}

func processMatchesI2PD(pid int, binary string) (bool, error) {
	output, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("run ps to verify process: %w", err)
	}
	name := filepath.Base(strings.TrimSpace(string(output)))
	want := filepath.Base(binary)
	return name == want, nil
}
