// Package checkinterceptor provides a Connect interceptor that runs one
// access check on both unary and streaming handler calls.
//
// connect.UnaryInterceptorFunc only wraps unary calls: streaming handlers
// pass through it untouched, so an authentication or permission check built
// on it silently does not apply to server-, client- or bidi-streaming RPCs.
// The interceptor here applies the same check to streaming handlers.
package checkinterceptor

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
)

// CheckFunc decides whether a call may proceed. It gets the procedure
// (e.g. "/package.Service/Method") and the request headers, and returns the
// context to continue with, or an error (normally a *connect.Error) that
// rejects the call before the handler runs.
type CheckFunc func(ctx context.Context, procedure string, header http.Header) (context.Context, error)

// Interceptor applies a CheckFunc to unary calls and to streaming handlers.
type Interceptor struct {
	check CheckFunc
}

// New returns an Interceptor that runs check.
func New(check CheckFunc) *Interceptor {
	return &Interceptor{check: check}
}

// WrapUnary runs the check before a unary call.
func (i *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, err := i.check(ctx, req.Spec().Procedure, req.Header())
		if err != nil {
			return nil, err
		}

		return next(ctx, req)
	}
}

// WrapStreamingClient leaves outgoing streams unchanged: the check guards
// handlers, not clients.
func (i *Interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler runs the check before a streaming handler is called,
// using the headers the client sent when it opened the stream.
func (i *Interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, err := i.check(ctx, conn.Spec().Procedure, conn.RequestHeader())
		if err != nil {
			return err
		}

		return next(ctx, conn)
	}
}
