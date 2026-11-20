package dindenault

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strings"

	"connectrpc.com/connect"

	"github.com/navigacontentlab/dindenault/internal/checkinterceptor"
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
	// Permissions are organization-level permissions that are all
	// required (AND).
	Permissions []string
	// AnyPermissions are organization-level permissions of which at
	// least one is required (OR). When both Permissions and
	// AnyPermissions are set, both conditions must hold. Leave empty to
	// skip the check.
	AnyPermissions []string
}

// missingPermission returns a description of the first unmet
// permission requirement in cfg, or "" if the claims satisfy it.
func (cfg *PathPermissionConfig) missingPermission(claims navigaid.Claims) string {
	for _, permission := range cfg.Permissions {
		if !claims.HasPermissionsInOrganisation(permission) {
			return permission
		}
	}

	if len(cfg.AnyPermissions) > 0 && !claims.HasAnyPermissionInOrganisation(cfg.AnyPermissions...) {
		return "one of " + strings.Join(cfg.AnyPermissions, ", ")
	}

	return ""
}

// pathMatcher finds the PathPermissionConfig for a path. Prefixes are
// lowercased once, when the matcher is built, instead of on every request.
type pathMatcher struct {
	configs  []PathPermissionConfig
	prefixes []string
}

// newPathMatcher copies configs, so later changes to the caller's slice do
// not alter the permissions that are enforced.
func newPathMatcher(configs []PathPermissionConfig) *pathMatcher {
	m := &pathMatcher{
		configs:  slices.Clone(configs),
		prefixes: make([]string, len(configs)),
	}

	for i := range m.configs {
		m.prefixes[i] = strings.ToLower(m.configs[i].PathPrefix)
	}

	return m
}

// match returns the configuration with the longest matching PathPrefix, or
// nil if none matches. Matching is case-insensitive to stay consistent with
// the App's request routing.
func (m *pathMatcher) match(path string) *PathPermissionConfig {
	path = strings.ToLower(path)

	var (
		matched   *PathPermissionConfig
		matchedAt int
	)

	for i, prefix := range m.prefixes {
		if strings.HasPrefix(path, prefix) && (matched == nil || len(prefix) > matchedAt) {
			matched, matchedAt = &m.configs[i], len(prefix)
		}
	}

	return matched
}

// cleanPath resolves "." and ".." segments and repeated slashes, keeping a
// trailing slash, the way http.ServeMux does before routing.
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}

	if p[0] != '/' {
		p = "/" + p
	}

	cleaned := path.Clean(p)
	if strings.HasSuffix(p, "/") && cleaned != "/" {
		cleaned += "/"
	}

	return cleaned
}

// PathPermissionHandler wraps a Connect handler with path-specific permission checking.
type PathPermissionHandler struct {
	handler http.Handler
	logger  *slog.Logger
	matcher *pathMatcher
}

// ServeHTTP implements the http.Handler interface and applies path-based permission checks.
//
// A path with "..", "." or repeated slashes is checked against the rules for
// the path as received and for its cleaned form, and must satisfy both: the
// wrapped handler may route on either, and "/api/users/../admin" must not be
// let through on the strength of a "/api/users" rule.
func (h *PathPermissionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	matched := []*PathPermissionConfig{h.matcher.match(path)}
	if cleaned := cleanPath(path); cleaned != path {
		if c := h.matcher.match(cleaned); c != matched[0] {
			matched = append(matched, c)
		}
	}

	matched = slices.DeleteFunc(matched, func(c *PathPermissionConfig) bool { return c == nil })

	// If no matching configuration, just pass through to the handler
	if len(matched) == 0 {
		h.handler.ServeHTTP(w, r)

		return
	}

	// Get auth info from context
	authInfo, err := navigaid.GetAuth(r.Context())
	if err != nil {
		h.logger.Info("authentication required", "error", err)
		http.Error(w, "Authentication required", http.StatusUnauthorized)

		return
	}

	// Check org permissions
	for _, cfg := range matched {
		if missing := cfg.missingPermission(authInfo.Claims); missing != "" {
			h.logger.Info("permission denied",
				"path", path,
				"permission", missing,
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
			handler: handler,
			logger:  a.logger,
			matcher: newPathMatcher(configs),
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
// enforces the specified permissions, for unary calls and streaming handlers
// alike. If no matching configuration is found,
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
// Example - Any one of several permissions (OR):
//
//	permissionConfigs := []dindenault.PathPermissionConfig{
//	    {
//	        PathPrefix:     "/service.v1.Service/Search",
//	        AnyPermissions: []string{"service:read", "service:admin"},
//	    },
//	}
//
//nolint:ireturn // Returning interface as intended by connect.Interceptor design
func PathInterceptors(logger *slog.Logger, configs []PathPermissionConfig) connect.Interceptor {
	matcher := newPathMatcher(configs)

	return checkinterceptor.New(func(ctx context.Context, path string, _ http.Header) (context.Context, error) {
		// Find the most specific matching path configuration. The
		// procedure comes from the handler's own spec, not from the
		// client, so it needs no cleaning.
		matchedConfig := matcher.match(path)

		// If no matching configuration, just pass through to the handler
		if matchedConfig == nil {
			return ctx, nil
		}

		// Get auth info from context
		authInfo, err := navigaid.GetAuth(ctx)
		if err != nil {
			logger.Info("authentication required", "error", err)

			return ctx, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
		}

		// Check org permissions
		if missing := matchedConfig.missingPermission(authInfo.Claims); missing != "" {
			logger.Info("permission denied",
				"path", path,
				"permission", missing,
				"user", authInfo.Claims.Subject,
				"org", authInfo.Claims.Org)

			return ctx, connect.NewError(connect.CodePermissionDenied,
				errors.New("missing required permission: "+missing))
		}

		// All permissions passed, continue with the request
		return ctx, nil
	})
}
