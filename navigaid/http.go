package navigaid

import (
	"context"
	"net/http"
	"time"

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
// Typical usage in a ConnectRPC handler calling a downstream service:
//
//	func (s *Server) MyRPC(ctx context.Context, req *connect.Request[pb.Req]) (*connect.Response[pb.Resp], error) {
//	    client := navigaid.NewHTTPClient(ctx, http.DefaultTransport, 15*time.Second)
//	    resp, err := client.Get("https://oc-service.example.com/api/search")
//	    ...
//	}
func NewHTTPClient(ctx context.Context, base http.RoundTripper, timeout time.Duration) *http.Client {
	token := ""

	if auth, err := GetAuth(ctx); err == nil && auth.AccessToken != "" {
		token = "Bearer " + auth.AccessToken
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: httpforward.NewTransport(token, base),
	}
}
