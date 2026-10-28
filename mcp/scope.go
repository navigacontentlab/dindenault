package mcp

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

const (
	grantedScopesKey contextKey = correlationKey + 1
	metadataURLKey   contextKey = correlationKey + 2
)

// ContextWithGrantedScopes returns a copy of ctx that carries the OAuth scopes
// granted to the caller's token. The Server hides tools whose RequiredScopes
// they do not cover from tools/list and refuses calls to them. AuthMiddleware
// sets them from WithScopes; use this to set them from your own auth layer.
func ContextWithGrantedScopes(ctx context.Context, scopes []string) context.Context {
	return context.WithValue(ctx, grantedScopesKey, slices.Clone(scopes))
}

// GrantedScopesFromContext returns the OAuth scopes granted to the caller of
// the MCP request being handled, or nil when none were set.
func GrantedScopesFromContext(ctx context.Context) []string {
	scopes, _ := ctx.Value(grantedScopesKey).([]string)

	return slices.Clone(scopes)
}

// missingScopes returns the tool's RequiredScopes that granted does not hold,
// in the tool's order.
func (t *Tool) missingScopes(granted []string) []string {
	var missing []string

	for _, s := range t.RequiredScopes {
		if !slices.Contains(granted, s) && !slices.Contains(missing, s) {
			missing = append(missing, s)
		}
	}

	return missing
}

// writeInsufficientScope answers a tools/call the granted scopes do not cover
// with 403 and an RFC 6750 insufficient_scope challenge. The challenge asks
// for the scopes already granted plus the missing ones, so the client's new
// login keeps what it could already do. resource_metadata is only sent when
// AuthMiddleware runs WithChallenge.
func writeInsufficientScope(ctx context.Context, w http.ResponseWriter, tool string, missing []string) {
	scopes := append(GrantedScopesFromContext(ctx), missing...)

	value := fmt.Sprintf(`Bearer error="insufficient_scope", scope=%q`, strings.Join(scopes, " "))
	if metadataURL, _ := ctx.Value(metadataURLKey).(string); metadataURL != "" {
		value += fmt.Sprintf(`, resource_metadata=%q`, metadataURL)
	}

	value += fmt.Sprintf(`, error_description=%q`, "the "+tool+" tool needs "+strings.Join(missing, " "))

	w.Header().Set("WWW-Authenticate", value)
	http.Error(w, "Forbidden", http.StatusForbidden)
}
