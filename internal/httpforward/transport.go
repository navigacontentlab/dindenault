// Package httpforward provides a shared http.RoundTripper that injects a
// fixed Authorization header on every outbound request. It is the transport
// backing both mcp.NewHTTPClient and navigaid.NewHTTPClient.
package httpforward

import (
	"net/http"
	"strings"
)

// NewTransport returns an http.RoundTripper that sets the Authorization header
// to token on every request, cloning the request first so the original is
// never mutated. If base is nil, http.DefaultTransport is used.
//
// The token is not sent on a redirect that leaves the scheme and host of the
// request the client started with: an http.Client drops Authorization on such
// a redirect itself, and adding it back would hand the caller's token to
// whatever host the redirect names. An empty token sets no header.
func NewTransport(token string, base http.RoundTripper) http.RoundTripper {
	return NewTransportWithHeaders(token, nil, base)
}

// NewTransportWithHeaders is NewTransport that also sets each of extra on
// every request (e.g. correlation headers), overriding any value the request
// already carries.
func NewTransportWithHeaders(token string, extra http.Header, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}

	return &forwardingTransport{base: base, token: token, extra: extra}
}

type forwardingTransport struct {
	base  http.RoundTripper
	token string
	extra http.Header
}

func (t *forwardingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())

	if t.token != "" && sameOrigin(r, originalRequest(r)) {
		r.Header.Set("Authorization", t.token)
	}

	for name, values := range t.extra {
		r.Header[name] = values
	}

	return t.base.RoundTrip(r) //nolint:wrapcheck // RoundTrip errors must not be wrapped; callers inspect the concrete type (e.g. *url.Error)
}

// originalRequest returns the request an http.Client started with: for a
// redirect, r.Response is the response that caused it and its Request the
// previous hop.
func originalRequest(r *http.Request) *http.Request {
	for r.Response != nil && r.Response.Request != nil {
		r = r.Response.Request
	}

	return r
}

// sameOrigin reports whether a and b share scheme and host (including port).
func sameOrigin(a, b *http.Request) bool {
	return strings.EqualFold(a.URL.Scheme, b.URL.Scheme) && strings.EqualFold(a.URL.Host, b.URL.Host)
}
