// Package dindenault provides a framework for building Connect RPC services
// for AWS Lambda with a clean, intuitive API.
//
// # Features
//
//   - Single service registration method (WithService)
//   - Optional CORS configuration (WithConnectRPC)
//   - Authentication with Naviga ID
//   - Method-level permissions with PathInterceptors
//   - Telemetry (logging, tracing, metrics)
//   - Integration with Connect's native compression
//
// # Core API
//
// Dindenault provides a simplified API with clear, single-purpose functions:
//
//   - WithService: Register any HTTP handler (the only service registration method)
//   - WithConnectRPC: Add optional CORS support for web clients
//   - PathInterceptors: Apply method-level permissions at handler creation
//   - AuthInterceptors: Add authentication to handlers
//   - WithInterceptors: Add global interceptors for cross-cutting concerns
//
// # Architecture
//
// The core abstraction is the App, which is configured with options and
// manages service registration and request routing. Connect interceptors
// are used for cross-cutting concerns like authentication, logging,
// tracing, and CORS.
//
// # Basic Usage
//
// Create a simple internal service:
//
//	impl := service.NewServiceImpl()
//	path, handler := servicev1connect.NewServiceHandler(impl)
//
//	app := dindenault.New(logger,
//	    dindenault.WithService(path, handler),
//	)
//
//	lambda.Start(app.Handle())
//
// # Web Service with CORS
//
// Add CORS for web-facing services:
//
//	app := dindenault.New(logger,
//	    dindenault.WithConnectRPC(cors.Options{
//	        AllowedDomains: []string{".mycompany.com"},
//	    }),
//	    dindenault.WithService(path, handler),
//	)
//
// # Service with Authentication and Permissions
//
// Apply authentication and method-level permissions at handler creation.
// This is the recommended way to apply interceptors — it works with any
// connect-go generated handler:
//
//	permissionConfigs := []dindenault.PathPermissionConfig{
//	    {PathPrefix: "/service.v1.Service/SecureMethod", Permissions: []string{"service:access"}},
//	}
//
//	path, handler := servicev1connect.NewServiceHandler(
//	    impl,
//	    connect.WithCompressMinBytes(1024), // Enable compression
//	    connect.WithInterceptors(
//	        dindenault.LoggingInterceptors(logger),
//	        dindenault.AuthInterceptors(logger, "https://imas.example.com"),
//	        dindenault.PathInterceptors(logger, permissionConfigs),
//	    ),
//	)
//
//	app := dindenault.New(logger,
//	    dindenault.WithService(path, handler),
//	)
//
//	lambda.Start(app.Handle())
//
// Note: WithInterceptors (app-level interceptors) only works with handlers
// that implement ConnectHandlerWithInterceptor. Standard connect-go
// generated handlers do NOT — the app will refuse to start (panic) rather
// than silently run without the configured interceptors.
//
// # Migration from Deprecated Functions
//
// If you're migrating from older versions:
//
//   - Replace WithConnectService with WithService (identical functionality)
//   - Replace WithSecureService with PathInterceptors at handler creation
//   - Replace WithConnectRPCCORS with WithConnectRPC (renamed for clarity)
//
// See the README.md for detailed examples and best practices.
package dindenault

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"github.com/aws/aws-lambda-go/events"

	"github.com/navigacontentlab/dindenault/cors"
	"github.com/navigacontentlab/dindenault/internal/lambda"
)

// App handles Connect services in Lambda.
type App struct {
	registrations      []Registration
	logger             *slog.Logger
	globalInterceptors []connect.Interceptor
	telemetryProvider  TelemetryProvider
	telemetryOptions   TelemetryOptions
	corsOptions        *cors.Options
	prepareOnce        sync.Once
}

// GlobalInterceptors returns the list of global interceptors for testing.
func (a *App) GlobalInterceptors() []connect.Interceptor {
	return a.globalInterceptors
}

// Registrations returns the list of service registrations for testing.
func (a *App) Registrations() []Registration {
	return a.registrations
}

// CORSOptions returns the configured CORS options for testing.
// Returns nil when CORS is not configured.
func (a *App) CORSOptions() *cors.Options {
	return a.corsOptions
}

// Registration represents a Connect service registration.
type Registration struct {
	Path    string
	Handler http.Handler

	// SkipGlobalInterceptors marks the handler as a plain HTTP handler
	// that intentionally does not receive app-level Connect
	// interceptors (e.g. MCP endpoints). See WithPlainService.
	SkipGlobalInterceptors bool
}

// New creates a new App with the given options.
func New(logger *slog.Logger, options ...Option) *App {
	app := &App{
		logger: logger,
	}

	// Apply options
	for _, opt := range options {
		opt(app)
	}

	return app
}

// pathMatches checks if a request path matches a registered service path.
func (a *App) pathMatches(requestPath, servicePath string) bool {
	// Case-insensitive path prefix matching
	return strings.HasPrefix(
		strings.ToLower(requestPath),
		strings.ToLower(servicePath),
	)
}

// prepareHandlers applies interceptors and CORS to all handlers and
// sorts registrations by path specificity. It runs exactly once, so
// calling Handle and/or HandleAPIGateway multiple times is safe.
//
// If app-level interceptors are configured and a handler cannot
// receive them, prepareHandlers panics rather than silently running
// the handler without (for example) authentication. Apply interceptors
// at handler creation with connect.WithInterceptors, or register the
// handler with WithPlainService if it should not receive them.
func (a *App) prepareHandlers() {
	a.prepareOnce.Do(func() {
		// Sort by path length (descending) so that more specific
		// handlers are matched before catch-all handlers.
		sort.SliceStable(a.registrations, func(i, j int) bool {
			return len(a.registrations[i].Path) > len(a.registrations[j].Path)
		})

		// Resolve the telemetry interceptor once. Telemetry is
		// best-effort: unlike auth interceptors, a missing telemetry
		// interceptor is logged rather than treated as fatal.
		var telemetryInterceptor connect.Interceptor
		if a.telemetryProvider != nil {
			telemetryInterceptor = a.telemetryProvider.Interceptor(a.logger, a.telemetryOptions)
		}

		for i, reg := range a.registrations {
			handler := reg.Handler

			// Apply Connect interceptors
			if len(a.globalInterceptors) > 0 && !reg.SkipGlobalInterceptors {
				interceptorHandler, ok := handler.(ConnectHandlerWithInterceptor)
				if !ok {
					panic(fmt.Sprintf(
						"dindenault: handler registered at %q does not implement "+
							"ConnectHandlerWithInterceptor, so app-level interceptors "+
							"(WithInterceptors) cannot be applied. Refusing to start "+
							"without them. Apply interceptors at handler creation with "+
							"connect.WithInterceptors, or use WithPlainService for "+
							"handlers that should not receive them.",
						reg.Path,
					))
				}

				handler = interceptorHandler.WithInterceptors(a.globalInterceptors...)
			}

			// Apply the telemetry interceptor configured via WithTelemetry.
			if telemetryInterceptor != nil && !reg.SkipGlobalInterceptors {
				if interceptorHandler, ok := handler.(ConnectHandlerWithInterceptor); ok {
					handler = interceptorHandler.WithInterceptors(telemetryInterceptor)
				} else {
					a.logger.Warn("telemetry interceptor not applied; "+
						"handler does not implement ConnectHandlerWithInterceptor",
						"path", reg.Path)
				}
			}

			// Apply CORS at the HTTP level so it works for every handler,
			// including preflight requests.
			if a.corsOptions != nil {
				handler = cors.Middleware(*a.corsOptions, handler)
			}

			a.registrations[i].Handler = handler
		}
	})
}

// processRequest handles an HTTP request and returns the result.
// The context is currently unused but may be needed for future extensions.
func (a *App) processRequest(_ context.Context, req *http.Request, path string) (*lambda.Response, error) {
	a.logger.Debug("GeneratedHTTPRequest",
		"Method", req.Method,
		"host", req.Host,
		"URI", req.RequestURI,
		"Headers", redactHeaders(req.Header),
	)

	w := lambda.NewProxyResponseWriter()

	// Registrations are sorted by path specificity in prepareHandlers.
	// Find and execute handler.
	for _, reg := range a.registrations {
		a.logger.Debug("Handle:", "reg.Path", reg.Path)

		if a.pathMatches(path, reg.Path) {
			reg.Handler.ServeHTTP(w, req)

			resp, err := w.GetLambdaResponse()
			if err != nil {
				a.logger.Error("Failed to get lambda response", "error", err)

				return nil, fmt.Errorf("failed to get lambda response: %w", err)
			}

			return &resp, nil
		}
	}

	return &lambda.Response{
		StatusCode: http.StatusNotFound,
		Body:       "Not found",
	}, nil
}

// internalServerErrorBody is the generic body returned for unexpected
// failures. Internal error details are logged, never returned to clients.
const internalServerErrorBody = "Internal server error"

// sensitiveHeaders are request headers whose values must never be logged.
var sensitiveHeaders = []string{"Authorization", "X-Imid-Token", "Cookie", "Proxy-Authorization"}

// redactHeaders returns a copy of headers with credential-bearing
// values masked, so debug logging cannot leak tokens.
func redactHeaders(h http.Header) http.Header {
	redacted := h.Clone()

	for _, name := range sensitiveHeaders {
		if redacted.Get(name) != "" {
			redacted.Set(name, "[REDACTED]")
		}
	}

	return redacted
}

// Handle returns a Lambda handler function for ALB events.
func (a *App) Handle() func(context.Context, events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
	a.prepareHandlers()

	return func(ctx context.Context, event events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
		// Convert to our internal request type
		request := lambda.FromALBRequest(event)

		req, err := lambda.AWSRequestToHTTPRequest(ctx, request)
		if err != nil {
			a.logger.Error("Failed to create HTTP request", "error", err)

			return events.ALBTargetGroupResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       internalServerErrorBody,
			}, nil
		}

		resp, err := a.processRequest(ctx, req, request.Path)
		if err != nil {
			a.logger.Error("Failed to process request", "error", err)

			return events.ALBTargetGroupResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       internalServerErrorBody,
			}, nil
		}

		// Convert to ALB response
		return events.ALBTargetGroupResponse{
			StatusCode:        resp.StatusCode,
			Headers:           resp.Headers,
			MultiValueHeaders: resp.MultiValueHeaders,
			Body:              resp.Body,
			IsBase64Encoded:   resp.IsBase64Encoded,
		}, nil
	}
}

// HandleAPIGateway returns a Lambda handler function for API Gateway events.
func (a *App) HandleAPIGateway() func(context.Context, events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	a.prepareHandlers()

	return func(ctx context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		// Convert to our internal request type
		request := lambda.FromAPIGatewayRequest(event)

		req, err := lambda.AWSRequestToHTTPRequest(ctx, request)
		if err != nil {
			a.logger.Error("Failed to create HTTP request", "error", err)

			return events.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       internalServerErrorBody,
			}, nil
		}

		resp, err := a.processRequest(ctx, req, request.Path)
		if err != nil {
			a.logger.Error("Failed to process request", "error", err)

			return events.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       internalServerErrorBody,
			}, nil
		}

		// Convert to API Gateway response
		return events.APIGatewayV2HTTPResponse{
			StatusCode:        resp.StatusCode,
			Headers:           resp.Headers,
			MultiValueHeaders: resp.MultiValueHeaders,
			Body:              resp.Body,
			IsBase64Encoded:   resp.IsBase64Encoded,
			Cookies:           resp.Cookies,
		}, nil
	}
}
