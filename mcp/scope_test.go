package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault/mcp"
	"github.com/navigacontentlab/dindenault/navigaid"
)

const (
	scopeRead  = "articles:read"
	scopeWrite = "articles:write"
	listBody   = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	bearerOK   = "Bearer ok"
	toolOpen   = "open"
	toolRead   = "lookup"
	toolWrite  = "publish"
)

// scopedServer has a tool without scopes (toolOpen), one that needs
// articles:read (toolRead) and one that needs articles:read and articles:write
// (toolWrite). ran records which tool handlers ran.
func scopedServer(rec *auditRecorder, ran *[]string) *mcp.Server {
	tool := func(name string, scopes ...string) mcp.Tool {
		return mcp.Tool{
			Name:           name,
			RequiredScopes: scopes,
			Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
				*ran = append(*ran, name)

				return json.RawMessage(`"ok"`), nil
			},
		}
	}

	return mcp.NewServer("svc", "1",
		tool(toolOpen),
		tool(toolRead, scopeRead),
		tool(toolWrite, scopeRead, scopeWrite),
	).WithAuditSink(rec.sink)
}

func grants(scopes ...string) mcp.AuthOption {
	return mcp.WithScopes(func(*http.Request) []string { return scopes })
}

func listedTools(t *testing.T, rr *httptest.ResponseRecorder) []string {
	t.Helper()

	require.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}

	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	names := make([]string, 0, len(resp.Result.Tools))
	for _, tool := range resp.Result.Tools {
		names = append(names, tool.Name)
	}

	return names
}

func TestScopes_ToolsListLeavesOutUncoveredTools(t *testing.T) {
	for name, tc := range map[string]struct {
		granted []string
		want    []string
	}{
		"none":       {nil, []string{toolOpen}},
		toolRead:     {[]string{scopeRead}, []string{toolOpen, toolRead}},
		"write only": {[]string{scopeWrite}, []string{toolOpen}},
		"read+write": {[]string{scopeWrite, scopeRead}, []string{toolOpen, toolRead, toolWrite}},
	} {
		t.Run(name, func(t *testing.T) {
			var ran []string

			handler := mcp.AuthMiddleware(discardLogger(), validJWKS(), scopedServer(&auditRecorder{}, &ran),
				mcp.WithChallenge(metadataURL), grants(tc.granted...))

			rr := serve(t, handler, listBody, map[string]string{headerAuth: bearerOK})

			assert.Equal(t, tc.want, listedTools(t, rr))
		})
	}
}

func TestScopes_CallOutsideScopesIs403(t *testing.T) {
	rec := &auditRecorder{}

	var ran []string

	handler := mcp.AuthMiddleware(discardLogger(), orgJWKS("acme"), scopedServer(rec, &ran),
		mcp.WithChallenge(metadataURL), grants(scopeRead))

	rr := serve(t, handler, callBody(toolWrite, `{}`), map[string]string{
		headerAuth:              bearerOK,
		mcp.HeaderCorrelationID: testChatID,
	})

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Equal(t,
		`Bearer error="insufficient_scope", scope="articles:read articles:write", `+
			`resource_metadata="`+testMetadataURL+`", error_description="the publish tool needs articles:write"`,
		rr.Header().Get("WWW-Authenticate"))
	assert.Empty(t, ran)

	e := rec.only(t)
	assert.Equal(t, mcp.OutcomeDenied, e.Outcome)
	assert.Equal(t, "missing scopes: [articles:write]", e.Error)
	assert.Equal(t, toolWrite, e.Tool)
	assert.Equal(t, "acme", e.Org)
	assert.Equal(t, testChatID, e.CorrelationID)
	assert.Equal(t, sha(`{}`), e.ArgsSHA256)
}

func TestScopes_CallWithinScopesRuns(t *testing.T) {
	rec := &auditRecorder{}

	var ran []string

	handler := mcp.AuthMiddleware(discardLogger(), validJWKS(), scopedServer(rec, &ran),
		mcp.WithChallenge(metadataURL), grants(scopeRead, scopeWrite))

	for _, tool := range []string{toolOpen, toolRead, toolWrite} {
		rr := serve(t, handler, callBody(tool, `{}`), map[string]string{headerAuth: bearerOK})

		assert.Equal(t, http.StatusOK, rr.Code, tool)
		assert.Empty(t, rr.Header().Get("WWW-Authenticate"), tool)
	}

	assert.Equal(t, []string{toolOpen, toolRead, toolWrite}, ran)
}

func TestScopes_WithoutChallengeOmitsResourceMetadata(t *testing.T) {
	var ran []string

	handler := mcp.AuthMiddleware(discardLogger(), validJWKS(), scopedServer(&auditRecorder{}, &ran))

	rr := serve(t, handler, callBody(toolRead, `{}`), map[string]string{headerAuth: bearerOK})

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Equal(t,
		`Bearer error="insufficient_scope", scope="articles:read", error_description="the lookup tool needs articles:read"`,
		rr.Header().Get("WWW-Authenticate"))
	assert.Empty(t, ran)
}

func TestScopes_ToolsWithoutScopesAreUnchanged(t *testing.T) {
	// No WithScopes and no scopes in the context: a server whose tools set no
	// RequiredScopes lists and runs them as before.
	server := mcp.NewServer("svc", "1", echoTool(), failTool()).WithAuditSink(mcp.DisableAudit)

	assert.Equal(t, []string{"echo", "fail"}, listedTools(t, serve(t, server, listBody, nil)))

	rr := serve(t, server, callBody("echo", `{"x":1}`), nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Header().Get("WWW-Authenticate"))
}

func TestScopes_WithScopesSeesClaimsAndHandlersSeeScopes(t *testing.T) {
	var (
		seenOrg string
		inTool  []string
	)

	server := mcp.NewServer("svc", "1", mcp.Tool{
		Name:           toolRead,
		RequiredScopes: []string{scopeRead},
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			inTool = mcp.GrantedScopesFromContext(ctx)

			return json.RawMessage(`"ok"`), nil
		},
	}).WithAuditSink(mcp.DisableAudit)

	handler := mcp.AuthMiddleware(discardLogger(), orgJWKS("acme"), server,
		mcp.WithScopes(func(r *http.Request) []string {
			if auth, err := navigaid.GetAuth(r.Context()); err == nil {
				seenOrg = auth.Claims.Org
			}

			return []string{scopeRead}
		}))

	rr := serve(t, handler, callBody(toolRead, `{}`), map[string]string{headerAuth: bearerOK})

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "acme", seenOrg)
	assert.Equal(t, []string{scopeRead}, inTool)
}

func TestScopes_ContextWithGrantedScopesWithoutMiddleware(t *testing.T) {
	var ran []string

	server := scopedServer(&auditRecorder{}, &ran)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(listBody))
	req = req.WithContext(mcp.ContextWithGrantedScopes(req.Context(), []string{scopeRead}))

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	assert.Equal(t, []string{toolOpen, toolRead}, listedTools(t, rr))
}

func TestScopes_PublicToolWithScopesIsStillRefused(t *testing.T) {
	// WithPublicTools skips the token, so no scopes are granted: a public
	// tool that lists RequiredScopes fails closed rather than open.
	var ran []string

	handler := mcp.AuthMiddleware(discardLogger(), validJWKS(), scopedServer(&auditRecorder{}, &ran),
		mcp.WithChallenge(metadataURL), mcp.WithPublicTools(toolRead), grants(scopeRead))

	rr := serve(t, handler, callBody(toolRead, `{}`), nil)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Empty(t, ran)
}
