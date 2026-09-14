package tmhi

import (
	"errors"
	"fmt"
)

// Sentinel errors for gateway operations.
var (
	// ErrAuthentication indicates an authentication failure.
	ErrAuthentication = errors.New("could not authenticate")
	// ErrNotImplemented indicates an unsupported operation.
	ErrNotImplemented = errors.New("command not implemented")
	// ErrRebootFailed indicates a reboot operation failed.
	ErrRebootFailed = errors.New("reboot failed")
	// ErrSignalFailed indicates a signal operation failed.
	ErrSignalFailed = errors.New("signal failed")
	// ErrStatusFailed indicates a status operation failed.
	ErrStatusFailed = errors.New("status failed")
	// ErrRequestFailed indicates an HTTP request returned an error status.
	ErrRequestFailed = errors.New("request failed")
)

// GatewayError represents a gateway operation failure.
type GatewayError struct {
	Op         string
	Err        error
	HTTPStatus int
	Message    string
}

func (e *GatewayError) Error() string {
	if e.HTTPStatus > 0 {
		return fmt.Sprintf("%s failed: %s (status %d): %v", e.Op, e.Message, e.HTTPStatus, e.Err)
	}

	if e.Message != "" {
		return fmt.Sprintf("%s failed: %s: %v", e.Op, e.Message, e.Err)
	}

	return fmt.Sprintf("%s failed: %v", e.Op, e.Err)
}

func (e *GatewayError) Unwrap() error {
	return e.Err
}

// NewAuthError creates a new GatewayError matching ErrAuthentication, wrapping an optional cause.
func NewAuthError(status int, message string, err error) error {
	if err == nil {
		err = ErrAuthentication
	} else {
		err = fmt.Errorf("%w: %w", ErrAuthentication, err)
	}

	return NewGatewayError("authentication", status, message, err)
}

// NewGatewayError creates a new GatewayError.
func NewGatewayError(op string, status int, message string, err error) error {
	return &GatewayError{
		Op:         op,
		Err:        err,
		HTTPStatus: status,
		Message:    message,
	}
}
