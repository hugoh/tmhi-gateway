package tmhi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticationError(t *testing.T) {
	t.Run("error message with status", func(t *testing.T) {
		err := &AuthenticationError{
			Status:  http.StatusUnauthorized,
			Message: "invalid credentials",
		}
		assert.Contains(t, err.Error(), "authentication failed")
		assert.Contains(t, err.Error(), "401")
		assert.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("error message without status", func(t *testing.T) {
		err := &AuthenticationError{
			Message: "connection refused",
		}
		assert.Contains(t, err.Error(), "authentication failed")
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("Is matches sentinel error", func(t *testing.T) {
		err := &AuthenticationError{
			Message: "failed",
		}
		assert.ErrorIs(t, err, ErrAuthentication)
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("connection refused")
		err := &AuthenticationError{
			Message: "failed",
			Err:     cause,
		}
		assert.Equal(t, cause, errors.Unwrap(err))
		require.ErrorIs(t, err, cause)
		require.ErrorIs(t, err, ErrAuthentication)
		assert.Contains(t, err.Error(), "connection refused")
	})
}

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

	authErr, ok := errors.AsType[*AuthenticationError](err)
	require.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, authErr.Status)
	assert.Equal(t, "invalid token", authErr.Message)
	assert.Equal(t, cause, authErr.Err)
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
