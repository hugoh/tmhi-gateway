package tmhi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/jarcoal/httpmock"
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

func newTestNokiaGateway(
	client *resty.Client,
	cfg *GatewayConfig,
	sid, token string,
) *NokiaGateway {
	gw := &NokiaGateway{
		GatewayCommon: &GatewayCommon{
			client: client,
			config: cfg,
		},
	}
	if sid != "" {
		gw.credentials = nokiaLoginData{SID: sid, CSRFToken: token}
	}

	return gw
}

func Test_LoginSuccess(t *testing.T) {
	success := &nokiaLoginResp{
		Success:   0,
		Reason:    0,
		Sid:       "foo",
		CsrfToken: "bar",
	}
	assert.True(t, success.success())
}

func Test_LoginFailure(t *testing.T) {
	fail := &nokiaLoginResp{
		Success: 0,
		Reason:  600,
	}
	assert.False(t, fail.success())
}

func TestNokiaGateway_getCredentials_ErrorResponse(t *testing.T) {
	t.Run("server error", func(t *testing.T) {
		client := newMockedClient(t)

		httpmock.RegisterResponder("POST", testBaseURL+"/login_web_app.cgi",
			textResponder(http.StatusInternalServerError, "server error"))

		gw := newTestNokiaGateway(
			client,
			&GatewayConfig{Username: testUsername, Password: testPassword},
			"",
			"",
		)

		_, err := gw.getCredentials(nonceResp{Nonce: "test"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAuthentication)
	})

	t.Run("invalid credentials", func(t *testing.T) {
		client := newMockedClient(t)

		httpmock.RegisterResponder("POST", testBaseURL+"/login_web_app.cgi",
			jsonResponder(http.StatusOK, `{"success":0,"reason":600}`))

		gw := newTestNokiaGateway(
			client,
			&GatewayConfig{Username: testUsername, Password: testPassword},
			"",
			"",
		)

		_, err := gw.getCredentials(nonceResp{Nonce: "test"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAuthentication)
	})
}

func TestNokiaGateway_Reboot_Success(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("POST", testBaseURL+"/reboot_web_app.cgi",
		httpmock.NewStringResponder(http.StatusOK, ""))

	gw := newTestNokiaGateway(
		client,
		&GatewayConfig{Username: testUsername, Password: testPassword},
		testValidSID,
		testValidToken,
	)

	err := gw.Reboot()
	assert.NoError(t, err)
}

func TestNokiaGateway_Status(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("HEAD", testBaseURL+"/",
		httpmock.NewStringResponder(http.StatusOK, ""))

	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	result, err := gw.Status()
	require.NoError(t, err)
	assert.True(t, result.WebInterfaceUp)
	assert.Equal(t, http.StatusOK, result.StatusCode)
}

func TestNokiaGateway_getNonce_ErrorResponse(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("GET", testBaseURL+"/login_web_app.cgi?nonce",
		httpmock.NewErrorResponder(errors.New("network error")))

	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	_, err := gw.getNonce()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error getting nonce")
}

func TestNokiaGateway_getNonce_Success(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("GET", testBaseURL+"/login_web_app.cgi?nonce",
		jsonResponder(http.StatusOK, testNonceBody))

	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	nonceResp, err := gw.getNonce()
	require.NoError(t, err)
	assert.Equal(t, "testNonce", nonceResp.Nonce)
	assert.Equal(t, "testPubkey", nonceResp.Pubkey)
	assert.Equal(t, "testRandomKey", nonceResp.RandomKey)
}

func TestNokiaGateway_getCredentials_Success(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("POST", testBaseURL+"/login_web_app.cgi",
		jsonResponder(http.StatusOK, testLoginRespBody))

	gw := newTestNokiaGateway(
		client,
		&GatewayConfig{Username: testUsername, Password: testPassword},
		"",
		"",
	)

	loginResp, err := gw.getCredentials(nonceResp{Nonce: "testNonce", RandomKey: "testRandomKey"})
	require.NoError(t, err)
	assert.Equal(t, "testSid", loginResp.Sid)
	assert.Equal(t, "testToken", loginResp.CsrfToken)
}

func TestNokiaGateway_Login_Alreadyauthenticated(t *testing.T) {
	gw := newTestNokiaGateway(nil, &GatewayConfig{}, "valid-sid", "valid-token")

	result, err := gw.Login()
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestNokiaGateway_Login_NonceError(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("GET", testBaseURL+"/login_web_app.cgi?nonce",
		httpmock.NewErrorResponder(errors.New("network error")))

	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	_, err := gw.Login()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error getting nonce")
}

func TestNokiaGateway_Login_CredentialsError(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("GET", testBaseURL+"/login_web_app.cgi?nonce",
		jsonResponder(http.StatusOK, testNonceBody))
	httpmock.RegisterResponder("POST", testBaseURL+"/login_web_app.cgi",
		jsonResponder(http.StatusOK, testNonceBody))

	gw := newTestNokiaGateway(
		client,
		&GatewayConfig{Username: testUsername, Password: testPassword},
		"",
		"",
	)

	_, err := gw.Login()
	assert.Error(t, err)
}

func TestNokiaGateway_Reboot_DryRun(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("POST", testBaseURL+"/reboot_web_app.cgi",
		func(_ *http.Request) (*http.Response, error) {
			t.Fatal("unexpected HTTP call in dry run")

			return nil, errors.New("unexpected call")
		})

	gw := newTestNokiaGateway(client, &GatewayConfig{DryRun: true}, testValidSID, testValidToken)

	err := gw.Reboot()
	assert.NoError(t, err)
}

func TestNokiaGateway_Reboot_ErrorResponse(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("POST", testBaseURL+"/reboot_web_app.cgi",
		textResponder(http.StatusInternalServerError, "reboot failed"))

	gw := newTestNokiaGateway(client, &GatewayConfig{}, testValidSID, testValidToken)

	err := gw.Reboot()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRebootFailed)
}

func TestNokiaGateway_Reboot_LoginFailure(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("POST", testBaseURL+"/reboot_web_app.cgi",
		func(_ *http.Request) (*http.Response, error) {
			t.Fatal("should not reach reboot HTTP call")

			return nil, errors.New("unexpected call")
		})

	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	err := gw.Reboot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reboot without successful login")
}

func TestNokiaGateway_Login_NonceSuccessCredentialsError(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("GET", testBaseURL+"/login_web_app.cgi?nonce",
		jsonResponder(http.StatusOK, testNonceBody))
	httpmock.RegisterResponder("POST", testBaseURL+"/login_web_app.cgi",
		jsonResponder(http.StatusOK, `{"success":0,"reason":600}`))

	gw := newTestNokiaGateway(
		client,
		&GatewayConfig{Username: testUsername, Password: testPassword},
		"",
		"",
	)

	_, err := gw.Login()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthentication)
}

func TestNokiaGateway_Login_Success(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("GET", testBaseURL+"/login_web_app.cgi?nonce",
		jsonResponder(http.StatusOK, testNonceBody))
	httpmock.RegisterResponder("POST", testBaseURL+"/login_web_app.cgi",
		jsonResponder(http.StatusOK, testLoginRespBody))

	gw := newTestNokiaGateway(
		client,
		&GatewayConfig{Username: testUsername, Password: testPassword},
		"",
		"",
	)

	result, err := gw.Login()
	require.NoError(t, err)
	assert.Equal(t, "testSid", gw.credentials.SID)
	assert.Equal(t, "testToken", gw.credentials.CSRFToken)
	assert.True(t, result.Success)
	assert.Equal(t, "testSid", result.SessionID)
	assert.Equal(t, "testToken", result.CSRFToken)
}

func TestNokiaGateway_Reboot_RequestError(t *testing.T) {
	client := newMockedClient(t)

	httpmock.RegisterResponder("POST", testBaseURL+"/reboot_web_app.cgi",
		httpmock.NewErrorResponder(errors.New("network error")))

	gw := newTestNokiaGateway(client, &GatewayConfig{}, testValidSID, testValidToken)

	err := gw.Reboot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error sending reboot request")
}

func TestNokiaGateway_NotImplemented(t *testing.T) {
	gw := newTestNokiaGateway(nil, &GatewayConfig{}, "", "")

	t.Run("Request", func(t *testing.T) {
		_, err := gw.Request("GET", "/test")
		assert.ErrorIs(t, err, ErrNotImplemented)
	})

	t.Run("Info", func(t *testing.T) {
		_, err := gw.Info()
		assert.ErrorIs(t, err, ErrNotImplemented)
	})

	t.Run("Signal", func(t *testing.T) {
		_, err := gw.Signal()
		assert.ErrorIs(t, err, ErrNotImplemented)
	})
}

func TestNewNokiaGateway(t *testing.T) {
	cfg := &GatewayConfig{IP: testIP}
	gw := NewNokiaGateway(cfg)
	assert.NotNil(t, gw)
	assert.NotNil(t, gw.client)
	assert.Equal(t, testBaseURL, gw.client.BaseURL)
	assert.Empty(t, gw.credentials.SID)
	assert.Empty(t, gw.credentials.CSRFToken)
}
