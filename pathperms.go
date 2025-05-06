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
type PathPermissionConfig struct {
	// PathPrefix is the prefix of the request path
	PathPrefix string
	// Permissions are the organization-level permissions required
	Permissions []string
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

	// Find matching path configuration
	var matchedConfig *PathPermissionConfig

	for _, config := range h.configurations {
		if strings.HasPrefix(path, config.PathPrefix) {
			matchedConfig = &config

			break
		}
	}

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

// WithPathPermissionService adds a service with path-specific permission requirements.
// This allows defining different permission requirements for different API paths.
// Authentication must be handled separately using WithInterceptors and AuthInterceptors.
//
// Parameters:
// - path: The base URL path prefix for the service
// - handler: The HTTP handler for the service
// - configs: Path-specific permission configurations
//
// Example:
//
//	app := dindenault.New(Logger,
//	    // Add authentication interceptor
//	    dindenault.WithInterceptors(
//	        dindenault.AuthInterceptors(Logger, "https://imas.example.com"),
//	    ),
//	    // Register service with path-specific permissions
//	    dindenault.WithPathPermissionService(
//	        "/api/",
//	        apiHandler,
//	        []dindenault.PathPermissionConfig{
//	            {
//	                PathPrefix: "/api/users",
//	                Permissions: []string{"users:read"},
//	            },
//	            {
//	                PathPrefix: "/api/admin",
//	                Permissions: []string{"admin:access"},
//	                UnitPermissions: map[string][]string{
//	                    "HQ": {"admin:superuser"},
//	                },
//	            },
//	        },
//	    ),
//	)
func WithPathPermissionService(
	path string,
	handler http.Handler,
	configs []PathPermissionConfig,
) Option {
	return func(a *App) {
		// Create the handler with path-specific permissions
		permHandler := &PathPermissionHandler{
			handler:        handler,
			logger:         a.Logger,
			configurations: configs,
		}

		// Register the service
		WithService(path, permHandler)(a)

		a.Logger.Info("Registered service with path-specific permissions",
			"path", path,
			"path_configs", len(configs))
	}
}

// PathInterceptor creates an interceptor for Connect that applies path-specific permission checks.
// This is an alternative to WithPathPermissionService for use with Connect handlers that implement
// the ConnectHandlerWithInterceptor interface.
//
//nolint:ireturn // Returning interface as intended by connect.Interceptor design
func PathInterceptors(logger *slog.Logger, configs []PathPermissionConfig) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Get the request path
			path := req.Spec().Procedure

			// Find matching path configuration
			var matchedConfig *PathPermissionConfig

			for _, config := range configs {
				if strings.HasPrefix(path, config.PathPrefix) {
					matchedConfig = &config

					break
				}
			}

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
