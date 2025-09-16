package dindenault

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/navigacontentlab/dindenault/internal/interceptors"
)

type Option func(*App)

// WithInterceptors adds multiple connect interceptors at once.
//
// Example:
//
//	app := dindenault.New(logger,
//		dindenault.WithInterceptors(
//			dindenault.LoggingInterceptors(logger),
//			// Add your custom interceptors here
//		),
//	)
func WithInterceptors(interceptorsList ...connect.Interceptor) Option {
	return func(a *App) {
		a.globalInterceptors = append(a.globalInterceptors, interceptorsList...)
	}
}

// LoggingInterceptors returns logging interceptors for Connect RPC.
// This creates interceptors that log request details and timing information.
//
//nolint:ireturn // Returning interface as intended by connect.Interceptor design
func LoggingInterceptors(logger *slog.Logger) connect.Interceptor {
	if logger == nil {
		logger = slog.Default()
	}

	return interceptors.Logging(logger)
}

// WithService adds a service to the application.
//
// Example:
//
//	app := dindenault.New(logger,
//		dindenault.WithService("/hello/", helloServiceHandler),
//	)
func WithService(path string, handler http.Handler) Option {
	return func(a *App) {
		a.registrations = append(a.registrations, Registration{
			Path:    path,
			Handler: handler,
		})
	}
}
