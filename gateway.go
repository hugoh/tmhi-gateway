package tmhi

import "github.com/go-resty/resty/v2"

// Gateway defines the interface for T-Mobile gateway implementations.
type Gateway interface {
	NewClient(cfg *GatewayConfig)
	AddCredentials(username, password string)
	Login() (*LoginResult, error)
	Reboot() error
	Request(method, path string) (*InfoResult, error)
	Info() (*InfoResult, error)
	Status() (*StatusResult, error)
	Signal() (*SignalResult, error)
}

// GatewayCommon provides shared functionality for gateway implementations.
type GatewayCommon struct {
	Client        *resty.Client
	Username      string
	Password      string
	Authenticated bool
	config        *GatewayConfig
}

// NewGatewayCommon creates a new GatewayCommon with default client.
func NewGatewayCommon() *GatewayCommon {
	return &GatewayCommon{Client: resty.New()}
}

// NewClient configures the HTTP client for the gateway.
func (gc *GatewayCommon) NewClient(cfg *GatewayConfig) {
	if gc.Client == nil {
		gc.Client = resty.New()
	}

	gc.config = cfg

	gc.Client.
		SetBaseURL("http://" + cfg.IP).
		SetDebug(cfg.Debug).
		SetTimeout(cfg.Timeout)

	if cfg.Retries > 0 {
		gc.Client.SetRetryCount(cfg.Retries)
	}
}

// AddCredentials sets the username and password for gateway authentication.
func (gc *GatewayCommon) AddCredentials(username, password string) {
	gc.Username = username
	gc.Password = password
}

// CheckWebInterface checks if the gateway web interface is accessible.
func (gc *GatewayCommon) CheckWebInterface() *StatusResult {
	resp, err := gc.Client.R().Head("/")

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
