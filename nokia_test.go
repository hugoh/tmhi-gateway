package tmhi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testUsername      = "user"
	testPassword      = "pass"
	testIP            = "192.168.1.1"
	testNonceBody     = `{"nonce":"testNonce","pubkey":"testPubkey","randomKey":"testRandomKey"}`
	testLoginRespBody = `{"success":0,"reason":0,"sid":"testSid","token":"testToken"}`
	testValidSID      = "valid-sid"
	testValidToken    = "valid-token"
)

func newNokia(gc *GatewayCommon, sid, token string) *NokiaGateway {
	gw := &NokiaGateway{GatewayCommon: gc}
	if sid != "" {
		gw.credentials = nokiaLoginData{SID: sid, csrfToken: token}
	}

	return gw
}

func nokiaConfig(ts *httptest.Server) *GatewayConfig {
	return &GatewayConfig{
		IP:       strings.TrimPrefix(ts.URL, "http://"),
		Username: testUsername,
		Password: testPassword,
	}
}

func nokiaTestGw(ts *httptest.Server, cfg *GatewayConfig, sid, token string) *NokiaGateway {
	return newNokia(&GatewayCommon{
		client: resty.NewWithClient(&http.Client{}).SetBaseURL(ts.URL),
		config: cfg,
	}, sid, token)
}

func nokiaTestGwClosed(cfg *GatewayConfig, sid, token string) *NokiaGateway {
	ts := httptest.NewServer(http.HandlerFunc(
		func(_ http.ResponseWriter, _ *http.Request) {},
	))
	ts.Close()

	return newNokia(&GatewayCommon{
		client: resty.NewWithClient(&http.Client{}).SetBaseURL(ts.URL),
		config: cfg,
	}, sid, token)
}

func Test_LoginSuccess(t *testing.T) {
	valid := &nokiaLoginResp{
		Success:   0,
		Reason:    0,
		Sid:       "foo",
		CsrfToken: "bar",
	}
	assert.True(t, valid.hasCredentials())
}

func Test_LoginFailure(t *testing.T) {
	invalid := &nokiaLoginResp{
		Success: 0,
		Reason:  600,
	}
	assert.False(t, invalid.hasCredentials())
}

func TestNokiaGateway_getCredentials_ErrorResponse(t *testing.T) {
	cases := []struct {
		name string
		ts   *httptest.Server
	}{
		{
			name: "server error",
			ts:   newTestServer(t, textResponder(http.StatusInternalServerError, "server error")),
		},
		{
			name: "invalid credentials",
			ts:   newTestServer(t, jsonResponder(http.StatusOK, `{"success":0,"reason":600}`)),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := nokiaTestGw(tc.ts, nokiaConfig(tc.ts), "", "")

			_, err := gw.getCredentials(t.Context(), nonceResp{Nonce: "test"})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrAuthentication)
		})
	}
}

func TestNokiaGateway_Reboot_Success(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/reboot_web_app.cgi", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	gw := nokiaTestGw(ts, nokiaConfig(ts), testValidSID, testValidToken)

	err := gw.Reboot(t.Context())
	assert.NoError(t, err)
}

func TestNokiaGateway_Status(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		w.WriteHeader(http.StatusOK)
	})

	gw := nokiaTestGw(ts, &GatewayConfig{}, "", "")

	result, err := gw.Status(t.Context())
	require.NoError(t, err)
	assert.True(t, result.WebInterfaceUp)
	assert.Equal(t, http.StatusOK, result.StatusCode)
}

func TestNokiaGateway_getNonce_ErrorResponse(t *testing.T) {
	gw := nokiaTestGwClosed(&GatewayConfig{}, "", "")

	_, err := gw.getNonce(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error getting nonce")
}

func TestNokiaGateway_getNonce_Success(t *testing.T) {
	ts := newTestServer(t, jsonResponder(http.StatusOK, testNonceBody))
	gw := nokiaTestGw(ts, &GatewayConfig{}, "", "")

	nonceResp, err := gw.getNonce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "testNonce", nonceResp.Nonce)
	assert.Equal(t, "testPubkey", nonceResp.Pubkey)
	assert.Equal(t, "testRandomKey", nonceResp.RandomKey)
}

func TestNokiaGateway_getCredentials_Success(t *testing.T) {
	ts := newTestServer(t, jsonResponder(http.StatusOK, testLoginRespBody))
	gw := nokiaTestGw(ts, nokiaConfig(ts), "", "")

	loginResp, err := gw.getCredentials(
		t.Context(),
		nonceResp{Nonce: "testNonce", RandomKey: "testRandomKey"},
	)
	require.NoError(t, err)
	assert.Equal(t, "testSid", loginResp.Sid)
	assert.Equal(t, "testToken", loginResp.CsrfToken)
}

func TestNokiaGateway_Login_Alreadyauthenticated(t *testing.T) {
	gw := newNokia(&GatewayCommon{config: &GatewayConfig{}}, "valid-sid", "valid-token")

	err := gw.Login(t.Context())
	require.NoError(t, err)
}

func TestNokiaGateway_Login_NonceError(t *testing.T) {
	gw := nokiaTestGwClosed(&GatewayConfig{}, "", "")

	err := gw.Login(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error getting nonce")
}

func TestNokiaGateway_Login_CredentialsError(t *testing.T) {
	ts := newTestServer(t, jsonResponder(http.StatusOK, testNonceBody))
	gw := nokiaTestGw(ts, nokiaConfig(ts), "", "")

	err := gw.Login(t.Context())
	assert.Error(t, err)
}

func TestNokiaGateway_Reboot_DryRun(t *testing.T) {
	ts := newTestServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("unexpected HTTP call in dry run")
	})

	gw := nokiaTestGw(ts, &GatewayConfig{DryRun: true}, testValidSID, testValidToken)

	err := gw.Reboot(t.Context())
	assert.NoError(t, err)
}

func TestNokiaGateway_Reboot_ErrorResponse(t *testing.T) {
	ts := newTestServer(t, textResponder(http.StatusInternalServerError, "reboot failed"))
	gw := nokiaTestGw(ts, &GatewayConfig{}, testValidSID, testValidToken)

	err := gw.Reboot(t.Context())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRebootFailed)
}

func TestNokiaGateway_Reboot_LoginFailure(t *testing.T) {
	ts := newTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reboot_web_app.cgi" {
			t.Fatal("should not reach reboot HTTP call")
		}
	})

	gw := nokiaTestGw(ts, &GatewayConfig{}, "", "")

	err := gw.Reboot(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reboot without successful login")
}

func TestNokiaGateway_Login_NonceSuccessCredentialsError(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.RawQuery == "nonce" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testNonceBody))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":0,"reason":600}`))
		}
	})

	gw := nokiaTestGw(ts, nokiaConfig(ts), "", "")

	err := gw.Login(t.Context())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthentication)
}

func TestNokiaGateway_Login_Success(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.RawQuery == "nonce" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testNonceBody))
		} else if r.Method == http.MethodPost && r.URL.Path == "/login_web_app.cgi" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testLoginRespBody))
		}
	})

	gw := nokiaTestGw(ts, nokiaConfig(ts), "", "")

	err := gw.Login(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "testSid", gw.credentials.SID)
	assert.Equal(t, "testToken", gw.credentials.csrfToken)
}

func TestNokiaGateway_Reboot_RequestError(t *testing.T) {
	gw := nokiaTestGwClosed(&GatewayConfig{}, testValidSID, testValidToken)

	err := gw.Reboot(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error sending reboot request")
}

func TestNokiaGateway_NotImplemented(t *testing.T) {
	gw := newNokia(&GatewayCommon{config: &GatewayConfig{}}, "", "")

	t.Run("Request", func(t *testing.T) {
		_, err := gw.Request(t.Context(), "GET", "/test")
		assert.ErrorIs(t, err, ErrNotImplemented)
	})

	t.Run("Info", func(t *testing.T) {
		_, err := gw.Info(t.Context())
		assert.ErrorIs(t, err, ErrNotImplemented)
	})

	t.Run("Signal", func(t *testing.T) {
		_, err := gw.Signal(t.Context())
		assert.ErrorIs(t, err, ErrNotImplemented)
	})
}

func TestNewNokiaGateway(t *testing.T) {
	cfg := &GatewayConfig{IP: testIP}
	gw := NewNokiaGateway(cfg)
	assert.NotNil(t, gw)
	assert.NotNil(t, gw.client)
	assert.Empty(t, gw.credentials.SID)
	assert.Empty(t, gw.credentials.csrfToken)
}
