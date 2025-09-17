// Package dindenault provides a framework for building Connect RPC services
// for AWS Lambda. It offers:
//
// # Features
//
//   - Service registration for Connect RPC handlers
//   - Core logging interceptors
//   - Integration with Connect's native compression
//   - Modular architecture with separate packages for auth, CORS, X-Ray, etc.
//
// # Architecture
//
// The core abstraction is the App, which is configured with options and
// manages service registration and request routing. All advanced features
// like authentication, CORS, X-Ray tracing, and telemetry are available
// as separate modules that you import as needed.
//
// # Usage
//
// Create a new App with options:
//
//	// Import the modules you need
//	import (
//		"github.com/navigacontentlab/dindenault"
//		"github.com/navigacontentlab/dindenault/navigaid"
//		"github.com/navigacontentlab/dindenault/telemetry"
//		"github.com/navigacontentlab/dindenault/cors"
//	)
//
//	// Create your service implementation
//	impl := service.NewServiceImpl()
//
//	// Create Connect handler with compression and other options
//	path, handler := servicev1connect.NewServiceHandler(
//	    impl,
//	    connect.WithCompressMinBytes(1024), // Enable compression
//	)
//
//	// Initialize OpenTelemetry (do this early)
//	shutdown, err := telemetry.Initialize(ctx, "my-service", &telemetry.Options{
//	    MetricNamespace: "my-service",
//	    OrganizationFn:  telemetry.DefaultOrganizationFunction(),
//	})
//	if err != nil {
//	    return err
//	}
//	defer shutdown(ctx)
//
//	// Create JWKS for authentication
//	jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint("https://imas.example.com"))
//
//	// Create app with the handler and global interceptors
//	app := dindenault.New(logger,
//	    dindenault.WithInterceptors(
//	        dindenault.LoggingInterceptors(logger),
//	        telemetry.Interceptor(logger, &telemetry.Options{
//	            OrganizationFn: telemetry.DefaultOrganizationFunction(),
//	        }),
//	        navigaid.ConnectInterceptor(logger, jwks),
//	        cors.Interceptor([]string{"https://app.example.com"}, false),
//	    ),
//	    dindenault.WithService(path, handler),
//	)
//
// Then start the Lambda handler:
//
//	// For ALB events:
//	lambda.Start(telemetry.InstrumentHandler(app.Handle()))
//
//	// For API Gateway events:
//	lambda.Start(telemetry.InstrumentHandler(app.HandleAPIGateway()))
//
// Note: You'll need to import "github.com/aws/aws-lambda-go/lambda" separately
// in your main function to use lambda.Start()
package dindenault

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/navigacontentlab/dindenault/internal/lambda"
	"github.com/navigacontentlab/dindenault/types"
)

// App handles Connect services in Lambda.
type App struct {
	registrations      []Registration
	globalInterceptors []connect.Interceptor
}

// GlobalInterceptors returns the list of global interceptors for testing.
func (a *App) GlobalInterceptors() []connect.Interceptor {
	return a.globalInterceptors
}

// Registration represents a Connect service registration.
type Registration struct {
	Path    string
	Handler http.Handler
}

// New creates a new App with the given options.
func New(options ...Option) *App {
	app := &App{}

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

// prepareHandlers applies interceptors to all handlers.
func (a *App) prepareHandlers() {
	for i, reg := range a.registrations {
		handler := reg.Handler

		// Apply Connect interceptors if the handler supports it
		if connectHandler, ok := handler.(interface {
			WithInterceptors(interceptors ...connect.Interceptor) http.Handler
		}); ok && len(a.globalInterceptors) > 0 {
			handler = connectHandler.WithInterceptors(a.globalInterceptors...)
		}

		a.registrations[i].Handler = handler
	}
}

// processRequest handles an HTTP request and returns the result.
// The context is currently unused but may be needed for future extensions.
func (a *App) processRequest(_ context.Context, req *http.Request, path string) (*lambda.Response, error) {
	var attr []slog.Attr
	attr = append(attr, slog.String("Method", req.Method))
	attr = append(attr, slog.String("host", req.Host))
	attr = append(attr, slog.String("URI", req.RequestURI))
	attr = append(attr, slog.Any("Headers", req.Header))

	args := make([]any, 0, len(attr)*2)
	for _, a := range attr {
		args = append(args, a.Key, a.Value.Any())
	}

	slog.Debug("GeneratedHTTPRequest", args...)

	w := lambda.NewProxyResponseWriter()

	// Sort handlers by path specificity (longer paths first)
	// This ensures more specific handlers are tried before catch-all handlers
	sortedRegistrations := make([]Registration, len(a.registrations))
	copy(sortedRegistrations, a.registrations)

	// Sort by path length (descending) to prioritize more specific paths
	for i := 0; i < len(sortedRegistrations)-1; i++ {
		for j := i + 1; j < len(sortedRegistrations); j++ {
			if len(sortedRegistrations[i].Path) < len(sortedRegistrations[j].Path) {
				sortedRegistrations[i], sortedRegistrations[j] = sortedRegistrations[j], sortedRegistrations[i]
			}
		}
	}

	// Find and execute handler
	for _, reg := range sortedRegistrations {
		slog.Debug("Handle:", "reg.Path", reg.Path)

		if a.pathMatches(path, reg.Path) {
			reg.Handler.ServeHTTP(w, req)

			resp, err := w.GetLambdaResponse()
			if err != nil {
				slog.Error("Failed to get lambda response", "error", err)

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

// Handle returns a Lambda handler function for ALB events.
func (a *App) Handle() func(context.Context, types.ALBTargetGroupRequest) (types.ALBTargetGroupResponse, error) {
	a.prepareHandlers()

	return func(ctx context.Context, event types.ALBTargetGroupRequest) (types.ALBTargetGroupResponse, error) {
		// Convert to our internal request type
		request := lambda.FromALBRequest(event)

		req, err := lambda.AWSRequestToHTTPRequest(ctx, request)
		if err != nil {
			slog.Error("Failed to create HTTP request", "error", err)

			return types.ALBTargetGroupResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       "Failed to create request: " + err.Error(),
			}, nil
		}

		resp, err := a.processRequest(ctx, req, request.Path)
		if err != nil {
			return types.ALBTargetGroupResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       "Internal server error: " + err.Error(),
			}, nil
		}

		// Convert to ALB response
		return types.ALBTargetGroupResponse{
			StatusCode:        resp.StatusCode,
			Headers:           resp.Headers,
			MultiValueHeaders: resp.MultiValueHeaders,
			Body:              resp.Body,
			IsBase64Encoded:   resp.IsBase64Encoded,
		}, nil
	}
}

// HandleAPIGateway returns a Lambda handler function for API Gateway events.
func (a *App) HandleAPIGateway() func(context.Context, types.APIGatewayV2HTTPRequest) (types.APIGatewayV2HTTPResponse, error) {
	a.prepareHandlers()

	return func(ctx context.Context, event types.APIGatewayV2HTTPRequest) (types.APIGatewayV2HTTPResponse, error) {
		// Convert to our internal request type
		request := lambda.FromAPIGatewayRequest(event)

		req, err := lambda.AWSRequestToHTTPRequest(ctx, request)
		if err != nil {
			slog.Error("Failed to create HTTP request", "error", err)

			return types.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       "Failed to create request: " + err.Error(),
			}, nil
		}

		resp, err := a.processRequest(ctx, req, request.Path)
		if err != nil {
			return types.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       "Internal server error: " + err.Error(),
			}, nil
		}

		// Convert to API Gateway response
		return types.APIGatewayV2HTTPResponse{
			StatusCode:        resp.StatusCode,
			Headers:           resp.Headers,
			MultiValueHeaders: resp.MultiValueHeaders,
			Body:              resp.Body,
			IsBase64Encoded:   resp.IsBase64Encoded,
			Cookies:           resp.Cookies,
		}, nil
	}
}
