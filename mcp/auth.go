package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/navigacontentlab/dindenault/navigaid"
)

// AuthOption configures optional behaviour of AuthMiddleware.
type AuthOption func(*authConfig)

type authConfig struct {
	publicTools map[string]struct{}
	audit       AuditSink
	metadataURL func(r *http.Request) string
	checkClaims func(r *http.Request, claims navigaid.Claims) error
	scopes      func(r *http.Request) []string
}

// WithScopes tells AuthMiddleware which OAuth scopes the caller's token was
// granted, for tools that set RequiredScopes. scopes runs once the token has
// been validated, with the claims already in r's context (navigaid.GetAuth),
// and its result is what GrantedScopesFromContext returns. The Server leaves
// tools whose RequiredScopes the scopes do not cover out of tools/list, and
// answers a tools/call to one with 403 and
//
//	WWW-Authenticate: Bearer error="insufficient_scope", scope="<granted + needed>", resource_metadata="…"
//
// (resource_metadata only with WithChallenge), audited as denied. Tools
// without RequiredScopes are not affected.
func WithScopes(scopes func(r *http.Request) []string) AuthOption {
	return func(c *authConfig) {
		c.scopes = scopes
	}
}

// WithChallenge turns on the MCP authorization spec's behaviour, which clients
// such as Claude need before they start an OAuth login: every request needs a
// valid token, initialize and tools/list included, and every 401 carries
//
//	WWW-Authenticate: Bearer resource_metadata="<metadataURL(r)>"
//
// with error="invalid_token" when a token was sent but refused. metadataURL
// returns the RFC 9728 protected resource metadata URL for the request, which
// names the authorization server. Tools exempted with WithPublicTools stay
// public, and only rejected tools/call requests are audited, so the 401 every
// client gets when it first connects does not fill the audit log.
//
// Without this option, discovery methods need no token and a 401 has no
// challenge. That suits agents that are handed a token up front, but an MCP
// client never learns that it should log in.
func WithChallenge(metadataURL func(r *http.Request) string) AuthOption {
	return func(c *authConfig) {
		c.metadataURL = metadataURL
	}
}

// WithClaimsCheck adds a check that runs after the token has been validated,
// for rules the signature alone cannot express, such as a token having to
// belong to the organisation named by the request's hostname. A non-nil error
// rejects the request like an invalid token: 401, with error="invalid_token"
// when WithChallenge is set, and an unauthenticated audit event for a
// tools/call. The error is logged at info level and not sent to the client.
func WithClaimsCheck(check func(r *http.Request, claims navigaid.Claims) error) AuthOption {
	return func(c *authConfig) {
		c.checkClaims = check
	}
}

// WithAuthAuditSink sets where AuthMiddleware sends the audit event for a
// tools/call it rejects as unauthenticated. By default it uses the wrapped
// *Server's sink (see Server.WithAuditSink), or DefaultAuditSink when the
// wrapped handler is not a *Server.
func WithAuthAuditSink(sink AuditSink) AuthOption {
	return func(c *authConfig) {
		c.audit = sink
	}
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
// exempt from authentication — they carry no user data and agents use them
// for discovery before a token is available. For MCP clients that log the
// user in themselves (Claude, VS Code), use WithChallenge instead: it requires
// a token on every request and answers with the challenge that starts the
// client's OAuth login.
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
				Name string         `json:"name"`
				Meta map[string]any `json:"_meta"`
			} `json:"params"`
		}

		_ = json.Unmarshal(body, &peek)

		switch peek.Method {
		case "initialize", "notifications/initialized", "tools/list":
			if cfg.metadataURL == nil {
				next.ServeHTTP(w, r)

				return
			}
		case methodToolsCall:
			if _, ok := cfg.publicTools[peek.Params.Name]; ok {
				next.ServeHTTP(w, r)

				return
			}
		}

		// A rejected tools/call never reaches the server, so it is audited
		// here — failed calls are what a security team alerts on.
		rejectCall := func(reason, errCode string) {
			if peek.Method == methodToolsCall {
				auditRejected(r, cfg, next, peek.Params.Name, peek.Params.Meta, reason)
			}

			if cfg.metadataURL != nil {
				w.Header().Set("WWW-Authenticate", challenge(cfg.metadataURL(r), errCode))
			}

			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}

		token, err := navigaid.GetAuthToken(r.Header)
		if err != nil {
			logger.Debug("mcp: missing authorization token", "error", err)
			rejectCall("missing token", "")

			return
		}

		claims, err := jwks.Validate(token)
		if err != nil {
			logger.Debug("mcp: invalid token", "error", err)
			rejectCall("invalid token", "invalid_token")

			return
		}

		if cfg.checkClaims != nil {
			if err := cfg.checkClaims(r, claims); err != nil {
				logger.InfoContext(r.Context(), "mcp: token refused by claims check", "error", err)
				rejectCall("claims check failed", "invalid_token")

				return
			}
		}

		ctx := navigaid.SetAuth(r.Context(), navigaid.AuthInfo{
			AccessToken: token,
			Claims:      claims,
		}, nil)

		if cfg.metadataURL != nil {
			ctx = context.WithValue(ctx, metadataURLKey, cfg.metadataURL(r))
		}

		if cfg.scopes != nil {
			ctx = ContextWithGrantedScopes(ctx, cfg.scopes(r.WithContext(ctx)))
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// challenge is the WWW-Authenticate value of a 401 (RFC 6750, RFC 9728).
func challenge(metadataURL, errCode string) string {
	value := fmt.Sprintf(`Bearer resource_metadata=%q`, metadataURL)
	if errCode != "" {
		value += fmt.Sprintf(`, error=%q`, errCode)
	}

	return value
}

func auditRejected(r *http.Request, cfg *authConfig, next http.Handler, tool string, meta map[string]any, reason string) {
	sink, name := cfg.audit, ""

	// The call never reaches the server, so the tool is looked up here: its
	// read_only must match what an accepted call to it would record.
	var registered *Tool

	if srv, ok := next.(*Server); ok {
		name = srv.name
		registered = srv.toolMap[tool]

		if sink == nil {
			sink = srv.audit
		}
	}

	if sink == nil {
		sink = DefaultAuditSink
	}

	ctx := withMetaTraceParent(withCorrelation(r.Context(), r), meta)
	event := newAuditEvent(ctx, auditServiceName(name), tool)
	event.Outcome = OutcomeUnauthenticated
	event.Error = reason
	event.ReadOnly = registered.readOnly()
	sink(ctx, event)
}
