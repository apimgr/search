package health

// OverlayState is the public, detail-free state shared by Tor and I2P health
// serializers.
type OverlayState struct {
	Enabled  bool
	Running  bool
	Status   string
	State    string
	Hostname string
	Provider string
}

// TorState maps provider lifecycle facts to the four public status values.
func TorState(enabled, starting, running bool, hostname string) OverlayState {
	switch {
	case running:
		return OverlayState{Enabled: true, Running: true, State: "running", Status: "healthy", Hostname: hostname}
	case starting:
		return OverlayState{Enabled: true, State: "starting", Status: "starting"}
	case enabled:
		return OverlayState{Enabled: true, State: "error", Status: "unavailable"}
	default:
		return OverlayState{State: "disabled", Status: "disabled"}
	}
}

// I2PState maps the full I2P lifecycle into its public four-state status.
func I2PState(state, provider, hostname string) OverlayState {
	switch state {
	case "no_provider":
		return OverlayState{Status: "disabled", State: state, Provider: "none"}
	case "starting", "building":
		return OverlayState{Enabled: true, Status: "starting", State: state, Provider: safeProvider(provider)}
	case "running":
		return OverlayState{Enabled: true, Running: true, Status: "healthy", State: state, Hostname: hostname, Provider: safeProvider(provider)}
	case "error":
		return OverlayState{Enabled: true, Status: "unavailable", State: state, Provider: safeProvider(provider)}
	default:
		return OverlayState{Status: "disabled", State: "disabled", Provider: "none"}
	}
}

func safeProvider(provider string) string {
	switch provider {
	case "i2pd", "sam":
		return provider
	default:
		return ""
	}
}
