package navigaid

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
)

// ConnectInterceptor returns an interceptor for Connect RPC
// that adds authentication to requests.
//
//nolint:ireturn
func ConnectInterceptor(logger *slog.Logger, jwks *JWKS) connect.Interceptor {
	logger.Debug("Creating Connect interceptor for authentication")

	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Try to extract token from multiple possible headers
			accessToken := extractAccessToken(req)

			if accessToken == "" {
				logger.Info("no access token in request")
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
			}

			// Validate the token
			claims, err := jwks.Validate(accessToken)
			if err != nil {
				logger.Error("token validation failed", "error", err)

				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
			}

			// Add annotations
			AddUserAnnotation(ctx, claims.Subject)
			AddAnnotation(ctx, "imid_org", claims.Org)

			// Set auth info in context
			newCtx := SetAuth(ctx, AuthInfo{
				AccessToken: accessToken,
				Claims:      claims,
			}, nil)

			// Call the next handler with the authenticated context
			return next(newCtx, req)
		}
	})
}

// extractAccessToken tries to extract the access token from various headers
// to maintain compatibility with panurge.
func extractAccessToken(req connect.AnyRequest) string {
	// First try Authorization header (standard)
	authHeader := req.Header().Get("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	imidToken := req.Header().Get("x-imid-token")
	if imidToken != "" {
		return imidToken
	}

	return ""
}

// RequirePermission returns an interceptor that checks
// if the user has the specified permission.
//
//nolint:ireturn
func RequirePermission(logger *slog.Logger, permission string) connect.Interceptor {
	logger.Debug("Creating Connect interceptor for permission", "permission", permission)

	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Check if the user has the required permission
			if err := CheckPermissionConnect(ctx, logger, permission); err != nil {
				return nil, connect.NewError(connect.CodePermissionDenied,
					errors.New("missing required permission: "+permission))
			}

			// Call the next handler
			return next(ctx, req)
		}
	})
}

// RequireUnitPermission returns an interceptor that checks
// if the user has the specified permission for a unit.
//
//nolint:ireturn
func RequireUnitPermission(logger *slog.Logger, unit string, permission string) connect.Interceptor {
	logger.Debug("Creating Connect interceptor for unit permission",
		"unit", unit,
		"permission", permission)

	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Check if the user has the required permission for the unit
			if err := CheckUnitPermissionConnect(ctx, logger, unit, permission); err != nil {
				return nil, connect.NewError(connect.CodePermissionDenied,
					errors.New("missing required permission for unit: "+unit+"/"+permission))
			}

			// Call the next handler
			return next(ctx, req)
		}
	})
}

// ConnectAuthError represents an authentication error for Connect RPC.
type ConnectAuthError struct {
	Message string
	Code    string
}

// Error implements the error interface.
func (e *ConnectAuthError) Error() string {
	return e.Message
}

// NewAuthRequiredError creates a new authentication required error.
func NewAuthRequiredError() error {
	return &ConnectAuthError{
		Message: "Authentication required",
		Code:    "unauthenticated",
	}
}

// NewPermissionDeniedError creates a new permission denied error.
func NewPermissionDeniedError(permission string) error {
	return &ConnectAuthError{
		Message: "Permission denied: " + permission,
		Code:    "permission_denied",
	}
}

// AuthenticateConnect checks the authentication information in a Connect context.
func AuthenticateConnect(ctx context.Context, logger *slog.Logger) (AuthInfo, error) {
	// Get auth info from context
	authInfo, err := GetAuth(ctx)
	if err != nil {
		logger.Info("authentication required", "error", err)

		return AuthInfo{}, errors.New("authentication required")
	}

	return authInfo, nil
}

// CheckPermissionConnect checks if the authenticated user has the required permission.
func CheckPermissionConnect(ctx context.Context, logger *slog.Logger, permission string) error {
	// Get auth info from context
	authInfo, err := GetAuth(ctx)
	if err != nil {
		logger.Info("authentication required", "error", err)

		return errors.New("authentication required")
	}

	// Check if the user has the required permission
	if !authInfo.Claims.HasPermissionsInOrganisation(permission) {
		logger.Info("permission denied",
			"permission", permission,
			"user", authInfo.Claims.Subject,
			"org", authInfo.Claims.Org)

		return errors.New("missing required permission: " + permission)
	}

	return nil
}

// CheckUnitPermissionConnect checks if the authenticated user has the required permission for a unit.
func CheckUnitPermissionConnect(ctx context.Context, logger *slog.Logger, unit, permission string) error {
	// Get auth info from context
	authInfo, err := GetAuth(ctx)
	if err != nil {
		logger.Info("authentication required", "error", err)

		return errors.New("authentication required")
	}

	// Check if the user has the required permission in the specified unit
	if !authInfo.Claims.HasPermissionsInUnit(unit, permission) {
		logger.Info("permission denied for unit",
			"unit", unit,
			"permission", permission,
			"user", authInfo.Claims.Subject,
			"org", authInfo.Claims.Org)

		return errors.New("missing required permission for unit: " + permission)
	}

	return nil
}
