// Package tmhi provides gateway communication for TMHI modems.
package tmhi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// infoURL is the endpoint for gateway information.
const (
	infoURL         = "/TMI/v1/gateway/?get=all"
	jsonContentType = "application/json"
)

// ArcadyanGateway implements Gateway for Arcadyan-based T-Mobile gateways.
type ArcadyanGateway struct {
	*GatewayCommon

	credentials arcadianLoginData
}

type arcadianLoginData struct {
	Expiration int64
	Token      string
}

// expirationMargin re-authenticates slightly before token expiry so a
// token cannot lapse mid-request.
const expirationMargin = 30 * time.Second

// NewArcadyanGateway creates a new Arcadyan gateway instance.
func NewArcadyanGateway(cfg *GatewayConfig) *ArcadyanGateway {
	gc := NewGatewayCommon(cfg)
	gc.client.SetHeader("Accept", "application/json")

	return &ArcadyanGateway{GatewayCommon: gc}
}

// Login authenticates with the Arcadyan gateway.
func (a *ArcadyanGateway) Login(ctx context.Context) error {
	if a.isLoggedIn() {
		return nil
	}

	bodyMap := map[string]string{
		"username": a.config.Username,
		"password": a.config.Password,
	}

	reqPath := "/TMI/v1/auth/login"

	var loginResp struct {
		Auth struct {
			Expiration       int64
			RefreshCountLeft int
			RefreshCountMax  int
			Token            string
		}
	}

	resp, err := a.client.R().SetContext(ctx).SetResult(&loginResp).SetBody(bodyMap).Post(reqPath)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}

	if resp.IsError() {
		return NewAuthError(resp.StatusCode(), resp.String(), nil)
	}

	if loginResp.Auth.Token == "" {
		return NewAuthError(0, "login response missing auth token", nil)
	}

	a.credentials = arcadianLoginData{
		Expiration: loginResp.Auth.Expiration,
		Token:      loginResp.Auth.Token,
	}
	a.client.SetAuthToken(a.credentials.Token)

	return nil
}

// Reboot restarts the Arcadyan gateway.
func (a *ArcadyanGateway) Reboot(ctx context.Context) error {
	err := a.Login(ctx)
	if err != nil {
		return fmt.Errorf("cannot reboot without successful login flow: %w", err)
	}

	if a.config.DryRun {
		return nil
	}

	rebootRequestPath := "/TMI/v1/gateway/reset?set=reboot"

	resp, err := a.client.R().SetContext(ctx).Post(rebootRequestPath)
	if err != nil {
		return fmt.Errorf("reboot request failed: %w", err)
	}

	if resp.IsError() {
		status := resp.StatusCode()
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			a.logout()
		}

		return NewGatewayError("reboot", status, resp.String(), ErrRebootFailed)
	}

	// A successful reboot invalidates the session on the gateway side.
	a.logout()

	return nil
}

// Info retrieves gateway information.
func (a *ArcadyanGateway) Info(ctx context.Context) (*InfoResult, error) {
	return a.Request(ctx, "GET", infoURL)
}

// Request makes an HTTP request to the gateway.
func (a *ArcadyanGateway) Request(ctx context.Context, method, path string) (*InfoResult, error) {
	resp, err := a.client.R().SetContext(ctx).Execute(method, path)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("request failed: HTTP %d", resp.StatusCode())
	}

	contentType := resp.Header().Get("Content-Type")
	body := resp.Body()

	var data map[string]any
	if strings.HasPrefix(contentType, jsonContentType) {
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("json unmarshal failed: %w", err)
		}
	}

	return &InfoResult{
		Data:        data,
		Raw:         body,
		ContentType: contentType,
		StatusCode:  resp.StatusCode(),
	}, nil
}

// Status checks the gateway connection status.
func (a *ArcadyanGateway) Status(ctx context.Context) (*StatusResult, error) {
	webResult := a.CheckWebInterface(ctx)
	webResult.Registration = "unknown"

	if err := a.Login(ctx); err != nil {
		if webResult.Error == nil {
			webResult.Error = fmt.Errorf("login: %w", err)
		}

		return webResult, nil
	}

	var result struct {
		Signal struct {
			Generic struct { // NOSONAR
				Registration string
			}
		}
	}

	resp, err := a.client.R().SetContext(ctx).SetResult(&result).Get(infoURL)

	switch {
	case err != nil:
		if webResult.Error == nil {
			webResult.Error = NewGatewayError("status", 0, "failed to get registration status", err)
		}
	case resp.IsError():
		if webResult.Error == nil {
			webResult.Error = NewGatewayError(
				"status",
				resp.StatusCode(),
				"failed to get registration status",
				ErrSignalFailed,
			)
		}
	default:
		webResult.Registration = result.Signal.Generic.Registration
	}

	return webResult, nil
}

// Signal retrieves signal strength information.
func (a *ArcadyanGateway) Signal(ctx context.Context) (*SignalResult, error) {
	if err := a.Login(ctx); err != nil {
		return nil, fmt.Errorf("cannot get signal without successful login flow: %w", err)
	}

	var result struct {
		Signal SignalResult
	}

	resp, err := a.client.R().SetContext(ctx).SetResult(&result).Get(infoURL)
	if err != nil {
		return nil, NewGatewayError("signal", 0, "failed to get signal info", err)
	}

	if resp.IsError() {
		return nil, NewGatewayError(
			"signal",
			resp.StatusCode(),
			"failed to get signal info",
			ErrSignalFailed,
		)
	}

	return &result.Signal, nil
}

func (a *ArcadyanGateway) logout() {
	a.credentials = arcadianLoginData{}
	a.client.SetAuthToken("")
}

func (a *ArcadyanGateway) isLoggedIn() bool {
	deadline := time.Now().Add(expirationMargin).Unix()

	return a.credentials.Token != "" && a.credentials.Expiration > deadline
}
