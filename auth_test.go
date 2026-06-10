package dindenault_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	da "github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/navigaid"
)

func TestAuthorizeWithDetails(t *testing.T) {
	// Create a mock context with auth info
	ctx := createAuthContext()

	// Test with no permission required
	result, err := da.AuthorizeWithDetails(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "test-org", result.Organization)
	assert.Equal(t, "John", result.GivenName)
	assert.Equal(t, "Doe", result.FamilyName)
	assert.Equal(t, "john.doe@example.com", result.Email)
	assert.Contains(t, result.Permissions, "content:read")
	assert.Contains(t, result.Groups, "editors")

	// Unit permissions are exposed with their unit context preserved,
	// not flattened into the org-level permission list.
	assert.NotContains(t, result.Permissions, "content:write")
	assert.Contains(t, result.UnitPermissions["unit1"], "content:write")

	// Test with permission that exists in org permissions
	result, err = da.AuthorizeWithDetails(ctx, "content:read")
	require.NoError(t, err)
	assert.Equal(t, "test-org", result.Organization)

	// A permission that only exists in a unit must NOT satisfy an
	// organization-level permission check.
	result, err = da.AuthorizeWithDetails(ctx, "content:write")
	require.Error(t, err)
	assert.Nil(t, result)

	// Test with permission that doesn't exist
	result, err = da.AuthorizeWithDetails(ctx, "admin:manage")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "missing required permission: admin:manage")
}

func TestGetAuthResultFromContext(t *testing.T) {
	// Create a mock context with auth info
	ctx := createAuthContext()

	// Test that it works without permission check
	result, err := da.GetAuthResultFromContext(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test-org", result.Organization)
	assert.Equal(t, "John", result.GivenName)
	assert.Equal(t, "Doe", result.FamilyName)
}

func TestOrganizationFromContext(t *testing.T) {
	// Create a mock context with auth info
	ctx := createAuthContext()

	// Test that it extracts the organization
	org, err := da.OrganizationFromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "test-org", org)

	// Test with empty context
	_, err = da.OrganizationFromContext(context.Background())
	if err != nil {
		return
	}

	t.Fail()
}

func TestUserFromContext(t *testing.T) {
	// Create a mock context with auth info
	ctx := createAuthContext()

	// Test that it extracts the user info
	given, family := da.UserFromContext(ctx)
	assert.Equal(t, "John", given)
	assert.Equal(t, "Doe", family)

	// Test with empty context
	emptyGiven, emptyFamily := da.UserFromContext(context.Background())
	assert.Equal(t, "", emptyGiven)
	assert.Equal(t, "", emptyFamily)
}

func TestEmailFromContext(t *testing.T) {
	// Create a mock context with auth info
	ctx := createAuthContext()

	// Test that it extracts the email
	email := da.EmailFromContext(ctx)
	assert.Equal(t, "john.doe@example.com", email)

	// Test with empty context
	emptyEmail := da.EmailFromContext(context.Background())
	assert.Equal(t, "", emptyEmail)
}

func TestHasPermission(t *testing.T) {
	// Create a mock context with auth info
	ctx := createAuthContext()

	// Test with permission that exists in org permissions
	assert.True(t, da.HasPermission(ctx, "content:read"))

	// A permission that only exists in a unit must NOT satisfy an
	// organization-level permission check.
	assert.False(t, da.HasPermission(ctx, "content:write"))

	// Test with permission that doesn't exist
	assert.False(t, da.HasPermission(ctx, "admin:manage"))

	// Test with empty context
	assert.False(t, da.HasPermission(context.Background(), "content:read"))
}

func TestHasPermissionInUnit(t *testing.T) {
	ctx := createAuthContext()

	// Permission granted directly in the unit
	assert.True(t, da.HasPermissionInUnit(ctx, "unit1", "content:write"))

	// Org-level permissions are inherited by units
	assert.True(t, da.HasPermissionInUnit(ctx, "unit1", "content:read"))

	// Permission from another unit does not leak
	assert.False(t, da.HasPermissionInUnit(ctx, "unit1", "content:publish"))

	// Unknown unit only has org-level permissions
	assert.True(t, da.HasPermissionInUnit(ctx, "unknown", "content:read"))
	assert.False(t, da.HasPermissionInUnit(ctx, "unknown", "content:write"))

	// Test with empty context
	assert.False(t, da.HasPermissionInUnit(context.Background(), "unit1", "content:write"))
}

// Helper to create a context with mock auth info.
func createAuthContext() context.Context {
	// Create mock claims
	claims := navigaid.Claims{
		Org:    "test-org",
		Groups: []string{"editors", "writers"},
		Userinfo: navigaid.Userinfo{
			GivenName:  "John",
			FamilyName: "Doe",
			Email:      "john.doe@example.com",
		},
		TokenType: "",
		Permissions: navigaid.PermissionsClaim{
			Org: []string{"content:read", "content:view"},
			Units: map[string][]string{
				"unit1": {"content:write", "content:delete"},
				"unit2": {"content:publish"},
			},
		},
	}

	// Create mock auth info
	authInfo := navigaid.AuthInfo{
		AccessToken: "test-token",
		Claims:      claims,
	}

	// Create context with auth info
	return navigaid.SetAuth(context.Background(), authInfo, nil)
}
