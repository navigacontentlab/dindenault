package navigaid

import (
	"context"
	"net/http"

	"github.com/navigacontentlab/dindenault/internal/httpforward"
)

// NewHTTPClient returns an *http.Client that forwards the authenticated
// caller's token on every outbound request. The token is read from ctx via
// GetAuth at call time; if no auth info is present the client makes
// unauthenticated requests.
//
// Pass a shared base RoundTripper (e.g. http.DefaultTransport or a cached
// *http.Transport) to preserve TCP connection pooling across calls. If base
// is nil, http.DefaultTransport is used.
//
// This works for any handler whose context was populated by an auth middleware
// or interceptor that calls SetAuth — both MCP (mcp.AuthMiddleware) and
// ConnectRPC auth interceptors qualify.
//
// Set Timeout on the returned client to enforce a request deadline:
//
//	client := navigaid.NewHTTPClient(ctx, http.DefaultTransport)
//	client.Timeout = 15 * time.Second
func NewHTTPClient(ctx context.Context, base http.RoundTripper) *http.Client {
	token := ""

	if auth, err := GetAuth(ctx); err == nil && auth.AccessToken != "" {
		token = "Bearer " + auth.AccessToken
	}

	return &http.Client{
		Transport: httpforward.NewTransport(token, base),
	}
}
