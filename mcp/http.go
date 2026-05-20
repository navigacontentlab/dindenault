package mcp

import (
	"context"
	"net/http"
	"time"
)

// NewHTTPClient returns an *http.Client that forwards the MCP caller's
// Authorization token on every outbound request. The token is read from ctx
// via AuthorizationFromContext at call time and baked into the transport.
//
// Pass a shared base RoundTripper — e.g. http.DefaultTransport or a cached
// *http.Transport — to preserve TCP connection pooling across calls. If base
// is nil, http.DefaultTransport is used.
//
// Typical usage inside a tool handler that calls a downstream service:
//
//	func myHandler(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
//	    client := mcp.NewHTTPClient(ctx, http.DefaultTransport, 15*time.Second)
//	    resp, err := client.Get("https://internal-api.example.com/data")
//	    ...
//	}
func NewHTTPClient(ctx context.Context, base http.RoundTripper, timeout time.Duration) *http.Client {
	if base == nil {
		base = http.DefaultTransport
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: &forwardingTransport{base: base, token: AuthorizationFromContext(ctx)},
	}
}

// forwardingTransport injects a fixed Authorization header on every outbound request.
type forwardingTransport struct {
	base  http.RoundTripper
	token string
}

func (t *forwardingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", t.token)

	return t.base.RoundTrip(r)
}
