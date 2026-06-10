package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navigacontentlab/dindenault/mcp"
	"github.com/navigacontentlab/dindenault/navigaid"
)

func permTestServer() *mcp.Server {
	return mcp.NewServer("test", "0.0.1",
		mcp.Tool{
			Name:                "secure_tool",
			Description:         "requires content:write",
			RequiredPermissions: []string{"content:write"},
			Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`"secure ok"`), nil
			},
		},
		mcp.Tool{
			Name:        "open_tool",
			Description: "no permission requirement",
			Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`"open ok"`), nil
			},
		},
	)
}

func callTool(ctx context.Context, t *testing.T, server *mcp.Server, tool string) map[string]any {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `"}}`

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	if ctx != nil {
		req = req.WithContext(ctx)
	}

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	return resp
}

func authContext(orgPerms []string) context.Context {
	return navigaid.SetAuth(context.Background(), navigaid.AuthInfo{
		AccessToken: "test-token",
		Claims: navigaid.Claims{
			Org: "test-org",
			Permissions: navigaid.PermissionsClaim{
				Org: orgPerms,
			},
		},
	}, nil)
}

func TestRequiredPermissions(t *testing.T) {
	server := permTestServer()

	t.Run("allowed with permission", func(t *testing.T) {
		resp := callTool(authContext([]string{"content:write"}), t, server, "secure_tool")
		if resp["error"] != nil {
			t.Fatalf("expected success, got error: %v", resp["error"])
		}
	})

	t.Run("denied without permission", func(t *testing.T) {
		resp := callTool(authContext([]string{"content:read"}), t, server, "secure_tool")
		if resp["error"] == nil {
			t.Fatal("expected permission denied error")
		}
	})

	t.Run("denied without auth in context", func(t *testing.T) {
		resp := callTool(context.Background(), t, server, "secure_tool")
		if resp["error"] == nil {
			t.Fatal("a tool with RequiredPermissions must reject unauthenticated calls")
		}
	})

	t.Run("tool without requirements is callable without auth", func(t *testing.T) {
		resp := callTool(context.Background(), t, server, "open_tool")
		if resp["error"] != nil {
			t.Fatalf("expected success, got error: %v", resp["error"])
		}
	})
}

func TestAuthorizationFromContextFallback(t *testing.T) {
	t.Run("falls back to navigaid auth info", func(t *testing.T) {
		ctx := authContext(nil)

		if got := mcp.AuthorizationFromContext(ctx); got != "Bearer test-token" {
			t.Errorf("expected reconstructed bearer token, got %q", got)
		}
	})

	t.Run("empty without any auth", func(t *testing.T) {
		if got := mcp.AuthorizationFromContext(context.Background()); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}
