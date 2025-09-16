// Package cors provides CORS interceptors for Connect RPC services.
package cors

import (
	"context"
	"errors"

	"connectrpc.com/connect"
)

// Interceptor creates a Connect interceptor that handles Cross-Origin Resource Sharing (CORS).
// Unlike other interceptors, CORS works at the HTTP header level, so this interceptor
// adds appropriate CORS headers to the response headers.
//
//nolint:ireturn
func Interceptor(allowedOrigins []string, allowHTTP bool) connect.Interceptor {
	// Use the standardAllowOriginFunc from cors.go for consistency
	originValidator := StandardAllowOriginFunc(allowHTTP, allowedOrigins)

	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Get origin from request
			origin := req.Header().Get("Origin")
			if origin == "" {
				// No origin, no CORS headers needed
				return next(ctx, req)
			}

			// Check if the origin is allowed using the standard validator
			originAllowed := originValidator(origin)

			// If origin is not allowed, continue without CORS headers
			if !originAllowed {
				return next(ctx, req)
			}

			// Call the next handler to get the response
			resp, err := next(ctx, req)
			if err != nil {
				// If there was an error, we still need to add CORS headers to the error response
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					// Add CORS headers to the error
					connectErr.Meta().Set("Access-Control-Allow-Origin", origin)
					connectErr.Meta().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
					connectErr.Meta().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Connect-Protocol-Version, Authorization, X-Requested-With")
					connectErr.Meta().Set("Access-Control-Allow-Credentials", "true")
				}

				return nil, err
			}

			// Add CORS headers to the response
			resp.Header().Set("Access-Control-Allow-Origin", origin)
			resp.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			resp.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Connect-Protocol-Version, Authorization, X-Requested-With")
			resp.Header().Set("Access-Control-Allow-Credentials", "true")

			return resp, nil
		}
	})
}
