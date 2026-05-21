package navigaid_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault/navigaid"
)

func TestNewHTTPClient_ForwardsToken(t *testing.T) {
	const bareToken = "eyJhbGciOiJSUzI1NiJ9.payload.sig" //nolint:gosec
	var gotHeader string

	ds := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
	}))
	defer ds.Close()

	ctx := navigaid.SetAuth(context.Background(), navigaid.AuthInfo{AccessToken: bareToken}, nil)
	client := navigaid.NewHTTPClient(ctx, nil)

	resp, err := client.Get(ds.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "Bearer "+bareToken, gotHeader)
}

func TestNewHTTPClient_NoAuth_NoHeader(t *testing.T) {
	var gotHeader string

	ds := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
	}))
	defer ds.Close()

	client := navigaid.NewHTTPClient(context.Background(), nil)

	resp, err := client.Get(ds.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Empty(t, gotHeader, "no outbound Authorization header when context carries no auth")
}

func TestNewHTTPClient_UsesProvidedBaseTransport(t *testing.T) {
	var baseCalled bool

	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		baseCalled = true

		return http.DefaultTransport.RoundTrip(r)
	})

	ds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ds.Close()

	ctx := navigaid.SetAuth(context.Background(), navigaid.AuthInfo{AccessToken: "tok"}, nil)
	client := navigaid.NewHTTPClient(ctx, base)

	resp, err := client.Get(ds.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.True(t, baseCalled, "provided base RoundTripper must be used")
}

func TestNewHTTPClient_DoesNotMutateRequest(t *testing.T) {
	ds := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer ds.Close()

	ctx := navigaid.SetAuth(context.Background(), navigaid.AuthInfo{AccessToken: "tok"}, nil)
	client := navigaid.NewHTTPClient(ctx, nil)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ds.URL, nil)
	require.NoError(t, err)

	before := req.Header.Get("Authorization")

	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, before, req.Header.Get("Authorization"),
		"transport must clone the request, not mutate the original")
}

// roundTripperFunc is a functional http.RoundTripper for tests.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
