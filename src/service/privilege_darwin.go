//go:build darwin

package service

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/apimgr/search/src/config"
)

// GetServiceUser returns the canonical service account name.
// Per AI.md PART 23, the account name is the frozen internal name on every OS.
func GetServiceUser(serviceName string) string {
	return serviceName
}

// GetServiceGroup returns the canonical service group name.
func GetServiceGroup(serviceName string) string {
	return serviceName
}

// FindAvailableSystemID finds an available UID/GID in the macOS safe range (200–399).
// Per AI.md PART 24: macOS uses dscl and a separate ID range from Linux/BSD.
func FindAvailableSystemID() (int, error) {
	return findAvailableMacOSSystemID()
}

// CreateSystemUser creates a macOS service account.
// Per AI.md PART 24: System user creation logic.
func CreateSystemUser(name string) (*SystemUser, error) {
	created, err := createMacOSServiceAccount(name)
	if err != nil {
		return nil, err
	}
	if created.Created {
		if err := markSystemUserOwned(name); err != nil {
			return nil, err
		}
	}
	if err := migrateLegacyMacOSAccount(name); err != nil {
		return nil, err
	}
	return created, nil
}

// findAvailableMacOSSystemID finds an available ID in the macOS safe range.
// Per AI.md PART 24: macOS safe range is 200–399.
func findAvailableMacOSSystemID() (int, error) {
	usedIDs := make(map[int]bool)

	for id := range reservedSystemIDs {
		usedIDs[id] = true
	}

	usersOutput, err := exec.Command("dscl", ".", "-list", "/Users", "UniqueID").Output()
	if err != nil {
		return 0, fmt.Errorf("list macOS user IDs: %w", err)
	}
	groupsOutput, err := exec.Command("dscl", ".", "-list", "/Groups", "PrimaryGroupID").Output()
	if err != nil {
		return 0, fmt.Errorf("list macOS group IDs: %w", err)
	}
	for id := range parseMacOSAccountIDs(string(usersOutput), string(groupsOutput)) {
		usedIDs[id] = true
	}

	// Scan descending to prefer higher IDs.
	// Per AI.md PART 23: highest available safe ID selected first.
	for id := 399; id >= 200; id-- {
		if !usedIDs[id] {
			return id, nil
		}
	}
	return 0, fmt.Errorf("no available system ID in range 200-399")
}

func parseMacOSAccountIDs(usersOutput, groupsOutput string) map[int]bool {
	ids := make(map[int]bool)
	for _, listing := range []string{usersOutput, groupsOutput} {
		for _, line := range strings.Split(listing, "\n") {
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			id, err := strconv.Atoi(parts[len(parts)-1])
			if err == nil {
				ids[id] = true
			}
		}
	}
	return ids
}

// createMacOSServiceAccount creates a macOS service account via dscl.
// Per AI.md PART 23: the account uses the frozen internal name and is hidden from login.
func createMacOSServiceAccount(name string) (*SystemUser, error) {
	svcName := name
	homeDir := config.GetDataDir()
	if homeDir == "" {
		return nil, fmt.Errorf("data directory is unavailable for the macOS service account home")
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return nil, fmt.Errorf("create macOS service account home %s: %w", homeDir, err)
	}

	cmd := exec.Command("dscl", ".", "-read", "/Users/"+svcName)
	if err := cmd.Run(); err == nil {
		uidCmd := exec.Command("dscl", ".", "-read", "/Users/"+svcName, "UniqueID")
		output, _ := uidCmd.Output()
		uid := 0
		if len(output) > 0 {
			parts := strings.Fields(string(output))
			if len(parts) >= 2 {
				uid, _ = strconv.Atoi(parts[1])
			}
		}
		return &SystemUser{
			Name:  svcName,
			UID:   uid,
			GID:   uid,
			Home:  homeDir,
			Shell: "/usr/bin/false",
		}, nil
	}

	id, err := FindAvailableSystemID()
	if err != nil {
		return nil, fmt.Errorf("failed to find available ID: %w", err)
	}

	cmds := [][]string{
		{"dscl", ".", "-create", "/Groups/" + svcName},
		{"dscl", ".", "-create", "/Groups/" + svcName, "PrimaryGroupID", strconv.Itoa(id)},
		{"dscl", ".", "-create", "/Groups/" + svcName, "Password", "*"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to create group: %w", err)
		}
	}

	cmds = [][]string{
		{"dscl", ".", "-create", "/Users/" + svcName},
		{"dscl", ".", "-create", "/Users/" + svcName, "UniqueID", strconv.Itoa(id)},
		{"dscl", ".", "-create", "/Users/" + svcName, "PrimaryGroupID", strconv.Itoa(id)},
		{"dscl", ".", "-create", "/Users/" + svcName, "UserShell", "/usr/bin/false"},
		{"dscl", ".", "-create", "/Users/" + svcName, "NFSHomeDirectory", homeDir},
		{"dscl", ".", "-create", "/Users/" + svcName, "RealName", name + " service account"},
		{"dscl", ".", "-create", "/Users/" + svcName, "IsHidden", "1"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
	}

	return &SystemUser{
		Name:    svcName,
		UID:     id,
		GID:     id,
		Home:    homeDir,
		Shell:   "/usr/bin/false",
		Created: true,
	}, nil
}

// DeleteSystemUser removes the macOS service account and group created by the
// server. Per AI.md PART 23 uninstall: dscl . -delete entries are the inverse
// of the dscl . -create commands.
func DeleteSystemUser(name string) error {
	if !systemUserOwned(name) {
		return nil
	}
	var lastErr error
	for _, args := range macOSAccountDeleteCommands(name) {
		cmd := exec.Command(args[0], args[1:]...)
		if err := cmd.Run(); err != nil {
			lastErr = fmt.Errorf("%s %s: %w", args[0], strings.Join(args[1:], " "), err)
		}
	}
	if lastErr != nil && UserExists(name) {
		return lastErr
	}
	return clearSystemUserOwnership(name)
}

func macOSAccountDeleteCommands(name string) [][]string {
	return [][]string{
		{"dscl", ".", "-delete", "/Users/" + name},
		{"dscl", ".", "-delete", "/Groups/" + name},
	}
}

func migrateLegacyMacOSAccount(name string) error {
	legacyName := "_" + name
	if _, err := exec.Command("dscl", ".", "-read", "/Users/"+legacyName).Output(); err != nil {
		return nil
	}

	markerPath := filepath.Join(config.GetConfigDir(), ".legacy-service-account-migrated")
	if markerData, err := os.ReadFile(markerPath); err == nil && strings.TrimSpace(string(markerData)) == legacyName {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read legacy service-account migration marker: %w", err)
	}

	for _, directory := range []string{config.GetConfigDir(), config.GetDataDir(), config.GetLogDir(), config.GetCacheDir()} {
		if directory == "" {
			continue
		}
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect application directory %s during legacy account migration: %w", directory, err)
		}
		if err := exec.Command("chown", "-R", name+":"+name, directory).Run(); err != nil {
			return fmt.Errorf("re-own %s from legacy service account %s: %w", directory, legacyName, err)
		}
	}

	if err := os.MkdirAll(config.GetConfigDir(), 0755); err != nil {
		return fmt.Errorf("create configuration directory for legacy account migration marker: %w", err)
	}
	if err := os.WriteFile(markerPath, []byte(legacyName+"\n"), 0600); err != nil {
		return fmt.Errorf("write legacy service-account migration marker: %w", err)
	}
	slog.Warn("legacy macOS service account remains; remove it manually when no longer needed", "account", legacyName)
	return nil
}
