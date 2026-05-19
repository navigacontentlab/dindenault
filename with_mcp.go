package dindenault

import "github.com/navigacontentlab/dindenault/mcp"

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
