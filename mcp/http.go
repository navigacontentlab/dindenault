package mcp

import (
	"context"
	"net/http"
	"time"

	"github.com/navigacontentlab/dindenault/internal/httpforward"
)

// NewHTTPClient returns an *http.Client that forwards the MCP caller's
// Authorization token on every outbound request. The token is read from ctx
// via AuthorizationFromContext at call time and baked into the transport.
//
// Pass a shared base RoundTripper — e.g. http.DefaultTransport or a cached
// *http.Transport — to preserve TCP connection pooling across calls. If base
// is nil, http.DefaultTransport is used.
//
// Typical usage inside a tool handler that needs to call a downstream service:
//
//	func myHandler(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
//	    client := mcp.NewHTTPClient(ctx, http.DefaultTransport, 15*time.Second)
//	    resp, err := client.Get("https://internal-api.example.com/data")
//	    ...
//	}
func NewHTTPClient(ctx context.Context, base http.RoundTripper, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: httpforward.NewTransport(AuthorizationFromContext(ctx), base),
	}
}
