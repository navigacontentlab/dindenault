package mcp

import (
	"log/slog"
	"net/http"

	"github.com/navigacontentlab/dindenault/navigaid"
)

// AuthMiddleware validates the incoming JWT before passing the request to the
// MCP handler. Requests with no token or an invalid token are rejected with
// HTTP 401 before any tool logic runs.
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
func AuthMiddleware(logger *slog.Logger, jwks *navigaid.JWKS, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
