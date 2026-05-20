package mcp

import (
	"context"
	"net/http"

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
// Set Timeout on the returned client to enforce a request deadline:
//
//	client := mcp.NewHTTPClient(ctx, http.DefaultTransport)
//	client.Timeout = 15 * time.Second
func NewHTTPClient(ctx context.Context, base http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: httpforward.NewTransport(AuthorizationFromContext(ctx), base),
	}
}
