package dindenault

import (
	"connectrpc.com/connect"
	"context"
	"log/slog"

	"github.com/navigacontentlab/dindenault/navigaid"
)

// TelemetryProvider defines the interface for telemetry functionality.
// This allows for optional OpenTelemetry integration without requiring
// the full OpenTelemetry dependency in the main module.
type TelemetryProvider interface {
	// Initialize sets up telemetry with the given service name and options.
	// Returns a shutdown function that should be called when the service stops.
	Initialize(ctx context.Context, serviceName string, opts TelemetryOptions) (func(context.Context) error, error)

	// Interceptor returns a Connect interceptor that adds telemetry to RPC calls.
	Interceptor(logger *slog.Logger, opts TelemetryOptions) connect.Interceptor

	// InstrumentHandler instruments a handler with telemetry.
	// This is used for Lambda handlers and similar.
	InstrumentHandler(handler interface{}) interface{}
}

// TelemetryOptions contains configuration for telemetry.
type TelemetryOptions struct {
	// MetricNamespace is the CloudWatch namespace for metrics
	MetricNamespace string

	// OrganizationFn extracts organization from context
	OrganizationFn func(context.Context) string

	// DisableMetrics disables metric collection
	DisableMetrics bool
}

// NoopTelemetry provides a no-operation implementation of TelemetryProvider.
// This is used when OpenTelemetry is not available or disabled.
type NoopTelemetry struct{}

// Initialize implements TelemetryProvider for NoopTelemetry.
func (n NoopTelemetry) Initialize(ctx context.Context, serviceName string, opts TelemetryOptions) (func(context.Context) error, error) {
	// Return a no-op shutdown function
	return func(context.Context) error { return nil }, nil
}

// Interceptor implements TelemetryProvider for NoopTelemetry.
func (n NoopTelemetry) Interceptor(logger *slog.Logger, opts TelemetryOptions) connect.Interceptor {
	return nil // No interceptor
}

// InstrumentHandler implements TelemetryProvider for NoopTelemetry.
func (n NoopTelemetry) InstrumentHandler(handler interface{}) interface{} {
	return handler // Return handler unchanged
}

// DefaultTelemetryOptions returns default telemetry options.
func DefaultTelemetryOptions() TelemetryOptions {
	return TelemetryOptions{
		OrganizationFn: func(ctx context.Context) string {
			// Try to extract organization from JWT claims using navigaid
			auth, err := navigaid.GetAuth(ctx)
			if err != nil {
				return "unknown"
			}
			return auth.Claims.Org
		},
	}
}
