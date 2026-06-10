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
// IMPORTANT: app-level interceptors can only be applied to handlers
// that implement ConnectHandlerWithInterceptor. Standard connect-go
// generated handlers do NOT implement it — for those, pass the
// interceptors at handler creation instead:
//
//	path, handler := servicev1connect.NewServiceHandler(
//	    impl,
//	    connect.WithInterceptors(
//	        dindenault.LoggingInterceptors(logger),
//	        dindenault.AuthInterceptors(logger, "https://imas.example.com"),
//	    ),
//	)
//
// If WithInterceptors is configured and a registered handler cannot
// receive the interceptors, the app panics at startup instead of
// silently running the handler without them (which could mean running
// without authentication). Handlers that intentionally should not
// receive interceptors can be registered with WithPlainService.
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
//
// The provider's interceptor is applied to registered handlers that
// implement ConnectHandlerWithInterceptor. Telemetry is best-effort:
// for handlers that cannot receive interceptors a warning is logged
// (unlike WithInterceptors, which fails fast, since missing telemetry
// is not a security risk). Handlers registered with WithPlainService,
// WithMCP, or WithMCPAuth are skipped.
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
// Deprecated: use WithConnectRPC, which applies CORS at the HTTP level
// for all registered handlers and also answers preflight requests, or
// cors.Middleware directly for a single handler. This interceptor does
// not handle preflight requests and will be removed in a future major
// version.
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
//
// Deprecated: pass interceptors at handler creation with
// connect.WithInterceptors instead (see AuthInterceptors,
// PathInterceptors, navigaid.RequirePermission and
// navigaid.RequireUnitPermission). Will be removed in a future major
// version.
type ConnectOptions struct {
	RequiredPermissions []string
	UnitPermissions     map[string][]string
}

// ConnectOption is a function that configures ConnectOptions.
//
// Deprecated: see ConnectOptions.
type ConnectOption func(*ConnectOptions)

// WithRequiredPermissions adds required permissions.
//
// Deprecated: see ConnectOptions.
func WithRequiredPermissions(permissions ...string) ConnectOption {
	return func(opts *ConnectOptions) {
		opts.RequiredPermissions = append(opts.RequiredPermissions, permissions...)
	}
}

// WithUnitPermissions adds unit-specific permissions.
//
// Deprecated: see ConnectOptions.
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
//
// Deprecated: this only works for handlers that implement
// ConnectHandlerWithInterceptor, which connect-go generated handlers do
// not. Pass interceptors at handler creation with
// connect.WithInterceptors instead. Will be removed in a future major
// version.
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

// WithPlainService registers a plain HTTP handler at the specified path,
// explicitly opting out of app-level interceptors configured with
// WithInterceptors. Use this for non-Connect handlers (health checks,
// webhooks, and similar) registered on an App that uses WithInterceptors.
//
// Note that opting out also opts out of any authentication configured
// via app-level interceptors — the handler is responsible for its own
// access control.
func WithPlainService(path string, handler http.Handler) Option {
	return func(a *App) {
		a.registrations = append(a.registrations, Registration{
			Path:                   path,
			Handler:                handler,
			SkipGlobalInterceptors: true,
		})
	}
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

// WithConnectRPC adds optional CORS support for all registered services.
//
// Use this when your service needs to be accessed from web browsers.
// For internal services (backend-to-backend), simply omit this option.
//
// CORS is applied as an HTTP-level middleware around every registered
// handler, which automatically handles:
//  1. CORS headers on all responses (including error responses)
//  2. OPTIONS preflight requests
//  3. Origin validation against the allowed domains list
//  4. Connect-specific headers and methods
//
// If no domains are specified in the options, default domains will be used.
//
// Note: when AllowedDomains contains "*", Access-Control-Allow-Credentials
// is not set — reflecting arbitrary origins with credentials enabled
// would disable the browser's cross-origin protections entirely.
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

		a.corsOptions = &opts

		a.logger.Info("Connect RPC CORS support added globally",
			"allowed_domains", opts.AllowedDomains,
			"allow_http", opts.AllowHTTP)
	}
}
