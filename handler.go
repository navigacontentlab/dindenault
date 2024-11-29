package didenault

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/navigacontentlab/dindenault/lambda"
)

// App handles Connect services in Lambda.
type App struct {
	registrations []Registration
	logger        *slog.Logger
	middlewares   []Middleware
}

// Registration represents a Connect service registration.
type Registration struct {
	Path    string
	Handler http.Handler
}

// Option is a function that configures the App.
type Option func(*App)

// Middleware is a function that wraps a http.Handler.
type Middleware func(http.Handler) http.Handler

// New creates a new App with the given options.
func New(logger *slog.Logger, opts ...Option) *App {
	app := &App{
		logger: logger,
	}

	for _, opt := range opts {
		opt(app)
	}

	return app
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

// WithMiddleware adds middleware to the app.
func WithMiddleware(m ...Middleware) Option {
	return func(a *App) {
		a.middlewares = append(a.middlewares, m...)
	}
}

// Handle returns a Lambda handler function.
func (a *App) Handle() func(context.Context, lambda.Request) (lambda.Response, error) {
	// Apply middlewares to all handlers
	for i, reg := range a.registrations {
		handler := reg.Handler
		// Apply middlewares in reverse order
		for j := len(a.middlewares) - 1; j >= 0; j-- {
			handler = a.middlewares[j](handler)
		}

		a.registrations[i].Handler = handler
	}

	return func(ctx context.Context, event lambda.Request) (lambda.Response, error) {
		req, err := lambda.AWSRequestToHTTPRequest(ctx, event)

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

		if err != nil {
			return lambda.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       "Failed to create request",
			}, nil
		}

		w := lambda.NewProxyResponseWriter()

		// Find and execute handler
		for _, reg := range a.registrations {
			if strings.HasPrefix(event.Path, reg.Path) {
				reg.Handler.ServeHTTP(w, req)

				return w.GetLambdaResponse()
			}
		}

		return lambda.Response{
			StatusCode: http.StatusNotFound,
			Body:       "Not found",
		}, nil
	}
}
