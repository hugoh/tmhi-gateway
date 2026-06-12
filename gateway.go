package tmhi

import (
	"context"
	"fmt"
	"net"

	"github.com/go-resty/resty/v2"
)

// Gateway defines the interface for T-Mobile gateway implementations.
//
// Implementations are not safe for concurrent use: Login mutates shared
// client state such as auth headers and cookies.
type Gateway interface {
	Login(ctx context.Context) error
	Reboot(ctx context.Context) error
	Request(ctx context.Context, method, path string) (*InfoResult, error)
	Info(ctx context.Context) (*InfoResult, error)
	Status(ctx context.Context) (*StatusResult, error)
	Signal(ctx context.Context) (*SignalResult, error)
}

// GatewayCommon provides shared functionality for gateway implementations.
type GatewayCommon struct {
	client *resty.Client
	config *GatewayConfig
}

// NewGatewayCommon creates a new GatewayCommon with the given configuration.
func NewGatewayCommon(cfg *GatewayConfig) *GatewayCommon {
	host := cfg.Host
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		// Bare IPv6 literals must be bracketed in URLs.
		host = "[" + host + "]"
	}

	client := resty.New()
	client.SetBaseURL("http://" + host)
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
func (gc *GatewayCommon) CheckWebInterface(ctx context.Context) *StatusResult {
	resp, err := gc.client.R().SetContext(ctx).Head("/")

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
