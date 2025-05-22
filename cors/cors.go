// Package cors provides CORS support for dindenault.
package cors

import (
	"strings"
)

// DefaultDomains returns the default allowed domain suffixes.
func DefaultDomains() []string {
	return []string{".infomaker.io", ".navigacloud.com"}
}

// Options controls the configuration for CORS.
type Options struct {
	// AllowHTTP determines if HTTP (non-HTTPS) origins are allowed
	AllowHTTP bool

	// AllowedDomains is a list of domain suffixes that are allowed in CORS requests
	// e.g. [".navigaglobal.com", ".infomaker.io"]
	// You can also use "*" to allow all origins
	AllowedDomains []string
}

// StandardAllowOriginFunc creates a function that validates CORS origins
// based on the configured allowed domains and HTTP settings.
func StandardAllowOriginFunc(
	allowHTTP bool, allowedDomains []string,
) func(origin string) bool {
	return func(origin string) bool {
		// Check for wildcard origin
		for _, domain := range allowedDomains {
			if domain == "*" {
				// If wildcard is specified and HTTP is allowed, allow any origin
				if allowHTTP {
					return true
				}
				// If HTTP is not allowed, only allow HTTPS origins
				return strings.HasPrefix(origin, "https://")
			}
		}

		// Reject non-HTTPS origins if HTTP is not allowed
		if !allowHTTP && !strings.HasPrefix(origin, "https://") {
			return false
		}

		// Check if origin ends with any of the allowed domain suffixes
		for _, domain := range allowedDomains {
			if domain != "*" && strings.HasSuffix(origin, domain) {
				return true
			}
		}

		return false
	}
}
