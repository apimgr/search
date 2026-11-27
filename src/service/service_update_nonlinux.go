//go:build !linux

package service

// UpdateNeedsManualRestart reports that supported non-Linux service managers
// restart the process after it exits for a staged update.
func (sm *ServiceManager) UpdateNeedsManualRestart() bool {
	return false
}
