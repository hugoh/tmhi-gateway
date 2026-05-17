package tmhi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
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
		client := NewTestClient(&http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString("server error")),
		}, nil)

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
		client := NewTestClient(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"success":0,"reason":600}`)),
		}, nil)

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
	client := NewTestClient(&http.Response{StatusCode: http.StatusOK}, nil)
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
	client := NewTestClient(&http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
	}, nil)
	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	result, err := gw.Status()
	require.NoError(t, err)
	assert.True(t, result.WebInterfaceUp)
	assert.Equal(t, http.StatusOK, result.StatusCode)
}

func TestNokiaGateway_getNonce_ErrorResponse(t *testing.T) {
	client := NewTestClient(nil, errors.New("network error"))
	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	_, err := gw.getNonce()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error getting nonce")
}

func TestNokiaGateway_getNonce_Success(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{contentType: []string{jsonContentType}},
		Body:       io.NopCloser(bytes.NewBufferString(testNonceBody)),
	}
	client := NewTestClient(resp, nil)

	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	nonceResp, err := gw.getNonce()
	require.NoError(t, err)
	assert.Equal(t, "testNonce", nonceResp.Nonce)
	assert.Equal(t, "testPubkey", nonceResp.Pubkey)
	assert.Equal(t, "testRandomKey", nonceResp.RandomKey)
}

func TestNokiaGateway_getCredentials_Success(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{contentType: []string{jsonContentType}},
		Body:       io.NopCloser(bytes.NewBufferString(testLoginRespBody)),
	}
	client := NewTestClient(resp, nil)

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
	client := NewTestClient(nil, errors.New("network error"))

	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	_, err := gw.Login()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error getting nonce")
}

func TestNokiaGateway_Login_CredentialsError(t *testing.T) {
	client := NewTestClient(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(testNonceBody)),
	}, nil)

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
	client := NewTestClient(nil, errors.New("should not be called"))
	gw := newTestNokiaGateway(client, &GatewayConfig{DryRun: true}, testValidSID, testValidToken)

	err := gw.Reboot()
	assert.NoError(t, err)
}

func TestNokiaGateway_Reboot_ErrorResponse(t *testing.T) {
	client := NewTestClient(&http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString("reboot failed")),
	}, nil)
	gw := newTestNokiaGateway(client, &GatewayConfig{}, testValidSID, testValidToken)

	err := gw.Reboot()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRebootFailed)
}

func TestNokiaGateway_Reboot_LoginFailure(t *testing.T) {
	client := NewTestClient(nil, errors.New("network error"))
	gw := newTestNokiaGateway(client, &GatewayConfig{}, "", "")

	err := gw.Reboot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reboot without successful login")
}

func TestNokiaGateway_Login_NonceSuccessCredentialsError(t *testing.T) {
	client := NewMultiTestClient([]*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(testNonceBody))},
		{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"success":0,"reason":600}`)),
		},
	}, []error{nil, nil})

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
	client := NewMultiTestClient([]*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{contentType: []string{jsonContentType}},
			Body:       io.NopCloser(bytes.NewBufferString(testNonceBody)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{contentType: []string{jsonContentType}},
			Body:       io.NopCloser(bytes.NewBufferString(testLoginRespBody)),
		},
	}, []error{nil, nil})

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
	client := NewTestClient(nil, errors.New("network error"))
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
	assert.Equal(t, "http://192.168.1.1", gw.client.BaseURL)
	assert.Empty(t, gw.credentials.SID)
	assert.Empty(t, gw.credentials.CSRFToken)
}
