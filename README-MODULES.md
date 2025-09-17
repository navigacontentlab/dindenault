# dindenault - Multi-Module Architecture

Dindenault is now structured as a multi-module repository with **clean separation of concerns**. The core module is minimal and you import only the functionality you need. This keeps binary sizes small and build times fast.

## Modules

### Core Module (github.com/navigacontentlab/dindenault)

The main module provides only **essential functionality**:
- Basic Connect RPC service registration
- Core logging interceptors
- Lambda request/response handling

**Dependencies**: Minimal - just Connect RPC, AWS Lambda, and basic logging.

```go
import "github.com/navigacontentlab/dindenault"

// Minimal setup with only core functionality
app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        // Add your custom interceptors here
    ),
    dindenault.WithService("/api/", myHandler),
)
```

### X-Ray Module (github.com/navigacontentlab/dindenault/xray)

Provides AWS X-Ray tracing interceptors.

**Dependencies**: AWS X-Ray SDK and all its dependencies.

```go
import (
    "github.com/navigacontentlab/dindenault"
    "github.com/navigacontentlab/dindenault/xray"
)

app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        xray.Interceptor("my-service"),
    ),
)
```

### Telemetry Module (github.com/navigacontentlab/dindenault/telemetry)

Provides comprehensive telemetry with OpenTelemetry, CloudWatch metrics, and Lambda instrumentation.

**Dependencies**: Full OpenTelemetry stack, AWS SDK, CloudWatch clients (heaviest module).

```go
import (
    "github.com/navigacontentlab/dindenault"
    "github.com/navigacontentlab/dindenault/telemetry"
)

// Initialize telemetry
telemetryOpts := &telemetry.Options{
    MetricNamespace: "my-service",
    OrganizationFn:  telemetry.DefaultOrganizationFunction(),
}

app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        telemetry.Interceptor(logger, telemetryOpts),
    ),
)
```

## Migration Guide

### From Old XRayInterceptors

**Before:**
```go
dindenault.XRayInterceptors("my-service")
```

**After:**
```go
import "github.com/navigacontentlab/dindenault/xray"

xray.Interceptor("my-service")
```

### From X-Ray to OpenTelemetry (Recommended)

**Before (X-Ray):**
```go
import "github.com/navigacontentlab/dindenault/xray"

app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        xray.Interceptor("my-service"),
    ),
)
```

**After (OpenTelemetry):**
```go
import "github.com/navigacontentlab/dindenault/telemetry"

// Initialize OpenTelemetry (do this early in your Lambda handler)
shutdown, err := telemetry.Initialize(ctx, "my-service", &telemetry.Options{
    MetricNamespace: "my-service",
    OrganizationFn:  telemetry.DefaultOrganizationFunction(),
})
if err != nil {
    return err
}
defer shutdown(ctx)

// Use telemetry interceptor instead of X-Ray
app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.LoggingInterceptors(logger),
        telemetry.Interceptor(logger, &telemetry.Options{
            OrganizationFn: telemetry.DefaultOrganizationFunction(),
        }),
    ),
)

// Wrap Lambda handler for OpenTelemetry instrumentation
lambda.Start(telemetry.InstrumentHandler(app.Handle()))
```

**Benefits of migrating to OpenTelemetry:**
- ✅ Modern observability standard
- ✅ AWS SDK v2 compatible (no more v1 dependency)
- ✅ Better performance and lower overhead
- ✅ More detailed tracing and metrics
- ✅ Compatible with multiple backends (not just X-Ray)

### From Old OpenTelemetryInterceptors

**Before (if it existed):**
```go
dindenault.OpenTelemetryInterceptors(logger, opts)
```

**After:**
```go
import "github.com/navigacontentlab/dindenault/telemetry"

opts := &telemetry.Options{
    MetricNamespace: "my-service",
    OrganizationFn:  telemetry.DefaultOrganizationFunction(),
}
telemetry.Interceptor(logger, opts)
```

## Binary Size Comparison

| Import | Approximate Binary Size |
|--------|------------------------|
| Core only | ~8MB |
| Core + X-Ray | ~25MB |
| Core + Telemetry | ~35MB |
| Core + All modules | ~42MB |

## Development

For local development, the main module uses `replace` directives to reference the submodules:

```go
// go.mod
replace github.com/navigacontentlab/dindenault/xray => ./xray
replace github.com/navigacontentlab/dindenault/telemetry => ./telemetry
```

When publishing, these `replace` directives should be removed and proper version tags used.
