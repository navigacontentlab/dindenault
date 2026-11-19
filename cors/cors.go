// Package cors provides CORS support for dindenault.
package cors

import (
	"net"
	"net/http"
	"net/url"
	"slices"
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
	// but never "evilexample.com". Use AllowedHosts for an exact host.
	//
	// The wildcard "*" allows all origins. Note that when "*" is used,
	// Access-Control-Allow-Credentials is NOT set, since reflecting
	// arbitrary origins with credentials enabled would defeat the
	// purpose of the same-origin policy.
	AllowedDomains []string

	// AllowedHosts is a list of hosts allowed exactly, without their
	// subdomains: "claude.ai" allows https://claude.ai but not
	// https://evil.claude.ai. Matching is case-insensitive.
	AllowedHosts []string

	// AllowLoopback allows http and https origins on localhost,
	// 127.0.0.1 and [::1] on any port, whatever AllowHTTP says. Use it
	// for local web clients and development tools.
	AllowLoopback bool

	// AllowHeaders lists request headers allowed in addition to the
	// defaults (the Connect and MCP headers, Authorization, Content-Type
	// and Accept).
	AllowHeaders []string

	// ExposeHeaders lists response headers a page may read in addition
	// to the defaults (WWW-Authenticate, MCP-Protocol-Version and
	// Mcp-Session-Id).
	ExposeHeaders []string

	// NoCredentials stops Access-Control-Allow-Credentials from being
	// sent. Set it when the service takes bearer tokens rather than
	// cookies, so that no other site's page can ride on a user's cookies.
	NoCredentials bool
}

const (
	allowMethods = "POST, GET, OPTIONS"
	// allowHeaders are those of Connect and of the MCP Streamable HTTP
	// transport (Mcp-Method and Mcp-Name from the 2026-07-28 revision).
	allowHeaders = "Content-Type, Accept, Connect-Protocol-Version, Connect-Timeout-Ms, Authorization, X-Requested-With, " +
		"MCP-Protocol-Version, Mcp-Method, Mcp-Name, Mcp-Session-Id, Last-Event-ID"
	// exposeHeaders: WWW-Authenticate carries resource_metadata on a 401,
	// which is where an MCP client in a browser finds the login.
	exposeHeaders = "WWW-Authenticate, MCP-Protocol-Version, Mcp-Session-Id"
	maxAge        = "86400" // 24 hours
)

// HasWildcard reports whether the domain list contains "*".
func HasWildcard(allowedDomains []string) bool {
	return slices.Contains(allowedDomains, "*")
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

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// parseOrigin parses an Origin header value: a scheme and a host,
// nothing else. "null" and values with a path, query or user info are
// rejected.
func parseOrigin(origin string) (*url.URL, bool) {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
		return nil, false
	}

	return u, true
}

// StandardAllowOriginFunc creates a function that validates CORS origins
// based on the configured allowed domains and HTTP settings.
//
// The origin is parsed as a URL and its hostname is matched against the
// allowed domains on label boundaries (see Options.AllowedDomains).
func StandardAllowOriginFunc(
	allowHTTP bool, allowedDomains []string,
) func(origin string) bool {
	return Options{AllowHTTP: allowHTTP, AllowedDomains: allowedDomains}.AllowsOrigin
}

// AllowsOrigin reports whether a page on origin may call the service.
func (o Options) AllowsOrigin(origin string) bool {
	u, ok := parseOrigin(origin)
	if !ok {
		return false
	}

	host := strings.ToLower(u.Hostname())
	loopback := o.AllowLoopback && isLoopback(host)

	switch u.Scheme {
	case "https":
	case "http":
		if !o.AllowHTTP && !loopback {
			return false
		}
	default:
		return false
	}

	if loopback || HasWildcard(o.AllowedDomains) {
		return true
	}

	for _, h := range o.AllowedHosts {
		if strings.EqualFold(host, h) {
			return true
		}
	}

	for _, domain := range o.AllowedDomains {
		if hostMatchesDomain(host, domain) {
			return true
		}
	}

	return false
}

// SetHeaders writes the standard CORS response headers for an
// allowed origin. Credentials are only allowed when the configuration
// does not use the "*" wildcard.
func SetHeaders(h http.Header, origin string, wildcard bool) {
	h.Add("Vary", "Origin")
	setHeaders(h, origin, allowHeaders, exposeHeaders, !wildcard)
}

func setHeaders(h http.Header, origin, allow, expose string, credentials bool) {
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Methods", allowMethods)
	h.Set("Access-Control-Allow-Headers", allow)
	h.Set("Access-Control-Expose-Headers", expose)

	if credentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}
}

func joinHeaders(defaults string, extra []string) string {
	if len(extra) == 0 {
		return defaults
	}

	return defaults + ", " + strings.Join(extra, ", ")
}

// Middleware wraps an http.Handler with CORS handling.
//
// Preflight (OPTIONS) requests from allowed origins are answered
// directly without reaching the wrapped handler; those from other
// origins get 403. For other requests from allowed origins, CORS headers
// are added to the response. Requests without an Origin header, or from
// origins that are not allowed, are passed through without CORS
// headers — the browser will then block the cross-origin read.
func Middleware(opts Options, next http.Handler) http.Handler {
	credentials := !opts.NoCredentials && !HasWildcard(opts.AllowedDomains)
	allow := joinHeaders(allowHeaders, opts.AllowHeaders)
	expose := joinHeaders(exposeHeaders, opts.ExposeHeaders)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)

			return
		}

		h := w.Header()
		h.Add("Vary", "Origin")

		isPreflight := r.Method == http.MethodOptions &&
			r.Header.Get("Access-Control-Request-Method") != ""

		if !opts.AllowsOrigin(origin) {
			if isPreflight {
				w.WriteHeader(http.StatusForbidden)

				return
			}

			next.ServeHTTP(w, r)

			return
		}

		setHeaders(h, origin, allow, expose, credentials)

		if isPreflight {
			h.Set("Access-Control-Max-Age", maxAge)
			w.WriteHeader(http.StatusNoContent)

			return
		}

		next.ServeHTTP(w, r)
	})
}
