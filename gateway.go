package tmhi

import (
	"fmt"

	"github.com/go-resty/resty/v2"
)

// Gateway defines the interface for T-Mobile gateway implementations.
type Gateway interface {
	Login() error
	Reboot() error
	Request(method, path string) (*InfoResult, error)
	Info() (*InfoResult, error)
	Status() (*StatusResult, error)
	Signal() (*SignalResult, error)
}

// GatewayCommon provides shared functionality for gateway implementations.
type GatewayCommon struct {
	client *resty.Client
	config *GatewayConfig
}

// NewGatewayCommon creates a new GatewayCommon with the given configuration.
func NewGatewayCommon(cfg *GatewayConfig) *GatewayCommon {
	client := resty.New()
	client.SetBaseURL("http://" + cfg.IP)
	client.SetTimeout(cfg.Timeout)

	if cfg.Retries > 0 {
		client.SetRetryCount(cfg.Retries)
	}

	if cfg.Debug {
		client.SetDebug(true)
	}

	return &GatewayCommon{
		client: client,
		config: cfg,
	}
}

// CheckWebInterface checks if the gateway web interface is accessible.
func (gc *GatewayCommon) CheckWebInterface() *StatusResult {
	resp, err := gc.client.R().Head("/")

	result := &StatusResult{}
	if err != nil {
		result.Error = fmt.Errorf("send request: %w", err)
		result.WebInterfaceUp = false

		return result
	}

	result.StatusCode = resp.StatusCode()
	result.WebInterfaceUp = resp.IsSuccess()

	return result
}
