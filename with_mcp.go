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
		a.mcpServers = append(a.mcpServers, server)

		a.registrations = append(a.registrations, Registration{
			Path:    path,
			Handler: server,
			// MCP handlers are plain HTTP handlers; Connect interceptors
			// cannot be applied to them.
			SkipGlobalInterceptors: true,
		})
	}
}

// WithMCPAuth is like WithMCP but wraps the server with JWT validation using
// Naviga ID. Requests with no token or an invalid token are rejected with 401
// before any tool logic runs.
//
// Pass mcp.AuthOption values (e.g. mcp.WithPublicTools) as authOpts to exempt
// specific tools from authentication. Pass nil when no exceptions are needed.
//
// On success the validated claims are placed in the context, so tool handlers
// can call navigaid.GetAuth(ctx) for org/subject/permissions, and
// mcp.AuthorizationFromContext(ctx) for the raw Bearer token to forward to
// downstream services such as OC or CCA.
//
//	// All tools require auth:
//	dindenault.WithMCPAuth("/mcp", logger, os.Getenv("IMAS_URL"), nil, tool1, tool2)
//
//	// get_search_fields is public; all other tools require auth:
//	dindenault.WithMCPAuth("/mcp", logger, os.Getenv("IMAS_URL"),
//	    []mcp.AuthOption{mcp.WithPublicTools("get_search_fields")},
//	    tool1, tool2,
//	)
func WithMCPAuth(path string, logger *slog.Logger, imasURL string, authOpts []mcp.AuthOption, tools ...mcp.Tool) Option {
	if imasURL == "" {
		panic("imasURL cannot be empty for WithMCPAuth")
	}

	return func(a *App) {
		server := mcp.NewServer("dindenault", "1.0.0", tools...)
		a.mcpServers = append(a.mcpServers, server)
		jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint(imasURL))
		handler := mcp.AuthMiddleware(logger, jwks, server, authOpts...)

		a.registrations = append(a.registrations, Registration{
			Path:    path,
			Handler: handler,
			// MCP handlers are plain HTTP handlers; Connect interceptors
			// cannot be applied to them.
			SkipGlobalInterceptors: true,
		})
	}
}

// WithMCPServerInfo sets how the MCP servers mounted by WithMCP and WithMCPAuth
// identify themselves in the initialize response — the name, version and
// description that registries and MCP clients show. Without it they report
// the name "dindenault" and version "1.0.0".
//
// Only non-empty fields are applied, and the option may appear before or after
// WithMCP/WithMCPAuth. A custom Name also replaces the Lambda function name
// that audit events otherwise use as their "service" field.
//
//	app := dindenault.New(logger,
//	    dindenault.WithMCPServerInfo(mcp.ServerInfo{
//	        Name:        "my-service",
//	        Version:     "2.1.0",
//	        Description: "Search and edit my content",
//	    }),
//	    dindenault.WithMCPAuth("/mcp", logger, imasURL, nil, tools...),
//	)
func WithMCPServerInfo(info mcp.ServerInfo) Option {
	return func(a *App) {
		a.mcpInfo = info
	}
}
