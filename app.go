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
//	app := dindenault.New(Logger,
//	    dindenault.WithSecureService(path, handler, []string{"service:access"}),
//	    dindenault.WithInterceptors(
//	        dindenault.LoggingInterceptors(Logger),
//	        dindenault.XRayInterceptors("my-service"),
//	        dindenault.AuthInterceptors(Logger, "https://imas.example.com"),
//	    ),
//	)
//
// Then start the Lambda handler:
//
//	lambda.Start(app.Handle())
package dindenault

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/aws/aws-lambda-go/events"
	"github.com/navigacontentlab/dindenault/internal/lambda"
	"github.com/navigacontentlab/dindenault/internal/telemetry"
)

type BucketEventHandler interface {
	HandleEvent(ctx context.Context, event *BucketEvent) error
}
type BucketEvent struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

// App handles Connect services in Lambda.
type App struct {
	Registrations       []Registration
	Logger              *slog.Logger
	globalInterceptors  []connect.Interceptor
	TelemetryOptions    *telemetry.Options
	BucketEventHandlers []BucketEventHandler
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
		Logger: logger,
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
	for i, reg := range a.Registrations {
		handler := reg.Handler

		// Apply the connect interceptors
		handler = a.applyGlobalInterceptors(handler)

		a.Registrations[i].Handler = handler
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

	a.Logger.Debug("GeneratedHTTPRequest", args...)

	w := lambda.NewProxyResponseWriter()

	// Find and execute handler
	for _, reg := range a.Registrations {
		a.Logger.Debug("Handle:", "reg.Path", reg.Path)

		if a.pathMatches(path, reg.Path) {
			reg.Handler.ServeHTTP(w, req)

			resp, err := w.GetLambdaResponse()
			if err != nil {
				a.Logger.Error("Failed to get lambda response", "error", err)

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

func (a *App) HandleMultiple() func(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	slog.Default().Debug("HandleMultiple")

	albHandler := a.HandleALB()
	apigatewayHandler := a.HandleAPIGateway()

	return func(ctx context.Context, raw json.RawMessage) (interface{}, error) {
		// Try ALB
		slog.Default().Debug("HandleMultiple: Try ALB")

		var albEvent events.ALBTargetGroupRequest
		if err := json.Unmarshal(raw, &albEvent); err == nil && albEvent.HTTPMethod != "" {
			return albHandler(ctx, albEvent)
		}

		// Try S3
		slog.Default().Debug("HandleMultiple: Try S3")

		var s3Event events.S3Event
		if err := json.Unmarshal(raw, &s3Event); err == nil && len(s3Event.Records) > 0 {
			return a.HandleS3()(ctx, s3Event)
		}

		// Try API Gateway
		var apigatewayEvent events.APIGatewayV2HTTPRequest
		if err := json.Unmarshal(raw, &apigatewayEvent); err == nil {
			return apigatewayHandler(ctx, apigatewayEvent)
		}

		// Unsupported
		return events.ALBTargetGroupResponse{
			StatusCode: http.StatusBadRequest,
			Body:       "Unsupported event type",
		}, nil
	}
}

// HandleALB returns a Lambda handler function for ALB events.
func (a *App) HandleALB() func(context.Context, events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
	a.prepareHandlers()

	return func(ctx context.Context, event events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
		// Convert to our internal request type
		request := lambda.FromALBRequest(event)

		req, err := lambda.AWSRequestToHTTPRequest(ctx, request)
		if err != nil {
			a.Logger.Error("Failed to create HTTP request", "error", err)

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

func (a *App) HandleS3() func(_ context.Context, event events.S3Event) (interface{}, error) {
	return func(ctx context.Context, event events.S3Event) (interface{}, error) {
		for _, record := range event.Records {
			bucketEvent := &BucketEvent{
				Bucket: record.S3.Bucket.Name,
				Key:    record.S3.Object.Key,
			}

			for _, handler := range a.BucketEventHandlers {
				if err := handler.HandleEvent(ctx, bucketEvent); err != nil {
					a.Logger.Error("Error handling bucket event",
						"error", err,
						"bucket", bucketEvent.Bucket,
						"key", bucketEvent.Key)
				}
			}
		}

		return nil, nil
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
			a.Logger.Error("Failed to create HTTP request", "error", err)

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
