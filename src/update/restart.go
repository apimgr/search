package update

// Restart applies an installed update by restarting the updated binary.
// Per AI.md PART 22 "Service-Aware Update": a supervised process is restarted
// through its service manager, an unsupervised process re-executes itself.
func Restart() error {
	if ServiceManaged() {
		return restartService()
	}
	return restartSelf()
}

// RestartService restarts the installed system service so it picks up the new
// binary. It returns an error when no service is installed or the service
// manager refuses the restart.
func RestartService() error {
	return restartService()
}

// RestartSelf re-executes the current process with the same arguments and
// environment so the replaced binary takes over.
func RestartSelf() error {
	return restartSelf()
}

// ServiceManaged reports whether this process was started by a service manager
// and must therefore be restarted through it rather than re-executed.
func ServiceManaged() bool {
	return serviceManaged()
}

// ServiceInstalled reports whether a service unit for this project exists on
// the host, which is what makes RestartService meaningful from the CLI.
func ServiceInstalled() bool {
	return serviceInstalled()
}
