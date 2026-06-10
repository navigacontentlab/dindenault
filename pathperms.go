package dindenault

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/navigacontentlab/dindenault/navigaid"
)

// PathPermissionConfig defines permission requirements for specific paths.
//
// When several configurations match a request path, the longest
// (most specific) PathPrefix wins. Paths that match no configuration
// pass through without additional permission checks — define a
// catch-all config (e.g. PathPrefix "/") if you want every path to
// require permissions.
type PathPermissionConfig struct {
	// PathPrefix is the prefix of the request path
	PathPrefix string
	// Permissions are the organization-level permissions required
	Permissions []string
}

// matchPathConfig returns the configuration with the longest matching
// PathPrefix, or nil if none matches. Matching is case-insensitive to
// stay consistent with the App's request routing.
func matchPathConfig(configs []PathPermissionConfig, path string) *PathPermissionConfig {
	path = strings.ToLower(path)

	var matched *PathPermissionConfig

	for i := range configs {
		prefix := strings.ToLower(configs[i].PathPrefix)

		if strings.HasPrefix(path, prefix) {
			if matched == nil || len(configs[i].PathPrefix) > len(matched.PathPrefix) {
				matched = &configs[i]
			}
		}
	}

	return matched
}

// PathPermissionHandler wraps a Connect handler with path-specific permission checking.
type PathPermissionHandler struct {
	handler        http.Handler
	logger         *slog.Logger
	configurations []PathPermissionConfig
}

// ServeHTTP implements the http.Handler interface and applies path-based permission checks.
func (h *PathPermissionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Find the most specific matching path configuration
	matchedConfig := matchPathConfig(h.configurations, path)

	// If no matching configuration, just pass through to the handler
	if matchedConfig == nil {
		h.handler.ServeHTTP(w, r)

		return
	}

	// Get auth info from context
	ctx := r.Context()

	authInfo, err := navigaid.GetAuth(ctx)
	if err != nil {
		h.logger.Info("authentication required", "error", err)
		http.Error(w, "Authentication required", http.StatusUnauthorized)

		return
	}

	// Check org permissions
	for _, permission := range matchedConfig.Permissions {
		if !authInfo.Claims.HasPermissionsInOrganisation(permission) {
			h.logger.Info("permission denied",
				"path", path,
				"permission", permission,
				"user", authInfo.Claims.Subject,
				"org", authInfo.Claims.Org)
			http.Error(w, "Permission denied", http.StatusForbidden)

			return
		}
	}

	// All permissions passed, serve the request
	h.handler.ServeHTTP(w, r)
}

// WithPathPermissionService adds a plain HTTP service with built-in
// authentication and path-specific permission requirements.
//
// Every request is first authenticated against the given JWKS (HTTP 401
// on failure), and then checked against the path permission
// configurations (HTTP 403 on missing permissions). The most specific
// (longest) matching PathPrefix wins; paths without a matching
// configuration require authentication but no specific permission.
//
// The service is registered as a plain handler — app-level Connect
// interceptors (WithInterceptors) are not applied to it.
//
// Parameters:
// - path: The base URL path prefix for the service
// - jwks: JWKS used to validate bearer tokens (see navigaid.NewJWKS)
// - handler: The HTTP handler for the service
// - configs: Path-specific permission configurations
//
// Example:
//
//	jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint("https://imas.example.com"))
//
//	app := dindenault.New(logger,
//	    dindenault.WithPathPermissionService(
//	        "/api/",
//	        jwks,
//	        apiHandler,
//	        []dindenault.PathPermissionConfig{
//	            {
//	                PathPrefix: "/api/users",
//	                Permissions: []string{"users:read"},
//	            },
//	            {
//	                PathPrefix: "/api/admin",
//	                Permissions: []string{"admin:access"},
//	            },
//	        },
//	    ),
//	)
func WithPathPermissionService(
	path string,
	jwks *navigaid.JWKS,
	handler http.Handler,
	configs []PathPermissionConfig,
) Option {
	return func(a *App) {
		// Create the handler with path-specific permissions
		permHandler := &PathPermissionHandler{
			handler:        handler,
			logger:         a.logger,
			configurations: configs,
		}

		// Authenticate before permission checks.
		authHandler := navigaid.HTTPMiddleware(a.logger, jwks, permHandler)

		// Register the service as a plain handler — Connect interceptors
		// cannot be applied to plain HTTP handlers.
		WithPlainService(path, authHandler)(a)

		a.logger.Info("Registered service with path-specific permissions",
			"path", path,
			"path_configs", len(configs))
	}
}

// PathInterceptors creates a Connect interceptor that applies method-level permission checks.
//
// This is the recommended way to apply permissions to specific RPC methods.
// Use this at handler creation time with connect.WithInterceptors.
//
// The interceptor checks the RPC method path against configured prefixes and
// enforces the specified permissions. If no matching configuration is found,
// the request proceeds without additional permission checks.
//
// Important: Always apply AuthInterceptors before PathInterceptors to ensure
// authentication happens first.
//
// Example - Basic usage:
//
//	permissionConfigs := []dindenault.PathPermissionConfig{
//	    {PathPrefix: "/service.v1.Service/Upload", Permissions: []string{"service:write"}},
//	    {PathPrefix: "/service.v1.Service/Search", Permissions: []string{"service:read"}},
//	}
//
//	path, handler := servicev1connect.NewServiceHandler(
//	    impl,
//	    connect.WithInterceptors(
//	        dindenault.AuthInterceptors(logger, imasURL),
//	        dindenault.PathInterceptors(logger, permissionConfigs),
//	    ),
//	)
//
// Example - Multiple permissions required:
//
//	permissionConfigs := []dindenault.PathPermissionConfig{
//	    {
//	        PathPrefix: "/service.v1.Service/DeleteAll",
//	        Permissions: []string{"service:write", "service:admin"},
//	    },
//	}
//
//nolint:ireturn // Returning interface as intended by connect.Interceptor design
func PathInterceptors(logger *slog.Logger, configs []PathPermissionConfig) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Get the request path
			path := req.Spec().Procedure

			// Find the most specific matching path configuration
			matchedConfig := matchPathConfig(configs, path)

			// If no matching configuration, just pass through to the handler
			if matchedConfig == nil {
				return next(ctx, req)
			}

			// Get auth info from context
			authInfo, err := navigaid.GetAuth(ctx)
			if err != nil {
				logger.Info("authentication required", "error", err)

				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
			}

			// Check org permissions
			for _, permission := range matchedConfig.Permissions {
				if !authInfo.Claims.HasPermissionsInOrganisation(permission) {
					logger.Info("permission denied",
						"path", path,
						"permission", permission,
						"user", authInfo.Claims.Subject,
						"org", authInfo.Claims.Org)

					return nil, connect.NewError(connect.CodePermissionDenied,
						errors.New("missing required permission: "+permission))
				}
			}

			// All permissions passed, continue with the request
			return next(ctx, req)
		}
	})
}
