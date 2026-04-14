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
	// ErrNoResponse indicates no response available from mock.
	ErrNoResponse = errors.New("no response available")
)

// AuthenticationError represents an authentication failure with the gateway.
type AuthenticationError struct {
	Status  int
	Message string
}

func (e *AuthenticationError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("authentication failed: %s (status %d)", e.Message, e.Status)
	}

	return "authentication failed: " + e.Message
}

// Is checks if the target error matches ErrAuthentication.
func (e *AuthenticationError) Is(target error) bool {
	return target == ErrAuthentication
}

func (e *AuthenticationError) Unwrap() error {
	return ErrAuthentication
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

// NewAuthError creates a new AuthenticationError.
func NewAuthError(status int, message string) error {
	return &AuthenticationError{
		Status:  status,
		Message: message,
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
