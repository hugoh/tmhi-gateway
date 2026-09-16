package tmhi

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"strings"
	"time"
)

// Gateway defines the interface for T-Mobile gateway implementations.
//
// Implementations are not safe for concurrent use: Login mutates shared
// client state such as auth headers and cookies.
type Gateway interface {
	Close()
	Login(ctx context.Context) error
	Reboot(ctx context.Context) error
	Request(ctx context.Context, method, path string) (*InfoResult, error)
	Info(ctx context.Context) (*InfoResult, error)
	Status(ctx context.Context) *StatusResult
	Signal(ctx context.Context) (*SignalResult, error)
}

const defaultUserAgent = "tmhi-gateway/v3"

// GatewayCommon provides shared functionality for gateway implementations.
type GatewayCommon struct {
	client    *http.Client
	baseURL   string
	header    http.Header
	authToken string
	retries   int
	retryWait time.Duration
	debug     bool
	config    *GatewayConfig
}

// NewGatewayCommon creates a new GatewayCommon with the given configuration.
func NewGatewayCommon(cfg *GatewayConfig) *GatewayCommon {
	if cfg == nil {
		panic("tmhi: GatewayConfig must not be nil")
	}

	host := cfg.Host
	// Bare IPv6 literals must be bracketed in URLs.
	// Use ContainsRune(':') rather than To4()==nil so IPv4-mapped IPv6
	// addresses (::ffff:x.x.x.x) are bracketed correctly — To4() returns
	// non-nil for those, which would incorrectly skip the bracket.
	if net.ParseIP(host) != nil && strings.ContainsRune(host, ':') {
		host = "[" + host + "]"
	}

	header := make(http.Header)
	header.Set("User-Agent", cmp.Or(cfg.UserAgent, defaultUserAgent))

	return &GatewayCommon{
		client:    &http.Client{Timeout: cfg.Timeout},
		baseURL:   "http://" + host,
		header:    header,
		retries:   cfg.Retries,
		retryWait: cfg.RetryWait,
		debug:     cfg.Debug,
		config:    cfg,
	}
}

// Close releases resources held by the underlying HTTP client.
func (gc *GatewayCommon) Close() {
	gc.client.CloseIdleConnections()
}

// SetHeader sets a default header sent with every subsequent request.
func (gc *GatewayCommon) SetHeader(key, value string) {
	gc.header.Set(key, value)
}

// SetAuthToken sets (or, given "", clears) the bearer token sent with every
// subsequent request.
func (gc *GatewayCommon) SetAuthToken(token string) {
	gc.authToken = token
}

// gwResponse is a minimal HTTP response wrapper carrying just what call
// sites need, without pulling in a full client library for it.
type gwResponse struct {
	statusCode int
	body       []byte
	header     http.Header
}

func (r *gwResponse) StatusCode() int       { return r.statusCode }
func (r *gwResponse) IsStatusSuccess() bool { return r.statusCode >= 200 && r.statusCode < 300 }
func (r *gwResponse) IsStatusFailure() bool { return !r.IsStatusSuccess() }
func (r *gwResponse) String() string        { return string(r.body) }
func (r *gwResponse) Bytes() []byte         { return r.body }
func (r *gwResponse) Header() http.Header   { return r.header }

// CheckWebInterface checks if the gateway web interface is accessible.
func (gc *GatewayCommon) CheckWebInterface(ctx context.Context) *StatusResult {
	resp, err := gc.doRequest(ctx, http.MethodHead, "/", nil, "")

	result := &StatusResult{}
	if err != nil {
		result.Error = fmt.Errorf("send request: %w", err)
		result.WebInterfaceUp = false

		return result
	}

	result.StatusCode = resp.StatusCode()
	result.WebInterfaceUp = resp.IsStatusSuccess()

	return result
}

// doRequest sends an HTTP request against the gateway and returns its
// response. It carries the client's default headers and bearer token (if
// set), and retries on transport-level errors up to gc.retries times,
// waiting gc.retryWait between attempts.
//
// ponytail: retry only re-runs on transport errors, not on 5xx status codes.
// Add status-based retry if a gateway needs it.
func (gc *GatewayCommon) doRequest(
	ctx context.Context,
	method, path string,
	body []byte,
	contentType string,
	cookies ...*http.Cookie,
) (*gwResponse, error) {
	url := gc.baseURL + path

	var lastErr error

	maxAttempt := max(gc.retries, 0)
	for attempt := 0; attempt <= maxAttempt; attempt++ {
		resp, err := gc.doOnce(ctx, method, url, body, contentType, cookies)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if attempt < maxAttempt && gc.retryWait > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("retry wait: %w", ctx.Err())
			case <-time.After(gc.retryWait):
			}
		}
	}

	return nil, lastErr
}

func (gc *GatewayCommon) doOnce(
	ctx context.Context,
	method, url string,
	body []byte,
	contentType string,
	cookies []*http.Cookie,
) (*gwResponse, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	maps.Copy(req.Header, gc.header)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if gc.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+gc.authToken)
	}

	for _, c := range cookies {
		req.AddCookie(c)
	}

	resp, err := gc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if gc.debug {
		log.Printf("tmhi: %s %s -> %d", method, url, resp.StatusCode)
	}

	return &gwResponse{statusCode: resp.StatusCode, body: respBody, header: resp.Header}, nil
}

// authSession is implemented by gateway credential holders that must be
// invalidated when the gateway rejects a request as unauthenticated.
type authSession interface {
	logout()
}

// performReboot runs the shared reboot flow used by every gateway
// implementation: skip entirely in dry-run mode, otherwise authenticate,
// issue the vendor-supplied reboot request, and invalidate the session on
// success or on an auth rejection.
func (gc *GatewayCommon) performReboot(
	ctx context.Context,
	sess authSession,
	login func(context.Context) error,
	doRequest func() (*gwResponse, error),
) error {
	if gc.config.DryRun {
		return nil
	}

	if err := login(ctx); err != nil {
		return fmt.Errorf("cannot reboot without successful login flow: %w", err)
	}

	resp, err := doRequest()
	if err != nil {
		return fmt.Errorf("reboot request failed: %w", err)
	}

	if resp.IsStatusFailure() {
		status := resp.StatusCode()
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			sess.logout()
		}

		return NewGatewayError("reboot", status, resp.String(), ErrRebootFailed)
	}

	// A successful reboot invalidates the session on the gateway side.
	sess.logout()

	return nil
}
