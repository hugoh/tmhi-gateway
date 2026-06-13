// Package comparison contains a minimal net/http wrapper that replicates the
// resty/v2 API surface used by tmhi-gateway.  It exists solely to make the
// dependency trade-off concrete — it is NOT a production implementation.
package comparison

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client mirrors resty.Client for the call-sites used in this project.
type Client struct {
	BaseURL string // exported to match resty.Client.BaseURL used in tests

	http    *http.Client
	headers map[string]string
	cookies []*http.Cookie
	retries int
	debug   bool
}

func New() *Client {
	return NewWithClient(&http.Client{})
}

func NewWithClient(hc *http.Client) *Client {
	return &Client{http: hc, headers: make(map[string]string)}
}

func (c *Client) SetBaseURL(base string) *Client {
	c.BaseURL = base
	return c
}

func (c *Client) SetTimeout(d time.Duration) *Client {
	c.http.Timeout = d
	return c
}

func (c *Client) SetRetryCount(n int) *Client {
	c.retries = n
	return c
}

func (c *Client) SetDebug(on bool) *Client {
	c.debug = on
	return c
}

func (c *Client) SetHeader(key, val string) *Client {
	c.headers[key] = val
	return c
}

func (c *Client) SetAuthToken(token string) *Client {
	c.headers["Authorization"] = "Bearer " + token
	return c
}

func (c *Client) SetCookie(cookie *http.Cookie) *Client {
	for i, existing := range c.cookies {
		if existing.Name == cookie.Name {
			c.cookies[i] = cookie
			return c
		}
	}
	c.cookies = append(c.cookies, cookie)
	return c
}

func (c *Client) R() *Request {
	return &Request{client: c, headers: make(map[string]string)}
}

// Request mirrors resty.Request.
type Request struct {
	client  *Client
	ctx     context.Context
	result  any
	body    any
	form    map[string]string
	headers map[string]string
}

func (r *Request) SetContext(ctx context.Context) *Request {
	r.ctx = ctx
	return r
}

func (r *Request) SetResult(v any) *Request {
	r.result = v
	return r
}

func (r *Request) SetBody(v any) *Request {
	r.body = v
	return r
}

func (r *Request) SetFormData(form map[string]string) *Request {
	r.form = form
	return r
}

func (r *Request) Get(path string) (*Response, error)  { return r.Execute(http.MethodGet, path) }
func (r *Request) Post(path string) (*Response, error) { return r.Execute(http.MethodPost, path) }
func (r *Request) Head(path string) (*Response, error) { return r.Execute(http.MethodHead, path) }

func (r *Request) Execute(method, path string) (*Response, error) {
	attempts := max(1, r.client.retries+1)
	var (
		resp *Response
		err  error
	)
	for i := range attempts {
		resp, err = r.client.do(r, method, path)
		if err == nil || i == attempts-1 {
			break
		}
		// Exponential backoff: 100ms, 200ms, 400ms, …
		time.Sleep(time.Duration(1<<uint(i)) * 100 * time.Millisecond)
	}
	return resp, err
}

func (c *Client) do(r *Request, method, path string) (*Response, error) {
	var bodyReader io.Reader
	contentType := ""

	switch {
	case r.form != nil:
		vals := url.Values{}
		for k, v := range r.form {
			vals.Set(k, v)
		}
		bodyReader = strings.NewReader(vals.Encode())
		contentType = "application/x-www-form-urlencoded"
	case r.body != nil:
		data, err := json.Marshal(r.body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
		contentType = "application/json"
	}

	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, cookie := range c.cookies {
		req.AddCookie(cookie)
	}

	if c.debug {
		log.Printf("[DEBUG] → %s %s", method, req.URL)
	}

	raw, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer raw.Body.Close()

	body, err := io.ReadAll(raw.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if c.debug {
		log.Printf("[DEBUG] ← %d %s", raw.StatusCode, body)
	}

	resp := &Response{statusCode: raw.StatusCode, body: body, headers: raw.Header}

	if r.result != nil && raw.StatusCode < 400 {
		if err := json.Unmarshal(body, r.result); err != nil {
			return resp, fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return resp, nil
}

// Response mirrors resty.Response.
type Response struct {
	statusCode int
	body       []byte
	headers    http.Header
}

func (r *Response) StatusCode() int      { return r.statusCode }
func (r *Response) IsSuccess() bool      { return r.statusCode >= 200 && r.statusCode < 300 }
func (r *Response) IsError() bool        { return r.statusCode >= 400 }
func (r *Response) Body() []byte         { return r.body }
func (r *Response) String() string       { return string(r.body) }
func (r *Response) Header() http.Header  { return r.headers }
