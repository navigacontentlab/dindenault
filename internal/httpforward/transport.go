// Package httpforward provides a shared http.RoundTripper that injects a
// fixed Authorization header on every outbound request. It is the transport
// backing both mcp.NewHTTPClient and navigaid.NewHTTPClient.
package httpforward

import "net/http"

// NewTransport returns an http.RoundTripper that sets the Authorization header
// to token on every request, cloning the request first so the original is
// never mutated. If base is nil, http.DefaultTransport is used.
func NewTransport(token string, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}

	return &forwardingTransport{base: base, token: token}
}

type forwardingTransport struct {
	base  http.RoundTripper
	token string
}

func (t *forwardingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", t.token)

	return t.base.RoundTrip(r)
}
