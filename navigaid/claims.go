package navigaid

import (
	"slices"

	"github.com/golang-jwt/jwt/v5"
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
	for _, p := range permissions {
		if !c.Permissions.hasInUnit(unit, p) {
			return false
		}
	}

	return true
}

// HasPermissionsInOrganisation checks if the holder has a set of permissions
// in the organisation.
func (c Claims) HasPermissionsInOrganisation(permissions ...string) bool {
	for _, p := range permissions {
		if !slices.Contains(c.Permissions.Org, p) {
			return false
		}
	}

	return true
}

// HasAnyPermissionInUnit checks if the holder has at least one of the
// given permissions in a unit, either directly, or inherited from the
// organisation. It returns false when no permissions are given.
func (c Claims) HasAnyPermissionInUnit(unit string, permissions ...string) bool {
	return slices.ContainsFunc(permissions, func(p string) bool {
		return c.Permissions.hasInUnit(unit, p)
	})
}

// HasAnyPermissionInOrganisation checks if the holder has at least one
// of the given permissions in the organisation. It returns false when
// no permissions are given.
func (c Claims) HasAnyPermissionInOrganisation(permissions ...string) bool {
	return slices.ContainsFunc(permissions, func(p string) bool {
		return slices.Contains(c.Permissions.Org, p)
	})
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

// hasInUnit reports whether permission is granted in unit, directly or
// through the organisation. The permission lists in a token are short, so a
// linear scan beats building a set on every check.
func (p PermissionsClaim) hasInUnit(unit, permission string) bool {
	return slices.Contains(p.Org, permission) || slices.Contains(p.Units[unit], permission)
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
