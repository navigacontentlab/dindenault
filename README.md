# Dindenault

Dindenault provides a framework for building Connect RPC services in AWS Lambda. It offers a clean, maintainable API with a Connect-native architecture.

## Table of Contents

- [Features](#features)
- [Core Concepts](#core-concepts)
- [Security Model](#security-model)
- [Getting Started](#getting-started)
- [Working with Connect Handlers](#working-with-connect-handlers)
- [Interceptor Architecture](#interceptor-architecture)
- [Method-Level Permissions with PathInterceptors](#method-level-permissions-with-pathinterceptors)
- [Authentication with Naviga ID](#authentication-with-naviga-id)
- [Response Compression](#response-compression)
- [CORS Support](#cors-support)
- [MCP (Model Context Protocol) Support](#mcp-model-context-protocol-support)
- [Telemetry and Observability](#telemetry-and-observability)
- [Architecture Details](#architecture-details)
- [Advanced Features](#advanced-features)
- [Releasing](#releasing)
- [Contributing](#contributing)

Upgrading from an earlier version? See [MIGRATION.md](./MIGRATION.md).

## Features

- **Connect-native architecture**: Built from the ground up for Connect RPC services
- **Unified interceptor system**: Single interface for logging, tracing, auth, and more
- **Authentication and authorization**: Built-in support for Naviga ID
- **Comprehensive observability**: Integrated logging, tracing, and metrics
- **Response compression**: Native support through Connect's compression capabilities
- **MCP support**: Expose tools to AI agents (e.g. AWS Bedrock AgentCore) via the Model Context Protocol
- **Simplified deployment**: Ready for AWS Lambda environments

## Core Concepts

Dindenault provides a clean, simple API for building Connect RPC services:

### Single Service Registration Method

Use `WithService` to register all your services. This is the only service registration method you need:

```go
app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

### Optional CORS Configuration

Use `WithConnectRPC` when your service needs to be accessed from web browsers. For internal services, simply omit it:

```go
// Web-facing service with CORS
app := dindenault.New(logger,
    dindenault.WithConnectRPC(cors.Options{
        AllowedDomains: []string{".mycompany.com"},
    }),
    dindenault.WithService(path, handler),
)

// Internal service without CORS
app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

### Method-Level Permissions with PathInterceptors

Apply permissions at the RPC method level using `PathInterceptors` when creating your Connect handler:

```go
permissionConfigs := []dindenault.PathPermissionConfig{
    {PathPrefix: "/service.v1.Service/Method", Permissions: []string{"service:access"}},
}

path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithInterceptors(
        dindenault.AuthInterceptors(logger, imasURL),
        dindenault.PathInterceptors(logger, permissionConfigs),
    ),
)

app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

## Security Model

A short summary of what dindenault does — and does not — handle for you.

### What token validation checks

`AuthInterceptors`, `navigaid.HTTPMiddleware`, and `mcp.AuthMiddleware`
all validate JWTs against the IMAS JWKS endpoint. A token is accepted
only if all of the following hold:

- Signed with RSA (`RS256`/`RS384`/`RS512`) by a key published in the JWKS, matched via the `kid` header
- The token's `alg` matches the algorithm declared for that key
- Not expired — the `exp` claim is **required**; `nbf`/`iat` are honored when present
- The Naviga token type (`ntt` claim) matches (`access_token`)
- Issuer and audience match, **if** configured via `navigaid.WithExpectedIssuer` / `navigaid.WithExpectedAudience` (opt-in — enable these where possible)

JWKS keys are cached (10 min TTL). If a refresh fails, previously
fetched keys are used and the fetch retried after 30 s, so a brief IMAS
outage does not fail all authentication. All outbound auth HTTP calls
have a 10 s timeout.

### Fail-fast philosophy

Misconfiguration fails at startup, not silently at runtime: an empty
IMAS URL panics, and app-level interceptors that cannot be applied to a
handler panic (see [Interceptor Architecture](#interceptor-architecture)).
A service that starts is a service whose auth is wired up.

### What you are responsible for

- **Authorization** — authentication only proves identity. Enforce
  permissions with `PathInterceptors`, `navigaid.RequirePermission`,
  `Tool.RequiredPermissions` (MCP), or explicit checks
  (`dindenault.HasPermission`, `HasPermissionInUnit`) in your handlers.
- **Unmatched paths** — `PathInterceptors`/`WithPathPermissionService`
  pass requests through when no `PathPrefix` matches. Add a catch-all
  config if every path must require a permission.
- **MCP discovery is public** — `initialize` and `tools/list` respond
  without authentication (clients need them before they have a token),
  so tool names and descriptions are visible. `tools/call` requires
  auth unless a tool is listed in `WithPublicTools`.
- **Permission scope** — org-level checks (`HasPermission`) are not
  satisfied by unit-scoped grants. Be deliberate about which scope a
  feature requires.

### Logging

Request logging redacts `Authorization`, `x-imid-token`, `Cookie`, and
`Proxy-Authorization`. HTTP 500 responses contain a generic message;
details go to the log only.

## Getting Started

### Basic Implementation

Dindenault provides a single, unified way to register services using `WithService`. This simplifies service registration and makes the API more intuitive.

```go
import (
    "github.com/navigacontentlab/dindenault"
    "connectrpc.com/connect"
)

// Apply interceptors at handler creation — this works with any
// connect-go generated handler:
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        dindenault.AuthInterceptors(logger, "https://imas.example.com"),
    ),
)

app := dindenault.New(logger,
    // Register a service - this is the only service registration method
    dindenault.WithService(path, handler),
)

// Start the Lambda handler
lambda.Start(app.Handle())
```

> **Note:** App-level interceptors (`dindenault.WithInterceptors`) can only be
> applied to handlers that implement `ConnectHandlerWithInterceptor`. Standard
> connect-go generated handlers do **not** implement it — pass interceptors at
> handler creation as shown above. If app-level interceptors cannot be applied
> to a registered handler, the app panics at startup instead of silently
> running without them. Use `WithPlainService` for handlers (health checks,
> webhooks) that intentionally should not receive interceptors.

### Service Registration with Optional CORS

For web-facing services that need CORS support, use `WithConnectRPC` to add CORS configuration:

```go
app := dindenault.New(logger,
    // Optional: Add CORS support for web clients
    dindenault.WithConnectRPC(cors.Options{
        AllowedDomains: []string{".mycompany.com"},
        AllowHTTP:      false, // Require HTTPS
    }),
    
    // Register your service
    dindenault.WithService("api/", myServiceHandler),
)
```

For internal services that don't need CORS, simply omit `WithConnectRPC`.

## Working with Connect Handlers

Dindenault is designed to work seamlessly with Connect's generated handlers. All services are registered using the single `WithService` method.

### Creating Connect Handlers

When creating Connect handlers, you can apply options directly during handler creation:

```go
import (
    "connectrpc.com/connect"
    "yourpackage/servicev1connect"
)

// Create your service implementation
impl := service.NewServiceImpl()

// Create Connect handler with options applied directly
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithCompressMinBytes(1024), // Compression threshold
    connect.WithInterceptors(customInterceptor),
)

// Register the handler using WithService - the only service registration method
app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

### Applying Compression

The recommended way to enable compression for Connect handlers is to apply it directly when creating the handler:

```go
// Apply compression when creating the handler
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithCompressMinBytes(1024), // Compress responses larger than 1KB
)
```

This approach uses Connect's native compression system, which:
- Compresses responses based on the client's `Accept-Encoding` header
- Only compresses responses larger than the specified threshold
- Properly handles content negotiation and all compression headers

### Combining Security and Compression

For handlers requiring both security features and compression:

```go
// Create permission configurations
permissionConfigs := []dindenault.PathPermissionConfig{
    {PathPrefix: "/service.v1.Service/SecureMethod", Permissions: []string{"service:access"}},
}

// Create handler with compression and interceptors
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithCompressMinBytes(1024),
    connect.WithInterceptors(
        dindenault.AuthInterceptors(logger, "https://imas.example.com"),
        dindenault.PathInterceptors(logger, permissionConfigs),
    ),
)

// Register using WithService
app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

### Creating Handlers with Multiple Options

For more complex configurations, you can combine multiple options:

```go
// Create permission configurations
permissionConfigs := []dindenault.PathPermissionConfig{
    {PathPrefix: "/service.v1.Service/SecureMethod", Permissions: []string{"service:access"}},
}

// Create Connect handler with multiple options
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithCompressMinBytes(1024),
    connect.WithInterceptors(
        dindenault.AuthInterceptors(logger, "https://imas.example.com"),
        dindenault.PathInterceptors(logger, permissionConfigs),
        yourCustomInterceptor,
        anotherInterceptor,
    ),
    connect.WithCodec(connect.NewJSONCodec()),
)

// Register using WithService
app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

## Interceptor Architecture

Dindenault's architecture is built around Connect interceptors. All cross-cutting concerns are implemented using interceptors, providing a consistent approach to logging, tracing, authentication, and more.

### Available Interceptors

- **`LoggingInterceptors(logger)`**: Adds request logging with timing information
- **`TelemetryInterceptor(logger, provider, opts)`**: Adds optional telemetry (see [Telemetry](#telemetry-and-observability))
- **`AuthInterceptors(logger, imasURL)`**: Adds Naviga ID authentication
- **`PathInterceptors(logger, configs)`**: Adds method-level permission checks

CORS is not an interceptor — it is HTTP middleware applied by
[`WithConnectRPC`](#cors-support).

For AWS X-Ray tracing, use the [`xray` submodule](./xray/README.md):

```go
import xrayprovider "github.com/navigacontentlab/dindenault/xray"

provider := xrayprovider.New()
```

### Using Interceptors

Pass interceptors at handler creation with `connect.WithInterceptors`:

```go
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        dindenault.TelemetryInterceptor(logger, provider, telemetryOpts),
        dindenault.AuthInterceptors(logger, "https://imas.example.com"),
    ),
)
```

The app-level `dindenault.WithInterceptors` option only works with handlers
that implement `ConnectHandlerWithInterceptor`; the app refuses to start
(panics) if it cannot apply the configured interceptors to a handler.

### Global vs. Handler-Specific Interceptors

You can apply interceptors at two levels:

1. **Handler-specific interceptors** (recommended) applied when creating the handler:
   ```go
   path, handler := servicev1connect.NewServiceHandler(
       impl,
       connect.WithInterceptors(
           dindenault.LoggingInterceptors(logger),
           dindenault.AuthInterceptors(logger, imasURL),
       ),
   )
   ```

2. **App-level interceptors** via `dindenault.WithInterceptors`. These only
   work for handlers that implement `ConnectHandlerWithInterceptor` —
   standard connect-go generated handlers do **not**. If the app cannot
   apply configured interceptors to a registered handler it panics at
   startup rather than silently running without them. Use
   `WithPlainService` for handlers that intentionally should not receive
   interceptors.

### Interceptor Chaining

The `AuthInterceptors` function creates a chain of interceptors:
1. Base authentication interceptor
2. "authenticated" permission check

This ensures that authentication is always validated before permissions.

## Method-Level Permissions with PathInterceptors

For fine-grained permission control at the RPC method level, use `PathInterceptors`. This is the recommended and only approach for applying permissions to your services.

### Basic Usage

```go
import (
    "github.com/navigacontentlab/dindenault"
    "connectrpc.com/connect"
)

// Define permission requirements for specific RPC methods
permissionConfigs := []dindenault.PathPermissionConfig{
    {
        PathPrefix:  "/service.v1.ServiceName/UploadDocument",
        Permissions: []string{"document:write"},
    },
    {
        PathPrefix:  "/service.v1.ServiceName/SearchDocuments",
        Permissions: []string{"document:read"},
    },
    {
        PathPrefix:  "/service.v1.ServiceName/DeleteDocument",
        Permissions: []string{"document:delete"},
    },
}

// Create handler with authentication and path-based permissions
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithInterceptors(
        dindenault.AuthInterceptors(logger, "https://imas.example.com"),
        dindenault.PathInterceptors(logger, permissionConfigs),
    ),
)

// Register with dindenault using the single service registration method
app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

### How PathInterceptors Work

`PathInterceptors` checks the RPC method path against configured prefixes and enforces the specified permissions:

1. **Path Matching**: When a request comes in, the interceptor checks if the method path starts with any configured `PathPrefix`
2. **Permission Checking**: If a match is found, it verifies the authenticated user has all required permissions
3. **Pass-through**: If no match is found, the request proceeds without additional permission checks

### PathPermissionConfig Structure

```go
type PathPermissionConfig struct {
    // PathPrefix is the RPC method path prefix to match
    // Example: "/service.v1.ServiceName/MethodName"
    PathPrefix string
    
    // Permissions are the organization-level permissions required
    // All permissions must be present for the request to succeed
    Permissions []string
}
```

### Complete Example with Multiple Services

Here's a complete example showing how to register multiple services with different permission requirements:

```go
// Service 1: Public API with read-only access
publicPermissions := []dindenault.PathPermissionConfig{
    {PathPrefix: "/api.v1.Public/Search", Permissions: []string{"api:read"}},
    {PathPrefix: "/api.v1.Public/GetDetails", Permissions: []string{"api:read"}},
}

publicPath, publicHandler := apiv1connect.NewPublicServiceHandler(
    publicImpl,
    connect.WithInterceptors(
        dindenault.AuthInterceptors(logger, imasURL),
        dindenault.PathInterceptors(logger, publicPermissions),
    ),
)

// Service 2: Admin API with write access
adminPermissions := []dindenault.PathPermissionConfig{
    {PathPrefix: "/api.v1.Admin/CreateUser", Permissions: []string{"admin:write", "user:create"}},
    {PathPrefix: "/api.v1.Admin/DeleteUser", Permissions: []string{"admin:write", "user:delete"}},
    {PathPrefix: "/api.v1.Admin/ViewAuditLog", Permissions: []string{"admin:read", "audit:view"}},
}

adminPath, adminHandler := apiv1connect.NewAdminServiceHandler(
    adminImpl,
    connect.WithInterceptors(
        dindenault.AuthInterceptors(logger, imasURL),
        dindenault.PathInterceptors(logger, adminPermissions),
    ),
)

// Register both services using WithService
app := dindenault.New(logger,
    dindenault.WithService(publicPath, publicHandler),
    dindenault.WithService(adminPath, adminHandler),
)
```

### Benefits of PathInterceptors

- **Method-level granularity**: Different permissions for different RPC methods
- **Multiple permission requirements**: Require multiple permissions for a single method
- **Clearer intent**: Permission configuration is explicit and visible
- **Standard Connect pattern**: Uses Connect's native interceptor system
- **Better testability**: Interceptors can be tested independently

### Advanced: Combining with Other Interceptors

PathInterceptors work seamlessly with other interceptors:

```go
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithInterceptors(
        dindenault.LoggingInterceptors(logger),           // Log all requests
        dindenault.AuthInterceptors(logger, imasURL),     // Authenticate users
        dindenault.PathInterceptors(logger, permissions), // Check permissions
        customRateLimitInterceptor,                       // Custom logic
    ),
)
```

Interceptors are executed in order, so place `AuthInterceptors` before `PathInterceptors` to ensure authentication happens first.

## Authentication with Naviga ID

Dindenault provides built-in support for Naviga ID authentication with several integration options.

### Basic Authentication

The simplest approach is using `AuthInterceptors` at handler creation:

```go
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithInterceptors(
        dindenault.AuthInterceptors(logger, "https://imas.example.com"),
    ),
)

app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

For plain (non-Connect) HTTP handlers, use `navigaid.HTTPMiddleware`:

```go
jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint("https://imas.example.com"))

app := dindenault.New(logger,
    dindenault.WithPlainService("/webhook", navigaid.HTTPMiddleware(logger, jwks, webhookHandler)),
)
```

### Combining Authentication and Permissions

Combine authentication with permission checks by stacking interceptors
at handler creation. Order matters — authentication must run first:

```go
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithInterceptors(
        dindenault.AuthInterceptors(logger, imasURL),
        navigaid.RequirePermission(logger, "my:permission"),
        navigaid.RequireUnitPermission(logger, "news", "article:publish"),
    ),
)

app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

For method-level granularity, use [`PathInterceptors`](#method-level-permissions-with-pathinterceptors)
instead of `RequirePermission`.

### Accessing Authentication in Services

Once authenticated, you can access the authentication information in your service using several convenient methods:

#### Using Convenience Helper Functions (Recommended)

```go
import "github.com/navigacontentlab/dindenault"

func (s *Service) YourMethod(ctx context.Context, req *connect.Request<api.YourRequest>) (*connect.Response<api.YourResponse>, error) {
    // Get organization directly
    organization := dindenault.OrganizationFromContext(ctx)
    if organization == "" {
        return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
    }
    
    // Get user information
    givenName, familyName := dindenault.UserFromContext(ctx)
    email := dindenault.EmailFromContext(ctx)
    
    // Check permissions
    if !dindenault.HasPermission(ctx, "service:access") {
        return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
    }
    
    // Continue with your implementation
    // ...
}
```

#### Using AuthorizeWithDetails for Permission Check and User Details

```go
import "github.com/navigacontentlab/dindenault"

func (s *Service) YourMethod(ctx context.Context, req *connect.Request<api.YourRequest>) (*connect.Response<api.YourResponse>, error) {
    // Get auth details and check permission in one call
    authResult, err := dindenault.AuthorizeWithDetails(ctx, "service:access") 
    if err != nil {
        return nil, err // Error is already properly formatted with connect.NewError
    }
    
    // Now you have access to all user details
    organization := authResult.Organization
    userFullName := fmt.Sprintf("%s %s", authResult.GivenName, authResult.FamilyName)
    email := authResult.Email
    userId := authResult.UserID
    
    // Access permissions and groups if needed
    userPermissions := authResult.Permissions
    userGroups := authResult.Groups
    
    // Log access for auditing
    s.logger.Info("Access granted",
        "user", userFullName,
        "userId", userId,
        "organization", organization)
    
    // Continue with your implementation
    // ...
}
```

#### Using GetAuthResultFromContext Without Permission Check

```go
import "github.com/navigacontentlab/dindenault"

func (s *Service) YourMethod(ctx context.Context, req *connect.Request<api.YourRequest>) (*connect.Response<api.YourResponse>, error) {
    // Get auth details without permission check
    authResult, err := dindenault.GetAuthResultFromContext(ctx)
    if err != nil {
        return nil, connect.NewError(connect.CodeUnauthenticated, err)
    }
    
    // Use auth details
    organization := authResult.Organization
    userFullName := fmt.Sprintf("%s %s", authResult.GivenName, authResult.FamilyName)
    
    // Continue with your implementation
    // ...
}
```

#### Using Raw navigaid Access (Legacy Method)

For more advanced scenarios or legacy code, you can still access the raw authentication data:

```go
import "github.com/navigacontentlab/dindenault/navigaid"

func (s *Service) YourMethod(ctx context.Context, req *connect.Request<api.YourRequest>) (*connect.Response<api.YourResponse>, error) {
    // Get authentication info
    authInfo, err := navigaid.GetAuth(ctx)
    if err != nil {
        return nil, connect.NewError(connect.CodeUnauthenticated, err)
    }
    
    // Access claims
    org := authInfo.Claims.Org
    userEmail := authInfo.Claims.Userinfo.Email
    userId := authInfo.Claims.Subject
    
    // Continue with your implementation
    // ...
}
```

## Response Compression

Dindenault enables response compression through Connect's native compression capabilities. 

### Recommended Approach

The recommended way to enable compression is to apply it directly when creating the Connect handler:

```go
// Create handler with compression enabled
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithCompressMinBytes(1024), // 1KB threshold
)

// Register using WithService
app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

This approach uses Connect's built-in compression system which:
- Automatically handles content negotiation via the `Accept-Encoding` header
- Only compresses responses larger than the specified threshold
- Supports multiple compression algorithms (gzip, deflate, br)
- Properly manages all compression-related headers

### Testing Compression with HTTP Clients

To test compression with an HTTP client like Postman:
1. Add a header: `Accept-Encoding: gzip, deflate, br`
2. Send a request that would generate a response larger than your threshold
3. Check the response headers for `Content-Encoding: gzip` (or other algorithm)

## CORS Support

CORS support is optional and provided through the `WithConnectRPC` function. Use this when your service needs to be accessed from web browsers:

```go
// Add CORS support for all Connect RPC services
app := dindenault.New(logger,
    dindenault.WithConnectRPC(cors.Options{
        AllowedDomains: []string{".navigacloud.com", ".infomaker.io"},
        AllowHTTP:      false, // Require HTTPS for security
    }),
    dindenault.WithService("api/", myServiceHandler),
)
```

For internal services that don't need CORS (e.g., backend-to-backend communication), simply omit `WithConnectRPC`:

```go
// Internal service without CORS
app := dindenault.New(logger,
    dindenault.WithService("api/", myServiceHandler),
)
```

When enabled, `WithConnectRPC` wraps every registered handler in an
HTTP-level CORS middleware that automatically:
1. Adds CORS headers to all responses (including error responses)
2. Answers OPTIONS preflight requests (`204 No Content`) before they reach your handler
3. Validates origins against the allowed list
4. Uses Connect RPC appropriate headers and methods

Requests without an `Origin` header, or from origins that are not
allowed, are passed through without CORS headers — the browser then
blocks the cross-origin read.

### Configuration Options

The `cors.Options` struct provides the following configuration:

```go
type Options struct {
    AllowedDomains []string // Domain suffixes or "*" for all origins
    AllowHTTP      bool     // Allow HTTP origins (for development)
}
```

### Usage Examples

#### Production Configuration (HTTPS Only)

```go
app := dindenault.New(logger,
    dindenault.WithConnectRPC(cors.Options{
        AllowedDomains: []string{".mycompany.com", ".myapp.io"},
        AllowHTTP:      false, // HTTPS only for security
    }),
)
```

#### Development Configuration (Allow HTTP)

```go
app := dindenault.New(logger,
    dindenault.WithConnectRPC(cors.Options{
        AllowedDomains: []string{"localhost", ".dev.myapp.com"},
        AllowHTTP:      true, // Allow HTTP for local development
    }),
)
```

#### Allow All Origins (Not Recommended for Production)

```go
app := dindenault.New(logger,
    dindenault.WithConnectRPC(cors.Options{
        AllowedDomains: []string{"*"}, // Allow all origins
        AllowHTTP:      true,
    }),
)
```

#### Multiple Domain Configuration

```go
app := dindenault.New(logger,
    dindenault.WithConnectRPC(cors.Options{
        AllowedDomains: []string{
            ".navigacloud.com",    // All subdomains of navigacloud.com
            ".infomaker.io",       // All subdomains of infomaker.io
            "app.example.com",     // Specific domain
        },
        AllowHTTP: false,
    }),
)
```

### CORS Headers

When CORS is enabled, the following headers are automatically added for
allowed origins:

- `Access-Control-Allow-Origin`: The validated origin from the request
- `Access-Control-Allow-Methods`: `POST, GET, OPTIONS`
- `Access-Control-Allow-Headers`: `Content-Type, Accept, Connect-Protocol-Version, Connect-Timeout-Ms, Authorization, X-Requested-With`
- `Access-Control-Allow-Credentials`: `true` — **omitted** when `AllowedDomains` contains `"*"`, since reflecting arbitrary origins with credentials would disable the browser's same-origin protections
- `Access-Control-Max-Age`: `86400` (24 hours, for preflight requests)
- `Vary: Origin`

### Domain Matching Rules

The `Origin` header is parsed as a URL and its hostname is matched on
label boundaries:

- **Domain + subdomains**: `"example.com"` (with or without a leading dot) matches `https://example.com` and `https://app.example.com`, but never `https://evilexample.com`
- **Wildcard**: `"*"` matches any origin (credentials are disabled; use with caution)
- **Protocol**: HTTP origins are only allowed when `AllowHTTP: true`; only `http`/`https` schemes are accepted
- **Ports**: ignored — `https://app.example.com:8443` matches `"example.com"`

## MCP (Model Context Protocol) Support

Dindenault includes a built-in stateless MCP server that lets AI agents such as AWS Bedrock AgentCore discover and call your Lambda's business logic as tools.

The implementation uses the **MCP Streamable HTTP transport** (JSON-RPC 2.0 over plain HTTP POST), which requires no SSE or persistent connections and maps naturally to the Lambda execution model.

### How It Works

A POST to your MCP endpoint carries a JSON-RPC 2.0 request. The server handles three methods:

- `initialize` — returns server info and capabilities
- `tools/list` — returns the list of available tools and their JSON schemas
- `tools/call` — invokes a tool by name and returns the result

### Registering an MCP Endpoint

Use `WithMCP` to mount an MCP server at any path alongside your existing Connect RPC services:

```go
import (
    "context"
    "encoding/json"

    "github.com/navigacontentlab/dindenault"
    "github.com/navigacontentlab/dindenault/mcp"
)

app := dindenault.New(logger,
    dindenault.WithService(connectPath, connectHandler), // existing Connect RPC
    dindenault.WithMCP("/mcp",
        mcp.Tool{
            Name:        "search_articles",
            Description: "Search articles in the content archive by free-text query",
            InputSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Free-text search query"},
                    "limit": {"type": "integer", "default": 10}
                },
                "required": ["query"]
            }`),
            Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
                token := mcp.AuthorizationFromContext(ctx) // forward JWT downstream
                var args struct {
                    Query string `json:"query"`
                    Limit int    `json:"limit"`
                }
                if err := json.Unmarshal(input, &args); err != nil {
                    return nil, err
                }
                // ... call your service
                return json.Marshal(map[string]any{"results": results})
            },
        },
    ),
)

lambda.Start(app.Handle())
```

### Tool Definition

```go
type Tool struct {
    Name                string          // Tool identifier shown to the AI model
    Description         string          // Explains what the tool does — shown to the model
    InputSchema         json.RawMessage // JSON Schema for the arguments (optional)
    RequiredPermissions []string        // Org-level permissions required to call the tool (optional)
    Handler             ToolHandler     // func(ctx, json.RawMessage) (json.RawMessage, error)
}
```

If `InputSchema` is `nil`, the server defaults to `{"type":"object","properties":{}}`.

### Per-Tool Authorization

`RequiredPermissions` enforces organization-level permissions per tool.
The check runs against the validated claims that `mcp.AuthMiddleware`
(used by `WithMCPAuth`) places in the context:

```go
mcp.Tool{
    Name:                "write_document",
    Description:         "Create or update a document",
    RequiredPermissions: []string{"content:write"},
    Handler:             writeDocument,
}
```

Calls without validated auth information, or from callers missing any of
the listed permissions, are rejected with a JSON-RPC error before the
handler runs. A tool with `RequiredPermissions` therefore cannot be
exposed unauthenticated by accident. Tools without `RequiredPermissions`
are responsible for their own authorization (e.g. via
`navigaid.GetAuth` or `dindenault.HasPermission`).

### Authentication Pass-Through

The `Authorization` header from the incoming HTTP request is propagated to every tool handler via the context. Use `mcp.AuthorizationFromContext` to retrieve it:

```go
func myHandler(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
    token := mcp.AuthorizationFromContext(ctx) // "Bearer eyJ..."
    // Forward token to downstream APIs
}
```

If no raw header was captured, the function reconstructs the bearer
token from validated auth information in the context (set by
`mcp.AuthMiddleware`, `navigaid.HTTPMiddleware`, or a navigaid Connect
interceptor) — so it works with any of dindenault's authentication
entry points. `mcp.NewHTTPClient(ctx, base)` gives you an `http.Client`
that forwards the token automatically.

### Tool Errors

When a handler returns an error, the MCP server returns a successful JSON-RPC response with `isError: true` in the result (per MCP spec). This lets the AI model observe the error and decide how to proceed, rather than treating it as a protocol-level failure.

### Custom Server Name and Version

`WithMCP` uses `"dindenault"` as the server name by default. For full control, register the server manually:

```go
server := mcp.NewServer("my-service", "2.1.0", tools...)
app := dindenault.New(logger,
    dindenault.WithService("/mcp", server),
)
```

### Note on Global Interceptors

MCP handlers are plain HTTP handlers and are automatically registered
with `SkipGlobalInterceptors` — Connect interceptors registered with
`WithInterceptors` are never applied to them, and do not cause the
startup panic described in [Interceptor Architecture](#interceptor-architecture).
Authentication is handled by `WithMCPAuth`/`mcp.AuthMiddleware`, and
authorization per tool via `RequiredPermissions` or inside the tool
handlers themselves.

## Telemetry and Observability

Dindenault includes comprehensive support for observability through logging, tracing, and metrics.

### Optional OpenTelemetry Integration

OpenTelemetry integration is available as an optional feature. By default, dindenault uses no-op telemetry to keep dependencies minimal.

#### Without OpenTelemetry (Default)

```go
app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        // No telemetry - lightweight build
    ),
)
```

#### With OpenTelemetry

First install the OpenTelemetry submodule:

```bash
go get github.com/navigacontentlab/dindenault/otel@latest
```

Then use it in your application:

```go
import (
    "github.com/navigacontentlab/dindenault"
    "github.com/navigacontentlab/dindenault/otel"
    "github.com/aws/aws-sdk-go-v2/config"
)

// Load AWS config
awsConfig, err := config.LoadDefaultConfig(ctx)
if err != nil {
    return err
}

// Create OpenTelemetry provider
telemetryProvider := otel.New(awsConfig)

// Configure telemetry options
telemetryOpts := dindenault.TelemetryOptions{
    MetricNamespace: "MyService",
    OrganizationFn:  otel.DefaultOrganizationFunction,
}

// Initialize telemetry
shutdown, err := telemetryProvider.Initialize(ctx, "my-service", telemetryOpts)
if err != nil {
    return err
}
defer shutdown(ctx)

app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        dindenault.TelemetryInterceptor(logger, telemetryProvider, telemetryOpts),
    ),
)
```

#### Available Metrics

When using OpenTelemetry, the following metrics are collected:

- `rpc.requests`: Counter for incoming requests
- `rpc.responses`: Counter for outgoing responses  
- `rpc.duration_ms`: Histogram for request duration in milliseconds

All metrics include dimensions for service, method, and organization.

#### Explicitly Disabling Telemetry

```go
app := dindenault.New(logger,
    dindenault.WithNoopTelemetry(), // Explicitly disable telemetry
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
    ),
)
```


### Benefits of Optional Telemetry

- **Minimal dependencies**: Applications that don't need telemetry won't pull in heavy OpenTelemetry dependencies
- **Clean separation**: Telemetry code is isolated in its own module  
- **Easy testing**: Use `NoopTelemetry{}` in tests
- **Flexible**: Easy to swap between different telemetry providers

## Architecture Details

Dindenault uses a Connect-native architecture where all functionality is implemented through Connect's built-in mechanisms:

### Connect-Native Approach

The implementation relies entirely on Connect's native capabilities:

1. **Connect Services with Built-in Compression**: Connect's built-in compression support via `WithCompressMinBytes` allows for efficient response compression with proper content negotiation.

2. **Connect Interceptors**: These operate at the RPC layer and provide logging, tracing, authentication, and metrics collection with full access to the Connect context.

### Benefits of Connect-Native Architecture

1. **Architectural consistency**: All functionality is handled within Connect's protocol mechanisms.
2. **Better protocol integration**: Connect's features are designed specifically for the Connect protocol.
3. **Per-service configuration**: Settings can be customized for each service based on its specific needs.
4. **Simplified code**: The solution is more maintainable with fewer layers and integration points.

## Advanced Features

### API Gateway Support

In addition to ALB support, Dindenault also supports API Gateway v2:

```go
// Use API Gateway handler instead of ALB handler
lambda.Start(app.HandleAPIGateway())
```

### Token Refresh for Long Operations

For long-running operations, `TokenRefresher` caches access tokens and
fetches new ones when they are about to expire:

```go
refresher := navigaid.NewTokenRefresher(logger, navigaid.AccessTokenEndpoint(imasURL))

accessToken, err := refresher.GetAccessToken(ctx, navigaIDToken)
if err != nil {
    return err
}
```

## Releasing

Dindenault uses semantic versioning for releases. You can create releases either manually using the Makefile or automatically via GitHub Actions.

### Automated Releases (Recommended)

Use the GitHub Actions workflow for automated releases:

1. Go to the "Actions" tab in GitHub
2. Select "Release" workflow
3. Click "Run workflow"
4. Choose the module (root or otel) and bump type (patch/minor/major)

The workflow will:
- Run tests to ensure everything passes
- Create and push the version tag
- Create a GitHub release with changelog reference

### Manual Releases

For local releases, use the `make release` command:

```bash
# Release the root module
make release MODULE=root BUMP=patch

# Release a submodule (e.g., otel)
make release MODULE=otel BUMP=minor
```

### Bump Types

- `patch`: Bug fixes and minor changes (e.g., v1.2.3 → v1.2.4)
- `minor`: New features, backward compatible (e.g., v1.2.3 → v1.3.0)
- `major`: Breaking changes (e.g., v1.2.3 → v2.0.0)

### Changelog

All notable changes are documented in [CHANGELOG.md](CHANGELOG.md). When making changes:

1. Add your changes under the `[Unreleased]` section
2. Use appropriate categories: Added, Changed, Deprecated, Removed, Fixed, Security
3. When releasing, the unreleased changes become part of the version history

## Contributing

### Running Tests

```bash
# Run all tests
go test -v ./...

# Run tests with race detection
go test -v -race ./...

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...
```

### Linting

The project uses golangci-lint v2.8.0 with a comprehensive set of linters. The configuration has been updated to v2 format:

```bash
# Run linter locally
golangci-lint run

# Run with auto-fix where possible
golangci-lint run --fix

# Run with timeout for large projects
golangci-lint run --timeout=5m
```

#### Configuration Changes in v2

The `.golangci.yml` configuration has been updated for v2:
- Formatters (gofmt, goimports) are now in a separate `formatters` section
- Some linters have been removed or renamed (typecheck, gosimple, stylecheck, tenv)
- The `wsl` linter has been deprecated in favor of `wsl_v5`
