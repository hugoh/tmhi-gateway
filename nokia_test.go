package tmhi

import (
	"net/http"
	"net/http/httptest"
	"testing"

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

func nokiaTestGw(ts *httptest.Server, cfg *GatewayConfig, sid, token string) *NokiaGateway {
	gc := testCommon(ts)
	gc.config = cfg

	return newNokia(gc, sid, token)
}

func nokiaTestGwClosed(t *testing.T, sid, token string) *NokiaGateway {
	t.Helper()

	return newNokia(newClosedServerCommon(t), sid, token)
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
			name: testServerErrMsg,
			ts:   newTestServer(t, textResponder(http.StatusInternalServerError, testServerErrMsg)),
		},
		{
			name: "invalid credentials",
			ts:   newTestServer(t, jsonResponder(http.StatusOK, `{"success":0,"reason":600}`)),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := nokiaTestGw(tc.ts, testConfig(tc.ts), "", "")

			_, err := gw.getCredentials(t.Context(), nokiaNonce{Nonce: "test"})
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

	gw := nokiaTestGw(ts, testConfig(ts), testValidSID, testValidToken)

	err := gw.Reboot(t.Context())
	require.NoError(t, err)
	assert.False(t, gw.isLoggedIn(), "successful reboot should invalidate the cached session")
}

func TestNokiaGateway_Reboot_StaleSession(t *testing.T) {
	ts := newTestServer(t, textResponder(http.StatusUnauthorized, "session expired"))
	gw := nokiaTestGw(ts, testConfig(ts), testValidSID, testValidToken)

	err := gw.Reboot(t.Context())
	require.ErrorIs(t, err, ErrRebootFailed)
	assert.False(t, gw.isLoggedIn(), "auth rejection should clear cached credentials")
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
	gw := nokiaTestGwClosed(t, "", "")

	_, err := gw.getNonce(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error getting nonce")
}

func TestNokiaGateway_getNonce_ErrorStatus(t *testing.T) {
	ts := newTestServer(t, textResponder(http.StatusInternalServerError, "server error"))
	gw := nokiaTestGw(ts, &GatewayConfig{}, "", "")

	_, err := gw.getNonce(t.Context())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthentication)
}

func TestNokiaGateway_getNonce_Success(t *testing.T) {
	ts := newTestServer(t, jsonResponder(http.StatusOK, testNonceBody))
	gw := nokiaTestGw(ts, &GatewayConfig{}, "", "")

	nonce, err := gw.getNonce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "testNonce", nonce.Nonce)
	assert.Equal(t, "testPubkey", nonce.Pubkey)
	assert.Equal(t, "testRandomKey", nonce.RandomKey)
}

func TestNokiaGateway_getCredentials_NonceFormField(t *testing.T) {
	// Nonce with chars that base64urlEscape would transform (+, =).
	// The form field must echo the raw nonce so it matches what the hashes use.
	const rawNonce = "abc+def/ghi="

	var receivedNonce string

	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		receivedNonce = r.FormValue(nonceParam)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(testLoginRespBody))
	})

	gw := nokiaTestGw(ts, testConfig(ts), "", "")
	_, err := gw.getCredentials(t.Context(), nokiaNonce{Nonce: rawNonce, RandomKey: "key"})
	require.NoError(t, err)
	assert.Equal(
		t,
		rawNonce,
		receivedNonce,
		"nonce form field must be the raw value used in hashes",
	)
}

func TestNokiaGateway_getCredentials_Success(t *testing.T) {
	ts := newTestServer(t, jsonResponder(http.StatusOK, testLoginRespBody))
	gw := nokiaTestGw(ts, testConfig(ts), "", "")

	loginResp, err := gw.getCredentials(
		t.Context(),
		nokiaNonce{Nonce: "testNonce", RandomKey: "testRandomKey"},
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
	gw := nokiaTestGwClosed(t, "", "")

	err := gw.Login(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error getting nonce")
}

func TestNokiaGateway_Login_CredentialsError(t *testing.T) {
	ts := newTestServer(t, jsonResponder(http.StatusOK, testNonceBody))
	gw := nokiaTestGw(ts, testConfig(ts), "", "")

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

func TestNokiaGateway_Reboot_DryRun_SkipsLogin(t *testing.T) {
	// Not logged in: if DryRun didn't short-circuit before Login, this would
	// hit the network and the handler below would fail the test.
	ts := newTestServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("unexpected HTTP call in dry run")
	})

	gw := nokiaTestGw(ts, &GatewayConfig{DryRun: true}, "", "")

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

	gw := nokiaTestGw(ts, testConfig(ts), "", "")

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
		} else if r.Method == http.MethodPost && r.URL.Path == loginWebAppCGI {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testLoginRespBody))
		}
	})

	gw := nokiaTestGw(ts, testConfig(ts), "", "")

	err := gw.Login(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "testSid", gw.credentials.SID)
	assert.Equal(t, "testToken", gw.credentials.csrfToken)
}

func TestNokiaGateway_Reboot_RequestError(t *testing.T) {
	gw := nokiaTestGwClosed(t, testValidSID, testValidToken)

	err := gw.Reboot(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reboot request failed")
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

func TestNokiaGateway_logout_ClearsCredentials(t *testing.T) {
	gw := newNokia(&GatewayCommon{config: &GatewayConfig{}}, testValidSID, testValidToken)

	gw.logout()

	assert.False(t, gw.isLoggedIn(), "logout should clear credentials")
}

func TestNokiaGateway_Reboot_SendsSIDCookie(t *testing.T) {
	// Reboot must include the current session SID in the request cookie.
	var gotCookie string

	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		if r.Method == http.MethodPost && r.URL.Path == "/reboot_web_app.cgi" {
			if c, err := r.Cookie(sidCookieName); err == nil {
				gotCookie = c.Value
			}
		}
	})

	gw := nokiaTestGw(ts, testConfig(ts), testValidSID, testValidToken)

	require.NoError(t, gw.Reboot(t.Context()))
	assert.Equal(t, testValidSID, gotCookie, "reboot request must carry the session SID cookie")
}

func TestNokiaGateway_Reboot_ReloginHasFreshSID(t *testing.T) {
	// After reboot (which calls logout), a subsequent Login must use fresh credentials,
	// not carry over the old SID.
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		switch {
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(testNonceBody))
		case r.Method == http.MethodPost && r.URL.Path == loginWebAppCGI:
			_, _ = w.Write([]byte(testLoginRespBody))
		}
	})

	gw := nokiaTestGw(ts, testConfig(ts), testValidSID, testValidToken)

	require.NoError(t, gw.Reboot(t.Context()))
	require.NoError(t, gw.Login(t.Context()))

	assert.Equal(
		t,
		"testSid",
		gw.credentials.SID,
		"re-login after reboot must have fresh SID, not the old one",
	)
	assert.NotEqual(t, testValidSID, gw.credentials.SID)
}

func TestNewNokiaGateway(t *testing.T) {
	cfg := &GatewayConfig{Host: testIP}
	gw := NewNokiaGateway(cfg)
	assert.NotNil(t, gw)
	assert.NotNil(t, gw.client)
	assert.Empty(t, gw.credentials.SID)
	assert.Empty(t, gw.credentials.csrfToken)
}
