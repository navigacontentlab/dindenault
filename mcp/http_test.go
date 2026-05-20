package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/navigacontentlab/dindenault/mcp"
)

// callThroughMCP dispatches a tools/call to a server built from tool, setting
// authHeader on the incoming MCP request. It returns the recorder so callers
// can inspect the JSON-RPC response.
func callThroughMCP(t *testing.T, tool mcp.Tool, authHeader string) *httptest.ResponseRecorder {
	t.Helper()

	server := mcp.NewServer("test", "0", tool)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool.Name + `","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	return rr
}

// downstreamServer returns a test HTTP server and a pointer to the last
// Authorization header it received.
func downstreamServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()

	var got string

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	t.Cleanup(srv.Close)

	return srv, &got
}

// fetchTool creates a tool that calls targetURL using mcp.NewHTTPClient.
func fetchTool(targetURL string, base http.RoundTripper) mcp.Tool {
	return mcp.Tool{
		Name: "fetch",
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			client := mcp.NewHTTPClient(ctx, base)

			resp, err := client.Get(targetURL)
			if err != nil {
				return nil, err
			}

			resp.Body.Close()

			return json.Marshal("ok")
		},
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestNewHTTPClient_ForwardsToken(t *testing.T) {
	const token = "Bearer eyJhbGciOiJSUzI1NiJ9.payload.sig" //nolint:gosec

	ds, got := downstreamServer(t)

	callThroughMCP(t, fetchTool(ds.URL, nil), token)

	assert.Equal(t, token, *got, "Authorization header must be forwarded to downstream service")
}

func TestNewHTTPClient_NoToken_NoHeader(t *testing.T) {
	ds, got := downstreamServer(t)

	callThroughMCP(t, fetchTool(ds.URL, nil), "")

	assert.Empty(t, *got, "no outbound Authorization header when MCP request carried none")
}

func TestNewHTTPClient_UsesProvidedBaseTransport(t *testing.T) {
	var baseCalled bool

	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		baseCalled = true
		return http.DefaultTransport.RoundTrip(r)
	})

	ds, _ := downstreamServer(t)

	callThroughMCP(t, fetchTool(ds.URL, base), "Bearer tok") //nolint:gosec

	assert.True(t, baseCalled, "provided base RoundTripper must be used, not http.DefaultTransport")
}

func TestNewHTTPClient_DoesNotMutateIncomingRequest(t *testing.T) {
	// Verifies that RoundTrip clones the request before modifying headers,
	// leaving the original request object intact.
	const token = "Bearer original" //nolint:gosec

	var mutated bool

	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// The outbound request should have the forwarded token.
		// But the original request object the tool created should be untouched.
		return http.DefaultTransport.RoundTrip(r)
	})

	tool := mcp.Tool{
		Name: "check-mutation",
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			ds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
			defer ds.Close()

			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ds.URL, nil)
			before := req.Header.Get("Authorization")

			client := mcp.NewHTTPClient(ctx, base)
			resp, err := client.Do(req)
			if err != nil {
				return nil, err
			}
			resp.Body.Close()

			after := req.Header.Get("Authorization")
			mutated = before != after

			return json.Marshal("ok")
		},
	}

	callThroughMCP(t, tool, token)

	assert.False(t, mutated, "RoundTrip must not mutate the original request's headers")
}

// roundTripFunc is a functional RoundTripper for tests.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
