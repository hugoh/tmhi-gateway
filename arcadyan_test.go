package tmhi

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newArcadyan(gc *GatewayCommon, token string, exp time.Time) *ArcadyanGateway {
	gc.client.SetHeader("Accept", "application/json")

	ag := &ArcadyanGateway{
		GatewayCommon: gc,
	}
	if token != "" {
		ag.credentials = arcadianLoginData{
			Token:      token,
			Expiration: exp.Unix(),
		}
	}

	return ag
}

func TestArcadyanGateway_Login_Success(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/TMI/v1/auth/login", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(
			`{"auth":{"expiration":1234567890,"refreshCountLeft":5,"refreshCountMax":10,"token":"testtoken"}}`,
		))
	})

	gw := newArcadyan(testCommon(ts), "", time.Time{})
	gw.config = testConfig(ts)

	err := gw.Login(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1234567890), gw.credentials.Expiration)
	assert.Equal(t, "testtoken", gw.credentials.Token)
}

func TestArcadyanGateway_Reboot_Failure(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/TMI/v1/gateway/reset", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	})

	gw := newArcadyan(
		testCommon(ts),
		"valid-token",
		time.Now().Add(1*time.Hour),
	)
	gw.config = testConfig(ts)

	err := gw.Reboot(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reboot failed")
}

func TestArcadyanGateway_Reboot_DryRun(t *testing.T) {
	cfg := &GatewayConfig{Username: testUsername, Password: testPassword, DryRun: true}
	gc := &GatewayCommon{
		client: resty.NewWithClient(&http.Client{}).SetBaseURL("http://" + cfg.Host),
		config: cfg,
	}
	gw := newArcadyan(gc, "valid-token", time.Now().Add(1*time.Hour))

	err := gw.Reboot(t.Context())
	require.NoError(t, err)
}

func TestArcadyanGateway_Reboot_Success(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/TMI/v1/gateway/reset", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	gw := newArcadyan(
		testCommon(ts),
		"valid-token",
		time.Now().Add(1*time.Hour),
	)
	gw.config = testConfig(ts)

	err := gw.Reboot(t.Context())
	require.NoError(t, err)
	assert.False(t, gw.isLoggedIn(), "successful reboot should invalidate the cached session")
}

func TestArcadyanGateway_Reboot_AuthRejection_ClearsCredentials(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{name: "unauthorized", status: http.StatusUnauthorized},
		{name: "forbidden", status: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t, textResponder(tc.status, "auth error"))

			gw := newArcadyan(testCommon(ts), "valid-token", time.Now().Add(1*time.Hour))
			gw.config = testConfig(ts)

			err := gw.Reboot(t.Context())
			require.ErrorIs(t, err, ErrRebootFailed)
			assert.False(t, gw.isLoggedIn(), "auth rejection should clear cached credentials")
		})
	}
}

func TestArcadyanGateway_Login_Errors(t *testing.T) {
	cases := []struct {
		name          string
		setup         func(t *testing.T) *ArcadyanGateway
		errorContains []string
		errorIs       error
	}{
		{
			name: "non-200 status",
			setup: func(t *testing.T) *ArcadyanGateway {
				t.Helper()
				ts := newTestServer(t, textResponder(http.StatusUnauthorized, "unauthorized"))
				gw := newArcadyan(testCommon(ts), "", time.Time{})
				gw.config = testConfig(ts)

				return gw
			},
			errorContains: []string{"authentication failed", "401"},
			errorIs:       ErrAuthentication,
		},
		{
			name: "invalid JSON",
			setup: func(t *testing.T) *ArcadyanGateway {
				t.Helper()
				ts := newTestServer(t, jsonResponder(http.StatusOK, "{invalid json"))
				gw := newArcadyan(testCommon(ts), "", time.Time{})
				gw.config = testConfig(ts)

				return gw
			},
			errorContains: []string{"login request failed", "invalid character"},
		},
		{
			name: "HTTP client error",
			setup: func(t *testing.T) *ArcadyanGateway {
				t.Helper()
				ts := newTestServer(t, http.HandlerFunc(
					func(_ http.ResponseWriter, _ *http.Request) {},
				))
				ts.Close()

				gw := newArcadyan(
					&GatewayCommon{
						client: resty.NewWithClient(&http.Client{}).SetBaseURL(ts.URL),
						config: testConfigNoCreds(ts),
					},
					"",
					time.Time{},
				)
				gw.config.Username = testUsername
				gw.config.Password = testPassword

				return gw
			},
			errorContains: []string{"login request failed"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := tc.setup(t)

			err := gw.Login(t.Context())
			require.Error(t, err)

			for _, msg := range tc.errorContains {
				assert.Contains(t, err.Error(), msg)
			}

			if tc.errorIs != nil {
				assert.ErrorIs(t, err, tc.errorIs)
			}
		})
	}
}

func TestArcadyanGateway_Info_Success(t *testing.T) {
	ts := newTestServer(t, jsonResponder(http.StatusOK, `{"system": {"model": "TEST123"}}`))

	gw := newArcadyan(testCommon(ts), "", time.Time{})
	gw.config = testConfig(ts)

	result, err := gw.Info(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
}

func TestArcadyanGateway_isLoggedIn(t *testing.T) {
	t.Run("valid login", func(t *testing.T) {
		gw := &ArcadyanGateway{
			GatewayCommon: &GatewayCommon{config: &GatewayConfig{}},
			credentials: arcadianLoginData{
				Expiration: time.Now().Add(1 * time.Hour).Unix(),
				Token:      "valid",
			},
		}
		assert.True(t, gw.isLoggedIn())
	})

	t.Run("expired token", func(t *testing.T) {
		gw := &ArcadyanGateway{
			GatewayCommon: &GatewayCommon{config: &GatewayConfig{}},
			credentials: arcadianLoginData{
				Expiration: time.Now().Add(-1 * time.Hour).Unix(),
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
	cfg := &GatewayConfig{Host: testIP}
	gw := NewArcadyanGateway(cfg)
	assert.NotNil(t, gw)
	assert.NotNil(t, gw.client)
}

func TestArcadyanGateway_Status_Success(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)

			return
		}

		if r.Method == http.MethodGet &&
			r.URL.Path == "/TMI/v1/gateway/" &&
			r.URL.RawQuery == "get=all" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"signal":{"generic":{"registration":"registered"}}}`))

			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	gw := newArcadyan(
		testCommon(ts),
		"valid-token",
		time.Now().Add(1*time.Hour),
	)
	gw.config = testConfig(ts)

	result, err := gw.Status(t.Context())
	require.NoError(t, err)
	assert.True(t, result.WebInterfaceUp)
	assert.Equal(t, "registered", result.Registration)
}

func TestArcadyanGateway_Status_Error(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)

			return
		}

		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	gw := newArcadyan(
		testCommon(ts),
		"valid-token",
		time.Now().Add(1*time.Hour),
	)
	gw.config = testConfig(ts)

	result, err := gw.Status(t.Context())
	require.NoError(t, err)
	assert.True(t, result.WebInterfaceUp)
	assert.Error(t, result.Error)
}

func TestArcadyanGateway_Request_ErrorStatus(t *testing.T) {
	cases := []struct{ name string; status int }{
		{"unauthorized", http.StatusUnauthorized},
		{"server error", http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t, textResponder(tc.status, "error body"))

			gw := newArcadyan(testCommon(ts), "valid-token", time.Now().Add(1*time.Hour))
			gw.config = testConfigNoCreds(ts)

			result, err := gw.Request(t.Context(), "GET", "/test")
			require.Error(t, err, "Request() must return an error for HTTP %d", tc.status)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), fmt.Sprintf("%d", tc.status))
		})
	}
}

func TestArcadyanGateway_Request_Methods(t *testing.T) {
	t.Run("GET request", func(t *testing.T) {
		ts := newTestServer(t, jsonResponder(http.StatusOK, `{"status": "ok"}`))

		gw := newArcadyan(testCommon(ts), "valid-token", time.Now().Add(1*time.Hour))
		gw.config = testConfigNoCreds(ts)

		result, err := gw.Request(t.Context(), "GET", "/test")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, http.StatusOK, result.StatusCode)
	})

	t.Run("POST request", func(t *testing.T) {
		ts := newTestServer(t, jsonResponder(http.StatusOK, `{"status": "created"}`))

		gw := newArcadyan(testCommon(ts), "valid-token", time.Now().Add(1*time.Hour))
		gw.config = testConfig(ts)

		result, err := gw.Request(t.Context(), "POST", "/test")
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("non-JSON response", func(t *testing.T) {
		ts := newTestServer(t, textResponder(http.StatusOK, "plain text response"))

		gw := newArcadyan(testCommon(ts), "valid-token", time.Now().Add(1*time.Hour))
		gw.config = testConfigNoCreds(ts)

		result, err := gw.Request(t.Context(), "GET", "/test")
		require.NoError(t, err)
		assert.Equal(t, "text/plain", result.ContentType)
	})

	t.Run("empty response", func(t *testing.T) {
		ts := newTestServer(t, textResponder(http.StatusNoContent, ""))

		gw := newArcadyan(testCommon(ts), "valid-token", time.Now().Add(1*time.Hour))
		gw.config = testConfigNoCreds(ts)

		result, err := gw.Request(t.Context(), "GET", "/test")
		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, result.StatusCode)
	})
}

func TestArcadyanGateway_Signal(t *testing.T) {
	t.Run("successful signal retrieval", func(t *testing.T) {
		cases := []struct {
			name  string
			body  string
			check func(t *testing.T, result *SignalResult)
		}{
			{
				name: "with 4g and 5g",
				body: `{
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
				}`,
				check: func(t *testing.T, result *SignalResult) {
					t.Helper()
					require.NotNil(t, result.FourG)
					assert.Equal(t, []string{"b2"}, result.FourG.Bands)
					assert.InEpsilon(t, 4.0, result.FourG.Bars, 0.001)
					assert.Equal(t, 12, result.FourG.CID)
					assert.Equal(t, 310463, result.FourG.ENBID)
					assert.Equal(t, -95, result.FourG.RSRP)
					assert.Equal(t, -8, result.FourG.RSRQ)
					assert.Equal(t, -85, result.FourG.RSSI)
					assert.Equal(t, 15, result.FourG.SINR)

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

					assert.Equal(t, "FBB.HOME", result.Generic.APN)
					assert.True(t, result.Generic.HasIPv6)
					assert.Equal(t, "registered", result.Generic.Registration)
					assert.False(t, result.Generic.Roaming)
				},
			},
			{
				name: "5g only",
				body: `{
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
				}`,
				check: func(t *testing.T, result *SignalResult) {
					t.Helper()
					assert.Nil(t, result.FourG)
					require.NotNil(t, result.FiveG)
					assert.Equal(t, []string{"n41"}, result.FiveG.Bands)
					assert.InEpsilon(t, 5.0, result.FiveG.Bars, 0.001)
				},
			},
			{
				name: "4g only",
				body: `{
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
				}`,
				check: func(t *testing.T, result *SignalResult) {
					t.Helper()
					require.NotNil(t, result.FourG)
					assert.Equal(t, 310463, result.FourG.ENBID)
					assert.Nil(t, result.FiveG)
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				ts := newTestServer(t, jsonResponder(http.StatusOK, tc.body))

				gw := newArcadyan(
					testCommon(ts),
					"valid-token",
					time.Now().Add(1*time.Hour),
				)
				gw.config = testConfig(ts)

				result, err := gw.Signal(t.Context())
				require.NoError(t, err)
				require.NotNil(t, result)
				tc.check(t, result)
			})
		}
	})

	t.Run("signal errors", func(t *testing.T) {
		cases := []struct {
			name          string
			setup         func(t *testing.T) *ArcadyanGateway
			errorContains string
		}{
			{
				name: "with network error",
				setup: func(t *testing.T) *ArcadyanGateway {
					t.Helper()
					ts := newTestServer(t, http.HandlerFunc(
						func(_ http.ResponseWriter, _ *http.Request) {},
					))
					ts.Close()

					gw := newArcadyan(
						&GatewayCommon{
							client: resty.NewWithClient(&http.Client{}).SetBaseURL(ts.URL),
							config: testConfigNoCreds(ts),
						},
						"valid-token",
						time.Now().Add(1*time.Hour),
					)
					gw.config = testConfigNoCreds(ts)

					return gw
				},
				errorContains: "failed to get signal info",
			},
			{
				name: "with non-200 status",
				setup: func(t *testing.T) *ArcadyanGateway {
					t.Helper()
					ts := newTestServer(t, jsonResponder(http.StatusInternalServerError, "{}"))
					gw := newArcadyan(testCommon(ts), "valid-token", time.Now().Add(1*time.Hour))
					gw.config = testConfigNoCreds(ts)

					return gw
				},
				errorContains: "signal failed",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				gw := tc.setup(t)

				_, err := gw.Signal(t.Context())
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			})
		}
	})
}

func TestInfoResult_String(t *testing.T) {
	t.Run("pretty prints JSON content", func(t *testing.T) {
		result := &InfoResult{
			Raw:         []byte(`{"name":"test","value":123}`),
			ContentType: jsonContentType,
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

func TestArcadyanGateway_Login_AlreadyLoggedIn(t *testing.T) {
	// When already logged in, Login should not make HTTP requests
	gw := &ArcadyanGateway{
		GatewayCommon: &GatewayCommon{},
		credentials: arcadianLoginData{
			Token:      "existing-token",
			Expiration: time.Now().Add(1 * time.Hour).Unix(),
		},
	}

	err := gw.Login(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "existing-token", gw.credentials.Token)
}

func TestArcadyanGateway_Reboot_LoginFailure(t *testing.T) {
	// When not logged in and login fails, reboot should return error
	ts := newTestServer(t, http.HandlerFunc(
		func(_ http.ResponseWriter, _ *http.Request) {},
	))
	ts.Close()

	gw := newArcadyan(
		&GatewayCommon{
			client: resty.NewWithClient(&http.Client{}).SetBaseURL(ts.URL),
			config: testConfigNoCreds(ts),
		},
		"",
		time.Time{},
	)
	gw.config.Username = testUsername
	gw.config.Password = testPassword

	err := gw.Reboot(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reboot without successful login")
}
