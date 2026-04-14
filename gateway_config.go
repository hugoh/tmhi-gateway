package tmhi

import "time"

// GatewayConfig holds configuration values needed for gateway client operations.
type GatewayConfig struct {
	IP       string
	Username string
	Password string
	Timeout  time.Duration
	Retries  int
	DryRun   bool
	Debug    bool
}
