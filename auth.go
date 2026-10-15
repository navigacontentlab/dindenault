package dindenault

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/navigacontentlab/dindenault/navigaid"
)

// AuthResult contains comprehensive authentication and authorization information.
//
// This struct provides all relevant user details extracted from the authentication
// token, including identity information, permissions, and group memberships.
//
// Fields:
//   - Organization: The authenticated user's organization identifier
//   - GivenName: The user's first name
//   - FamilyName: The user's last name
//   - Email: The user's email address
//   - UserID: The user's unique identifier from the JWT sub claim
//   - Permissions: The organization-level permissions granted to the user
//   - UnitPermissions: Unit-specific permissions, keyed by unit
//   - Groups: All groups the user belongs to
type AuthResult struct {
	// Organization is the authenticated user's organization
	Organization string
	// GivenName is the user's first name
	GivenName string
	// FamilyName is the user's last name
	FamilyName string
	// Email is the user's email address
	Email string
	// UserID is the user's unique identifier from the sub claim
	UserID string
	// Permissions is the list of organization-level permissions
	// granted to the user
	Permissions []string
	// UnitPermissions contains unit-specific permissions, keyed by
	// unit identifier
	UnitPermissions map[string][]string
	// Groups is the list of groups the user belongs to
	Groups []string
}

// checkUserPermission verifies if the user has the requested permission
// at the organization level.
//
// Note: unit-scoped permissions do NOT satisfy this check. A permission
// granted in a single unit must not grant organization-wide access —
// use HasPermissionInUnit for unit-scoped checks.
func checkUserPermission(claims navigaid.Claims, permission string) bool {
	// If no permission specified, skip check
	if permission == "" {
		return true
	}

	return claims.HasPermissionsInOrganisation(permission)
}

// AuthorizeWithDetails retrieves authentication details and checks permissions in one call.
//
// This is a convenience function that combines authentication retrieval and
// permission checking. It returns detailed user information including organization,
// name, email, and all permissions.
//
// Parameters:
//   - ctx: The request context containing authentication information
//   - permission: The required permission (empty string to skip permission check)
//
// Returns:
//   - *AuthResult: Detailed authentication information if successful
//   - error: connect.Error with appropriate code if authentication or authorization fails
//
// Example - Check permission and get user details:
//
//	authResult, err := dindenault.AuthorizeWithDetails(ctx, "service:access")
//	if err != nil {
//	    return nil, err // Already formatted as connect.Error
//	}
//	logger.Info("Access granted", "user", authResult.Email, "org", authResult.Organization)
//
// Example - Get user details without permission check:
//
//	authResult, err := dindenault.AuthorizeWithDetails(ctx, "")
//	if err != nil {
//	    return nil, err
//	}
//
// The function returns an error if:
//   - The user is not authenticated
//   - The user lacks the required permission
func AuthorizeWithDetails(ctx context.Context, permission string) (*AuthResult, error) {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("failed to get authorization: %w", err))
	}

	// Verify permission if specified
	if !checkUserPermission(auth.Claims, permission) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("missing required permission: %s", permission))
	}

	return newAuthResult(auth.Claims), nil
}

// AuthorizeWithAnyPermission works like AuthorizeWithDetails, but
// succeeds when the user holds at least one of the given
// organization-level permissions (OR). With no permissions given, no
// permission check is made.
//
// Example:
//
//	authResult, err := dindenault.AuthorizeWithAnyPermission(ctx, "service:read", "service:admin")
//	if err != nil {
//	    return nil, err // Already formatted as connect.Error
//	}
func AuthorizeWithAnyPermission(ctx context.Context, permissions ...string) (*AuthResult, error) {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("failed to get authorization: %w", err))
	}

	if len(permissions) > 0 && !auth.Claims.HasAnyPermissionInOrganisation(permissions...) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("missing required permission: one of %s", strings.Join(permissions, ", ")))
	}

	return newAuthResult(auth.Claims), nil
}

// newAuthResult copies the user details from claims into an AuthResult.
func newAuthResult(claims navigaid.Claims) *AuthResult {
	// Copy unit permissions with their unit context preserved
	unitPermissions := make(map[string][]string, len(claims.Permissions.Units))
	for unit, unitPerms := range claims.Permissions.Units {
		unitPermissions[unit] = append([]string(nil), unitPerms...)
	}

	return &AuthResult{
		Organization:    claims.Org,
		GivenName:       claims.Userinfo.GivenName,
		FamilyName:      claims.Userinfo.FamilyName,
		Email:           claims.Userinfo.Email,
		UserID:          claims.Subject,
		Permissions:     append([]string(nil), claims.Permissions.Org...),
		UnitPermissions: unitPermissions,
		Groups:          claims.Groups,
	}
}

// GetAuthResultFromContext retrieves authentication details from the context
// without performing permission checks.
//
// This is a convenience function when you only need the user details without
// checking specific permissions. It's equivalent to calling AuthorizeWithDetails
// with an empty permission string.
//
// Example:
//
//	authResult, err := dindenault.GetAuthResultFromContext(ctx)
//	if err != nil {
//	    return nil, connect.NewError(connect.CodeUnauthenticated, err)
//	}
//	logger.Info("User info", "email", authResult.Email, "org", authResult.Organization)
func GetAuthResultFromContext(ctx context.Context) (*AuthResult, error) {
	return AuthorizeWithDetails(ctx, "")
}

// OrganizationFromContext retrieves the organization from the context.
//
// This is a convenience function for quickly accessing just the organization
// without retrieving all authentication details.
//
// Returns:
//   - string: The organization identifier
//   - error: An error if authentication information is not available
//
// Example:
//
//	org, err := dindenault.OrganizationFromContext(ctx)
//	if err != nil {
//	    return nil, connect.NewError(connect.CodeUnauthenticated, err)
//	}
//	logger.Info("Processing request", "organization", org)
func OrganizationFromContext(ctx context.Context) (string, error) {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get organization: %w", err)
	}

	return auth.Claims.Org, nil
}

// UserFromContext retrieves user information (first and last name) from the context.
//
// This is a convenience function for quickly accessing user name information
// without retrieving all authentication details.
//
// Returns:
//   - givenName: The user's first name (empty string if not available)
//   - familyName: The user's last name (empty string if not available)
//
// Example:
//
//	givenName, familyName := dindenault.UserFromContext(ctx)
//	if givenName != "" {
//	    logger.Info("User request", "name", fmt.Sprintf("%s %s", givenName, familyName))
//	}
func UserFromContext(ctx context.Context) (givenName, familyName string) {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return "", ""
	}

	return auth.Claims.Userinfo.GivenName, auth.Claims.Userinfo.FamilyName
}

// EmailFromContext retrieves the user's email from the context.
//
// This is a convenience function for quickly accessing just the email
// without retrieving all authentication details.
//
// Returns:
//   - string: The user's email address (empty string if not available)
//
// Example:
//
//	email := dindenault.EmailFromContext(ctx)
//	if email != "" {
//	    logger.Info("Request from user", "email", email)
//	}
func EmailFromContext(ctx context.Context) string {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return ""
	}

	return auth.Claims.Userinfo.Email
}

// HasPermission checks if the user has the specified permission at the
// organization level.
//
// Unit-scoped permissions do not satisfy this check — use
// HasPermissionInUnit for unit-scoped checks. HasPermission is useful
// for conditional logic based on permissions without failing the
// request.
//
// Returns:
//   - bool: true if the user has the permission, false otherwise
//
// Example - Conditional feature access:
//
//	if dindenault.HasPermission(ctx, "feature:beta") {
//	    // Enable beta features
//	}
//
// Example - Permission-based filtering:
//
//	if dindenault.HasPermission(ctx, "admin:access") {
//	    // Include admin-only data in response
//	}
func HasPermission(ctx context.Context, permission string) bool {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return false
	}

	return checkUserPermission(auth.Claims, permission)
}

// HasAnyPermission checks if the user has at least one of the
// specified permissions at the organization level (OR). It returns
// false when no permissions are given.
//
// Example:
//
//	if dindenault.HasAnyPermission(ctx, "content:write", "content:admin") {
//	    // Allow editing
//	}
func HasAnyPermission(ctx context.Context, permissions ...string) bool {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return false
	}

	return auth.Claims.HasAnyPermissionInOrganisation(permissions...)
}

// HasPermissionInUnit checks if the user has the specified permission
// in the given unit, either directly or inherited from the
// organization.
//
// Example:
//
//	if dindenault.HasPermissionInUnit(ctx, "HQ", "admin:access") {
//	    // Allow unit-scoped admin operation
//	}
func HasPermissionInUnit(ctx context.Context, unit, permission string) bool {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return false
	}

	if permission == "" {
		return true
	}

	return auth.Claims.HasPermissionsInUnit(unit, permission)
}
