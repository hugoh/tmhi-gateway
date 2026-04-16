package tmhi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strBody(s string) io.ReadCloser { return io.NopCloser(bytes.NewBufferString(s)) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       strBody(body),
	}
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       strBody(body),
	}
}

func newArcadyan(
	client *resty.Client,
	cfg *GatewayConfig,
	token string,
	exp time.Time,
) *ArcadyanGateway {
	ag := &ArcadyanGateway{
		GatewayCommon: &GatewayCommon{
			client: client,
			config: cfg,
		},
	}
	if token != "" {
		ag.credentials = arcadianLoginData{
			Token:      token,
			Expiration: int(exp.Unix()),
		}
	}

	return ag
}

func TestArcadyanGateway_Login_Success(t *testing.T) {
	body := `{"auth":{"expiration":1234567890,"refreshCountLeft":5,"refreshCountMax":10,"token":"testtoken"}}`
	client := NewTestClient(jsonResponse(http.StatusOK, body), nil) //nolint:bodyclose // test mock

	gw := newArcadyan(client, &GatewayConfig{Username: "user", Password: "pass"}, "", time.Time{})

	result, err := gw.Login()
	require.NoError(t, err)
	assert.Equal(t, 1234567890, gw.credentials.Expiration)
	assert.Equal(t, "testtoken", gw.credentials.Token)
	assert.True(t, result.Success)
	assert.Equal(t, "testtoken", result.Token)
}

func TestArcadyanGateway_Reboot_Failure(t *testing.T) {
	//nolint:bodyclose // test mock
	client := NewTestClient(textResponse(http.StatusInternalServerError, "server error"), nil)

	gw := newArcadyan(
		client,
		&GatewayConfig{Username: "user", Password: "pass"},
		"valid-token",
		time.Now().Add(1*time.Hour),
	)

	err := gw.Reboot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reboot failed")
}

func TestArcadyanGateway_Reboot_DryRun(t *testing.T) {
	cfg := &GatewayConfig{Username: "user", Password: "pass", DryRun: true}
	gw := newArcadyan(
		nil,
		cfg,
		"valid-token",
		time.Now().Add(1*time.Hour),
	)

	err := gw.Reboot()
	require.NoError(t, err)
}

func TestArcadyanGateway_Reboot_Success(t *testing.T) {
	//nolint:bodyclose // test mock
	client := NewTestClient(textResponse(http.StatusOK, "reboot initiated"), nil)

	gw := newArcadyan(
		client,
		&GatewayConfig{Username: "user", Password: "pass"},
		"valid-token",
		time.Now().Add(1*time.Hour),
	)

	err := gw.Reboot()
	require.NoError(t, err)
}

func TestArcadyanGateway_Login_Non200Status(t *testing.T) {
	//nolint:bodyclose // test mock
	client := NewTestClient(textResponse(http.StatusUnauthorized, "unauthorized"), nil)

	gw := newArcadyan(client, &GatewayConfig{Username: "user", Password: "pass"}, "", time.Time{})

	_, err := gw.Login()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Contains(t, err.Error(), "401")
	assert.ErrorIs(t, err, ErrAuthentication)
}

func TestArcadyanGateway_Login_InvalidJSON(t *testing.T) {
	//nolint:bodyclose // test mock
	client := NewTestClient(jsonResponse(http.StatusOK, "{invalid json"), nil)

	gw := newArcadyan(client, &GatewayConfig{Username: "user", Password: "pass"}, "", time.Time{})

	_, err := gw.Login()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode login response")
}

func TestArcadyanGateway_Login_HTTPClientError(t *testing.T) {
	client := NewTestClient(nil, errors.New("network error"))

	gw := newArcadyan(client, &GatewayConfig{Username: "user", Password: "pass"}, "", time.Time{})

	_, err := gw.Login()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "login request failed")
}

func TestArcadyanGateway_Info_Success(t *testing.T) {
	//nolint:bodyclose // test mock
	client := NewTestClient(jsonResponse(http.StatusOK, `{"system": {"model": "TEST123"}}`), nil)

	gw := newArcadyan(client, &GatewayConfig{Username: "user", Password: "pass"}, "", time.Time{})

	result, err := gw.Info()
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
}

func TestArcadyanGateway_isLoggedIn(t *testing.T) {
	t.Run("valid login", func(t *testing.T) {
		gw := &ArcadyanGateway{
			GatewayCommon: &GatewayCommon{config: &GatewayConfig{}},
			credentials: arcadianLoginData{
				Expiration: int(time.Now().Add(1 * time.Hour).Unix()),
				Token:      "valid",
			},
		}
		assert.True(t, gw.isLoggedIn())
	})

	t.Run("expired token", func(t *testing.T) {
		gw := &ArcadyanGateway{
			GatewayCommon: &GatewayCommon{config: &GatewayConfig{}},
			credentials: arcadianLoginData{
				Expiration: int(time.Now().Add(-1 * time.Hour).Unix()),
				Token:      "expired",
			},
		}
		assert.False(t, gw.isLoggedIn())
	})

	t.Run("no token", func(t *testing.T) {
		gw := &ArcadyanGateway{GatewayCommon: &GatewayCommon{config: &GatewayConfig{}}}
		assert.False(t, gw.isLoggedIn())
	})
}

func TestNewArcadyanGateway(t *testing.T) {
	cfg := &GatewayConfig{IP: "192.168.1.1"}
	gw := NewArcadyanGateway(cfg)
	assert.NotNil(t, gw)
	assert.NotNil(t, gw.client)
	assert.Equal(t, "application/json", gw.client.Header.Get("Accept"))
}

func TestArcadyanGateway_Status(t *testing.T) {
	t.Run("successful status with registration info", func(t *testing.T) {
		headResp := &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}
		infoBody := `{"signal":{"generic":{"registration":"registered"}}}`
		//nolint:bodyclose // test mock
		infoResp := jsonResponse(http.StatusOK, infoBody)
		client := NewMultiTestClient([]*http.Response{headResp, infoResp}, []error{nil, nil})

		gw := newArcadyan(
			client,
			&GatewayConfig{Username: "user", Password: "pass"},
			"valid-token",
			time.Now().Add(1*time.Hour),
		)

		result, err := gw.Status()
		require.NoError(t, err)
		assert.True(t, result.WebInterfaceUp)
		assert.Equal(t, "registered", result.Registration)
	})

	t.Run("status with network error returns unknown", func(t *testing.T) {
		headResp := &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}
		client := NewMultiTestClient([]*http.Response{headResp}, []error{nil})

		gw := newArcadyan(
			client,
			&GatewayConfig{Username: "user", Password: "pass"},
			"valid-token",
			time.Now().Add(1*time.Hour),
		)

		result, err := gw.Status()
		require.NoError(t, err)
		assert.True(t, result.WebInterfaceUp)
		assert.Error(t, result.Error)
	})
}

func TestArcadyanGateway_Request_Methods(t *testing.T) {
	t.Run("GET request", func(t *testing.T) {
		//nolint:bodyclose // test mock
		client := NewTestClient(jsonResponse(http.StatusOK, `{"status": "ok"}`), nil)

		gw := newArcadyan(client, &GatewayConfig{}, "valid-token", time.Now().Add(1*time.Hour))

		result, err := gw.Request("GET", "/test")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, http.StatusOK, result.StatusCode)
	})

	t.Run("POST request", func(t *testing.T) {
		//nolint:bodyclose // test mock
		client := NewTestClient(jsonResponse(http.StatusOK, `{"status": "created"}`), nil)

		gw := newArcadyan(
			client,
			&GatewayConfig{Username: "user", Password: "pass"},
			"valid-token",
			time.Now().Add(1*time.Hour),
		)

		result, err := gw.Request("POST", "/test")
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("non-JSON response", func(t *testing.T) {
		//nolint:bodyclose // test mock
		client := NewTestClient(textResponse(http.StatusOK, "plain text response"), nil)

		gw := newArcadyan(client, &GatewayConfig{}, "valid-token", time.Now().Add(1*time.Hour))

		result, err := gw.Request("GET", "/test")
		require.NoError(t, err)
		assert.Equal(t, "text/plain", result.ContentType)
	})

	t.Run("empty response", func(t *testing.T) {
		//nolint:bodyclose // test mock
		client := NewTestClient(textResponse(http.StatusNoContent, ""), nil)

		gw := newArcadyan(client, &GatewayConfig{}, "valid-token", time.Now().Add(1*time.Hour))

		result, err := gw.Request("GET", "/test")
		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, result.StatusCode)
	})
}

func TestArcadyanGateway_Signal(t *testing.T) {
	t.Run("successful signal retrieval with 4g and 5g", func(t *testing.T) {
		body := `{
			"signal": {
				"4g": {
					"bands": ["b2"],
					"bars": 4.0,
					"cid": 12,
					"eNBID": 310463,
					"rsrp": -95,
					"rsrq": -8,
					"rssi": -85,
					"sinr": 15
				},
				"5g": {
					"antennaUsed": "Internal_directional",
					"bands": ["n41"],
					"bars": 5.0,
					"cid": 311,
					"gNBID": 1076984,
					"rsrp": -84,
					"rsrq": -10,
					"rssi": -72,
					"sinr": 28
				},
				"generic": {
					"apn": "FBB.HOME",
					"hasIPv6": true,
					"registration": "registered",
					"roaming": false
				}
			}
		}`
		//nolint:bodyclose // test mock
		client := NewTestClient(jsonResponse(http.StatusOK, body), nil)

		gw := newArcadyan(
			client,
			&GatewayConfig{Username: "user", Password: "pass"},
			"valid-token",
			time.Now().Add(1*time.Hour),
		)

		result, err := gw.Signal()
		require.NoError(t, err)
		require.NotNil(t, result)

		// Check 4G
		require.NotNil(t, result.FourG)
		assert.Equal(t, []string{"b2"}, result.FourG.Bands)
		assert.InEpsilon(t, 4.0, result.FourG.Bars, 0.001)
		assert.Equal(t, 12, result.FourG.CID)
		assert.Equal(t, 310463, result.FourG.ENBID)
		assert.Equal(t, -95, result.FourG.RSRP)
		assert.Equal(t, -8, result.FourG.RSRQ)
		assert.Equal(t, -85, result.FourG.RSSI)
		assert.Equal(t, 15, result.FourG.SINR)

		// Check 5G
		require.NotNil(t, result.FiveG)
		assert.Equal(t, "Internal_directional", result.FiveG.AntennaUsed)
		assert.Equal(t, []string{"n41"}, result.FiveG.Bands)
		assert.InEpsilon(t, 5.0, result.FiveG.Bars, 0.001)
		assert.Equal(t, 311, result.FiveG.CID)
		assert.Equal(t, 1076984, result.FiveG.GNBID)
		assert.Equal(t, -84, result.FiveG.RSRP)
		assert.Equal(t, -10, result.FiveG.RSRQ)
		assert.Equal(t, -72, result.FiveG.RSSI)
		assert.Equal(t, 28, result.FiveG.SINR)

		// Check generic
		assert.Equal(t, "FBB.HOME", result.Generic.APN)
		assert.True(t, result.Generic.HasIPv6)
		assert.Equal(t, "registered", result.Generic.Registration)
		assert.False(t, result.Generic.Roaming)
	})

	t.Run("successful signal retrieval 5g only", func(t *testing.T) {
		body := `{
			"signal": {
				"5g": {
					"antennaUsed": "",
					"bands": ["n41"],
					"bars": 5.0,
					"cid": 311,
					"gNBID": 1076984,
					"rsrp": -84,
					"rsrq": -10,
					"rssi": -72,
					"sinr": 28
				},
				"generic": {
					"apn": "FBB.HOME",
					"hasIPv6": true,
					"registration": "registered",
					"roaming": false
				}
			}
		}`
		//nolint:bodyclose // test mock
		client := NewTestClient(jsonResponse(http.StatusOK, body), nil)

		gw := newArcadyan(
			client,
			&GatewayConfig{Username: "user", Password: "pass"},
			"valid-token",
			time.Now().Add(1*time.Hour),
		)

		result, err := gw.Signal()
		require.NoError(t, err)
		assert.Nil(t, result.FourG)
		require.NotNil(t, result.FiveG)
		assert.Equal(t, []string{"n41"}, result.FiveG.Bands)
		assert.InEpsilon(t, 5.0, result.FiveG.Bars, 0.001)
	})

	t.Run("successful signal retrieval 4g only", func(t *testing.T) {
		body := `{
			"signal": {
				"4g": {
					"bands": ["b2"],
					"bars": 4.0,
					"cid": 12,
					"eNBID": 310463,
					"rsrp": -95,
					"rsrq": -8,
					"rssi": -85,
					"sinr": 15
				},
				"generic": {
					"apn": "FBB.HOME",
					"hasIPv6": true,
					"registration": "registered",
					"roaming": false
				}
			}
		}`
		//nolint:bodyclose // test mock
		client := NewTestClient(jsonResponse(http.StatusOK, body), nil)

		gw := newArcadyan(
			client,
			&GatewayConfig{Username: "user", Password: "pass"},
			"valid-token",
			time.Now().Add(1*time.Hour),
		)

		result, err := gw.Signal()
		require.NoError(t, err)
		require.NotNil(t, result.FourG)
		assert.Equal(t, 310463, result.FourG.ENBID)
		assert.Nil(t, result.FiveG)
	})

	t.Run("signal with network error", func(t *testing.T) {
		client := NewTestClient(nil, errors.New("network error"))

		gw := newArcadyan(
			client,
			&GatewayConfig{Username: "user", Password: "pass"},
			"valid-token",
			time.Now().Add(1*time.Hour),
		)

		_, err := gw.Signal()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get signal info")
	})

	t.Run("signal with non-200 status", func(t *testing.T) {
		//nolint:bodyclose // test mock
		client := NewTestClient(jsonResponse(http.StatusInternalServerError, "{}"), nil)

		gw := newArcadyan(
			client,
			&GatewayConfig{Username: "user", Password: "pass"},
			"valid-token",
			time.Now().Add(1*time.Hour),
		)

		_, err := gw.Signal()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signal failed")
	})
}

func TestInfoResult_String(t *testing.T) {
	t.Run("pretty prints JSON content", func(t *testing.T) {
		result := &InfoResult{
			Raw:         []byte(`{"name":"test","value":123}`),
			ContentType: "application/json",
		}

		expected := "{\n \"name\": \"test\",\n \"value\": 123\n}"
		assert.Equal(t, expected, result.String())
	})

	t.Run("returns raw content for non-JSON", func(t *testing.T) {
		result := &InfoResult{
			Raw:         []byte("plain text response"),
			ContentType: "text/plain",
		}

		assert.Equal(t, "plain text response", result.String())
	})

	t.Run("handles JSON with charset", func(t *testing.T) {
		result := &InfoResult{
			Raw:         []byte(`{"status":"ok"}`),
			ContentType: "application/json; charset=utf-8",
		}

		expected := "{\n \"status\": \"ok\"\n}"
		assert.Equal(t, expected, result.String())
	})

	t.Run("returns raw on invalid JSON", func(t *testing.T) {
		result := &InfoResult{
			Raw:         []byte("{invalid json"),
			ContentType: "application/json",
		}

		assert.Equal(t, "{invalid json", result.String())
	})

	t.Run("handles empty content", func(t *testing.T) {
		result := &InfoResult{
			Raw:         []byte{},
			ContentType: "application/json",
		}

		assert.Empty(t, result.String())
	})
}
