package navigaid

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/navigacontentlab/dindenault/internal/httpforward"
)

// HTTPMiddleware returns an http.Handler middleware that validates the
// bearer token in the Authorization header using the given JWKS. On
// success the validated claims are placed in the request context via
// SetAuth, so downstream handlers can call GetAuth. Requests with a
// missing or invalid token are rejected with HTTP 401.
//
// Use this for plain (non-Connect) HTTP handlers; Connect handlers
// should use ConnectInterceptor instead.
func HTTPMiddleware(logger *slog.Logger, jwks *JWKS, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := GetAuthToken(r.Header)
		if err != nil {
			logger.Debug("missing authorization token", "error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)

			return
		}

		claims, err := jwks.Validate(token)
		if err != nil {
			logger.Debug("invalid token", "error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)

			return
		}

		ctx := SetAuth(r.Context(), AuthInfo{
			AccessToken: token,
			Claims:      claims,
		}, nil)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

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
