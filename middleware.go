package didenault

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-xray-sdk-go/xray"
	"go.opentelemetry.io/otel"
)

// WithLogging returns a middleware that logs requests.
func WithLogging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			logger.Info("request started",
				"path", r.URL.Path,
				"method", r.Method)

			next.ServeHTTP(w, r)

			logger.Info("request completed",
				"path", r.URL.Path,
				"duration", time.Since(start))
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
func WithOpenTelemetry(name string) Middleware {
	tracer := otel.Tracer(name)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracer.Start(r.Context(), r.URL.Path)
			defer span.End()

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// WithCORS returns a middleware that adds CORS headers.
func WithCORS(allowedOrigins []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)

					break
				}
			}

			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.WriteHeader(http.StatusOK)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
