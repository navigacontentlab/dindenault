# OpenTelemetry Integration for Dindenault

The `otel` package provides optional OpenTelemetry integration for dindenault.

## Installation

To use OpenTelemetry functionality, install the otel submodule:

```bash
go get github.com/navigacontentlab/dindenault/otel@latest
```

## Usage

### Basic Usage (No Telemetry)

```go
package main

import (
	"log/slog"
	"github.com/navigacontentlab/dindenault"
)

func main() {
	logger := slog.Default()
	
	app := dindenault.New(logger,
		// This uses no-op telemetry by default
		dindenault.WithInterceptors(
			dindenault.LoggingInterceptors(logger),
		),
	)
	
	// Register your services...
	app.Register("myservice.v1.MyService", myHandler)
}
```

### With OpenTelemetry

```go
package main

import (
	"context"
	"log/slog"
	
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/otel"
)

func main() {
	logger := slog.Default()
	ctx := context.Background()
	
	// Load AWS config
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Error("Failed to load AWS config", "error", err)
		return
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
		logger.Error("Failed to initialize telemetry", "error", err)
		return
	}
	defer shutdown(ctx)
	
	app := dindenault.New(logger,
		dindenault.WithInterceptors(
			dindenault.LoggingInterceptors(logger),
			dindenault.TelemetryInterceptor(logger, telemetryProvider, telemetryOpts),
		),
	)
	
	// Register your services...
	app.Register("myservice.v1.MyService", myHandler)
}
```

### Using WithTelemetry Option

```go
package main

import (
	"context"
	"log/slog"
	
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/otel"
)

func main() {
	logger := slog.Default()
	ctx := context.Background()
	
	// Load AWS config
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Error("Failed to load AWS config", "error", err)
		return
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
		logger.Error("Failed to initialize telemetry", "error", err)
		return
	}
	defer shutdown(ctx)
	
	app := dindenault.New(logger,
		dindenault.WithTelemetry(telemetryProvider, telemetryOpts),
		dindenault.WithInterceptors(
			dindenault.LoggingInterceptors(logger),
		),
	)
	
	// Register your services...
	app.Register("myservice.v1.MyService", myHandler)
}
```

### Explicitly Disabling Telemetry

```go
app := dindenault.New(logger,
	dindenault.WithNoopTelemetry(), // Explicitly disable telemetry
	dindenault.WithInterceptors(
		dindenault.LoggingInterceptors(logger),
	),
)
```

## Lambda Handler Instrumentation

When using Lambda handlers, you can instrument them with OpenTelemetry:

```go
func main() {
	// ... setup code ...
	
	// Your original handler
	handler := func(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		// Your handler logic
		return app.HandleRequest(ctx, event)
	}
	
	// Instrument with OpenTelemetry
	instrumentedHandler := telemetryProvider.InstrumentHandler(handler)
	
	lambda.Start(instrumentedHandler)
}
```

## Benefits

- **No forced dependencies**: Applications that don't need OpenTelemetry won't pull in the dependencies
- **Clean separation**: OpenTelemetry code is isolated in its own module
- **Easy to test**: You can use `NoopTelemetry{}` in tests
- **Flexible**: Easy to swap between different telemetry providers

## Migration from Previous Version

### Before (with forced OpenTelemetry)

```go
app := dindenault.New(logger,
	dindenault.WithInterceptors(
		dindenault.OpenTelemetryInterceptors("my-service"),
	),
)
```

### After (optional OpenTelemetry)

```go
// Option 1: No telemetry
app := dindenault.New(logger,
	dindenault.WithInterceptors(
		dindenault.LoggingInterceptors(logger),
	),
)

// Option 2: With OpenTelemetry
import "github.com/navigacontentlab/dindenault/otel"

telemetryProvider := otel.New(awsConfig)
// ... initialize ...

app := dindenault.New(logger,
	dindenault.WithInterceptors(
		dindenault.LoggingInterceptors(logger),
		dindenault.TelemetryInterceptor(logger, telemetryProvider, telemetryOpts),
	),
)
```
