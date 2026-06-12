package tmhi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

const nonceParam = "nonce"

type nonceResp struct {
	Nonce     string
	Pubkey    string
	RandomKey string
}

type nokiaLoginData struct {
	SID       string
	csrfToken string
}

type nokiaLoginResp struct {
	Success   int
	Reason    int
	Sid       string
	CsrfToken string `json:"token"`
}

// NokiaGateway implements Gateway for Nokia-based T-Mobile gateways.
type NokiaGateway struct {
	*GatewayCommon

	credentials nokiaLoginData
}

// NewNokiaGateway creates a new Nokia gateway instance.
func NewNokiaGateway(cfg *GatewayConfig) *NokiaGateway {
	return &NokiaGateway{GatewayCommon: NewGatewayCommon(cfg)}
}

func (l *nokiaLoginResp) hasCredentials() bool {
	return l.Sid != "" && l.CsrfToken != ""
}

// Login authenticates with the Nokia gateway.
func (n *NokiaGateway) Login(ctx context.Context) error {
	if n.isLoggedIn() {
		return nil
	}

	nonceResp, nonceErr := n.getNonce(ctx)
	if nonceErr != nil {
		return fmt.Errorf("error getting nonce: %w", nonceErr)
	}

	loginResp, loginErr := n.getCredentials(ctx, *nonceResp)
	if loginErr != nil {
		return fmt.Errorf("login failed: %w", loginErr)
	}

	n.credentials.SID = loginResp.Sid
	n.credentials.csrfToken = loginResp.CsrfToken
	n.client.SetHeader("Cookie", "sid="+n.credentials.SID)

	return nil
}

// Reboot restarts the Nokia gateway.
func (n *NokiaGateway) Reboot(ctx context.Context) error {
	if err := n.Login(ctx); err != nil {
		return fmt.Errorf("cannot reboot without successful login flow: %w", err)
	}

	formData := map[string]string{
		"csrf_token": n.credentials.csrfToken,
	}

	if n.config.DryRun {
		return nil
	}

	resp, err := n.client.R().SetContext(ctx).SetFormData(formData).Post("/reboot_web_app.cgi")
	if err != nil {
		return fmt.Errorf("error sending reboot request: %w", err)
	}

	if resp.IsError() {
		status := resp.StatusCode()
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			n.logout()
		}

		return NewGatewayError("reboot", status, resp.String(), ErrRebootFailed)
	}

	// A successful reboot invalidates the session on the gateway side.
	n.logout()

	return nil
}

// Request is not implemented for Nokia gateway.
func (*NokiaGateway) Request(_ context.Context, _, _ string) (*InfoResult, error) {
	return nil, ErrNotImplemented
}

// Info is not implemented for Nokia gateway.
func (*NokiaGateway) Info(_ context.Context) (*InfoResult, error) {
	return nil, ErrNotImplemented
}

// Status checks the gateway connection status.
func (n *NokiaGateway) Status(ctx context.Context) (*StatusResult, error) {
	return n.CheckWebInterface(ctx), nil
}

// Signal is not implemented for Nokia gateway.
func (*NokiaGateway) Signal(_ context.Context) (*SignalResult, error) {
	return nil, ErrNotImplemented
}

func (n *NokiaGateway) isLoggedIn() bool {
	return n.credentials.SID != "" && n.credentials.csrfToken != ""
}

func (n *NokiaGateway) logout() {
	n.credentials = nokiaLoginData{}
}

func (n *NokiaGateway) getCredentials(
	ctx context.Context,
	nonceResp nonceResp,
) (*nokiaLoginResp, error) {
	passHashInput := strings.ToLower(n.config.Password)
	userPassHash := sha256Hash(n.config.Username, passHashInput)
	userPassNonceHash := sha256URL(userPassHash, nonceResp.Nonce)
	reqParams := map[string]string{
		"userhash":      sha256URL(n.config.Username, nonceResp.Nonce),
		"RandomKeyhash": sha256URL(nonceResp.RandomKey, nonceResp.Nonce),
		"response":      userPassNonceHash,
		nonceParam:      base64urlEscape(nonceResp.Nonce),
		"enckey":        random16bytes(),
		"enciv":         random16bytes(),
	}

	reqURL := "/login_web_app.cgi"

	var loginResp nokiaLoginResp

	resp, err := n.client.R().
		SetContext(ctx).
		SetResult(&loginResp).
		SetFormData(reqParams).
		Post(reqURL)
	if err != nil {
		return nil, NewAuthError(0, "login request failed", err)
	}

	if resp.IsError() {
		return nil, NewAuthError(resp.StatusCode(), resp.String(), nil)
	}

	var authErr error
	if loginResp.hasCredentials() {
		authErr = nil
	} else {
		authErr = NewAuthError(0, "no valid credentials returned", nil)
	}

	return &loginResp, authErr
}

func (n *NokiaGateway) getNonce(ctx context.Context) (*nonceResp, error) {
	var result nonceResp

	resp, err := n.client.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/login_web_app.cgi?" + nonceParam)
	if err != nil {
		return nil, fmt.Errorf("error getting nonce: %w", err)
	}

	if resp.IsError() {
		return nil, NewGatewayError("nonce", resp.StatusCode(), resp.String(), ErrAuthentication)
	}

	return &result, nil
}
