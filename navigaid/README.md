# Navigaid - Naviga ID Authentication for Go

Navigaid provides comprehensive Naviga ID authentication and authorization functionality for Go applications, especially those using Connect RPC.

## Features

- **JWT Token Validation**: Validate and parse Naviga ID tokens
- **JWKS Management**: Automatic fetching and caching of JSON Web Key Sets
- **Connect RPC Integration**: Ready-to-use Connect interceptors
- **Permission Management**: Check user permissions and group memberships
- **Token Refresh**: Automatic token refresh for long-running operations
- **Tracing Integration**: Built-in support for both OpenTelemetry and AWS X-Ray tracing
- **Context Management**: Store and retrieve authentication info from context

## Installation

```bash
go get github.com/navigacontentlab/dindenault/navigaid
```

## Quick Start

### Basic Authentication with Connect RPC

```go
import (
    "github.com/navigacontentlab/dindenault/navigaid"
    "connectrpc.com/connect"
)

// Create JWKS client
jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint("https://imas.example.com"))

// Create Connect interceptor
interceptor := navigaid.ConnectInterceptor(logger, jwks)

// Use with your Connect service
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithInterceptors(interceptor),
)
```

### Using Authentication in Services

```go
func (s *Service) MyMethod(ctx context.Context, req *connect.Request[MyRequest]) (*connect.Response[MyResponse], error) {
    // Get authentication info
    authInfo, err := navigaid.GetAuth(ctx)
    if err != nil {
        return nil, connect.NewError(connect.CodeUnauthenticated, err)
    }
    
    // Access user information
    org := authInfo.Claims.Org
    userEmail := authInfo.Claims.Userinfo.Email
    
    // Check permissions
    if !authInfo.Claims.HasPermission("content:read") {
        return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("insufficient permissions"))
    }
    
    // Continue with business logic...
}
```

## Core Components

### JWKS (JSON Web Key Set) Management

```go
// Create JWKS client with default TTL
jwks := navigaid.NewJWKS("https://imas.example.com/.well-known/jwks.json")

// Or with custom options
jwks := navigaid.NewJWKS(
    "https://imas.example.com/.well-known/jwks.json",
    navigaid.WithJWKSTTL(time.Hour),
    navigaid.WithJWKSClient(customHTTPClient),
)

// Validate a token
token, err := jwks.ValidateToken(tokenString)
```

### Claims and Permissions

```go
// Check if user has specific permissions
hasPermission := claims.HasPermission("content:read")

// Check unit-specific permissions
hasUnitPermission := claims.HasPermissionsInUnit("news", "article:publish")

// Access user information
organization := claims.Org
groups := claims.Groups
email := claims.Userinfo.Email
```

### Token Refresh

For long-running operations, you can refresh tokens automatically:

```go
refresher := navigaid.NewTokenRefresher(
    logger, 
    navigaid.AccessTokenEndpoint("https://imas.example.com"),
)

err := navigaid.WithTokenRefresh(ctx, refresher, func(refreshedCtx context.Context) error {
    // Your long-running operation using refreshedCtx
    return performLongOperation(refreshedCtx)
})
```

## Tracing Integration

Navigaid automatically integrates with both **OpenTelemetry** (preferred) and **AWS X-Ray** for observability:

```go
// Authentication events are automatically traced with OpenTelemetry spans
// User and organization info is added as span attributes

// Add custom annotations (works with both OpenTelemetry and X-Ray)
navigaid.AddAnnotation(ctx, "organization", authInfo.Claims.Org)
navigaid.AddUserAnnotation(ctx, authInfo.Claims.Subject)
```

**OpenTelemetry Support** (Recommended):
- Automatically adds user and organization attributes to spans
- Compatible with modern observability backends
- Works alongside the dindenault telemetry module

**X-Ray Support** (Legacy):
- Backwards compatible with existing X-Ray setups
- Will be deprecated in future versions

### Migration to OpenTelemetry

When using navigaid with the dindenault telemetry module, authentication information is automatically added to OpenTelemetry spans. No code changes needed - the module handles both OpenTelemetry and X-Ray for backwards compatibility.

## API Reference

### Types

- `Claims` - JWT claims structure with Naviga ID specific fields
- `AuthInfo` - Authentication information stored in context  
- `JWKS` - JSON Web Key Set manager for token validation
- `AccessTokenService` - Service for token operations
- `TokenRefresher` - Helper for refreshing tokens

### Functions

- `NewJWKS(endpoint string, opts...)` - Create new JWKS manager
- `ConnectInterceptor(logger, jwks)` - Create Connect RPC interceptor
- `GetAuth(ctx)` - Retrieve auth info from context
- `WithTokenRefresh(ctx, refresher, fn)` - Execute function with token refresh

### Constants

- `TokenTypeAccessToken` - Access token type identifier
- `TokenTypeIDToken` - ID token type identifier

## Error Handling

Navigaid uses Connect's error system:

```go
authInfo, err := navigaid.GetAuth(ctx)
if err != nil {
    // Return appropriate Connect error
    return nil, connect.NewError(connect.CodeUnauthenticated, err)
}
```

## Contributing

This module is part of the [Dindenault](https://github.com/navigacontentlab/dindenault) framework for building Connect RPC services in AWS Lambda.

## License

See the main Dindenault repository for license information.
