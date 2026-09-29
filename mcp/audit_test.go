package mcp_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault/mcp"
	"github.com/navigacontentlab/dindenault/navigaid"
)

// ── helpers ───────────────────────────────────────────────────────────────────

const (
	testChatID = "chat-1"
	testTurnID = "turn-7"
	headerAuth = "Authorization"
)

type auditRecorder struct {
	mu     sync.Mutex
	events []mcp.AuditEvent
}

func (a *auditRecorder) sink(_ context.Context, e mcp.AuditEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.events = append(a.events, e)
}

func (a *auditRecorder) only(t *testing.T) mcp.AuditEvent {
	t.Helper()

	a.mu.Lock()
	defer a.mu.Unlock()

	require.Len(t, a.events, 1, "expected exactly one audit event")

	return a.events[0]
}

func callBody(tool, args string) string {
	return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + args + `}}`
}

func serve(t *testing.T, h http.Handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	return rr
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))

	return hex.EncodeToString(sum[:])
}

// ── server audit ──────────────────────────────────────────────────────────────

func TestAudit_SuccessfulCall(t *testing.T) {
	rec := &auditRecorder{}
	tool := echoTool()
	tool.Annotations = mcp.ReadOnly()
	server := mcp.NewServer("editorial-mcp", "1", tool).WithAuditSink(rec.sink)

	args := `{"message":"hi"}`
	serve(t, server, callBody("echo", args), map[string]string{
		mcp.HeaderCorrelationID: testChatID,
		mcp.HeaderTurnID:        testTurnID,
		mcp.HeaderTraceParent:   "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	})

	e := rec.only(t)
	assert.Equal(t, mcp.AuditEventName, e.Event)
	assert.Equal(t, mcp.AuditVersion, e.AuditVersion)
	assert.Equal(t, "editorial-mcp", e.Service)
	assert.Equal(t, "echo", e.Tool)
	assert.True(t, e.ReadOnly)
	assert.Equal(t, mcp.OutcomeOK, e.Outcome)
	assert.Empty(t, e.Error)
	assert.Equal(t, testChatID, e.CorrelationID)
	assert.Equal(t, testTurnID, e.TurnID)
	assert.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", e.TraceParent)
	assert.Equal(t, sha(args), e.ArgsSHA256)
	assert.Equal(t, len(args), e.ArgsBytes)
	assert.Equal(t, len(args), e.ResultBytes, "echo returns its input")
	assert.NotEmpty(t, e.Time)
}

func TestAudit_ArgumentsAreNeverRecorded(t *testing.T) {
	var buf bytes.Buffer

	server := mcp.NewServer("s", "1", echoTool()).WithAuditSink(mcp.NewJSONAuditSink(&buf))
	serve(t, server, callBody("echo", `{"message":"secret article text"}`), nil)

	line := buf.String()
	assert.NotContains(t, line, "secret article text")
	assert.True(t, strings.HasSuffix(line, "\n"), "one JSON line per event")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &decoded))
	assert.Equal(t, "mcp_tool_call", decoded["event"])
	assert.Equal(t, "ok", decoded["outcome"])
}

func TestAudit_Outcomes(t *testing.T) {
	permTool := echoTool()
	permTool.Name = "guarded"
	permTool.RequiredPermissions = []string{"write"}

	cases := []struct {
		name    string
		body    string
		outcome string
		errText string
	}{
		{"tool error", callBody("fail", `{}`), mcp.OutcomeToolError, "something went wrong"},
		{"unknown tool", callBody("nope", `{}`), mcp.OutcomeUnknownTool, ""},
		{"invalid params", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"x"}`, mcp.OutcomeInvalidParams, "json"},
		{"permissions without auth", callBody("guarded", `{}`), mcp.OutcomeUnauthenticated, "no validated auth"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := &auditRecorder{}
			server := mcp.NewServer("s", "1", failTool(), permTool).WithAuditSink(rec.sink)

			serve(t, server, c.body, nil)

			e := rec.only(t)
			assert.Equal(t, c.outcome, e.Outcome)
			assert.Contains(t, e.Error, c.errText)
		})
	}
}

func TestAudit_DeniedCarriesCaller(t *testing.T) {
	rec := &auditRecorder{}
	tool := echoTool()
	tool.RequiredPermissions = []string{"write"}
	server := mcp.NewServer("s", "1", tool).WithAuditSink(rec.sink)

	jwks := makeJWKS(func(_ string) (navigaid.Claims, error) {
		c := navigaid.Claims{Org: "acme"}
		c.Subject = "user-1"

		return c, nil
	})
	handler := mcp.AuthMiddleware(discardLogger(), jwks, server)

	serve(t, handler, callBody("echo", `{"message":"x"}`), map[string]string{headerAuth: "Bearer t"})

	e := rec.only(t)
	assert.Equal(t, mcp.OutcomeDenied, e.Outcome)
	assert.Equal(t, "acme", e.Org)
	assert.Equal(t, "user-1", e.Subject)
}

func TestAudit_OnlyToolsCallIsAudited(t *testing.T) {
	rec := &auditRecorder{}
	server := mcp.NewServer("s", "1", echoTool()).WithAuditSink(rec.sink)

	serve(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	serve(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, nil)

	assert.Empty(t, rec.events)
}

func TestAudit_DisableAudit(t *testing.T) {
	var buf bytes.Buffer

	sink := mcp.NewJSONAuditSink(&buf)
	server := mcp.NewServer("s", "1", echoTool()).WithAuditSink(sink).WithAuditSink(mcp.DisableAudit)
	serve(t, server, callBody("echo", `{}`), nil)

	assert.Empty(t, buf.String())
}

func TestAudit_ServiceNameFallsBackToLambdaFunction(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "NavigaEditorialMCP-dev")

	rec := &auditRecorder{}
	server := mcp.NewServer("dindenault", "1.0.0", echoTool()).WithAuditSink(rec.sink)
	serve(t, server, callBody("echo", `{}`), nil)

	assert.Equal(t, "NavigaEditorialMCP-dev", rec.only(t).Service)
}

func TestAudit_LongCorrelationValuesAreClipped(t *testing.T) {
	rec := &auditRecorder{}
	server := mcp.NewServer("s", "1", echoTool()).WithAuditSink(rec.sink)
	serve(t, server, callBody("echo", `{}`), map[string]string{mcp.HeaderTurnID: strings.Repeat("a", 5000)})

	assert.Len(t, rec.only(t).TurnID, 128)
}

// ── rejected by AuthMiddleware ────────────────────────────────────────────────

func TestAudit_UnauthenticatedCallIsAuditedByMiddleware(t *testing.T) {
	rec := &auditRecorder{}
	server := mcp.NewServer("editorial-mcp", "1", echoTool()).WithAuditSink(rec.sink)
	handler := mcp.AuthMiddleware(discardLogger(), invalidJWKS(), server)

	rr := serve(t, handler, callBody("echo", `{}`), map[string]string{
		headerAuth:              "Bearer bad",
		mcp.HeaderCorrelationID: testChatID,
	})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	e := rec.only(t)
	assert.Equal(t, mcp.OutcomeUnauthenticated, e.Outcome)
	assert.Equal(t, "invalid token", e.Error)
	assert.Equal(t, "editorial-mcp", e.Service)
	assert.Equal(t, "echo", e.Tool)
	assert.Equal(t, testChatID, e.CorrelationID)
}

func TestAudit_RejectedDiscoveryIsNotAudited(t *testing.T) {
	rec := &auditRecorder{}
	handler := mcp.AuthMiddleware(discardLogger(), invalidJWKS(),
		http.NotFoundHandler(), mcp.WithAuthAuditSink(rec.sink))

	// A non-tools/call method without a token is rejected but is not a tool call.
	serve(t, handler, `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`, nil)
	assert.Empty(t, rec.events)

	serve(t, handler, callBody("x", `{}`), nil)
	assert.Equal(t, "missing token", rec.only(t).Error)
}

// ── correlation in handlers and downstream ────────────────────────────────────

func TestCorrelationFromContext_InHandler(t *testing.T) {
	var got mcp.Correlation

	tool := mcp.Tool{
		Name: "peek",
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			got = mcp.CorrelationFromContext(ctx)

			return json.Marshal("ok")
		},
	}
	server := mcp.NewServer("s", "1", tool).WithAuditSink(mcp.DisableAudit)
	serve(t, server, callBody("peek", `{}`), map[string]string{
		mcp.HeaderCorrelationID: testChatID,
		mcp.HeaderTurnID:        testTurnID,
	})

	assert.Equal(t, mcp.Correlation{ID: testChatID, TurnID: testTurnID}, got)
}

func TestNewHTTPClient_ForwardsCorrelationHeaders(t *testing.T) {
	var got http.Header

	ds := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	t.Cleanup(ds.Close)

	server := mcp.NewServer("s", "1", fetchTool(ds.URL, nil)).WithAuditSink(mcp.DisableAudit)
	serve(t, server, callBody("fetch", `{}`), map[string]string{
		headerAuth:              "Bearer t",
		mcp.HeaderCorrelationID: testChatID,
		mcp.HeaderTurnID:        testTurnID,
	})

	require.NotNil(t, got)
	assert.Equal(t, "Bearer t", got.Get(headerAuth))
	assert.Equal(t, testChatID, got.Get(mcp.HeaderCorrelationID))
	assert.Equal(t, testTurnID, got.Get(mcp.HeaderTurnID))
	assert.Empty(t, got.Get(mcp.HeaderTraceParent), "absent headers are not invented")
}

// ── traceparent from params._meta ─────────────────────────────────────────────

const metaTraceParent = "00-b10d0934bdecf35cd484e3e4192abe7e-4bea83b5ef762bb7-01"

func callWithMeta(tool, meta string) string {
	return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool +
		`","arguments":{},"_meta":` + meta + `}}`
}

func TestAudit_TraceParentFromMeta(t *testing.T) {
	var downstream http.Header

	ds := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		downstream = r.Header.Clone()
	}))
	t.Cleanup(ds.Close)

	rec := &auditRecorder{}
	server := mcp.NewServer("s", "1", fetchTool(ds.URL, nil)).WithAuditSink(rec.sink)
	serve(t, server, callWithMeta("fetch", `{"traceparent":"`+metaTraceParent+`"}`), nil)

	assert.Equal(t, metaTraceParent, rec.only(t).TraceParent)
	require.NotNil(t, downstream)
	assert.Equal(t, metaTraceParent, downstream.Get(mcp.HeaderTraceParent), "forwarded downstream as a header")
}

func TestAudit_TraceParentHeaderWinsOverMeta(t *testing.T) {
	rec := &auditRecorder{}
	server := mcp.NewServer("s", "1", echoTool()).WithAuditSink(rec.sink)
	serve(t, server, callWithMeta("echo", `{"traceparent":"`+metaTraceParent+`"}`),
		map[string]string{mcp.HeaderTraceParent: "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"})

	assert.Equal(t, "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01", rec.only(t).TraceParent)
}

func TestAudit_NonStringMetaTraceParentIgnored(t *testing.T) {
	rec := &auditRecorder{}
	server := mcp.NewServer("s", "1", echoTool()).WithAuditSink(rec.sink)
	serve(t, server, callWithMeta("echo", `{"traceparent":42}`), nil)

	assert.Empty(t, rec.only(t).TraceParent)
}

func TestAudit_RejectedCallKeepsMetaTraceParent(t *testing.T) {
	rec := &auditRecorder{}
	handler := mcp.AuthMiddleware(discardLogger(), invalidJWKS(),
		mcp.NewServer("s", "1", echoTool()).WithAuditSink(rec.sink))
	serve(t, handler, callWithMeta("echo", `{"traceparent":"`+metaTraceParent+`"}`), nil)

	e := rec.only(t)
	assert.Equal(t, mcp.OutcomeUnauthenticated, e.Outcome)
	assert.Equal(t, metaTraceParent, e.TraceParent)
}
