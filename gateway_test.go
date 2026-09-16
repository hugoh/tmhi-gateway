package tmhi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testServerErrMsg = "server error"

// Both vendor gateways must satisfy authSession so performReboot can
// invalidate their session on an auth rejection or a successful reboot.
var (
	_ authSession = (*ArcadyanGateway)(nil)
	_ authSession = (*NokiaGateway)(nil)
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return ts
}

func testCommon(ts *httptest.Server) *GatewayCommon {
	return &GatewayCommon{
		client:  &http.Client{},
		baseURL: ts.URL,
		header:  make(http.Header),
		config:  &GatewayConfig{},
	}
}

// newClosedServerCommon returns a GatewayCommon pointed at an already-closed
// server, used to simulate connection-refused errors.
func newClosedServerCommon(t *testing.T) *GatewayCommon {
	t.Helper()

	ts := newTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	baseURL := ts.URL
	ts.Close()

	return &GatewayCommon{
		client:  &http.Client{},
		baseURL: baseURL,
		header:  make(http.Header),
		config:  &GatewayConfig{},
	}
}

func TestGatewayCommon_Close(t *testing.T) {
	ts := newTestServer(t, func(_ http.ResponseWriter, _ *http.Request) {})
	gc := testCommon(ts)

	gc.Close()
	// Close is safe to call more than once.
	gc.Close()
}

func TestNewGatewayCommon(t *testing.T) {
	cfg := &GatewayConfig{
		Host:    testIP,
		Timeout: 5 * time.Second,
		Retries: 3,
		Debug:   true,
	}
	gc := NewGatewayCommon(cfg)

	assert.NotNil(t, gc.client)
	assert.Equal(t, cfg, gc.config)
}

func TestNewGatewayCommon_NilConfig(t *testing.T) {
	assert.PanicsWithValue(t, "tmhi: GatewayConfig must not be nil", func() {
		NewGatewayCommon(nil)
	})
}

func TestNewGatewayCommon_UserAgent(t *testing.T) {
	var gotUA string

	ts := newTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	})

	t.Run("default", func(t *testing.T) {
		gc := NewGatewayCommon(&GatewayConfig{Host: strings.TrimPrefix(ts.URL, "http://")})
		_, _ = gc.doRequest(t.Context(), http.MethodGet, "/", nil, "")

		assert.Equal(t, defaultUserAgent, gotUA)
	})

	t.Run("custom", func(t *testing.T) {
		const custom = "my-app/1.0"

		gc := NewGatewayCommon(&GatewayConfig{
			Host:      strings.TrimPrefix(ts.URL, "http://"),
			UserAgent: custom,
		})
		_, _ = gc.doRequest(t.Context(), http.MethodGet, "/", nil, "")

		assert.Equal(t, custom, gotUA)
	})
}

func TestGatewayCommon_doRequest_NegativeRetries(t *testing.T) {
	gc := newClosedServerCommon(t)
	gc.retries = -1

	resp, err := gc.doRequest(t.Context(), http.MethodGet, "/", nil, "")

	require.Error(t, err)
	assert.Nil(t, resp)
}

func TestGatewayCommon_doRequest_WaitsBetweenRetries(t *testing.T) {
	const (
		retries   = 2
		retryWait = 20 * time.Millisecond
	)

	gc := newClosedServerCommon(t)
	gc.retries = retries
	gc.retryWait = retryWait

	start := time.Now()
	_, err := gc.doRequest(t.Context(), http.MethodGet, "/", nil, "")
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.GreaterOrEqual(t, elapsed, retries*retryWait)
}

func TestGatewayCommon_doRequest_WaitRespectsContextCancellation(t *testing.T) {
	gc := newClosedServerCommon(t)
	gc.retries = 5
	gc.retryWait = time.Hour

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := gc.doRequest(ctx, http.MethodGet, "/", nil, "")
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, time.Second)
}

func TestNewGatewayCommon_HostForms(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{host: "192.168.12.1", want: "http://192.168.12.1"},
		{host: "gateway.local", want: "http://gateway.local"},
		{host: "192.168.12.1:8080", want: "http://192.168.12.1:8080"},
		{host: "fd00::1", want: "http://[fd00::1]"},
		{host: "[fd00::1]:8080", want: "http://[fd00::1]:8080"},
		{host: "::ffff:192.0.2.1", want: "http://[::ffff:192.0.2.1]"},
	}

	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			gc := NewGatewayCommon(&GatewayConfig{Host: tc.host})
			assert.Equal(t, tc.want, gc.baseURL)
		})
	}
}

func TestCheckWebInterface(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		useError       bool
		wantUp         bool
		wantStatusCode int
	}{
		{
			name:           "successful web interface check",
			status:         http.StatusOK,
			wantUp:         true,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "failed web interface status code",
			status:         http.StatusInternalServerError,
			wantUp:         false,
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:           "not found web interface",
			status:         http.StatusNotFound,
			wantUp:         false,
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:           "failed web interface check",
			useError:       true,
			wantUp:         false,
			wantStatusCode: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.useError {
				ts := httptest.NewServer(http.HandlerFunc(
					func(_ http.ResponseWriter, _ *http.Request) {},
				))
				ts.Close()

				gc := &GatewayCommon{
					client:  ts.Client(),
					baseURL: ts.URL,
					header:  make(http.Header),
					config:  &GatewayConfig{},
				}

				result := gc.CheckWebInterface(t.Context())
				assert.Equal(t, tc.wantUp, result.WebInterfaceUp)
				assert.Equal(t, tc.wantStatusCode, result.StatusCode)
				assert.Error(t, result.Error)

				return
			}

			ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodHead, r.Method)
				w.WriteHeader(tc.status)
			})

			gc := testCommon(ts)

			result := gc.CheckWebInterface(t.Context())
			assert.Equal(t, tc.wantUp, result.WebInterfaceUp)
			assert.Equal(t, tc.wantStatusCode, result.StatusCode)
		})
	}
}

func testConfig(ts *httptest.Server) *GatewayConfig {
	return &GatewayConfig{
		Host:     strings.TrimPrefix(ts.URL, "http://"),
		Username: testUsername,
		Password: testPassword,
	}
}

func testConfigNoCreds(ts *httptest.Server) *GatewayConfig {
	return &GatewayConfig{Host: strings.TrimPrefix(ts.URL, "http://")}
}

func jsonResponder(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func textResponder(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}
