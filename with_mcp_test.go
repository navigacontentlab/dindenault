package dindenault_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/mcp"
)

func noopTool() mcp.Tool {
	return mcp.Tool{
		Name: "noop",
		Handler: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	}
}

func initializeInfo(t *testing.T, app *dindenault.App) map[string]any {
	t.Helper()

	regs := app.Registrations()
	require.Len(t, regs, 1)

	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	rr := httptest.NewRecorder()
	regs[0].Handler.ServeHTTP(rr, req)

	var resp struct {
		Result struct {
			ServerInfo map[string]any `json:"serverInfo"`
		} `json:"result"`
	}

	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	return resp.Result.ServerInfo
}

func TestWithMCP_DefaultServerInfo(t *testing.T) {
	app := dindenault.New(slog.Default(), dindenault.WithMCP("/mcp", noopTool()))

	assert.Equal(t, map[string]any{"name": "dindenault", "version": "1.0.0"}, initializeInfo(t, app))
}

func TestWithMCPServerInfo_AppliesWhateverTheOptionOrder(t *testing.T) {
	info := mcp.ServerInfo{Name: "my-service", Version: "2.1.0", Description: "Search and edit my content"}
	want := map[string]any{"name": "my-service", "version": "2.1.0", "description": "Search and edit my content"}

	before := dindenault.New(slog.Default(),
		dindenault.WithMCPServerInfo(info),
		dindenault.WithMCP("/mcp", noopTool()),
	)
	after := dindenault.New(slog.Default(),
		dindenault.WithMCP("/mcp", noopTool()),
		dindenault.WithMCPServerInfo(info),
	)

	assert.Equal(t, want, initializeInfo(t, before))
	assert.Equal(t, want, initializeInfo(t, after))
}

func TestWithMCPServerInfo_PartialInfoKeepsDefaults(t *testing.T) {
	app := dindenault.New(slog.Default(),
		dindenault.WithMCP("/mcp", noopTool()),
		dindenault.WithMCPServerInfo(mcp.ServerInfo{Description: "only this"}),
	)

	assert.Equal(t, map[string]any{"name": "dindenault", "version": "1.0.0", "description": "only this"}, initializeInfo(t, app))
}

func TestWithMCPServerInfo_AppliesToWithMCPAuth(t *testing.T) {
	var events []mcp.AuditEvent

	previous := mcp.DefaultAuditSink
	mcp.DefaultAuditSink = func(_ context.Context, e mcp.AuditEvent) { events = append(events, e) }

	t.Cleanup(func() { mcp.DefaultAuditSink = previous })

	app := dindenault.New(slog.Default(),
		dindenault.WithMCPAuth("/mcp", slog.Default(), "https://imas.example.com", nil, noopTool()),
		dindenault.WithMCPServerInfo(mcp.ServerInfo{Name: "my-service", Version: "2.1.0"}),
	)

	require.Len(t, app.Registrations(), 1)

	// A tools/call without a token is rejected by the auth middleware, which
	// audits it with the name of the server it wraps. That name only reaches
	// the audit event if WithMCPServerInfo got through to the inner server.
	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"noop"}}`))
	rr := httptest.NewRecorder()
	app.Registrations()[0].Handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	require.Len(t, events, 1)
	assert.Equal(t, "my-service", events[0].Service)
}
