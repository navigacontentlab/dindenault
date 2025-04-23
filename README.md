# Dindenault

Dindenault provides a framework for building Connect RPC services in AWS Lambda. It offers a clean, maintainable API with a Connect-native architecture.

## Table of Contents

- [Features](#features)
- [Getting Started](#getting-started)
- [Interceptor Architecture](#interceptor-architecture)
- [Authentication with Naviga ID](#authentication-with-naviga-id)
- [Response Compression](#response-compression)
- [CORS Support](#cors-support)
- [Telemetry and Observability](#telemetry-and-observability)
- [Architecture Details](#architecture-details)
- [Advanced Features](#advanced-features)

## Features

- **Connect-native architecture**: Built from the ground up for Connect RPC services
- **Unified interceptor system**: Single interface for logging, tracing, auth, and more
- **Authentication and authorization**: Built-in support for Naviga ID
- **Comprehensive observability**: Integrated logging, tracing, and metrics
- **Response compression**: Native support through Connect's compression capabilities
- **Simplified deployment**: Ready for AWS Lambda environments

## Getting Started

### Basic Implementation

```go
import "github.com/navigacontentlab/dindenault"

app := dindenault.New(logger,
    // Set application info
    dindenault.WithName("my-service"),
    
    // Add all cross-cutting concerns in one call
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        dindenault.XRayInterceptors("my-service"),
        dindenault.AuthInterceptors("https://imas.example.com", []string{"service:read"}),
    ),
    
    // Register services with automatic compression
    dindenault.WithCompressedService(
        rpcconnect.ServiceHandler(myService),
        "api/",
        1024, // Compress responses larger than 1KB
    ),
)

// Start the Lambda handler
lambda.Start(app.Handle())
```

### With Secure and Compressed Services

For services requiring authentication, permissions, and compression:

```go
app := dindenault.New(logger,
    dindenault.WithName("my-service"),
    dindenault.WithVersion("1.0.0"),
    
    // Standard secure service with permissions
    dindenault.WithSecureService(
        "admin/",
        rpcconnect.AdminServiceHandler(adminService),
        []string{"admin:access"},
    ),
    
    // Service with standard compression (1KB threshold)
    dindenault.WithCompressedService(
        "api/",
        rpcconnect.ApiServiceHandler(apiService),
        1024,
    ),
    
    // Service with both permissions and custom compression (5KB threshold)
    dindenault.WithSecureCompressedService(
        "content/",
        rpcconnect.ContentServiceHandler(contentService),
        []string{"content:read", "content:write"},
        5120, // 5KB compression threshold
    ),
    
    // Add standard features (logging, tracing, telemetry)
    dindenault.WithDefaultServices(),
)

lambda.Start(app.Handle())
```

## Interceptor Architecture

Dindenault's architecture is built around Connect interceptors. All cross-cutting concerns are implemented using interceptors, providing a consistent approach to logging, tracing, authentication, and more.

### Available Interceptors

- **`LoggingInterceptors(logger)`**: Adds request logging with timing information
- **`XRayInterceptors(name)`**: Adds AWS X-Ray tracing
- **`OpenTelemetryInterceptors(name)`**: Adds OpenTelemetry tracing
- **`CORSInterceptors(allowedOrigins, allowHTTP)`**: Adds CORS support
- **`AuthInterceptors(imasURL, permissions)`**: Adds Naviga ID authentication and permission checks

### Using Interceptors

The `WithInterceptors` function allows adding multiple interceptors in a single call:

```go
app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        dindenault.XRayInterceptors("my-service"),
        dindenault.AuthInterceptors("https://imas.example.com", []string{"service:read"}),
    ),
)
```

For convenience, you can also use `WithDefaultServices()` to add logging and tracing:

```go
app := dindenault.New(logger,
    dindenault.WithName("my-service"),
    dindenault.WithVersion("1.0.0"),
    dindenault.WithDefaultServices(),
)
```

### Interceptor Chaining

The `AuthInterceptors` function creates a chain of interceptors:
1. Base authentication interceptor
2. "authenticated" permission check
3. Additional permission checks from the provided permissions list

This ensures that authentication is always validated before permissions.

## Authentication with Naviga ID

Dindenault provides built-in support for Naviga ID authentication with several integration options.

### Basic Authentication

The simplest approach is using `AuthInterceptors`:

```go
app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.AuthInterceptors("https://imas.example.com", []string{"service:read"}),
    ),
    dindenault.WithService("service/", serviceHandler),
)
```

### Using NewConnectHandler

For more control, use the `NewConnectHandler` utility which adds authentication and permissions checks to a specific handler:

```go
// Create the JWKS for token validation
jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint(imasURL))

// Create your service implementation
myService := service.NewMyService(logger)

// Create the basic handler without authentication
baseHandler := service.NewMyServiceHandler(myService)

// Wrap the handler with authentication
authHandler := dindenault.NewConnectHandler(
    logger,
    jwks,
    baseHandler,
    dindenault.WithRequiredPermissions("my:permission"),
    dindenault.WithUnitPermissions("news", "article:publish"),
)

// Add the authenticated handler to your app
app := dindenault.New(logger,
    dindenault.WithService("myservice/", authHandler),
)
```

### Accessing Authentication in Services

Once authenticated, you can access the authentication information in your service:

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

Dindenault provides efficient response compression using Connect's built-in compression support:

```go
// Add a service with compression (responses larger than 1KB will be compressed)
dindenault.WithCompressedService(
    myServiceHandler,
    "myservice/", 
    1024
)
```

Connect's built-in compression automatically:
- Compresses responses based on the client's `Accept-Encoding` header
- Only compresses responses larger than the specified threshold
- Handles all compression headers and encoding details

Benefits include reduced bandwidth usage, faster response times, and better performance for clients on slower connections.

## CORS Support

CORS support is provided through both interceptors and preflight request handlers:

```go
// Add CORS support
dindenault.WithCORS("/", []string{"https://app.example.com"})
```

This automatically:
1. Adds CORS headers to all responses via an interceptor
2. Handles OPTIONS preflight requests correctly
3. Validates origins against the allowed list

## Telemetry and Observability

Dindenault includes comprehensive support for observability through logging, tracing, and metrics.

### CloudWatch Metrics with OpenTelemetry

```go
// Create AWS session
sess, err := session.NewSession(&aws.Config{
    Region: aws.String(os.Getenv("AWS_REGION")),
})
if err != nil {
    logger.Error("Failed to create AWS session", "error", err)
    os.Exit(1)
}

// Add telemetry to your app
app := dindenault.New(logger,
    // Add telemetry configuration
    dindenault.WithTelemetryNamespace("MyService"),
    dindenault.WithTelemetryAWSSession(sess),
    dindenault.WithTelemetry(logger), // Enable telemetry collection
    
    // Add services and other configuration
    // ...
)
```

### Available Metrics

The default metrics include:

- `rpc.requests`: Counter for incoming requests
- `rpc.responses`: Counter for outgoing responses
- `rpc.duration_ms`: Histogram for request duration in milliseconds

All metrics include dimensions for service, method, and organization.

### Customizing Telemetry

```go
app := dindenault.New(logger,
    // Custom namespace (default is "Dindenault")
    dindenault.WithTelemetryNamespace("CustomNamespace"),
    
    // Custom organization function
    dindenault.WithTelemetryOrganizationFunction(func(ctx context.Context) string {
        return "my-organization"
    }),
    
    // Additional attributes for all metrics
    dindenault.WithTelemetryAttributes(
        attribute.String("environment", "production"),
        attribute.String("region", "us-west-2"),
    ),
    
    // Enable telemetry
    dindenault.WithTelemetry(logger),
)
```

### X-Ray Integration

Dindenault integrates seamlessly with AWS X-Ray for distributed tracing:

```go
app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.XRayInterceptors("my-service"),
    ),
)
```

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

For long-running operations, you can refresh tokens automatically:

```go
refresher := navigaid.NewTokenRefresher(logger, navigaid.AccessTokenEndpoint(imasURL))

err := navigaid.WithTokenRefresh(ctx, refresher, func(refreshedCtx context.Context) error {
    // Use refreshedCtx which will have a valid token throughout the operation
    return longRunningOperation(refreshedCtx)
})
```

### XRay Annotations

Authentication events are automatically added to XRay traces:

```go
// Add custom annotations to X-Ray segments
navigaid.AddAnnotation(ctx, "custom_field", value)
```
