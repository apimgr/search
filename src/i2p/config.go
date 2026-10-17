// Package i2p implements the OPTIONAL, opt-in I2P eepsite support described in
// AI.md PART 31.2. Unlike Tor (PART 31.1, auto-enabled when the binary is
// found), nothing here contacts a provider, allocates a port, or writes a file
// unless I2P is explicitly enabled.
package i2p

// I2PProvider identifies which backend created the eepsite.
type I2PProvider int

const (
	// I2PProviderNone means no provider was available (I2P disabled).
	I2PProviderNone I2PProvider = iota
	// I2PProviderI2PD spawns and manages a dedicated i2pd process (Model A).
	I2PProviderI2PD
	// I2PProviderSAM uses an external SAMv3 bridge (Model B).
	I2PProviderSAM
)

// I2PConfig holds I2P-related configuration from server config.
// Per AI.md PART 31.2 "Configuration".
type I2PConfig struct {
	// OPT-IN: I2P eepsite is created only when Enabled is true.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// i2pd binary path (empty = auto-detect). Model A when found.
	Binary string `yaml:"binary" json:"binary"`

	// SAM bridge address for Model B (used only when no i2pd binary is found).
	SAMAddress string `yaml:"sam_address" json:"sam_address"`

	// Virtual port the eepsite listens on: 1-65535, default 80
	VirtualPort int `yaml:"virtual_port" json:"virtual_port"`

	// Tunnel hops: 0-7, default 3
	InboundLength  int `yaml:"inbound_length" json:"inbound_length"`
	OutboundLength int `yaml:"outbound_length" json:"outbound_length"`

	// Parallel tunnels: 1-16, default 5
	InboundQuantity  int `yaml:"inbound_quantity" json:"inbound_quantity"`
	OutboundQuantity int `yaml:"outbound_quantity" json:"outbound_quantity"`

	// SAM/destination signature type (7 = EdDSA-SHA512-Ed25519)
	SignatureType int `yaml:"signature_type" json:"signature_type"`

	// Bootstrap timeout in seconds: 30-600, default 300
	BootstrapTimeout int `yaml:"bootstrap_timeout" json:"bootstrap_timeout"`
}

// DefaultI2PConfig returns the default (disabled) I2P configuration.
func DefaultI2PConfig() I2PConfig {
	return I2PConfig{
		// OPT-IN: default off
		Enabled:          false,
		Binary:           "",
		SAMAddress:       "127.0.0.1:7656",
		VirtualPort:      80,
		InboundLength:    3,
		OutboundLength:   3,
		InboundQuantity:  5,
		OutboundQuantity: 5,
		SignatureType:    7,
		BootstrapTimeout: 300,
	}
}

// Normalize clamps every field to the range AI.md PART 31.2 documents and
// substitutes the documented default for any zero/out-of-range value, so a
// partially-filled server.yml block still produces a usable tunnel definition.
func (c *I2PConfig) Normalize() {
	def := DefaultI2PConfig()

	if c.SAMAddress == "" {
		c.SAMAddress = def.SAMAddress
	}
	if c.VirtualPort < 1 || c.VirtualPort > 65535 {
		c.VirtualPort = def.VirtualPort
	}
	if c.InboundLength < 0 || c.InboundLength > 7 {
		c.InboundLength = def.InboundLength
	}
	if c.OutboundLength < 0 || c.OutboundLength > 7 {
		c.OutboundLength = def.OutboundLength
	}
	if c.InboundQuantity < 1 || c.InboundQuantity > 16 {
		c.InboundQuantity = def.InboundQuantity
	}
	if c.OutboundQuantity < 1 || c.OutboundQuantity > 16 {
		c.OutboundQuantity = def.OutboundQuantity
	}
	if c.SignatureType <= 0 {
		c.SignatureType = def.SignatureType
	}
	if c.BootstrapTimeout < 30 || c.BootstrapTimeout > 600 {
		c.BootstrapTimeout = def.BootstrapTimeout
	}
}

// providerName renders a provider for logs and the --status output.
func providerName(p I2PProvider) string {
	switch p {
	case I2PProviderI2PD:
		return "i2pd"
	case I2PProviderSAM:
		return "sam"
	default:
		return "none"
	}
}
