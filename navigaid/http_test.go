package navigaid_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

// redirectPair starts a target server that records the Authorization header
// it receives and a front server that redirects to targetURL(target).
func redirectPair(t *testing.T, targetURL func(*httptest.Server) string) (string, *string) {
	t.Helper()

	got := new(string)

	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		*got = r.Header.Get("Authorization")
	}))
	t.Cleanup(target.Close)

	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetURL(target), http.StatusFound)
	}))
	t.Cleanup(front.Close)

	return front.URL, got
}

func authCtx() context.Context {
	return navigaid.SetAuth(context.Background(), navigaid.AuthInfo{AccessToken: "secret-token"}, nil)
}

func TestNewHTTPClient_CrossHostRedirect_DropsToken(t *testing.T) {
	// "localhost" is another host than the front server's 127.0.0.1.
	frontURL, got := redirectPair(t, func(target *httptest.Server) string {
		return strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
	})

	resp, err := navigaid.NewHTTPClient(authCtx(), nil).Get(frontURL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Empty(t, *got, "token must not follow a redirect to another host")
}

func TestNewHTTPClient_SameHostRedirect_KeepsToken(t *testing.T) {
	var got string

	ds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)

			return
		}

		got = r.Header.Get("Authorization")
	}))
	defer ds.Close()

	resp, err := navigaid.NewHTTPClient(authCtx(), nil).Get(ds.URL + "/start")
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "Bearer secret-token", got)
}

func TestNewHTTPClient_NoAuth_KeepsCallerHeader(t *testing.T) {
	var got string

	ds := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer ds.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ds.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer own-token")

	resp, err := navigaid.NewHTTPClient(context.Background(), nil).Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "Bearer own-token", got, "without auth in ctx the caller's own header is left alone")
}

func TestGetAuthToken_NoTokenErrorAndDeprecatedAlias(t *testing.T) {
	_, err := navigaid.GetAuthToken(http.Header{})

	var noToken navigaid.NoTokenError
	require.ErrorAs(t, err, &noToken)

	var old navigaid.ErrNoToken // the deprecated name must keep working
	require.ErrorAs(t, err, &old)
}
