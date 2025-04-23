package dindenault

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
)

// Option is a function that configures the App.
type Option func(*App)

// Config holds configuration for dindenault.
type Config struct {
	// Logger for the application
	Logger *slog.Logger

	// IMAS URL for authentication
	ImasURL string

	// Name of the application
	Name string
}

// WithName sets the name for the application.
func WithName(name string) Option {
	return func(a *App) {
		a.config.Name = name
	}
}

// WithService adds a Connect service to the app.
func WithService(path string, handler http.Handler) Option {
	return func(a *App) {
		a.registrations = append(a.registrations, Registration{
			Path:    path,
			Handler: handler,
		})
	}
}

// - minBytes: Minimum size in bytes for responses to be compressed.
func WithCompressedService(path string, handler http.Handler, minBytes int) Option {
	// If the handler implements the Connect handler with compression interface,
	// we can apply compression directly
	if compressibleHandler, ok := handler.(ConnectHandlerWithCompression); ok {
		compressedHandler := compressibleHandler.WithCompressMinBytes(minBytes)

		return WithService(path, compressedHandler)
	}

	// If the handler doesn't support compression directly, just add it as is
	// and log a warning in the initialization phase
	return func(a *App) {
		a.logger.Warn("Handler does not support Connect compression", "path", path)
		a.registrations = append(a.registrations, Registration{
			Path:    path,
			Handler: handler,
		})
	}
}

// WithDefaultServices adds standard features to the application:
// - Logging for requests
// - X-Ray tracing
//
// Example:
//
//	app := dindenault.New(logger,
//	    dindenault.WithDefaultServices(),
//	)
func WithDefaultServices() Option {
	return func(a *App) {
		// Create interceptors using public API functions
		interceptorsList := []connect.Interceptor{
			LoggingInterceptors(a.logger),
			XRayInterceptors(a.config.Name),
		}

		// Add OpenTelemetry if necessary configuration is available
		if a.config.Name != "" {
			interceptorsList = append(interceptorsList,
				OpenTelemetryInterceptors(a.config.Name))
		}

		// Add all interceptors at once
		a.globalInterceptors = append(a.globalInterceptors, interceptorsList...)

		a.logger.Info("Added default features",
			"name", a.config.Name)
	}
}
