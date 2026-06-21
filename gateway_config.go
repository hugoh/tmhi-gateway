package tmhi

import "time"

// GatewayConfig holds configuration values needed for gateway client operations.
type GatewayConfig struct {
	// Host is the gateway address: a hostname, IPv4/IPv6 literal, or
	// host:port.
	Host     string
	Username string
	Password string
	Timeout  time.Duration
	Retries  int
	DryRun   bool
	Debug    bool
	// UserAgent overrides the User-Agent header sent with every request.
	// Defaults to "tmhi-gateway/v2" when empty.
	UserAgent string
}
