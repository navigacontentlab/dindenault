package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault/mcp"
	"github.com/navigacontentlab/dindenault/navigaid"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makeJWKS(fn navigaid.ValidateFunc) *navigaid.JWKS {
	j := navigaid.NewJWKS("http://test.invalid/jwks")
	j.SetValidationFunc(fn)

	return j
}

func validJWKS() *navigaid.JWKS {
	return makeJWKS(func(_ string) (navigaid.Claims, error) {
		return navigaid.Claims{}, nil
	})
}

func invalidJWKS() *navigaid.JWKS {
	return makeJWKS(func(_ string) (navigaid.Claims, error) {
		return navigaid.Claims{}, errors.New("token signature is invalid")
	})
}

func postThrough(t *testing.T, jwks *navigaid.JWKS, authHeader string) (statusCode int, nextCalled bool) {
	t.Helper()

	var called bool

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// Use tools/call so auth is enforced (tools/list is intentionally public).
	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x","arguments":{}}}`))

	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	rr := httptest.NewRecorder()
	mcp.AuthMiddleware(discardLogger(), jwks, next).ServeHTTP(rr, req)

	return rr.Code, called
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestAuthMiddleware_NoToken_Returns401(t *testing.T) {
	code, called := postThrough(t, validJWKS(), "")

	assert.Equal(t, http.StatusUnauthorized, code)
	assert.False(t, called, "next must not be called without a token")
}

func TestAuthMiddleware_InvalidToken_Returns401(t *testing.T) {
	code, called := postThrough(t, invalidJWKS(), "Bearer bad.token.value")

	assert.Equal(t, http.StatusUnauthorized, code)
	assert.False(t, called, "next must not be called with an invalid token")
}

func TestAuthMiddleware_DiscoveryMethods_NoAuthRequired(t *testing.T) {
	for _, method := range []string{"initialize", "notifications/initialized", "tools/list"} {
		t.Run(method, func(t *testing.T) {
			var called bool

			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{}}`
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			// Intentionally no Authorization header.

			rr := httptest.NewRecorder()
			mcp.AuthMiddleware(discardLogger(), validJWKS(), next).ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code, "method %q should not require auth", method)
			assert.True(t, called, "next must be called for discovery method %q without a token", method)
		})
	}
}

func TestAuthMiddleware_ValidToken_CallsNext(t *testing.T) {
	code, called := postThrough(t, validJWKS(), "Bearer valid.token.value")

	assert.Equal(t, http.StatusOK, code)
	assert.True(t, called)
}

func TestAuthMiddleware_ClaimsInContext(t *testing.T) {
	const org = "test-org"

	jwks := makeJWKS(func(_ string) (navigaid.Claims, error) {
		c := navigaid.Claims{}
		c.Org = org

		return c, nil
	})

	var gotOrg string

	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		auth, err := navigaid.GetAuth(r.Context())
		require.NoError(t, err)

		gotOrg = auth.Claims.Org
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer valid.token.value")

	mcp.AuthMiddleware(discardLogger(), jwks, next).ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, org, gotOrg)
}

func TestAuthMiddleware_AuthorizationHeaderPreservedForNext(t *testing.T) {
	const token = "Bearer eyJhbGciOiJSUzI1NiJ9.payload.sig" //nolint:gosec

	var gotHeader string

	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", token)

	mcp.AuthMiddleware(discardLogger(), validJWKS(), next).ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, token, gotHeader,
		"header must be intact so the MCP server can set AuthorizationFromContext for tool handlers")
}

// TestAuthMiddleware_Integration_BothContextValuesAvailable verifies that when
// AuthMiddleware wraps a real MCP server, tool handlers can use both
// navigaid.GetAuth (for claims) and mcp.AuthorizationFromContext (for forwarding
// the raw token to downstream services like OC or CCA).
func TestAuthMiddleware_Integration_BothContextValuesAvailable(t *testing.T) {
	const org = "acme"
	const token = "Bearer valid.jwt" //nolint:gosec

	var gotOrg, gotRawToken string

	jwks := makeJWKS(func(_ string) (navigaid.Claims, error) {
		c := navigaid.Claims{}
		c.Org = org

		return c, nil
	})

	server := mcp.NewServer("s", "1", mcp.Tool{
		Name: "capture",
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			auth, err := navigaid.GetAuth(ctx)
			require.NoError(t, err)

			gotOrg = auth.Claims.Org
			gotRawToken = mcp.AuthorizationFromContext(ctx)

			return json.Marshal(map[string]string{"org": gotOrg})
		},
	})

	handler := mcp.AuthMiddleware(discardLogger(), jwks, server)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/call",
		"params": {"name": "capture", "arguments": {}}
	}`))
	req.Header.Set("Authorization", token)

	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, org, gotOrg, "navigaid.GetAuth should return validated claims")
	assert.Equal(t, token, gotRawToken, "mcp.AuthorizationFromContext should return raw token for forwarding to OC/CCA")
}
