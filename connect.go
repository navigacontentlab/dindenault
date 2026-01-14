package dindenault

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	"github.com/navigacontentlab/dindenault/cors"
	"github.com/navigacontentlab/dindenault/internal/interceptors"
	"github.com/navigacontentlab/dindenault/navigaid"
)

// Option is a function that configures an App instance.
type Option func(*App)

// WithInterceptors adds multiple connect interceptors at once.
//
// Example:
//
//	app := dindenault.New(Logger,
//		dindenault.WithInterceptors(
//			dindenault.LoggingInterceptors(Logger),
//			dindenault.XRayInterceptors("my-service"),
//			dindenault.AuthInterceptors(Logger, "https://imas.example.com"),
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

// TelemetryInterceptor returns a telemetry interceptor using the provided TelemetryProvider.
// If provider is nil, returns nil (no telemetry).
//
//nolint:ireturn // Returning interface as intended by connect.Interceptor design
func TelemetryInterceptor(logger *slog.Logger, provider TelemetryProvider, opts TelemetryOptions) connect.Interceptor {
	if provider == nil {
		return nil
	}

	return provider.Interceptor(logger, opts)
}

// WithTelemetry configures telemetry for the App.
func WithTelemetry(provider TelemetryProvider, opts TelemetryOptions) Option {
	return func(a *App) {
		a.telemetryProvider = provider
		a.telemetryOptions = opts
	}
}

// WithNoopTelemetry configures the App to use no-op telemetry (disables telemetry).
func WithNoopTelemetry() Option {
	return func(a *App) {
		a.telemetryProvider = NoopTelemetry{}
		a.telemetryOptions = DefaultTelemetryOptions()
	}
}

// CORSInterceptors returns CORS interceptors for Connect RPC.
// This creates interceptors that add CORS headers to Connect RPC responses.
//
//nolint:ireturn // Returning interface as intended by connect.Interceptor design
func CORSInterceptors(allowedOrigins []string, allowHTTP bool) connect.Interceptor {
	return interceptors.CORS(allowedOrigins, allowHTTP)
}

// AuthInterceptors returns authentication interceptors for Connect RPC.
// This creates interceptors that handle authentication with Naviga ID.
//
// Parameters:
// - imasURL: The URL of the Naviga ID IMAS service
//
//nolint:ireturn // Returning interface as intended by connect.Interceptor design
func AuthInterceptors(logger *slog.Logger, imasURL string) connect.Interceptor {
	if imasURL == "" {
		panic("imasURL cannot be empty for AuthInterceptors")
	}
	// Create JWKS for token validation
	jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint(imasURL))

	return navigaid.ConnectInterceptor(logger, jwks)
}

// ConnectOptions configures Connect RPC services.
type ConnectOptions struct {
	RequiredPermissions []string
	UnitPermissions     map[string][]string
}

// ConnectOption is a function that configures ConnectOptions.
type ConnectOption func(*ConnectOptions)

// WithRequiredPermissions adds required permissions.
func WithRequiredPermissions(permissions ...string) ConnectOption {
	return func(opts *ConnectOptions) {
		opts.RequiredPermissions = append(opts.RequiredPermissions, permissions...)
	}
}

// WithUnitPermissions adds unit-specific permissions.
func WithUnitPermissions(unit string, permissions ...string) ConnectOption {
	return func(opts *ConnectOptions) {
		if opts.UnitPermissions == nil {
			opts.UnitPermissions = make(map[string][]string)
		}

		opts.UnitPermissions[unit] = append(opts.UnitPermissions[unit], permissions...)
	}
}

// NewConnectHandler creates a handler for Connect RPC with authentication.
// It automatically adds authentication and permission interceptors based on the options.
func NewConnectHandler(
	logger *slog.Logger,
	jwks *navigaid.JWKS,
	handler http.Handler,
	options ...ConnectOption,
) http.Handler {
	// Process options
	opts := &ConnectOptions{}
	for _, opt := range options {
		opt(opts)
	}

	// Log options for debugging
	logger.Debug("Creating Connect handler with authentication",
		"permissions", opts.RequiredPermissions,
		"unit_permissions", opts.UnitPermissions)

	// If the handler implements the ConnectHandlerWithInterceptor interface,
	// we can apply our interceptors
	if interceptorHandler, ok := handler.(ConnectHandlerWithInterceptor); ok {
		// Create interceptors
		var interceptorsList []connect.Interceptor

		// Add authentication interceptor
		interceptorsList = append(interceptorsList, navigaid.ConnectInterceptor(logger, jwks))

		// Add permission interceptors
		for _, permission := range opts.RequiredPermissions {
			interceptorsList = append(interceptorsList, navigaid.RequirePermission(logger, permission))
		}

		// Add unit permission interceptors
		for unit, permissions := range opts.UnitPermissions {
			for _, permission := range permissions {
				interceptorsList = append(interceptorsList, navigaid.RequireUnitPermission(logger, unit, permission))
			}
		}

		// Create a new handler with interceptors
		return interceptorHandler.WithInterceptors(interceptorsList...)
	}

	// If the handler doesn't implement the interface, log a warning
	logger.Warn("Handler does not implement ConnectHandlerWithInterceptor, interceptors not applied")

	return handler
}

// ConnectHandlerWithInterceptor is an interface for Connect handlers that support interceptors.
type ConnectHandlerWithInterceptor interface {
	WithInterceptors(...connect.Interceptor) http.Handler
}

// WithService registers an HTTP handler at the specified path.
// This is the only service registration method in Dindenault.
//
// The handler can be any http.Handler, including Connect RPC handlers
// created with servicev1connect.NewServiceHandler. Global interceptors
// configured with WithInterceptors will be automatically applied if the
// handler supports them.
//
// Example - Simple service registration:
//
//	path, handler := servicev1connect.NewServiceHandler(impl)
//	app := dindenault.New(logger,
//	    dindenault.WithService(path, handler),
//	)
//
// Example - Service with handler-specific interceptors:
//
//	path, handler := servicev1connect.NewServiceHandler(
//	    impl,
//	    connect.WithInterceptors(
//	        dindenault.AuthInterceptors(logger, imasURL),
//	        dindenault.PathInterceptors(logger, permissionConfigs),
//	    ),
//	)
//	app := dindenault.New(logger,
//	    dindenault.WithService(path, handler),
//	)
func WithService(path string, handler http.Handler) Option {
	return func(a *App) {
		a.registrations = append(a.registrations, Registration{
			Path:    path,
			Handler: handler,
		})
	}
}

// WithConnectRPC adds optional CORS support for Connect RPC services.
//
// Use this when your service needs to be accessed from web browsers.
// For internal services (backend-to-backend), simply omit this option.
//
// This automatically handles:
//  1. CORS headers for all Connect RPC responses via an interceptor
//  2. OPTIONS preflight requests for all registered Connect services
//  3. Origin validation against the allowed domains list
//  4. Connect-specific headers and methods
//
// If no domains are specified in the options, default domains will be used.
//
// Example - Web service with CORS:
//
//	app := dindenault.New(logger,
//	    dindenault.WithConnectRPC(cors.Options{
//	        AllowedDomains: []string{".mycompany.com"},
//	        AllowHTTP:      false, // Require HTTPS
//	    }),
//	    dindenault.WithService(path, handler),
//	)
//
// Example - Internal service without CORS:
//
//	app := dindenault.New(logger,
//	    dindenault.WithService(path, handler),
//	)
//
// Example - Development with default domains:
//
//	app := dindenault.New(logger,
//	    dindenault.WithConnectRPC(cors.Options{}), // Uses defaults
//	    dindenault.WithService(path, handler),
//	)
func WithConnectRPC(opts cors.Options) Option {
	return func(a *App) {
		// If no domains specified, use defaults
		if len(opts.AllowedDomains) == 0 {
			opts.AllowedDomains = cors.DefaultDomains()
		}

		// Add the CORS interceptor for all Connect services
		a.globalInterceptors = append(
			a.globalInterceptors,
			CORSInterceptors(opts.AllowedDomains, opts.AllowHTTP),
		)

		// Add a catch-all OPTIONS handler that works with Connect RPC
		a.registrations = append(a.registrations, Registration{
			Path: "/", // Catch all paths
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Only handle OPTIONS requests
				if r.Method != http.MethodOptions {
					// Let other handlers deal with non-OPTIONS requests
					w.WriteHeader(http.StatusNotFound)

					return
				}

				// Get origin from request
				origin := r.Header.Get("Origin")
				if origin == "" {
					w.WriteHeader(http.StatusBadRequest)

					return
				}

				// Use the standard validator for consistency
				originValidator := cors.StandardAllowOriginFunc(opts.AllowHTTP, opts.AllowedDomains)
				if !originValidator(origin) {
					w.WriteHeader(http.StatusForbidden)

					return
				}

				// Set CORS headers for Connect RPC preflight
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Connect-Protocol-Version, Authorization, X-Requested-With")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

				w.WriteHeader(http.StatusOK)
			}),
		})

		a.logger.Info("Connect RPC CORS support added globally",
			"allowed_domains", opts.AllowedDomains,
			"allow_http", opts.AllowHTTP)
	}
}

// applyGlobalInterceptors applies global interceptors to a Connect handler.
func (a *App) applyGlobalInterceptors(handler http.Handler) http.Handler {
	// If there are no global interceptors, return the handler as is
	if len(a.globalInterceptors) == 0 {
		return handler
	}

	// If the handler implements ConnectHandlerWithInterceptor, apply the interceptors
	if interceptorHandler, ok := handler.(ConnectHandlerWithInterceptor); ok {
		return interceptorHandler.WithInterceptors(a.globalInterceptors...)
	}

	// Otherwise, just return the original handler
	a.logger.Warn("Handler does not implement ConnectHandlerWithInterceptor, global interceptors not applied",
		"interceptors", len(a.globalInterceptors))

	return handler
}

// chainInterceptors chains multiple interceptors into a single interceptor.
// This is a replacement for connect.ChainInterceptors for older versions of the library.
//

// func chainInterceptors(interceptors ...connect.Interceptor) connect.Interceptor {
//	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
//		// Apply interceptors in reverse order
//		// Last interceptor is executed first, then the second-to-last, and so on
//		for i := len(interceptors) - 1; i >= 0; i-- {
//			next = interceptors[i].WrapUnary(next)
//		}
//
//		return next
//	})
//}
