package navigaid

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/navigacontentlab/dindenault/internal/checkinterceptor"
)

// ConnectInterceptor returns an interceptor for Connect RPC
// that adds authentication to requests.
//
// It authenticates unary calls and streaming handlers (server, client and
// bidi streaming) alike; a stream is checked once, against the headers the
// client sent when opening it.
//
//nolint:ireturn
func ConnectInterceptor(logger *slog.Logger, jwks *JWKS) connect.Interceptor {
	logger.Debug("Creating Connect interceptor for authentication")

	return checkinterceptor.New(func(ctx context.Context, _ string, header http.Header) (context.Context, error) {
		// Try to extract token from multiple possible headers
		accessToken := extractAccessToken(header)

		if accessToken == "" {
			logger.Info("no access token in request")

			return ctx, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
		}

		// Validate the token
		claims, err := jwks.Validate(accessToken)
		if err != nil {
			logger.Error("token validation failed", "error", err)

			return ctx, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
		}

		// Set auth info in context
		return SetAuth(ctx, AuthInfo{
			AccessToken: accessToken,
			Claims:      claims,
		}, nil), nil
	})
}

// extractAccessToken tries to extract the access token from various headers
// to maintain compatibility with panurge.
func extractAccessToken(header http.Header) string {
	// First try Authorization header (standard)
	authHeader := header.Get("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	imidToken := header.Get("x-imid-token")
	if imidToken != "" {
		return imidToken
	}

	return ""
}

// RequirePermission returns an interceptor that checks
// if the user has the specified permission. Like ConnectInterceptor, it
// covers unary calls and streaming handlers.
//
//nolint:ireturn
func RequirePermission(logger *slog.Logger, permission string) connect.Interceptor {
	logger.Debug("Creating Connect interceptor for permission", "permission", permission)

	return checkinterceptor.New(func(ctx context.Context, _ string, _ http.Header) (context.Context, error) {
		// Check if the user has the required permission
		if err := CheckPermissionConnect(ctx, logger, permission); err != nil {
			return ctx, connect.NewError(connect.CodePermissionDenied,
				errors.New("missing required permission: "+permission))
		}

		return ctx, nil
	})
}

// RequireUnitPermission returns an interceptor that checks
// if the user has the specified permission for a unit. Like
// ConnectInterceptor, it covers unary calls and streaming handlers.
//
//nolint:ireturn
func RequireUnitPermission(logger *slog.Logger, unit string, permission string) connect.Interceptor {
	logger.Debug("Creating Connect interceptor for unit permission",
		"unit", unit,
		"permission", permission)

	return checkinterceptor.New(func(ctx context.Context, _ string, _ http.Header) (context.Context, error) {
		// Check if the user has the required permission for the unit
		if err := CheckUnitPermissionConnect(ctx, logger, unit, permission); err != nil {
			return ctx, connect.NewError(connect.CodePermissionDenied,
				errors.New("missing required permission for unit: "+unit+"/"+permission))
		}

		return ctx, nil
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
