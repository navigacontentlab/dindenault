// Package interceptors provides Connect RPC interceptors for logging and CORS.
package interceptors

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
)

// ExtractServiceAndMethod extracts the service name and method name from a Connect RPC procedure path.
// Connect procedure paths are typically in the form "/package.Service/Method".
func ExtractServiceAndMethod(procedure string) (string, string) {
	// Default values in case we can't extract them
	service, method := "unknown", "unknown"

	// A Connect procedure path is typically in the form "/package.Service/Method"
	parts := strings.Split(procedure, "/")

	if len(parts) >= 3 {
		// Extract service name (might include package prefix)
		serviceWithPackage := parts[1]
		serviceParts := strings.Split(serviceWithPackage, ".")

		if len(serviceParts) > 0 {
			service = serviceParts[len(serviceParts)-1]
		}

		// Extract method name
		method = parts[2]
	}

	return service, method
}

// Logging creates a Connect interceptor that logs requests with timing information.
//
//nolint:ireturn
func Logging(logger *slog.Logger) connect.Interceptor {
	logger.Debug("Creating logging interceptor")

	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract procedure information
			procedure := req.Spec().Procedure
			service, method := ExtractServiceAndMethod(procedure)

			// Store start time
			start := time.Now()

			// Create log attributes
			logAttrs := []any{
				"service", service,
				"method", method,
				"procedure", procedure,
			}

			// Extract request ID if present in headers
			if requestID := req.Header().Get("X-Request-ID"); requestID != "" {
				logAttrs = append(logAttrs, "request_id", requestID)
			}

			// Log request start
			logger.Info("Connect RPC request started", logAttrs...)

			// Process the request
			resp, err := next(ctx, req)

			// Calculate duration
			duration := time.Since(start)

			// Add duration to log attributes
			logAttrs = append(logAttrs, "duration_ms", duration.Milliseconds())

			// Add error information if present
			if err != nil {
				logAttrs = append(logAttrs, "error", err.Error())
				logger.Error("Connect RPC request failed", logAttrs...)
			} else {
				logger.Info("Connect RPC request completed", logAttrs...)
			}

			return resp, err
		}
	})
}
