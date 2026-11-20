package httpforward

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingTransport struct{ got string }

func (rt *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.got = r.Header.Get("Authorization")

	return &http.Response{StatusCode: http.StatusOK, Request: r, Body: http.NoBody}, nil
}

// redirected returns a request for to, as http.Client builds it after from
// answered with a redirect.
func redirected(t *testing.T, from, to string) *http.Request {
	t.Helper()

	first, err := http.NewRequest(http.MethodGet, from, nil)
	require.NoError(t, err)

	next, err := http.NewRequest(http.MethodGet, to, nil)
	require.NoError(t, err)

	next.Response = &http.Response{StatusCode: http.StatusFound, Request: first}

	return next
}

const origin = "https://oc.example.com/a"

func TestRoundTrip_TokenOnlyForOriginalOrigin(t *testing.T) {
	tests := []struct {
		name     string
		from, to string
		expected string
	}{
		{"same origin", origin, "https://oc.example.com/b", "Bearer t"},
		{"other host", origin, "https://evil.example.net/b", ""},
		{"subdomain", origin, "https://x.oc.example.com/b", ""},
		{"other port", origin, "https://oc.example.com:8443/b", ""},
		{"scheme downgrade", origin, "http://oc.example.com/b", ""},
		{"host case", origin, "https://OC.example.com/b", "Bearer t"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &recordingTransport{}

			resp, err := NewTransport("Bearer t", rt).RoundTrip(redirected(t, tc.from, tc.to))
			require.NoError(t, err)
			_ = resp.Body.Close()

			assert.Equal(t, tc.expected, rt.got)
		})
	}
}

func TestRoundTrip_FirstRequestGetsToken(t *testing.T) {
	rt := &recordingTransport{}

	req, err := http.NewRequest(http.MethodGet, "https://any.example.com/", nil)
	require.NoError(t, err)

	resp, err := NewTransport("Bearer t", rt).RoundTrip(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "Bearer t", rt.got)
}
