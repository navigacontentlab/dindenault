package dindenault

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/aws/aws-sdk-go/aws/session"
	"go.opentelemetry.io/otel/attribute"

	"github.com/navigacontentlab/dindenault/internal/cors"
	"github.com/navigacontentlab/dindenault/internal/interceptors"
	"github.com/navigacontentlab/dindenault/internal/telemetry"
	"github.com/navigacontentlab/dindenault/navigaid"
)

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

// XRayInterceptors returns X-Ray tracing interceptors for Connect RPC.
// This creates interceptors that add AWS X-Ray tracing to Connect RPC calls.
//
//nolint:ireturn // Returning interface as intended by connect.Interceptor design
func XRayInterceptors(name string) connect.Interceptor {
	return interceptors.XRay(name)
}

// OpenTelemetryInterceptors returns OpenTelemetry tracing interceptors for Connect RPC.
// This creates interceptors that add OpenTelemetry tracing to Connect RPC calls.
//
//nolint:ireturn // Returning interface as intended by connect.Interceptor design
func OpenTelemetryInterceptors(name string) connect.Interceptor {
	return interceptors.OpenTelemetry(name)
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

// WithConnectService adds a Connect service with authentication.
func WithConnectService(
	path string,
	handler http.Handler,
) Option {
	return WithService(path, handler)
}

func WithService(path string, handler http.Handler) Option {
	return func(a *App) {
		a.Registrations = append(a.Registrations, Registration{
			Path:    path,
			Handler: handler,
		})
	}
}

// WithSecureService adds a Connect RPC service with permissions.
// If permissions are specified, it adds permission checks using interceptors.
//
// Parameters:
// - path: The URL path prefix where the service will be registered
// - handler: The HTTP handler for the Connect service
// - permissions: Optional slice of permission strings (can be nil or empty)
//
// Example:
//
//	// Basic service without permission requirements
//	app := dindenault.New(Logger,
//	    dindenault.WithSecureService("hello/", helloHandler, nil),
//	)
//
//	// Service with permission requirements
//	app := dindenault.New(Logger,
//	    dindenault.WithSecureService("secure/", secureHandler, []string{"service:access"}),
//	)
func WithSecureService(path string, handler http.Handler, permissions []string) Option {
	return func(a *App) {
		// Start with original handler
		serviceHandler := handler

		// Add permission requirements if:
		// 1. Permissions are specified (non-nil and non-empty)
		// 2. Handler supports interceptors
		if len(permissions) > 0 {
			if interceptorHandler, ok := handler.(ConnectHandlerWithInterceptor); ok {
				// Create interceptors with permissions
				var permInterceptors []connect.Interceptor

				for _, permission := range permissions {
					permInterceptors = append(
						permInterceptors,
						navigaid.RequirePermission(a.Logger, permission),
					)
				}

				// Apply interceptors
				serviceHandler = interceptorHandler.WithInterceptors(permInterceptors...)

				a.Logger.Info("Added permission requirements to service",
					"path", path,
					"permissions", permissions)
			} else {
				a.Logger.Warn("Handler does not support interceptors, permissions will be ignored",
					"path", path,
					"permissions", permissions)
			}
		}

		// Register the service
		WithService(path, serviceHandler)(a)

		a.Logger.Info("Registered service", "path", path)
	}
}

// WithCORSInterceptor adds complete CORS support to the app with custom options.
// This provides CORS headers for Connect responses and handles OPTIONS preflight requests.
func WithCORSInterceptor(path string, opts cors.Options) Option {
	return func(a *App) {
		// If no domains specified, use defaults
		if len(opts.AllowedDomains) == 0 {
			opts.AllowedDomains = cors.DefaultDomains()
		}

		// Add the CORS interceptor
		a.globalInterceptors = append(
			a.globalInterceptors,
			CORSInterceptors(opts.AllowedDomains, opts.AllowHTTP),
		)

		// Register preflight handler
		a.Registrations = append(a.Registrations, Registration{
			Path:    path,
			Handler: HandleCORSPreflight(opts.AllowedDomains),
		})

		a.Logger.Info("CORS support added",
			"path", path,
			"allowed_domains", opts.AllowedDomains,
			"allow_http", opts.AllowHTTP)
	}
}

// HandleCORSPreflight creates an http.Handler that responds to CORS preflight requests.
// This should be used in combination with CORSInterceptors to provide complete CORS support.
func HandleCORSPreflight(allowedOrigins []string) http.Handler {
	return HandleCORSPreflightWithOptions(cors.Options{
		AllowedDomains: allowedOrigins,
		AllowHTTP:      false, // Default to HTTPS only for security
	})
}

// HandleCORSPreflightWithOptions creates an http.Handler that responds to CORS preflight requests.
// This provides more control over which origins are allowed.
func HandleCORSPreflightWithOptions(opts cors.Options) http.Handler {
	// If no domains specified, use defaults
	if len(opts.AllowedDomains) == 0 {
		opts.AllowedDomains = cors.DefaultDomains()
	}

	// Use the standardAllowOriginFunc from cors.go for consistency
	originValidator := cors.StandardAllowOriginFunc(opts.AllowHTTP, opts.AllowedDomains)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only handle OPTIONS requests
		if r.Method != http.MethodOptions {
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		// Get origin from request
		origin := r.Header.Get("Origin")
		if origin == "" {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		// Check if the origin is allowed using the standard validator
		originAllowed := originValidator(origin)

		// If origin is not allowed, return 403 Forbidden
		if !originAllowed {
			w.WriteHeader(http.StatusForbidden)

			return
		}

		// Set CORS headers for preflight
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Connect-Protocol-Version, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

		// Respond with 200 OK
		w.WriteHeader(http.StatusOK)
	})
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
	a.Logger.Warn("Handler does not implement ConnectHandlerWithInterceptor, global interceptors not applied",
		"interceptors", len(a.globalInterceptors))

	return handler
}

// WithTelemetry adds OpenTelemetry and CloudWatch metrics.
func WithTelemetry(logger *slog.Logger) Option {
	return func(a *App) {
		// Create default options if none exist
		if a.TelemetryOptions == nil {
			a.TelemetryOptions = &telemetry.Options{
				MetricNamespace: "Dindenault",
				OrganizationFn:  telemetry.DefaultOrganizationFunction,
			}
		}

		// Create a telemetry interceptor for Connect
		telemetryInterceptor := telemetry.Interceptor(logger, a.TelemetryOptions)

		// Add the interceptor to global interceptors
		a.globalInterceptors = append(a.globalInterceptors, telemetryInterceptor)
	}
}

// WithTelemetryNamespace sets the CloudWatch namespace for metrics.
func WithTelemetryNamespace(namespace string) Option {
	return func(a *App) {
		if a.TelemetryOptions == nil {
			a.TelemetryOptions = &telemetry.Options{}
		}

		a.TelemetryOptions.MetricNamespace = namespace
	}
}

// WithTelemetryOrganizationFunction sets a custom function to extract organization from context.
func WithTelemetryOrganizationFunction(fn func(ctx context.Context) string) Option {
	return func(a *App) {
		if a.TelemetryOptions == nil {
			a.TelemetryOptions = &telemetry.Options{}
		}

		a.TelemetryOptions.OrganizationFn = fn
	}
}

// WithTelemetryAWSSession sets the AWS session for CloudWatch metrics.
func WithTelemetryAWSSession(sess *session.Session) Option {
	return func(a *App) {
		if a.TelemetryOptions == nil {
			a.TelemetryOptions = &telemetry.Options{}
		}

		a.TelemetryOptions.AWSSession = sess
	}
}

// WithTelemetryAttributes adds custom attributes to all metrics.
func WithTelemetryAttributes(attrs ...attribute.KeyValue) Option {
	return func(a *App) {
		if a.TelemetryOptions == nil {
			a.TelemetryOptions = &telemetry.Options{}
		}

		a.TelemetryOptions.MetricAttributes = append(a.TelemetryOptions.MetricAttributes, attrs...)
	}
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
