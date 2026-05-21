package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault/mcp"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func post(t *testing.T, server *mcp.Server, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	return rr
}

func postWithAuth(t *testing.T, server *mcp.Server, body, authHeader string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	return rr
}

func decodeResponse(t *testing.T, body io.Reader) map[string]any {
	t.Helper()

	var result map[string]any
	require.NoError(t, json.NewDecoder(body).Decode(&result))

	return result
}

// ── fixtures ──────────────────────────────────────────────────────────────────

func echoTool() mcp.Tool {
	return mcp.Tool{
		Name:        "echo",
		Description: "Echoes the input message back",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"message": {"type": "string"}
			},
			"required": ["message"]
		}`),
		Handler: func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
			return input, nil
		},
	}
}

func failTool() mcp.Tool {
	return mcp.Tool{
		Name:        "fail",
		Description: "Always returns an error",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("something went wrong")
		},
	}
}

func captureAuthTool(captured *string) mcp.Tool {
	return mcp.Tool{
		Name:        "whoami",
		Description: "Returns the Authorization header value",
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			*captured = mcp.AuthorizationFromContext(ctx)

			return json.Marshal(map[string]string{"auth": *captured})
		},
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestInitialize(t *testing.T) {
	server := mcp.NewServer("test-server", "0.1.0", echoTool())

	rr := post(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	resp := decodeResponse(t, rr.Body)
	assert.Equal(t, "2.0", resp["jsonrpc"])
	assert.Nil(t, resp["error"])

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "expected result object")
	assert.Equal(t, "2025-03-26", result["protocolVersion"])

	serverInfo, ok := result["serverInfo"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test-server", serverInfo["name"])
	assert.Equal(t, "0.1.0", serverInfo["version"])

	caps, ok := result["capabilities"].(map[string]any)
	require.True(t, ok)
	assert.NotNil(t, caps["tools"])
}

func TestToolsList(t *testing.T) {
	server := mcp.NewServer("s", "1", echoTool(), failTool())

	rr := post(t, server, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)

	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeResponse(t, rr.Body)
	result, ok := resp["result"].(map[string]any)
	require.True(t, ok)

	tools, ok := result["tools"].([]any)
	require.True(t, ok)
	assert.Len(t, tools, 2)

	// echoTool should appear with its schema
	first := tools[0].(map[string]any)
	assert.Equal(t, "echo", first["name"])
	assert.NotEmpty(t, first["description"])
	assert.NotNil(t, first["inputSchema"])
}

func TestToolsList_DefaultSchema(t *testing.T) {
	// failTool has no InputSchema — server must supply a default
	server := mcp.NewServer("s", "1", failTool())

	rr := post(t, server, `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`)

	resp := decodeResponse(t, rr.Body)
	result := resp["result"].(map[string]any)
	tools := result["tools"].([]any)
	require.Len(t, tools, 1)

	schema := tools[0].(map[string]any)["inputSchema"].(map[string]any)
	assert.Equal(t, "object", schema["type"])
}

func TestToolsCall_Success(t *testing.T) {
	server := mcp.NewServer("s", "1", echoTool())

	rr := post(t, server, `{
		"jsonrpc": "2.0",
		"id": "call-1",
		"method": "tools/call",
		"params": {
			"name": "echo",
			"arguments": {"message": "hello"}
		}
	}`)

	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeResponse(t, rr.Body)
	assert.Nil(t, resp["error"])

	result := resp["result"].(map[string]any)
	assert.NotEqual(t, true, result["isError"])

	content := result["content"].([]any)
	require.Len(t, content, 1)
	item := content[0].(map[string]any)
	assert.Equal(t, "text", item["type"])
	assert.Contains(t, item["text"].(string), "hello")
}

func TestToolsCall_ToolError(t *testing.T) {
	server := mcp.NewServer("s", "1", failTool())

	rr := post(t, server, `{
		"jsonrpc": "2.0",
		"id": "call-2",
		"method": "tools/call",
		"params": {"name": "fail", "arguments": {}}
	}`)

	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeResponse(t, rr.Body)
	// Tool errors are a successful JSON-RPC response with isError=true
	assert.Nil(t, resp["error"])

	result := resp["result"].(map[string]any)
	assert.Equal(t, true, result["isError"])
	content := result["content"].([]any)
	require.NotEmpty(t, content)
	assert.Contains(t, content[0].(map[string]any)["text"].(string), "something went wrong")
}

func TestToolsCall_UnknownTool(t *testing.T) {
	server := mcp.NewServer("s", "1", echoTool())

	rr := post(t, server, `{
		"jsonrpc": "2.0",
		"id": "call-3",
		"method": "tools/call",
		"params": {"name": "nonexistent", "arguments": {}}
	}`)

	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeResponse(t, rr.Body)
	rpcErr, ok := resp["error"].(map[string]any)
	require.True(t, ok, "expected JSON-RPC error")
	assert.Equal(t, float64(-32602), rpcErr["code"])
	assert.Contains(t, rpcErr["message"].(string), "nonexistent")
}

func TestToolsCall_AuthorizationForwarded(t *testing.T) {
	var captured string
	server := mcp.NewServer("s", "1", captureAuthTool(&captured))

	postWithAuth(t, server, `{
		"jsonrpc": "2.0",
		"id": "call-4",
		"method": "tools/call",
		"params": {"name": "whoami", "arguments": {}}
	}`, "Bearer test-jwt-token")

	assert.Equal(t, "Bearer test-jwt-token", captured)
}

func TestMethodNotFound(t *testing.T) {
	server := mcp.NewServer("s", "1")

	rr := post(t, server, `{"jsonrpc":"2.0","id":5,"method":"unknown/method","params":{}}`)

	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeResponse(t, rr.Body)
	rpcErr, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(-32601), rpcErr["code"])
}

func TestNotificationsInitialized(t *testing.T) {
	server := mcp.NewServer("s", "1")

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusAccepted, rr.Code)
}

func TestNonPostRejected(t *testing.T) {
	server := mcp.NewServer("s", "1")

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/mcp", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code, "method: %s", method)
	}
}

func TestParseError(t *testing.T) {
	server := mcp.NewServer("s", "1")

	rr := post(t, server, `not valid json {{{`)

	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeResponse(t, rr.Body)
	rpcErr, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(-32700), rpcErr["code"])
}

func TestAuthorizationFromContext_Missing(t *testing.T) {
	result := mcp.AuthorizationFromContext(context.Background())
	assert.Empty(t, result)
}
