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
	// ErrRequestFailed indicates an HTTP request returned an error status.
	ErrRequestFailed = errors.New("request failed")
)

// AuthenticationError represents an authentication failure with the gateway.
type AuthenticationError struct {
	Status  int
	Message string
	Err     error
}

func (e *AuthenticationError) Error() string {
	msg := "authentication failed: " + e.Message
	if e.Status > 0 {
		msg = fmt.Sprintf("%s (status %d)", msg, e.Status)
	}

	if e.Err != nil {
		msg = fmt.Sprintf("%s: %v", msg, e.Err)
	}

	return msg
}

// Is matches ErrAuthentication so callers can use errors.Is with the sentinel.
func (*AuthenticationError) Is(target error) bool {
	return target == ErrAuthentication
}

// Unwrap returns the underlying cause, if any.
func (e *AuthenticationError) Unwrap() error {
	return e.Err
}

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

// NewAuthError creates a new AuthenticationError wrapping an optional cause.
func NewAuthError(status int, message string, err error) error {
	return &AuthenticationError{
		Status:  status,
		Message: message,
		Err:     err,
	}
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
