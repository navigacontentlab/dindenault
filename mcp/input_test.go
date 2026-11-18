package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault/mcp"
	"github.com/navigacontentlab/dindenault/navigaid"
)

var stateKey = []byte("0123456789abcdef0123456789abcdef") //nolint:gochecknoglobals // test key

const (
	capsElicit   = `{"elicitation":{}}`
	capsNone     = `{}`
	capsURLOnly  = `{"elicitation":{"url":{}}}`
	capsFormOnly = `{"elicitation":{"form":{}}}`

	toolApprove   = "approve"
	badSignature  = "bad signature"
	anotherCaller = "another caller"
	argsX1        = `{"x":1}`
)

type approveState struct {
	IDs []string `json:"ids"`
}

// approveTool asks once which of the ids to approve and returns the answer.
func approveTool(seen *mcp.Input) mcp.Tool {
	return mcp.Tool{
		Name: toolApprove,
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			in := mcp.InputFromContext(ctx)
			if seen != nil {
				*seen = in
			}

			if !in.CanElicit() {
				return nil, assert.AnError
			}

			if !in.Retry() {
				return nil, mcp.RequireInput(approveState{IDs: []string{"a", "b"}}, map[string]mcp.Elicitation{
					toolApprove: {
						Message:         "Approve 2 changes?",
						RequestedSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
					},
				})
			}

			var st approveState
			if err := in.State(&st); err != nil {
				return nil, fmt.Errorf("state: %w", err)
			}

			ans, ok := in.Elicited(toolApprove)
			if !ok {
				return nil, assert.AnError
			}

			return json.Marshal(map[string]any{"ids": st.IDs, "action": ans.Action, "content": ans.Content})
		},
	}
}

func newInputServer(rec *auditRecorder, tools ...mcp.Tool) *mcp.Server {
	s := mcp.NewServer("s", "1", tools...).WithInputRequired(stateKey, time.Minute)
	if rec != nil {
		s.WithAuditSink(rec.sink)
	} else {
		s.WithAuditSink(mcp.DisableAudit)
	}

	return s
}

// callModern sends a 2026-07-28 tools/call; extra is spliced into params.
func callModern(t *testing.T, h http.Handler, caps, args, extra string, auth *navigaid.AuthInfo) map[string]any {
	t.Helper()

	rr := callModernRaw(t, h, caps, args, extra, auth)

	return decodeResponse(t, rr.Body)
}

func callModernRaw(t *testing.T, h http.Handler, caps, args, extra string, auth *navigaid.AuthInfo) *httptest.ResponseRecorder {
	t.Helper()

	params := `"name":"approve","arguments":` + args +
		`,"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":` + caps + `}`
	if extra != "" {
		params += "," + extra
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{`+params+`}}`))
	req.Header.Set("MCP-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", "approve")

	if auth != nil {
		req = req.WithContext(navigaid.SetAuth(req.Context(), *auth, nil))
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	return rr
}

func user(org, sub string) *navigaid.AuthInfo {
	return &navigaid.AuthInfo{Claims: navigaid.Claims{Org: org, RegisteredClaims: jwt.RegisteredClaims{Subject: sub}}}
}

// askFirst makes the first call and returns its requestState.
func askFirst(t *testing.T, h http.Handler, args string, auth *navigaid.AuthInfo) string {
	t.Helper()

	res := callModern(t, h, capsElicit, args, "", auth)["result"].(map[string]any)
	require.Equal(t, "input_required", res["resultType"])

	state, ok := res["requestState"].(string)
	require.True(t, ok)

	return state
}

func answer(state string) string {
	return `"requestState":"` + state + `","inputResponses":{"approve":{"action":"accept","content":{"ok":true}}}`
}

func TestInput_AsksWithInputRequiredResult(t *testing.T) {
	t.Parallel()

	rec := &auditRecorder{}
	res := callModern(t, newInputServer(rec, approveTool(nil)), capsElicit, argsX1, "", nil)["result"].(map[string]any)

	assert.Equal(t, "input_required", res["resultType"])
	assert.NotContains(t, res, "content")
	assert.NotEmpty(t, res["requestState"])

	reqs := res["inputRequests"].(map[string]any)
	el := reqs["approve"].(map[string]any)
	assert.Equal(t, "elicitation/create", el["method"])

	params := el["params"].(map[string]any)
	assert.Equal(t, "form", params["mode"])
	assert.Equal(t, "Approve 2 changes?", params["message"])
	assert.Equal(t, "object", params["requestedSchema"].(map[string]any)["type"])

	assert.Equal(t, mcp.OutcomeInputRequired, rec.only(t).Outcome)
}

func TestInput_RetryGetsStateAndAnswers(t *testing.T) {
	t.Parallel()

	for _, caps := range []string{capsElicit, capsFormOnly} {
		var seen mcp.Input

		srv := newInputServer(nil, approveTool(&seen))
		auth := user("org1", "u1")

		res := callModern(t, srv, caps, `{"x":1,"y":[1,2]}`, "", auth)["result"].(map[string]any)
		state := res["requestState"].(string)

		// Same arguments, other key order and whitespace.
		out := callModern(t, srv, caps, `{ "y":[1, 2], "x":1 }`, answer(state), auth)["result"].(map[string]any)
		require.Equal(t, "complete", out["resultType"], out)
		assert.True(t, seen.Retry())

		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(out["content"].([]any)[0].(map[string]any)["text"].(string)), &got))
		assert.Equal(t, []any{"a", "b"}, got["ids"])
		assert.Equal(t, "accept", got["action"])
		assert.Equal(t, map[string]any{"ok": true}, got["content"])
	}
}

func TestInput_RefusesBadState(t *testing.T) {
	t.Parallel()

	srv := newInputServer(nil, approveTool(nil))
	auth := user("org1", "u1")
	state := askFirst(t, srv, argsX1, auth)

	payload, sig, _ := strings.Cut(state, ".")
	tampered := payload[:len(payload)-2] + "AA." + sig

	cases := map[string]struct {
		state, args string
		auth        *navigaid.AuthInfo
		want        string
	}{
		"tampered":      {tampered, argsX1, auth, badSignature},
		"garbage":       {"not-a-state", argsX1, auth, badSignature},
		"other args":    {state, `{"x":2}`, auth, "another call"},
		"other user":    {state, argsX1, user("org1", "u2"), anotherCaller},
		"other org":     {state, argsX1, user("org2", "u1"), anotherCaller},
		"no auth":       {state, argsX1, nil, anotherCaller},
		"other key sig": {askFirst(t, mcp.NewServer("s", "1", approveTool(nil)).WithAuditSink(mcp.DisableAudit).WithInputRequired([]byte(strings.Repeat("k", 32)), 0), argsX1, auth), argsX1, auth, badSignature},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rr := callModernRaw(t, srv, capsElicit, tc.args, answer(tc.state), tc.auth)
			assert.Equal(t, http.StatusBadRequest, rr.Code)

			e := decodeResponse(t, rr.Body)["error"].(map[string]any)
			assert.InDelta(t, -32602, e["code"], 0)
			assert.Contains(t, e["message"], "requestState rejected")
			assert.Contains(t, e["message"], tc.want)
		})
	}
}

func TestInput_RefusesExpiredState(t *testing.T) {
	t.Parallel()

	rec := &auditRecorder{}
	srv := newInputServer(rec, approveTool(nil))
	now := time.Now()
	mcp.SetStateClock(srv, func() time.Time { return now })

	state := askFirst(t, srv, `{}`, nil)

	now = now.Add(2 * time.Minute)
	rr := callModernRaw(t, srv, capsElicit, `{}`, answer(state), nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "expired")
}

func TestInput_NotOfferedWithoutClientCapability(t *testing.T) {
	t.Parallel()

	for _, caps := range []string{capsNone, capsURLOnly, `{"elicitation":true}`} {
		var seen mcp.Input

		res := callModern(t, newInputServer(nil, approveTool(&seen)), caps, `{}`, "", nil)["result"].(map[string]any)
		assert.False(t, seen.CanElicit(), caps)
		assert.Equal(t, true, res["isError"], caps)
		assert.Equal(t, "complete", res["resultType"])
	}
}

func TestInput_NotOfferedWithoutKey(t *testing.T) {
	t.Parallel()

	var seen mcp.Input

	srv := mcp.NewServer("s", "1", approveTool(&seen)).WithAuditSink(mcp.DisableAudit)
	res := callModern(t, srv, capsElicit, `{}`, "", nil)["result"].(map[string]any)
	assert.False(t, seen.CanElicit())
	assert.Equal(t, true, res["isError"])

	rr := callModernRaw(t, srv, capsElicit, `{}`, answer("x.y"), nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "issues none")
}

func TestInput_NotOfferedToLegacyClients(t *testing.T) {
	t.Parallel()

	var seen mcp.Input

	srv := newInputServer(nil, approveTool(&seen))
	// A legacy request is not read for requestState at all.
	rr := serve(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"approve","arguments":{},`+
		`"requestState":"x.y","inputResponses":{"approve":{"action":"accept"}}}}`, nil)
	require.Equal(t, http.StatusOK, rr.Code)

	res := decodeResponse(t, rr.Body)["result"].(map[string]any)
	assert.False(t, seen.CanElicit())
	assert.False(t, seen.Retry())
	assert.Equal(t, true, res["isError"])
	assert.NotContains(t, res, "resultType")
}

// A handler that asks anyway, without CanElicit, gets ErrInputNotSupported as
// its tool error: the server never sends a request the client did not declare.
func TestInput_RequireInputWithoutSupportIsToolError(t *testing.T) {
	t.Parallel()

	tool := mcp.Tool{Name: toolApprove, Handler: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, mcp.RequireInput(nil, map[string]mcp.Elicitation{"k": {Message: "m", RequestedSchema: json.RawMessage(`{"type":"object"}`)}})
	}}

	res := callModern(t, newInputServer(nil, tool), capsNone, `{}`, "", nil)["result"].(map[string]any)
	assert.Equal(t, true, res["isError"])
	assert.Contains(t, res["content"].([]any)[0].(map[string]any)["text"], mcp.ErrInputNotSupported.Error())
}

func TestInput_InvalidElicitationIsToolError(t *testing.T) {
	t.Parallel()

	for name, els := range map[string]map[string]mcp.Elicitation{
		"none":      {},
		"no schema": {"k": {Message: "m"}},
		"no msg":    {"k": {RequestedSchema: json.RawMessage(`{"type":"object"}`)}},
	} {
		tool := mcp.Tool{Name: toolApprove, Handler: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, mcp.RequireInput(nil, els)
		}}

		res := callModern(t, newInputServer(nil, tool), capsElicit, `{}`, "", nil)["result"].(map[string]any)
		assert.Equal(t, true, res["isError"], name)
	}
}

func TestInput_MalformedInputResponses(t *testing.T) {
	t.Parallel()

	rr := callModernRaw(t, newInputServer(nil, approveTool(nil)), capsElicit, `{}`, `"inputResponses":[1]`, nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	rr = callModernRaw(t, newInputServer(nil, approveTool(nil)), capsElicit, `{}`, `"requestState":5`, nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestInput_Elicited(t *testing.T) {
	t.Parallel()

	var zero mcp.Input

	_, ok := zero.Elicited("x")
	assert.False(t, ok)
	require.Error(t, zero.State(&struct{}{}))
	assert.False(t, mcp.InputFromContext(context.Background()).CanElicit())
	assert.True(t, mcp.ElicitResult{Action: mcp.ElicitAccept}.Accepted())
	assert.False(t, mcp.ElicitResult{Action: mcp.ElicitDecline}.Accepted())
}

func TestWithInputRequired_ShortKeyPanics(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { mcp.NewServer("s", "1").WithInputRequired([]byte("short"), 0) })
}
