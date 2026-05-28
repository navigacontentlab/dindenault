package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/navigacontentlab/dindenault/navigaid"
)

// AuthOption configures optional behaviour of AuthMiddleware.
type AuthOption func(*authConfig)

type authConfig struct {
	publicTools map[string]struct{}
}

// WithPublicTools marks the named tools as exempt from authentication.
// A tools/call request for any listed tool is passed through without a JWT.
func WithPublicTools(names ...string) AuthOption {
	return func(c *authConfig) {
		for _, n := range names {
			c.publicTools[n] = struct{}{}
		}
	}
}

// AuthMiddleware validates the incoming JWT before passing the request to the
// MCP handler. Requests with no token or an invalid token are rejected with
// HTTP 401 before any tool logic runs.
//
// The methods "initialize", "notifications/initialized", and "tools/list" are
// exempt from authentication — they carry no user data and MCP clients need
// them for discovery before a token is available.
//
// Use WithPublicTools to additionally exempt specific tools from authentication:
//
//	mcp.AuthMiddleware(logger, jwks, server, mcp.WithPublicTools("get_search_fields"))
//
// On success the validated claims are placed in the context via
// navigaid.SetAuth, so tool handlers can call navigaid.GetAuth(ctx) to read
// the org, subject, and permissions. The raw Authorization header value is
// still available via mcp.AuthorizationFromContext — forward it as-is to
// downstream services such as OC or CCA.
//
// Typical usage via WithMCPAuth:
//
//	app := dindenault.New(logger,
//	    dindenault.WithMCPAuth("/mcp", logger, os.Getenv("IMAS_URL"),
//	        myTool,
//	    ),
//	)
//
// For full control, wrap the server directly:
//
//	server := mcp.NewServer("my-service", "1.0.0", tools...)
//	jwks  := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint(imasURL))
//	app := dindenault.New(logger,
//	    dindenault.WithService("/mcp", mcp.AuthMiddleware(logger, jwks, server)),
//	)
func AuthMiddleware(logger *slog.Logger, jwks *navigaid.JWKS, next http.Handler, opts ...AuthOption) http.Handler {
	cfg := &authConfig{publicTools: make(map[string]struct{})}

	for _, o := range opts {
		o(cfg)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)

			return
		}

		// Restore body so the next handler can decode it.
		r.Body = io.NopCloser(bytes.NewReader(body))

		// Discovery methods are exempt from auth — clients need them before
		// they have a token.
		var peek struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}

		_ = json.Unmarshal(body, &peek)

		switch peek.Method {
		case "initialize", "notifications/initialized", "tools/list":
			next.ServeHTTP(w, r)

			return
		case "tools/call":
			if _, ok := cfg.publicTools[peek.Params.Name]; ok {
				next.ServeHTTP(w, r)

				return
			}
		}

		token, err := navigaid.GetAuthToken(r.Header)
		if err != nil {
			logger.Debug("mcp: missing authorization token", "error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)

			return
		}

		claims, err := jwks.Validate(token)
		if err != nil {
			logger.Debug("mcp: invalid token", "error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)

			return
		}

		ctx := navigaid.SetAuth(r.Context(), navigaid.AuthInfo{
			AccessToken: token,
			Claims:      claims,
		}, nil)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
