package dindenault

import (
	"log/slog"

	"github.com/navigacontentlab/dindenault/mcp"
	"github.com/navigacontentlab/dindenault/navigaid"
)

// WithMCP registers an MCP (Model Context Protocol) Streamable HTTP endpoint
// at the given path.
//
// AI agents such as AWS Bedrock AgentCore can discover and invoke the provided
// tools via stateless JSON-RPC 2.0 POST requests — no SSE or persistent
// connections required, making it a natural fit for Lambda deployments.
//
// The path is configurable so MCP can coexist with Connect RPC services:
//
//	app := dindenault.New(logger,
//	    dindenault.WithService(connectPath, connectHandler),
//	    dindenault.WithMCP("/mcp",
//	        mcp.Tool{
//	            Name:        "search_articles",
//	            Description: "Search articles in OpenContent by free-text query",
//	            InputSchema: json.RawMessage(`{
//	                "type": "object",
//	                "properties": {
//	                    "query": {"type": "string", "description": "Free-text search query"},
//	                    "limit": {"type": "integer", "default": 10}
//	                },
//	                "required": ["query"]
//	            }`),
//	            Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
//	                token := mcp.AuthorizationFromContext(ctx) // forward JWT to downstream
//	                // ...
//	            },
//	        },
//	    ),
//	)
//
// For full control over the server name and version shown to clients, use
// WithService together with mcp.NewServer directly:
//
//	server := mcp.NewServer("my-service", "2.1.0", tools...)
//	app := dindenault.New(logger,
//	    dindenault.WithService("/mcp", server),
//	)
func WithMCP(path string, tools ...mcp.Tool) Option {
	return func(a *App) {
		server := mcp.NewServer("dindenault", "1.0.0", tools...)

		a.registrations = append(a.registrations, Registration{
			Path:    path,
			Handler: server,
		})
	}
}

// WithMCPAuth is like WithMCP but wraps the server with JWT validation using
// Naviga ID. Requests with no token or an invalid token are rejected with 401
// before any tool logic runs.
//
// On success the validated claims are placed in the context, so tool handlers
// can call navigaid.GetAuth(ctx) for org/subject/permissions, and
// mcp.AuthorizationFromContext(ctx) for the raw Bearer token to forward to
// downstream services such as OC or CCA.
//
//	app := dindenault.New(logger,
//	    dindenault.WithMCPAuth("/mcp", logger, os.Getenv("IMAS_URL"),
//	        mcp.Tool{
//	            Name:    "search_articles",
//	            Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
//	                auth, _ := navigaid.GetAuth(ctx)          // org, subject, permissions
//	                token := mcp.AuthorizationFromContext(ctx) // forward to OC/CCA
//	                // ...
//	            },
//	        },
//	    ),
//	)
func WithMCPAuth(path string, logger *slog.Logger, imasURL string, tools ...mcp.Tool) Option {
	return func(a *App) {
		server := mcp.NewServer("dindenault", "1.0.0", tools...)
		jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint(imasURL))
		handler := mcp.AuthMiddleware(logger, jwks, server)

		a.registrations = append(a.registrations, Registration{
			Path:    path,
			Handler: handler,
		})
	}
}
