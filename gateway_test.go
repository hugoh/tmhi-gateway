package tmhi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return ts
}

func testCommon(ts *httptest.Server) *GatewayCommon {
	return &GatewayCommon{
		client: resty.NewWithClient(&http.Client{}).SetBaseURL(ts.URL),
		config: &GatewayConfig{},
	}
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

func TestNewGatewayCommon_UserAgent(t *testing.T) {
	var gotUA string
	ts := newTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	})

	t.Run("default", func(t *testing.T) {
		gc := NewGatewayCommon(&GatewayConfig{Host: strings.TrimPrefix(ts.URL, "http://")})
		_, _ = gc.client.R().Get("/")
		assert.Equal(t, defaultUserAgent, gotUA)
	})

	t.Run("custom", func(t *testing.T) {
		const custom = "my-app/1.0"
		gc := NewGatewayCommon(&GatewayConfig{
			Host:      strings.TrimPrefix(ts.URL, "http://"),
			UserAgent: custom,
		})
		_, _ = gc.client.R().Get("/")
		assert.Equal(t, custom, gotUA)
	})
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
			assert.Equal(t, tc.want, gc.client.BaseURL)
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
					client: resty.NewWithClient(ts.Client()).SetBaseURL(ts.URL),
					config: &GatewayConfig{},
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
