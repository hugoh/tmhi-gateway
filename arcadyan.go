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
	gc.SetHeader("Accept", jsonContentType)

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

	reqBody, err := json.Marshal(bodyMap)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}

	resp, err := a.doRequest(ctx, http.MethodPost, "/TMI/v1/auth/login", reqBody, jsonContentType)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}

	if resp.IsStatusFailure() {
		return NewAuthError(resp.StatusCode(), resp.String(), nil)
	}

	var loginResp struct {
		Auth struct {
			Expiration       int64  `json:"expiration"`
			RefreshCountLeft int    `json:"refreshCountLeft"`
			RefreshCountMax  int    `json:"refreshCountMax"`
			Token            string `json:"token"`
		} `json:"auth"`
	}

	if err := json.Unmarshal(resp.Bytes(), &loginResp); err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}

	if loginResp.Auth.Token == "" {
		return NewAuthError(0, "login response missing auth token", nil)
	}

	a.credentials = arcadianLoginData{
		Expiration: loginResp.Auth.Expiration,
		Token:      loginResp.Auth.Token,
	}
	a.SetAuthToken(a.credentials.Token)

	return nil
}

// Reboot restarts the Arcadyan gateway.
func (a *ArcadyanGateway) Reboot(ctx context.Context) error {
	return a.performReboot(ctx, a, a.Login, func() (*gwResponse, error) {
		return a.doRequest(ctx, http.MethodPost, "/TMI/v1/gateway/reset?set=reboot", nil, "")
	})
}

// Info retrieves gateway information.
func (a *ArcadyanGateway) Info(ctx context.Context) (*InfoResult, error) {
	return a.Request(ctx, "GET", infoURL)
}

// Request makes an HTTP request to the gateway.
func (a *ArcadyanGateway) Request(ctx context.Context, method, path string) (*InfoResult, error) {
	resp, err := a.doRequest(ctx, method, path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("%w: HTTP %d", ErrRequestFailed, resp.StatusCode())
	}

	contentType := resp.Header().Get("Content-Type")
	body := resp.Bytes()

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
func (a *ArcadyanGateway) Status(ctx context.Context) *StatusResult {
	webResult := a.CheckWebInterface(ctx)

	var result struct {
		Signal struct {
			Generic struct { // NOSONAR
				Registration string `json:"registration"`
			} `json:"generic"`
		} `json:"signal"`
	}

	webResult.Registration = "unknown"

	resp, err := a.doRequest(ctx, http.MethodGet, infoURL, nil, "")

	switch {
	case err != nil:
		if webResult.Error == nil {
			webResult.Error = NewGatewayError("status", 0, "failed to get registration status", err)
		}
	case resp.IsStatusFailure():
		if webResult.Error == nil {
			webResult.Error = NewGatewayError(
				"status",
				resp.StatusCode(),
				"failed to get registration status",
				ErrStatusFailed,
			)
		}
	default:
		if err := json.Unmarshal(resp.Bytes(), &result); err != nil {
			if webResult.Error == nil {
				webResult.Error = NewGatewayError(
					"status",
					resp.StatusCode(),
					"failed to get registration status",
					err,
				)
			}
		} else {
			webResult.Registration = result.Signal.Generic.Registration
		}
	}

	return webResult
}

// Signal retrieves signal strength information.
func (a *ArcadyanGateway) Signal(ctx context.Context) (*SignalResult, error) {
	resp, err := a.doRequest(ctx, http.MethodGet, infoURL, nil, "")
	if err != nil {
		return nil, NewGatewayError("signal", 0, "failed to get signal info", err)
	}

	if resp.IsStatusFailure() {
		return nil, NewGatewayError(
			"signal",
			resp.StatusCode(),
			"failed to get signal info",
			ErrSignalFailed,
		)
	}

	var result struct {
		Signal SignalResult `json:"signal"`
	}

	if err := json.Unmarshal(resp.Bytes(), &result); err != nil {
		return nil, NewGatewayError("signal", resp.StatusCode(), "failed to get signal info", err)
	}

	return &result.Signal, nil
}

func (a *ArcadyanGateway) logout() {
	a.credentials = arcadianLoginData{}
	a.SetAuthToken("")
}

func (a *ArcadyanGateway) isLoggedIn() bool {
	deadline := time.Now().Add(expirationMargin).Unix()

	return a.credentials.Token != "" && a.credentials.Expiration > deadline
}
