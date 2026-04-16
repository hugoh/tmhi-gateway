package tmhi

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
)

type mockRoundTripper struct {
	resp *http.Response
	err  error
}

type multiMockRoundTripper struct {
	responses []*http.Response
	errors    []error
	callCount int
}

func (m *mockRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return m.resp, m.err
}

func (m *multiMockRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	if m.callCount < len(m.responses) {
		resp := m.responses[m.callCount]
		err := m.errors[m.callCount]
		m.callCount++

		return resp, err
	}

	return nil, ErrNoResponse
}

func NewTestClient(resp *http.Response, err error) *resty.Client {
	return resty.NewWithClient(&http.Client{
		Transport: &mockRoundTripper{resp: resp, err: err},
	})
}

func NewMultiTestClient(responses []*http.Response, errors []error) *resty.Client {
	return resty.NewWithClient(&http.Client{
		Transport: &multiMockRoundTripper{responses: responses, errors: errors},
	})
}

func TestNewGatewayCommon(t *testing.T) {
	cfg := &GatewayConfig{
		IP:      "192.168.1.1",
		Timeout: 5 * time.Second,
		Retries: 3,
		Debug:   true,
	}
	gc := NewGatewayCommon(cfg)

	assert.NotNil(t, gc.client)
	assert.Equal(t, "http://192.168.1.1", gc.client.BaseURL)
	assert.Equal(t, cfg.Timeout, gc.client.GetClient().Timeout)
	assert.Equal(t, cfg.Retries, gc.client.RetryCount)
	assert.True(t, gc.client.Debug)
	assert.False(t, gc.authenticated)
	assert.Equal(t, cfg, gc.config)
}

func TestCheckWebInterface(t *testing.T) {
	cases := []struct {
		name           string
		resp           *http.Response
		err            error
		wantUp         bool
		wantStatusCode int
	}{
		{
			name:           "successful web interface check",
			resp:           &http.Response{StatusCode: http.StatusOK, Body: http.NoBody},
			err:            nil,
			wantUp:         true,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "failed web interface status code",
			resp: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       http.NoBody,
			},
			err:            nil,
			wantUp:         false,
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:           "not found web interface",
			resp:           &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody},
			err:            nil,
			wantUp:         false,
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:           "failed web interface check",
			resp:           nil,
			err:            errors.New("connection refused"),
			wantUp:         false,
			wantStatusCode: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewTestClient(tc.resp, tc.err)
			gc := &GatewayCommon{client: client, config: &GatewayConfig{}}

			result := gc.CheckWebInterface()
			assert.Equal(t, tc.wantUp, result.WebInterfaceUp)
			assert.Equal(t, tc.wantStatusCode, result.StatusCode)

			if tc.err != nil {
				assert.Error(t, result.Error)
			}
		})
	}
}
