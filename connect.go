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
