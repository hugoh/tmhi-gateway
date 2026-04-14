package tmhi

import (
	"fmt"
	"strings"
)

type nonceResp struct {
	Nonce     string `json:"nonce"`
	Pubkey    string `json:"pubkey"`
	RandomKey string `json:"randomKey"`
}

type nokiaLoginData struct {
	SID       string
	CSRFToken string
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
func NewNokiaGateway() *NokiaGateway {
	return &NokiaGateway{GatewayCommon: &GatewayCommon{}}
}

func (l *nokiaLoginResp) success() bool {
	return l.Sid != "" && l.CsrfToken != ""
}

// Login authenticates with the Nokia gateway.
func (n *NokiaGateway) Login() (*LoginResult, error) {
	if n.Authenticated {
		return &LoginResult{Success: true}, nil
	}

	nonceResp, nonceErr := n.getNonce()
	if nonceErr != nil {
		return nil, fmt.Errorf("error getting nonce: %w", nonceErr)
	}

	loginResp, loginErr := n.getCredentials(*nonceResp)
	if loginErr != nil {
		return nil, fmt.Errorf("login failed: %w", loginErr)
	}

	n.credentials.SID = loginResp.Sid
	n.credentials.CSRFToken = loginResp.CsrfToken
	n.Authenticated = true
	n.Client.SetHeader("Cookie", "sid="+n.credentials.SID)

	return &LoginResult{
		Success:   true,
		SessionID: loginResp.Sid,
		CSRFToken: loginResp.CsrfToken,
	}, nil
}

// Reboot restarts the Nokia gateway.
func (n *NokiaGateway) Reboot() error {
	if _, err := n.Login(); err != nil {
		return fmt.Errorf("cannot reboot without successful login flow: %w", err)
	}

	formData := map[string]string{
		"csrf_token": n.credentials.CSRFToken,
	}

	if n.config != nil && n.config.DryRun {
		return nil
	}

	req := n.Client.R().SetFormData(formData)

	resp, err := req.Execute("POST", "/reboot_web_app.cgi")
	if err != nil {
		return fmt.Errorf("error sending reboot request: %w", err)
	}

	if resp.IsError() {
		return NewGatewayError("reboot", resp.StatusCode(), resp.String(), ErrRebootFailed)
	}

	return nil
}

// Request is not implemented for Nokia gateway.
func (n *NokiaGateway) Request(_, _ string) (*InfoResult, error) {
	return nil, ErrNotImplemented
}

// Info is not implemented for Nokia gateway.
func (n *NokiaGateway) Info() (*InfoResult, error) {
	return nil, ErrNotImplemented
}

// Status checks the gateway connection status.
func (n *NokiaGateway) Status() (*StatusResult, error) {
	return n.CheckWebInterface(), nil
}

// Signal is not implemented for Nokia gateway.
func (n *NokiaGateway) Signal() (*SignalResult, error) {
	return nil, ErrNotImplemented
}

func (n *NokiaGateway) getCredentials(nonceResp nonceResp) (*nokiaLoginResp, error) {
	passHashInput := strings.ToLower(n.Password)
	userPassHash := Sha256Hash(n.Username, passHashInput)
	userPassNonceHash := Sha256Url(userPassHash, nonceResp.Nonce)
	reqParams := map[string]string{
		"userhash":      Sha256Url(n.Username, nonceResp.Nonce),
		"RandomKeyhash": Sha256Url(nonceResp.RandomKey, nonceResp.Nonce),
		"response":      userPassNonceHash,
		"nonce":         Base64urlEscape(nonceResp.Nonce),
		"enckey":        Random16bytes(),
		"enciv":         Random16bytes(),
	}

	reqURL := "/login_web_app.cgi"

	var loginResp nokiaLoginResp

	resp, err := n.Client.R().
		SetFormData(reqParams).
		SetResult(&loginResp).
		Post(reqURL)
	if err != nil {
		return nil, NewAuthError(0, err.Error())
	}

	if resp.IsError() {
		return nil, NewAuthError(resp.StatusCode(), resp.String())
	}

	var authErr error
	if loginResp.success() {
		authErr = nil
	} else {
		authErr = NewAuthError(0, "no valid credentials returned")
	}

	return &loginResp, authErr
}

func (n *NokiaGateway) getNonce() (*nonceResp, error) {
	var resp nonceResp

	_, err := n.Client.R().
		SetResult(&resp).
		Get("/login_web_app.cgi?nonce")
	if err != nil {
		return nil, fmt.Errorf("error getting nonce: %w", err)
	}

	return &resp, nil
}
