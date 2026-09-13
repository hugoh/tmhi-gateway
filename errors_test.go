package tmhi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayError(t *testing.T) {
	t.Run("error message with all fields", func(t *testing.T) {
		err := &GatewayError{
			Op:         "login",
			HTTPStatus: http.StatusInternalServerError,
			Message:    testServerErrMsg,
			Err:        errors.New("connection timeout"),
		}
		assert.Contains(t, err.Error(), "login failed")
		assert.Contains(t, err.Error(), "500")
		assert.Contains(t, err.Error(), "server error")
		assert.Contains(t, err.Error(), "connection timeout")
	})

	t.Run("error message without status", func(t *testing.T) {
		err := &GatewayError{
			Op:      "reboot",
			Message: "no response",
			Err:     errors.New("timeout"),
		}
		assert.Contains(t, err.Error(), "reboot failed")
		assert.Contains(t, err.Error(), "no response")
	})

	t.Run("error message without message", func(t *testing.T) {
		err := &GatewayError{
			Op:  "status",
			Err: errors.New("network error"),
		}
		assert.Contains(t, err.Error(), "status failed")
		assert.Contains(t, err.Error(), "network error")
	})

	t.Run("Unwrap returns wrapped error", func(t *testing.T) {
		wrappedErr := errors.New("original error")
		err := &GatewayError{
			Op:  "login",
			Err: wrappedErr,
		}
		assert.Equal(t, wrappedErr, errors.Unwrap(err))
	})
}

func TestNewAuthError(t *testing.T) {
	cause := errors.New("network timeout")
	err := NewAuthError(http.StatusUnauthorized, "invalid token", cause)
	require.Error(t, err)

	gwErr, ok := errors.AsType[*GatewayError](err)
	require.True(t, ok)
	assert.Equal(t, "authentication", gwErr.Op)
	assert.Equal(t, http.StatusUnauthorized, gwErr.HTTPStatus)
	assert.Equal(t, "invalid token", gwErr.Message)
	require.ErrorIs(t, err, ErrAuthentication)
	require.ErrorIs(t, err, cause)
}

func TestNewAuthError_NoCause(t *testing.T) {
	err := NewAuthError(0, "login response missing auth token", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAuthentication)
}

func TestNewGatewayError(t *testing.T) {
	innerErr := errors.New("network timeout")
	err := NewGatewayError("reboot", http.StatusGatewayTimeout, "gateway unresponsive", innerErr)
	require.Error(t, err)

	gwErr, ok := errors.AsType[*GatewayError](err)
	require.True(t, ok)
	assert.Equal(t, "reboot", gwErr.Op)
	assert.Equal(t, http.StatusGatewayTimeout, gwErr.HTTPStatus)
	assert.Equal(t, "gateway unresponsive", gwErr.Message)
	assert.Equal(t, innerErr, gwErr.Err)
}
