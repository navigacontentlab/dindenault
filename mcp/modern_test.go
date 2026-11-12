package mcp_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault/mcp"
)

const methodList = "tools/list"

const modernMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

// postModern sends a 2026-07-28 request with the headers the revision requires.
func postModern(t *testing.T, server *mcp.Server, method, name, body string, edit func(http.Header)) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", method)

	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}

	if edit != nil {
		edit(req.Header)
	}

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	return rr
}

func newModernServer() *mcp.Server {
	return mcp.NewServer("test-server", "1.2.3", echoTool()).WithInfo(mcp.ServerInfo{Title: "Test"})
}

func TestModern_Discover(t *testing.T) {
	t.Parallel()

	rr := postModern(t, newModernServer(), "server/discover", "",
		`{"jsonrpc":"2.0","id":"d1","method":"server/discover","params":{`+modernMeta+`}}`, nil)
	require.Equal(t, http.StatusOK, rr.Code)

	res := decodeResponse(t, rr.Body)["result"].(map[string]any)
	assert.Equal(t, "complete", res["resultType"])
	assert.Equal(t, []any{"2026-07-28"}, res["supportedVersions"])
	assert.Contains(t, res["capabilities"], "tools")
	assert.Equal(t, "public", res["cacheScope"])
	assert.Positive(t, res["ttlMs"])

	info := res["_meta"].(map[string]any)["io.modelcontextprotocol/serverInfo"].(map[string]any)
	assert.Equal(t, "test-server", info["name"])
	assert.Equal(t, "Test", info["title"])
}

func TestModern_ToolsListAndCall(t *testing.T) {
	t.Parallel()

	server := newModernServer()

	rr := postModern(t, server, methodList, "",
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{`+modernMeta+`}}`, nil)
	require.Equal(t, http.StatusOK, rr.Code)

	list := decodeResponse(t, rr.Body)["result"].(map[string]any)
	assert.Equal(t, "complete", list["resultType"])
	assert.Equal(t, "public", list["cacheScope"])
	assert.Len(t, list["tools"], 1)

	rr = postModern(t, server, "tools/call", "echo",
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hi"},`+modernMeta+`}}`, nil)
	require.Equal(t, http.StatusOK, rr.Code)

	call := decodeResponse(t, rr.Body)["result"].(map[string]any)
	assert.Equal(t, "complete", call["resultType"])
	assert.Contains(t, call["content"].([]any)[0].(map[string]any)["text"], "hi")
}

func TestModern_Base64ToolName(t *testing.T) {
	t.Parallel()

	name := "=?base64?" + base64.StdEncoding.EncodeToString([]byte("echo")) + "?="
	rr := postModern(t, newModernServer(), "tools/call", name,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"message":"x"},`+modernMeta+`}}`, nil)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.NotContains(t, decodeResponse(t, rr.Body), "error")
}

func TestModern_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, method, toolName, body string
		edit                         func(http.Header)
		status, code                 int
	}{
		{
			name: "unsupported version", method: methodList,
			body:   `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"1900-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}`,
			edit:   func(h http.Header) { h.Set("MCP-Protocol-Version", "1900-01-01") },
			status: http.StatusBadRequest, code: -32022,
		},
		{
			name: "missing client capabilities", method: methodList,
			body:   `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
			status: http.StatusBadRequest, code: -32602,
		},
		{
			name: "version header mismatch", method: methodList,
			body:   `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{` + modernMeta + `}}`,
			edit:   func(h http.Header) { h.Set("MCP-Protocol-Version", "2025-11-25") },
			status: http.StatusBadRequest, code: -32020,
		},
		{
			name: "missing method header", method: methodList,
			body:   `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{` + modernMeta + `}}`,
			edit:   func(h http.Header) { h.Del("Mcp-Method") },
			status: http.StatusBadRequest, code: -32020,
		},
		{
			name: "tool name header mismatch", method: "tools/call", toolName: "other",
			body:   `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo",` + modernMeta + `}}`,
			status: http.StatusBadRequest, code: -32020,
		},
		{
			name: "ping is gone in 2026-07-28", method: "ping",
			body:   `{"jsonrpc":"2.0","id":1,"method":"ping","params":{` + modernMeta + `}}`,
			status: http.StatusNotFound, code: -32601,
		},
		{
			name: "modern header without _meta", method: methodList,
			body:   `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
			status: http.StatusBadRequest, code: -32602,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			rr := postModern(t, newModernServer(), c.method, c.toolName, c.body, c.edit)
			assert.Equal(t, c.status, rr.Code)

			resp := decodeResponse(t, rr.Body)
			require.Contains(t, resp, "error")
			assert.InDelta(t, c.code, resp["error"].(map[string]any)["code"], 0)
		})
	}
}

func TestModern_UnsupportedVersionListsSupported(t *testing.T) {
	t.Parallel()

	rr := postModern(t, newModernServer(), methodList, "",
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2099-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}`,
		func(h http.Header) { h.Set("MCP-Protocol-Version", "2099-01-01") })

	data := decodeResponse(t, rr.Body)["error"].(map[string]any)["data"].(map[string]any)
	assert.Equal(t, "2099-01-01", data["requested"])
	assert.Contains(t, data["supported"], "2026-07-28")
	assert.Contains(t, data["supported"], "2025-06-18")
}

func TestLegacy_Ping(t *testing.T) {
	t.Parallel()

	rr := post(t, newModernServer(), `{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	require.Equal(t, http.StatusOK, rr.Code)

	resp := decodeResponse(t, rr.Body)
	assert.Equal(t, map[string]any{}, resp["result"])
	assert.InDelta(t, 7, resp["id"], 0)
}

func TestLegacy_NotificationsAccepted(t *testing.T) {
	t.Parallel()

	rr := post(t, newModernServer(), `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`)
	assert.Equal(t, http.StatusAccepted, rr.Code)
	assert.Empty(t, rr.Body.String())
}

func TestLegacy_InitializeStillNegotiates(t *testing.T) {
	t.Parallel()

	// A client asking initialize for the stateless revision gets the newest
	// initialize-based one: initialize always selects legacy semantics.
	rr := post(t, newModernServer(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28"}}`)

	res := decodeResponse(t, rr.Body)["result"].(map[string]any)
	assert.Equal(t, "2025-11-25", res["protocolVersion"])
}

func TestLegacy_ToolsListHasNoModernFields(t *testing.T) {
	t.Parallel()

	rr := post(t, newModernServer(), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	res := decodeResponse(t, rr.Body)["result"].(map[string]any)

	assert.NotContains(t, res, "resultType")
	assert.NotContains(t, res, "ttlMs")
}
