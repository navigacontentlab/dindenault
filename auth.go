package dindenault

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/navigacontentlab/dindenault/navigaid"
)

// AuthResult contains authentication and authorization information.
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

// - The user lacks the required permission.
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
// This is a convenience function when you only need the user details.
func GetAuthResultFromContext(ctx context.Context) (*AuthResult, error) {
	return AuthorizeWithDetails(ctx, "")
}

// OrganizationFromContext retrieves the organization from the context.
// Returns an empty string if no authentication information is available.
func OrganizationFromContext(ctx context.Context) (string, error) {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get organization: %w", err)
	}

	return auth.Claims.Org, nil
}

// UserFromContext retrieves user information (first and last name) from the context.
// Returns empty strings if no authentication information is available.
func UserFromContext(ctx context.Context) (givenName, familyName string) {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return "", ""
	}

	return auth.Claims.Userinfo.GivenName, auth.Claims.Userinfo.FamilyName
}

// EmailFromContext retrieves the user's email from the context.
// Returns an empty string if no authentication information is available.
func EmailFromContext(ctx context.Context) string {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return ""
	}

	return auth.Claims.Userinfo.Email
}

// HasPermission checks if the user has the specified permission.
// Returns false if no authentication information is available.
func HasPermission(ctx context.Context, permission string) bool {
	auth, err := navigaid.GetAuth(ctx)
	if err != nil {
		return false
	}

	return checkUserPermission(auth.Claims, permission)
}
