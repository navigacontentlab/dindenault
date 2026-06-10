// Package cors provides CORS support for dindenault.
package cors

import (
	"net/http"
	"net/url"
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

	// AllowedDomains is a list of domains that are allowed in CORS
	// requests, e.g. [".navigaglobal.com", "app.example.com"].
	//
	// A domain with a leading dot allows the domain itself and any
	// subdomain. A domain without a leading dot is treated the same
	// way: "example.com" allows "example.com" and "sub.example.com",
	// but never "evilexample.com".
	//
	// The wildcard "*" allows all origins. Note that when "*" is used,
	// Access-Control-Allow-Credentials is NOT set, since reflecting
	// arbitrary origins with credentials enabled would defeat the
	// purpose of the same-origin policy.
	AllowedDomains []string
}

const (
	allowMethods = "POST, GET, OPTIONS"
	allowHeaders = "Content-Type, Accept, Connect-Protocol-Version, Connect-Timeout-Ms, Authorization, X-Requested-With"
	maxAge       = "86400" // 24 hours
)

// HasWildcard reports whether the domain list contains "*".
func HasWildcard(allowedDomains []string) bool {
	for _, domain := range allowedDomains {
		if domain == "*" {
			return true
		}
	}

	return false
}

// hostMatchesDomain checks whether host equals domain or is a
// subdomain of it. Domain may have a leading dot; matching is
// case-insensitive and always happens on label boundaries, so
// "example.com" never matches "evilexample.com".
func hostMatchesDomain(host, domain string) bool {
	host = strings.ToLower(host)
	domain = strings.ToLower(strings.TrimPrefix(domain, "."))

	if domain == "" {
		return false
	}

	return host == domain || strings.HasSuffix(host, "."+domain)
}

// StandardAllowOriginFunc creates a function that validates CORS origins
// based on the configured allowed domains and HTTP settings.
//
// The origin is parsed as a URL and its hostname is matched against the
// allowed domains on label boundaries (see Options.AllowedDomains).
func StandardAllowOriginFunc(
	allowHTTP bool, allowedDomains []string,
) func(origin string) bool {
	return func(origin string) bool {
		u, err := url.Parse(origin)
		if err != nil || u.Hostname() == "" {
			return false
		}

		switch u.Scheme {
		case "https":
		case "http":
			if !allowHTTP {
				return false
			}
		default:
			return false
		}

		if HasWildcard(allowedDomains) {
			return true
		}

		for _, domain := range allowedDomains {
			if hostMatchesDomain(u.Hostname(), domain) {
				return true
			}
		}

		return false
	}
}

// SetHeaders writes the standard CORS response headers for an
// allowed origin. Credentials are only allowed when the configuration
// does not use the "*" wildcard.
func SetHeaders(h http.Header, origin string, wildcard bool) {
	h.Set("Access-Control-Allow-Origin", origin)
	h.Add("Vary", "Origin")
	h.Set("Access-Control-Allow-Methods", allowMethods)
	h.Set("Access-Control-Allow-Headers", allowHeaders)

	if !wildcard {
		h.Set("Access-Control-Allow-Credentials", "true")
	}
}

// Middleware wraps an http.Handler with CORS handling.
//
// Preflight (OPTIONS) requests from allowed origins are answered
// directly without reaching the wrapped handler. For other requests
// from allowed origins, CORS headers are added to the response.
// Requests without an Origin header, or from origins that are not
// allowed, are passed through without CORS headers — the browser will
// then block the cross-origin read.
func Middleware(opts Options, next http.Handler) http.Handler {
	allowOrigin := StandardAllowOriginFunc(opts.AllowHTTP, opts.AllowedDomains)
	wildcard := HasWildcard(opts.AllowedDomains)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)

			return
		}

		isPreflight := r.Method == http.MethodOptions &&
			r.Header.Get("Access-Control-Request-Method") != ""

		if !allowOrigin(origin) {
			if isPreflight {
				w.WriteHeader(http.StatusForbidden)

				return
			}

			next.ServeHTTP(w, r)

			return
		}

		if isPreflight {
			SetHeaders(w.Header(), origin, wildcard)
			w.Header().Set("Access-Control-Max-Age", maxAge)
			w.WriteHeader(http.StatusNoContent)

			return
		}

		SetHeaders(w.Header(), origin, wildcard)
		next.ServeHTTP(w, r)
	})
}
