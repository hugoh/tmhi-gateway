package tmhi

import "github.com/go-resty/resty/v2"

// Gateway defines the interface for T-Mobile gateway implementations.
type Gateway interface {
	Login() (*LoginResult, error)
	Reboot() error
	Request(method, path string) (*InfoResult, error)
	Info() (*InfoResult, error)
	Status() (*StatusResult, error)
	Signal() (*SignalResult, error)
}

// GatewayCommon provides shared functionality for gateway implementations.
type GatewayCommon struct {
	client        *resty.Client
	config        *GatewayConfig
	authenticated bool
}

// NewGatewayCommon creates a new GatewayCommon with the given configuration.
func NewGatewayCommon(cfg *GatewayConfig) *GatewayCommon {
	gateway := &GatewayCommon{client: resty.New(), config: cfg}

	gateway.client.
		SetBaseURL("http://" + cfg.IP).
		SetDebug(cfg.Debug).
		SetTimeout(cfg.Timeout)

	if cfg.Retries > 0 {
		gateway.client.SetRetryCount(cfg.Retries)
	}

	return gateway
}

// CheckWebInterface checks if the gateway web interface is accessible.
func (gc *GatewayCommon) CheckWebInterface() *StatusResult {
	resp, err := gc.client.R().Head("/")

	result := &StatusResult{}
	if err != nil {
		result.Error = err
		result.WebInterfaceUp = false

		return result
	}

	result.StatusCode = resp.StatusCode()
	result.WebInterfaceUp = resp.IsSuccess()

	return result
}
