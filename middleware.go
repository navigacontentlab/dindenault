package dindenault

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-xray-sdk-go/xray"
	"go.opentelemetry.io/otel"
)

// WithLogging returns a middleware that logs requests with timing information.
// It logs both the start and completion of each request, including the duration.
func WithLogging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			requestID := r.Header.Get("X-Request-ID")
			logAttrs := []any{
				"path", r.URL.Path,
				"method", r.Method,
			}
			
			if requestID != "" {
				logAttrs = append(logAttrs, "request_id", requestID)
			}

			logger.Info("request started", logAttrs...)

			// Process the request
			next.ServeHTTP(w, r)

			// Log completion with duration
			duration := time.Since(start)
			logAttrs = append(logAttrs, "duration_ms", duration.Milliseconds())
			
			logger.Info("request completed", logAttrs...)
		})
	}
}

// WithXRay returns a middleware that adds AWS X-Ray tracing.
func WithXRay(name string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, seg := xray.BeginSegment(r.Context(), name)
			defer seg.Close(nil)

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// WithOpenTelemetry returns a middleware that adds OpenTelemetry tracing.
// It creates spans for each request with the URL path as the span name.
func WithOpenTelemetry(name string) Middleware {
	// Create a tracer instance for this service
	tracer := otel.Tracer(name)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Start a span for this request
			spanName := r.Method + " " + r.URL.Path
			ctx, span := tracer.Start(r.Context(), spanName)
			
			// Always end the span when we're done
			defer span.End()
			
			// Add common HTTP attributes to the span
			span.SetAttributes(
				// Add HTTP attributes like method, route, host
				// These could be expanded in the future
			)

			// Pass the span context to downstream handlers
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// WithCORS returns a middleware that adds CORS headers for cross-origin requests.
// It supports checking the Origin header against a list of allowed origins.
// For OPTIONS requests (preflight), it sets appropriate CORS headers and returns immediately.
func WithCORS(allowedOrigins []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			
			// Check if the origin is allowed
			originAllowed := false
			for _, allowed := range allowedOrigins {
				if origin == allowed || allowed == "*" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					originAllowed = true
					break
				}
			}
			
			// If origin is not allowed and not an OPTIONS request, return 403 Forbidden
			if !originAllowed && r.Method != http.MethodOptions && origin != "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			
			// Handle preflight OPTIONS requests
			if r.Method == http.MethodOptions {
				// Set standard CORS preflight headers
				w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
				w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours
				
				// No need to process the request further for OPTIONS
				w.WriteHeader(http.StatusOK)
				return
			}

			// For non-OPTIONS requests, continue to the next handler
			next.ServeHTTP(w, r)
		})
	}
}
