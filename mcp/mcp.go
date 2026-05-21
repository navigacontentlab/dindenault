// Package mcp implements a stateless MCP (Model Context Protocol) Streamable
// HTTP server for use with AWS Lambda and AWS Bedrock AgentCore.
//
// The server handles JSON-RPC 2.0 requests over plain HTTP POST, supporting
// the initialize, tools/list, and tools/call methods from the MCP spec.
//
// # Basic Usage
//
// Define tools and register them with the dindenault App via WithMCP:
//
//	app := dindenault.New(logger,
//	    dindenault.WithMCP("/mcp",
//	        mcp.Tool{
//	            Name:        "search_articles",
//	            Description: "Search articles in the content archive",
//	            InputSchema: json.RawMessage(`{
//	                "type": "object",
//	                "properties": {
//	                    "query": {"type": "string", "description": "Search query"}
//	                },
//	                "required": ["query"]
//	            }`),
//	            Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
//	                // Use mcp.AuthorizationFromContext(ctx) to access the JWT.
//	                var args struct{ Query string `json:"query"` }
//	                _ = json.Unmarshal(input, &args)
//	                return json.Marshal(map[string]any{"results": []string{}})
//	            },
//	        },
//	    ),
//	)
//
// # Authentication
//
// The Authorization header from the incoming HTTP request is forwarded to
// tool handlers via the context. Use AuthorizationFromContext to retrieve it:
//
//	func myHandler(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
//	    token := mcp.AuthorizationFromContext(ctx) // e.g. "Bearer eyJ..."
//	    // ...
//	}
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// contextKey is an unexported type for context keys in this package.
type contextKey int

const (
	authorizationKey contextKey = iota
)

// mcpProtocolVersion is the MCP spec version this server targets.
const mcpProtocolVersion = "2025-03-26"

// JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeInvalidParams  = -32602
	codeMethodNotFound = -32601
)

// defaultInputSchema is used when a Tool does not specify an InputSchema.
var defaultInputSchema = json.RawMessage(`{"type":"object","properties":{}}`)

// AuthorizationFromContext retrieves the raw Authorization header value
// (e.g. "Bearer eyJ...") from the context.  Returns an empty string if
// not present.
func AuthorizationFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(authorizationKey).(string); ok {
		return v
	}

	return ""
}

// ToolHandler is the function signature for MCP tool handlers.
//
// ctx may carry an Authorization header — use AuthorizationFromContext.
// input is the raw JSON arguments object sent by the caller.
// The returned value should be a valid JSON value (object, array, string, …).
type ToolHandler func(ctx context.Context, input json.RawMessage) (json.RawMessage, error)

// Tool defines a single callable tool exposed by the MCP server.
type Tool struct {
	// Name is the tool identifier seen by the AI model (e.g. "search_articles").
	Name string

	// Description explains what the tool does. The model uses this to decide
	// when to call the tool, so make it clear and specific.
	Description string

	// InputSchema is a JSON Schema object that describes the tool's arguments.
	// If nil, defaults to {"type":"object","properties":{}}.
	InputSchema json.RawMessage

	// Handler is invoked when the tool is called.
	Handler ToolHandler
}

// Server implements a stateless MCP Streamable HTTP server.
// It satisfies http.Handler and can be mounted on any path.
type Server struct {
	name    string
	version string
	tools   []Tool
	toolMap map[string]*Tool
}

// NewServer creates an MCP server with the given identity and tools.
// name and version appear in the initialize response (informational for clients).
func NewServer(name, version string, tools ...Tool) *Server {
	s := &Server{
		name:    name,
		version: version,
		tools:   tools,
		toolMap: make(map[string]*Tool, len(tools)),
	}

	for i := range tools {
		s.toolMap[tools[i].Name] = &s.tools[i]
	}

	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed — MCP requires POST", http.StatusMethodNotAllowed)

		return
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nil, codeParseError, "Parse error: "+err.Error())

		return
	}

	// Propagate Authorization header into context for tool handlers.
	ctx := r.Context()
	if auth := r.Header.Get("Authorization"); auth != "" {
		ctx = context.WithValue(ctx, authorizationKey, auth)
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(w, &req)
	case "notifications/initialized":
		// Notification — no response per spec.
		w.WriteHeader(http.StatusAccepted)
	case "tools/list":
		s.handleToolsList(w, &req)
	case "tools/call":
		s.handleToolsCall(ctx, w, &req)
	default:
		writeError(w, req.ID, codeMethodNotFound, "Method not found: "+req.Method)
	}
}

func (s *Server) handleInitialize(w http.ResponseWriter, req *jsonRPCRequest) {
	writeResult(w, req.ID, initializeResult{
		ProtocolVersion: mcpProtocolVersion,
		Capabilities: serverCapabilities{
			Tools: &toolsCapability{},
		},
		ServerInfo: serverInfo{
			Name:    s.name,
			Version: s.version,
		},
	})
}

func (s *Server) handleToolsList(w http.ResponseWriter, req *jsonRPCRequest) {
	defs := make([]toolDefinition, 0, len(s.tools))

	for _, t := range s.tools {
		schema := t.InputSchema
		if schema == nil {
			schema = defaultInputSchema
		}

		defs = append(defs, toolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}

	writeResult(w, req.ID, toolsListResult{Tools: defs})
}

func (s *Server) handleToolsCall(ctx context.Context, w http.ResponseWriter, req *jsonRPCRequest) {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, codeInvalidParams, "Invalid params: "+err.Error())

		return
	}

	tool, ok := s.toolMap[params.Name]
	if !ok {
		writeError(w, req.ID, codeInvalidParams, fmt.Sprintf("Unknown tool: %q", params.Name))

		return
	}

	args := params.Arguments
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}

	output, err := tool.Handler(ctx, args)
	if err != nil {
		// Per MCP spec, tool execution errors are returned as a successful
		// JSON-RPC response with isError=true in the result.
		writeResult(w, req.ID, toolsCallResult{
			Content: []contentItem{{Type: "text", Text: "Error: " + err.Error()}},
			IsError: true,
		})

		return
	}

	writeResult(w, req.ID, toolsCallResult{
		Content: []contentItem{{Type: "text", Text: string(output)}},
	})
}

// ── JSON-RPC wire types ────────────────────────────────────────────────────────

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapabilities `json:"capabilities"`
	ServerInfo      serverInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	Tools *toolsCapability `json:"tools,omitempty"`
}

type toolsCapability struct{}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []toolDefinition `json:"tools"`
}

type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolsCallResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ── Response helpers ───────────────────────────────────────────────────────────

func writeResult(w http.ResponseWriter, id json.RawMessage, result any) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	// JSON-RPC errors are always returned with HTTP 200 per spec.
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: message},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
