//go:build !windows

package service

import "errors"

// ErrWindowsServiceUnsupported is returned by RunAsWindowsService on
// non-Windows platforms, where the Service Control Manager does not exist.
var ErrWindowsServiceUnsupported = errors.New("windows service is only supported on windows")

// IsWindowsService reports whether the process was started by the Windows
// Service Control Manager. On non-Windows platforms the SCM does not exist,
// so this is always false (AI.md PART 24: only Windows runs under the SCM).
func IsWindowsService() bool { return false }

// RunAsWindowsService runs the server under the Windows SCM. It can never be
// called on non-Windows platforms because IsWindowsService is always false;
// the stub exists so the package builds on every platform.
func RunAsWindowsService(run func() error, shutdown func()) error {
	return ErrWindowsServiceUnsupported
}
