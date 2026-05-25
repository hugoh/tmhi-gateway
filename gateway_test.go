package tmhi

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

const testBaseURL = "http://192.168.1.1"

func newMockedClient(t *testing.T) *resty.Client {
	t.Helper()

	client := resty.New()
	client.SetBaseURL(testBaseURL)
	httpmock.ActivateNonDefault(client.GetClient())
	t.Cleanup(httpmock.DeactivateAndReset)

	return client
}

func jsonResponder(status int, body string) httpmock.Responder {
	return func(_ *http.Request) (*http.Response, error) {
		resp := httpmock.NewStringResponse(status, body)
		resp.Header.Set("Content-Type", "application/json")

		return resp, nil
	}
}

func textResponder(status int, body string) httpmock.Responder {
	return func(_ *http.Request) (*http.Response, error) {
		resp := httpmock.NewStringResponse(status, body)
		resp.Header.Set("Content-Type", "text/plain")

		return resp, nil
	}
}

func TestNewGatewayCommon(t *testing.T) {
	cfg := &GatewayConfig{
		IP:      testIP,
		Timeout: 5 * time.Second,
		Retries: 3,
		Debug:   true,
	}
	gc := NewGatewayCommon(cfg)

	assert.NotNil(t, gc.client)
	assert.Equal(t, testBaseURL, gc.client.BaseURL)
	assert.Equal(t, cfg.Timeout, gc.client.GetClient().Timeout)
	assert.Equal(t, cfg.Retries, gc.client.RetryCount)
	assert.True(t, gc.client.Debug)
	assert.Equal(t, cfg, gc.config)
}

func TestCheckWebInterface(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		err            error
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
			err:            errors.New("connection refused"),
			wantUp:         false,
			wantStatusCode: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newMockedClient(t)

			if tc.err != nil {
				httpmock.RegisterResponder("HEAD", testBaseURL+"/",
					httpmock.NewErrorResponder(tc.err))
			} else {
				httpmock.RegisterResponder("HEAD", testBaseURL+"/",
					httpmock.NewStringResponder(tc.status, ""))
			}

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
