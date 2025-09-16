package dindenault

import (
	"context"
)

// DEPRECATED: All authentication functionality has been moved to the navigaid module.
// Import "github.com/navigacontentlab/dindenault/navigaid" and use its functions directly.

// AuthResult is deprecated. Use navigaid module directly.
type AuthResult struct {
	Organization string
	GivenName    string
	FamilyName   string
	Email        string
	UserID       string
	Permissions  []string
	Groups       []string
}

// AuthorizeWithDetails is deprecated. Use navigaid module directly.
//
// Migration:
//
//	OLD: authResult, err := dindenault.AuthorizeWithDetails(ctx, permission)
//	NEW: import "github.com/navigacontentlab/dindenault/navigaid"
//	     authInfo, err := navigaid.GetAuth(ctx)
//	     // Then check permissions using authInfo.Claims
func AuthorizeWithDetails(ctx context.Context, permission string) (*AuthResult, error) {
	panic("AuthorizeWithDetails is deprecated. Use: import \"github.com/navigacontentlab/dindenault/navigaid\" and use navigaid.GetAuth(ctx) directly")
}

// GetAuthResultFromContext is deprecated. Use navigaid module directly.
//
// Migration:
//
//	OLD: authResult, err := dindenault.GetAuthResultFromContext(ctx)
//	NEW: import "github.com/navigacontentlab/dindenault/navigaid"
//	     authInfo, err := navigaid.GetAuth(ctx)
func GetAuthResultFromContext(ctx context.Context) (*AuthResult, error) {
	panic("GetAuthResultFromContext is deprecated. Use: import \"github.com/navigacontentlab/dindenault/navigaid\" and use navigaid.GetAuth(ctx) directly")
}

// OrganizationFromContext is deprecated. Use navigaid module directly.
//
// Migration:
//
//	OLD: org := dindenault.OrganizationFromContext(ctx)
//	NEW: import "github.com/navigacontentlab/dindenault/navigaid"
//	     authInfo, err := navigaid.GetAuth(ctx)
//	     if err == nil { org = authInfo.Claims.Org }
func OrganizationFromContext(ctx context.Context) string {
	panic("OrganizationFromContext is deprecated. Use navigaid module directly")
}

// UserFromContext is deprecated. Use navigaid module directly.
//
// Migration:
//
//	OLD: given, family := dindenault.UserFromContext(ctx)
//	NEW: import "github.com/navigacontentlab/dindenault/navigaid"
//	     authInfo, err := navigaid.GetAuth(ctx)
//	     if err == nil { given, family = authInfo.Claims.Userinfo.GivenName, authInfo.Claims.Userinfo.FamilyName }
func UserFromContext(ctx context.Context) (string, string) {
	panic("UserFromContext is deprecated. Use navigaid module directly")
}

// EmailFromContext is deprecated. Use navigaid module directly.
//
// Migration:
//
//	OLD: email := dindenault.EmailFromContext(ctx)
//	NEW: import "github.com/navigacontentlab/dindenault/navigaid"
//	     authInfo, err := navigaid.GetAuth(ctx)
//	     if err == nil { email = authInfo.Claims.Userinfo.Email }
func EmailFromContext(ctx context.Context) string {
	panic("EmailFromContext is deprecated. Use navigaid module directly")
}

// HasPermission is deprecated. Use navigaid module directly.
//
// Migration:
//
//	OLD: hasPermission := dindenault.HasPermission(ctx, permission)
//	NEW: import "github.com/navigacontentlab/dindenault/navigaid"
//	     authInfo, err := navigaid.GetAuth(ctx)
//	     hasPermission := err == nil && authInfo.Claims.HasPermission(permission)
func HasPermission(ctx context.Context, permission string) bool {
	panic("HasPermission is deprecated. Use navigaid module directly")
}

// checkUserPermission is deprecated. Use navigaid module directly.
func checkUserPermission(claims interface{}, permission string) bool {
	panic("checkUserPermission is deprecated. Use navigaid module directly")
}
