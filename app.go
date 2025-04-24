// Package dindenault provides a framework for building Connect RPC services
// for AWS Lambda. It offers:
//
// # Features
//
//   - Service registration for Connect RPC handlers
//   - Authentication with Naviga ID
//   - Permission checking
//   - Telemetry (logging, tracing, metrics)
//   - CORS support
//   - Integration with Connect's native compression
//
// # Architecture
//
// The core abstraction is the App, which is configured with options and
// manages service registration and request routing. Connect interceptors
// are used for cross-cutting concerns like authentication, logging,
// tracing, and CORS.
//
// # Usage
//
// Create a new App with options:
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
//	// Create app with the handler and global interceptors
//	app := dindenault.New(logger,
//	    dindenault.WithSecureService(path, handler, []string{"service:access"}),
//	    dindenault.WithInterceptors(
//	        dindenault.LoggingInterceptors(logger),
//	        dindenault.XRayInterceptors("my-service"),
//	        dindenault.AuthInterceptors("https://imas.example.com", []string{}),
//	    ),
//	)
//
// Then start the Lambda handler:
//
//	lambda.Start(app.Handle())
package dindenault

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/aws/aws-lambda-go/events"
	"github.com/navigacontentlab/dindenault/internal/lambda"
	"github.com/navigacontentlab/dindenault/internal/telemetry"
)

// App handles Connect services in Lambda.
type App struct {
	registrations      []Registration
	logger             *slog.Logger
	globalInterceptors []connect.Interceptor
	telemetryOptions   *telemetry.Options
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

// prepareHandlers applies interceptors to all handlers.
func (a *App) prepareHandlers() {
	for i, reg := range a.registrations {
		handler := reg.Handler

		// Apply Connect interceptors
		handler = a.applyGlobalInterceptors(handler)

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

	a.logger.Debug("GeneratedHTTPRequest", args...)

	w := lambda.NewProxyResponseWriter()

	// Find and execute handler
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

// Handle returns a Lambda handler function for ALB events.
// For backwards compatibility.
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
				Body:       "Failed to create request: " + err.Error(),
			}, nil
		}

		resp, err := a.processRequest(ctx, req, request.Path)
		if err != nil {
			return events.ALBTargetGroupResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       "Internal server error: " + err.Error(),
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
				Body:       "Failed to create request: " + err.Error(),
			}, nil
		}

		resp, err := a.processRequest(ctx, req, request.Path)
		if err != nil {
			return events.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       "Internal server error: " + err.Error(),
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
