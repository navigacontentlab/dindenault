// Package interceptors provides Connect RPC interceptors for logging, tracing, and more.
package interceptors

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/navigacontentlab/dindenault/internal/cors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

// XRay creates a Connect interceptor that adds AWS X-Ray tracing.
//
//nolint:ireturn
func XRay(name string) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract procedure information
			procedure := req.Spec().Procedure
			service, method := ExtractServiceAndMethod(procedure)

			// Create a subsegment for this RPC call
			subCtx, seg := xray.BeginSubsegment(ctx, name+":"+service+"."+method)
			defer seg.Close(nil)

			// Add procedure information as annotations
			// Ignore errors as we can't do anything if annotation fails
			_ = seg.AddAnnotation("rpc.service", service)
			_ = seg.AddAnnotation("rpc.method", method)
			_ = seg.AddAnnotation("rpc.procedure", procedure)

			// Call the next handler with the X-Ray context
			resp, err := next(subCtx, req)

			// If there was an error, record it
			if err != nil {
				_ = seg.AddError(err)
			}

			return resp, err
		}
	})
}

// OpenTelemetry creates a Connect interceptor that adds OpenTelemetry tracing.
//
//nolint:ireturn
func OpenTelemetry(name string) connect.Interceptor {
	// Create a tracer for this service
	tracer := otel.Tracer(name)

	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract procedure information
			procedure := req.Spec().Procedure
			service, method := ExtractServiceAndMethod(procedure)

			// Create a span for this RPC call
			spanName := service + "." + method
			spanCtx, span := tracer.Start(ctx, spanName,
				trace.WithAttributes(
					attribute.String("rpc.system", "connect"),
					attribute.String("rpc.service", service),
					attribute.String("rpc.method", method),
					attribute.String("rpc.procedure", procedure),
				),
			)

			// Ensure span is ended when we're done
			defer span.End()

			// Call the next handler with the span context
			resp, err := next(spanCtx, req)

			// If there was an error, record it
			if err != nil {
				span.RecordError(err)
			}

			return resp, err
		}
	})
}

// CORS creates a Connect interceptor that handles Cross-Origin Resource Sharing (CORS).
// Unlike other interceptors, CORS works at the HTTP header level, so this interceptor
// adds appropriate CORS headers to the response headers.
//
//nolint:ireturn
func CORS(allowedOrigins []string, allowHTTP bool) connect.Interceptor {
	// Use the standardAllowOriginFunc from cors.go for consistency
	originValidator := cors.StandardAllowOriginFunc(allowHTTP, allowedOrigins)

	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Get origin from request
			origin := req.Header().Get("Origin")
			if origin == "" {
				// No origin, no CORS headers needed
				return next(ctx, req)
			}

			// Check if the origin is allowed using the standard validator
			originAllowed := originValidator(origin)

			// If origin is not allowed, continue without CORS headers
			if !originAllowed {
				return next(ctx, req)
			}

			// Call the next handler to get the response
			resp, err := next(ctx, req)
			if err != nil {
				// If there was an error, we still need to add CORS headers to the error response
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					// Add CORS headers to the error
					connectErr.Meta().Set("Access-Control-Allow-Origin", origin)
					connectErr.Meta().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
					connectErr.Meta().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Connect-Protocol-Version")
				}

				return nil, err
			}

			// Add CORS headers to the response
			resp.Header().Set("Access-Control-Allow-Origin", origin)
			resp.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			resp.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Connect-Protocol-Version")

			return resp, nil
		}
	})
}
