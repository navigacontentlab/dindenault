package navigaid

import (
	"fmt"

	"github.com/golang-jwt/jwt/v4"
)

// Known token types.
const (
	TokenTypeAccessToken = "access_token"
	TokenTypeIDToken     = "id_token"
)

// Claims contains information regarding what org and groups (and more), that
// the claim belongs to.
type Claims struct {
	jwt.RegisteredClaims

	Org         string           `json:"org"`
	Groups      []string         `json:"groups"`
	Userinfo    Userinfo         `json:"userinfo"`
	TokenType   string           `json:"ntt"`
	Permissions PermissionsClaim `json:"permissions"`
}

// HasPermissionsInUnit checks if the holder has a set of permissions
// in a unit, either directly, or inherited from the organisation.
func (c Claims) HasPermissionsInUnit(unit string, permissions ...string) bool {
	perms := c.Permissions.PermissionsInUnit(unit)

	for _, p := range permissions {
		if !perms[p] {
			return false
		}
	}

	return true
}

// HasPermissionsInOrganisation checks if the holder has a set of permissions
// in the organisation.
func (c Claims) HasPermissionsInOrganisation(permissions ...string) bool {
	perms := c.Permissions.PermissionsInOrganisation()

	for _, p := range permissions {
		if !perms[p] {
			return false
		}
	}

	return true
}

// Userinfo contains name and similar data.
type Userinfo struct {
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Email      string `json:"email"`
	Picture    string `json:"picture"`
}

// PermissionsClaim describes the permissions the holder has in an
// organisation-wide and per-unit context.
type PermissionsClaim struct {
	Units map[string][]string `json:"units"`
	Org   []string            `json:"org"`
}

// PermissionsInOrganisation returns the permissions the holder has in the
// organisation.
func (p PermissionsClaim) PermissionsInOrganisation() map[string]bool {
	m := make(map[string]bool)
	for _, op := range p.Org {
		m[op] = true
	}

	return m
}

// PermissionsInUnit returns the permissions the holder has in a
// unit, either directly, or inherited from the organisation.
func (p PermissionsClaim) PermissionsInUnit(unit string) map[string]bool {
	m := make(map[string]bool)
	for _, op := range p.Org {
		m[op] = true
	}

	for _, up := range p.Units[unit] {
		m[up] = true
	}

	return m
}

func (c Claims) Valid() error {
	err := c.RegisteredClaims.Valid()
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}
