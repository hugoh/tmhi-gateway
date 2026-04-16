// Package tmhi provides gateway implementations for T-Mobile Home Internet devices.
package tmhi

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// InfoURL is the endpoint for gateway information.
const InfoURL = "/TMI/v1/gateway/?get=all"

// ArcadyanGateway implements Gateway for Arcadyan-based T-Mobile gateways.
type ArcadyanGateway struct {
	*GatewayCommon

	credentials arcadianLoginData
}

type arcadianLoginData struct {
	Expiration int
	Token      string
}

// NewArcadyanGateway creates a new Arcadyan gateway instance.
func NewArcadyanGateway(cfg *GatewayConfig) *ArcadyanGateway {
	gc := NewGatewayCommon(cfg)
	gc.client.SetHeader("Accept", "application/json")

	return &ArcadyanGateway{GatewayCommon: gc}
}

// Login authenticates with the Arcadyan gateway.
func (a *ArcadyanGateway) Login() (*LoginResult, error) {
	if a.isLoggedIn() {
		return &LoginResult{Success: true, Token: a.credentials.Token}, nil
	}

	bodyMap := map[string]string{
		"username": a.config.Username,
		"password": a.config.Password,
	}

	reqPath := "/TMI/v1/auth/login"

	var loginResp struct {
		Auth struct {
			Expiration       int
			RefreshCountLeft int
			RefreshCountMax  int
			Token            string
		}
	}

	resp, err := a.client.R().
		SetBody(bodyMap).
		SetResult(&loginResp).
		Post(reqPath)
	if err != nil {
		return nil, fmt.Errorf("login request failed: failed to decode login response: %w", err)
	}

	if resp.IsError() {
		return nil, NewAuthError(resp.StatusCode(), resp.String())
	}

	if loginResp.Auth.Token == "" {
		return nil, NewAuthError(0, "login response missing auth token")
	}

	a.credentials = arcadianLoginData{
		Expiration: loginResp.Auth.Expiration,
		Token:      loginResp.Auth.Token,
	}
	a.client.SetAuthToken(a.credentials.Token)
	a.authenticated = true

	return &LoginResult{
		Success:    true,
		Token:      loginResp.Auth.Token,
		Expiration: loginResp.Auth.Expiration,
	}, nil
}

// Reboot restarts the Arcadyan gateway.
func (a *ArcadyanGateway) Reboot() error {
	_, err := a.Login()
	if err != nil {
		return fmt.Errorf("cannot reboot without successful login flow: %w", err)
	}

	if a.config.DryRun {
		return nil
	}

	rebootRequestPath := "/TMI/v1/gateway/reset?set=reboot"

	resp, err := a.client.R().Post(rebootRequestPath)
	if err != nil {
		return fmt.Errorf("reboot request failed: %w", err)
	}

	if !resp.IsSuccess() {
		return NewGatewayError("reboot", resp.StatusCode(), resp.String(), ErrRebootFailed)
	}

	return nil
}

// Info retrieves gateway information.
func (a *ArcadyanGateway) Info() (*InfoResult, error) {
	return a.Request("GET", InfoURL)
}

// Request makes an HTTP request to the gateway.
func (a *ArcadyanGateway) Request(method, path string) (*InfoResult, error) {
	resp, err := a.client.R().Execute(method, path)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	contentType := resp.Header().Get("Content-Type")

	var data map[string]any
	if strings.HasPrefix(contentType, "application/json") {
		if err := json.Unmarshal(resp.Body(), &data); err != nil {
			return nil, fmt.Errorf("json unmarshal failed: %w", err)
		}
	}

	return &InfoResult{
		Data:        data,
		Raw:         resp.Body(),
		ContentType: contentType,
		StatusCode:  resp.StatusCode(),
	}, nil
}

// Status checks the gateway connection status.
func (a *ArcadyanGateway) Status() (*StatusResult, error) {
	webResult := a.CheckWebInterface()

	var result struct {
		Signal struct {
			Generic struct {
				Registration string
			}
		}
	}

	info, err := a.client.R().SetResult(&result).Get(InfoURL)
	if err != nil {
		return &StatusResult{
			WebInterfaceUp: webResult.WebInterfaceUp,
			StatusCode:     webResult.StatusCode,
			Error:          NewGatewayError("status", 0, "failed to get registration status", err),
		}, nil
	}

	regStatus := "unknown"
	if info.IsSuccess() {
		regStatus = result.Signal.Generic.Registration
	}

	webResult.Registration = regStatus
	if !info.IsSuccess() {
		webResult.Error = NewGatewayError(
			"status",
			info.StatusCode(),
			ErrSignalFailed.Error(),
			ErrSignalFailed,
		)
	}

	return webResult, nil
}

// Signal retrieves signal strength information.
func (a *ArcadyanGateway) Signal() (*SignalResult, error) {
	var result struct {
		Signal signalResult `json:"signal"`
	}

	info, err := a.client.R().SetResult(&result).Get(InfoURL)
	if err != nil {
		return nil, NewGatewayError("signal", 0, "failed to get signal info", err)
	}

	if !info.IsSuccess() {
		return nil, NewGatewayError(
			"signal",
			info.StatusCode(),
			ErrSignalFailed.Error(),
			ErrSignalFailed,
		)
	}

	return convertSignalResult(result.Signal), nil
}

type signalResult struct {
	FourG   *fourGSignal `json:"4g"`
	FiveG   *fiveGSignal `json:"5g"`
	Generic struct {
		APN          string `json:"apn"`
		HasIPv6      bool   `json:"hasIPv6"`
		Registration string `json:"registration"`
		Roaming      bool   `json:"roaming"`
	} `json:"generic"`
}

type fourGSignal struct {
	signalData

	ENBID int `json:"eNBID"` //nolint:tagliatelle
}

type fiveGSignal struct {
	signalData

	AntennaUsed string `json:"antennaUsed"`
	GNBID       int    `json:"gNBID"` //nolint:tagliatelle
}

type signalData struct {
	Bands []string `json:"bands"`
	Bars  float64  `json:"bars"`
	CID   int      `json:"cid"`
	RSRP  int      `json:"rsrp"`
	RSRQ  int      `json:"rsrq"`
	RSSI  int      `json:"rssi"`
	SINR  int      `json:"sinr"`
}

func convertSignalResult(sig signalResult) *SignalResult {
	result := &SignalResult{
		Generic: GenericSignalInfo{
			APN:          sig.Generic.APN,
			HasIPv6:      sig.Generic.HasIPv6,
			Registration: sig.Generic.Registration,
			Roaming:      sig.Generic.Roaming,
		},
	}

	if sig.FourG != nil {
		result.FourG = &FourGSignal{
			SignalData: SignalData{
				Bands: sig.FourG.Bands,
				Bars:  sig.FourG.Bars,
				CID:   sig.FourG.CID,
				RSRP:  sig.FourG.RSRP,
				RSRQ:  sig.FourG.RSRQ,
				RSSI:  sig.FourG.RSSI,
				SINR:  sig.FourG.SINR,
			},
			ENBID: sig.FourG.ENBID,
		}
	}

	if sig.FiveG != nil {
		result.FiveG = &FiveGSignal{
			SignalData: SignalData{
				Bands: sig.FiveG.Bands,
				Bars:  sig.FiveG.Bars,
				CID:   sig.FiveG.CID,
				RSRP:  sig.FiveG.RSRP,
				RSRQ:  sig.FiveG.RSRQ,
				RSSI:  sig.FiveG.RSSI,
				SINR:  sig.FiveG.SINR,
			},
			AntennaUsed: sig.FiveG.AntennaUsed,
			GNBID:       sig.FiveG.GNBID,
		}
	}

	return result
}

func (a *ArcadyanGateway) isLoggedIn() bool {
	now := int(time.Now().Unix())

	return a.credentials.Token != "" && a.credentials.Expiration > now
}
