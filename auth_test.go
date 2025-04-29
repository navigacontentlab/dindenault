package dindenault_test

import (
	"context"
	"testing"

	da "github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/navigaid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Contains(t, result.Permissions, "content:write")
	assert.Contains(t, result.Groups, "editors")

	// Test with permission that exists in org permissions
	result, err = da.AuthorizeWithDetails(ctx, "content:read")
	require.NoError(t, err)
	assert.Equal(t, "test-org", result.Organization)

	// Test with permission that exists in unit permissions
	result, err = da.AuthorizeWithDetails(ctx, "content:write")
	require.NoError(t, err)
	assert.Equal(t, "test-org", result.Organization)

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

	// Test with permission that exists in unit permissions
	assert.True(t, da.HasPermission(ctx, "content:write"))

	// Test with permission that doesn't exist
	assert.False(t, da.HasPermission(ctx, "admin:manage"))

	// Test with empty context
	assert.False(t, da.HasPermission(context.Background(), "content:read"))
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
