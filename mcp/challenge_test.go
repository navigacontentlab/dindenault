package mcp_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault/mcp"
	"github.com/navigacontentlab/dindenault/navigaid"
)

const testMetadataURL = "https://mcp.example.com/.well-known/oauth-protected-resource"

func metadataURL(*http.Request) string { return testMetadataURL }

func orgJWKS(org string) *navigaid.JWKS {
	return makeJWKS(func(_ string) (navigaid.Claims, error) {
		return navigaid.Claims{Org: org}, nil
	})
}

// challengeThrough posts body through AuthMiddleware and reports the status,
// the WWW-Authenticate header and whether next ran.
func challengeThrough(t *testing.T, jwks *navigaid.JWKS, body, auth string, opts ...mcp.AuthOption) (int, string, bool) {
	t.Helper()

	var called bool

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	if auth != "" {
		req.Header.Set(headerAuth, auth)
	}

	rr := httptest.NewRecorder()
	mcp.AuthMiddleware(discardLogger(), jwks, next, opts...).ServeHTTP(rr, req)

	return rr.Code, rr.Header().Get("WWW-Authenticate"), called
}

func TestWithChallenge_DiscoveryNeedsAToken(t *testing.T) {
	for _, method := range []string{"initialize", "notifications/initialized", "tools/list"} {
		t.Run(method, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{}}`

			code, header, called := challengeThrough(t, validJWKS(), body, "", mcp.WithChallenge(metadataURL))

			assert.Equal(t, http.StatusUnauthorized, code)
			assert.Equal(t, `Bearer resource_metadata="`+testMetadataURL+`"`, header)
			assert.False(t, called)
		})
	}
}

func TestWithChallenge_InvalidTokenNamesTheError(t *testing.T) {
	code, header, called := challengeThrough(t, invalidJWKS(), callBody("x", `{}`), "Bearer bad",
		mcp.WithChallenge(metadataURL))

	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, `Bearer resource_metadata="`+testMetadataURL+`", error="invalid_token"`, header)
	assert.False(t, called)
}

func TestWithChallenge_ValidTokenPassesEveryMethod(t *testing.T) {
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		callBody("x", `{}`),
	} {
		code, header, called := challengeThrough(t, validJWKS(), body, "Bearer ok", mcp.WithChallenge(metadataURL))

		assert.Equal(t, http.StatusOK, code)
		assert.Empty(t, header)
		assert.True(t, called)
	}
}

func TestWithChallenge_PublicToolsStayPublic(t *testing.T) {
	code, _, called := challengeThrough(t, validJWKS(), callBody("open", `{}`), "",
		mcp.WithChallenge(metadataURL), mcp.WithPublicTools("open"))

	assert.Equal(t, http.StatusOK, code)
	assert.True(t, called)
}

func TestWithChallenge_RejectedInitializeIsNotAudited(t *testing.T) {
	rec := &auditRecorder{}
	handler := mcp.AuthMiddleware(discardLogger(), validJWKS(), http.NotFoundHandler(),
		mcp.WithChallenge(metadataURL), mcp.WithAuthAuditSink(rec.sink))

	// Every client sends this once, without a token, to find out how to log in.
	serve(t, handler, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, nil)
	assert.Empty(t, rec.events)
}

func TestWithoutChallenge_NoHeader(t *testing.T) {
	code, header, _ := challengeThrough(t, invalidJWKS(), callBody("x", `{}`), "Bearer bad")

	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Empty(t, header)
}

func TestWithClaimsCheck(t *testing.T) {
	sameOrg := mcp.WithClaimsCheck(func(_ *http.Request, claims navigaid.Claims) error {
		if claims.Org != "tryout" {
			return errors.New("token for another organisation")
		}

		return nil
	})

	t.Run("passes", func(t *testing.T) {
		code, _, called := challengeThrough(t, orgJWKS("tryout"), callBody("x", `{}`), "Bearer ok", sameOrg)

		assert.Equal(t, http.StatusOK, code)
		assert.True(t, called)
	})

	t.Run("refuses like an invalid token", func(t *testing.T) {
		rec := &auditRecorder{}

		code, header, called := challengeThrough(t, orgJWKS("other"), callBody("x", `{}`), "Bearer ok",
			sameOrg, mcp.WithChallenge(metadataURL), mcp.WithAuthAuditSink(rec.sink))

		assert.Equal(t, http.StatusUnauthorized, code)
		assert.Equal(t, `Bearer resource_metadata="`+testMetadataURL+`", error="invalid_token"`, header)
		assert.False(t, called)

		require.Len(t, rec.events, 1)
		assert.Equal(t, mcp.OutcomeUnauthenticated, rec.events[0].Outcome)
		assert.Equal(t, "claims check failed", rec.events[0].Error)
	})
}
