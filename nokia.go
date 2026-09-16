package tmhi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	nonceParam         = "nonce"
	sidCookieName      = "sid"
	loginWebAppCGI     = "/login_web_app.cgi"
	formURLContentType = "application/x-www-form-urlencoded"
)

type nokiaNonce struct {
	Nonce     string `json:"nonce"`
	Pubkey    string `json:"pubkey"`
	RandomKey string `json:"randomKey"`
}

type nokiaLoginData struct {
	SID       string
	csrfToken string
}

type nokiaLoginResp struct {
	Success   int    `json:"success"`
	Reason    int    `json:"reason"`
	Sid       string `json:"sid"`
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

	nonce, nonceErr := n.getNonce(ctx)
	if nonceErr != nil {
		return fmt.Errorf("error getting nonce: %w", nonceErr)
	}

	loginResp, loginErr := n.getCredentials(ctx, *nonce)
	if loginErr != nil {
		return fmt.Errorf("login failed: %w", loginErr)
	}

	n.credentials.SID = loginResp.Sid
	n.credentials.csrfToken = loginResp.CsrfToken

	return nil
}

// Reboot restarts the Nokia gateway.
func (n *NokiaGateway) Reboot(ctx context.Context) error {
	return n.performReboot(ctx, n, n.Login, func() (*gwResponse, error) {
		form := url.Values{"csrf_token": {n.credentials.csrfToken}}

		//nolint:gosec // Secure/HttpOnly/SameSite only apply to response cookies, not outgoing requests.
		cookie := &http.Cookie{Name: sidCookieName, Value: n.credentials.SID}

		return n.doRequest(
			ctx, http.MethodPost, "/reboot_web_app.cgi",
			[]byte(form.Encode()), formURLContentType, cookie,
		)
	})
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
func (n *NokiaGateway) Status(ctx context.Context) *StatusResult {
	return n.CheckWebInterface(ctx)
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
	nonce nokiaNonce,
) (*nokiaLoginResp, error) {
	// The Nokia gateway normalizes the password to lowercase before verification.
	passHashInput := strings.ToLower(n.config.Password)
	userPassHash := sha256Hash(n.config.Username, passHashInput)
	userPassNonceHash := sha256URL(userPassHash, nonce.Nonce)
	reqParams := map[string]string{
		"userhash":      sha256URL(n.config.Username, nonce.Nonce),
		"RandomKeyhash": sha256URL(nonce.RandomKey, nonce.Nonce),
		"response":      userPassNonceHash,
		nonceParam:      nonce.Nonce,
		"enckey":        random16bytes(),
		"enciv":         random16bytes(),
	}

	form := url.Values{}
	for k, v := range reqParams {
		form.Set(k, v)
	}

	resp, err := n.doRequest(
		ctx,
		http.MethodPost,
		loginWebAppCGI,
		[]byte(form.Encode()),
		formURLContentType,
	)
	if err != nil {
		return nil, NewAuthError(0, "login request failed", err)
	}

	if resp.IsStatusFailure() {
		return nil, NewAuthError(resp.StatusCode(), resp.String(), nil)
	}

	var loginResp nokiaLoginResp
	if err := json.Unmarshal(resp.Bytes(), &loginResp); err != nil {
		return nil, NewAuthError(0, "login request failed", err)
	}

	if !loginResp.hasCredentials() {
		return nil, NewAuthError(0, fmt.Sprintf(
			"no valid credentials returned (success=%d, reason=%d)",
			loginResp.Success, loginResp.Reason,
		), nil)
	}

	return &loginResp, nil
}

func (n *NokiaGateway) getNonce(ctx context.Context) (*nokiaNonce, error) {
	resp, err := n.doRequest(ctx, http.MethodGet, loginWebAppCGI+"?"+nonceParam, nil, "")
	if err != nil {
		return nil, fmt.Errorf("error getting nonce: %w", err)
	}

	if resp.IsStatusFailure() {
		return nil, NewGatewayError("nonce", resp.StatusCode(), resp.String(), ErrAuthentication)
	}

	var result nokiaNonce
	if err := json.Unmarshal(resp.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("error getting nonce: %w", err)
	}

	return &result, nil
}
