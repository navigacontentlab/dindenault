package dindenault

import (
	"context"
	"fmt"

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
//   - Permissions: All permissions granted to the user (org + unit permissions flattened)
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
	// Permissions is the list of permissions granted to the user
	Permissions []string
	// Groups is the list of groups the user belongs to
	Groups []string
}

// checkUserPermission verifies if the user has the requested permission
// either at organization level or in any unit.
func checkUserPermission(claims navigaid.Claims, permission string) bool {
	// If no permission specified, skip check
	if permission == "" {
		return true
	}

	// Check organization-level permissions first
	if claims.HasPermissionsInOrganisation(permission) {
		return true
	}

	// Check if permission exists in any unit
	for unit := range claims.Permissions.Units {
		if claims.HasPermissionsInUnit(unit, permission) {
			return true
		}
	}

	return false
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

	// Extract all org permissions
	var allPermissions []string
	allPermissions = append(allPermissions, auth.Claims.Permissions.Org...)

	// Add unit permissions (flattened)
	for _, unitPerms := range auth.Claims.Permissions.Units {
		allPermissions = append(allPermissions, unitPerms...)
	}

	// Create and return the result with all available user information
	result := &AuthResult{
		Organization: auth.Claims.Org,
		GivenName:    auth.Claims.Userinfo.GivenName,
		FamilyName:   auth.Claims.Userinfo.FamilyName,
		Email:        auth.Claims.Userinfo.Email,
		UserID:       auth.Claims.Subject,
		Permissions:  allPermissions,
		Groups:       auth.Claims.Groups,
	}

	return result, nil
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

// HasPermission checks if the user has the specified permission.
//
// This function checks both organization-level and unit-level permissions.
// It's useful for conditional logic based on permissions without failing
// the request.
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
