package dindenault_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/navigaid"
)

const (
	streamProcedure = "/test.v1.Stream/Watch"
	streamToken     = "valid-token"
)

// streamJWKS accepts streamToken with the given org permissions.
func streamJWKS(perms ...string) *navigaid.JWKS {
	jwks := navigaid.NewJWKS("http://127.0.0.1:1/unused")
	jwks.SetValidationFunc(func(token string) (navigaid.Claims, error) {
		if token != streamToken {
			return navigaid.Claims{}, errors.New("bad token")
		}

		return navigaid.Claims{Org: "org", Permissions: navigaid.PermissionsClaim{Org: perms}}, nil
	})

	return jwks
}

// callStream serves a server-streaming handler behind interceptors and calls
// it with token (none when empty). It reports whether the handler ran and the
// org it saw in the auth context.
func callStream(t *testing.T, token string, interceptors ...connect.Interceptor) (bool, string, error) {
	t.Helper()

	var (
		reached bool
		org     string
	)

	handler := connect.NewServerStreamHandler(streamProcedure,
		func(ctx context.Context, _ *connect.Request[emptypb.Empty], s *connect.ServerStream[emptypb.Empty]) error {
			reached = true

			if auth, err := navigaid.GetAuth(ctx); err == nil {
				org = auth.Claims.Org
			}

			return s.Send(&emptypb.Empty{})
		},
		connect.WithInterceptors(interceptors...),
	)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := connect.NewClient[emptypb.Empty, emptypb.Empty](http.DefaultClient, srv.URL+streamProcedure)

	req := connect.NewRequest(&emptypb.Empty{})
	if token != "" {
		req.Header().Set("Authorization", "Bearer "+token)
	}

	stream, err := client.CallServerStream(context.Background(), req)
	require.NoError(t, err)

	for stream.Receive() {
		// drain the stream
	}

	return reached, org, stream.Err() //nolint:wrapcheck // callers inspect it with connect.CodeOf
}

func TestStreamingHandler_RequiresAuthentication(t *testing.T) {
	logger := slog.Default()
	auth := navigaid.ConnectInterceptor(logger, streamJWKS())

	reached, _, err := callStream(t, "", auth)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	assert.False(t, reached, "handler must not run without a token")

	reached, _, err = callStream(t, "wrong-token", auth)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	assert.False(t, reached, "handler must not run with an invalid token")

	reached, org, err := callStream(t, streamToken, auth)
	require.NoError(t, err)
	assert.True(t, reached)
	assert.Equal(t, "org", org, "claims must be in the handler's context")
}

func TestStreamingHandler_PathInterceptorsEnforcePermissions(t *testing.T) {
	logger := slog.Default()
	configs := []dindenault.PathPermissionConfig{
		{PathPrefix: "/test.v1.Stream/", Permissions: []string{"stream:read"}},
	}

	reached, _, err := callStream(t, streamToken,
		navigaid.ConnectInterceptor(logger, streamJWKS("other")),
		dindenault.PathInterceptors(logger, configs))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	assert.False(t, reached)

	reached, _, err = callStream(t, "", dindenault.PathInterceptors(logger, configs))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	assert.False(t, reached, "PathInterceptors alone must not let an unauthenticated stream through")

	reached, _, err = callStream(t, streamToken,
		navigaid.ConnectInterceptor(logger, streamJWKS("stream:read")),
		dindenault.PathInterceptors(logger, configs))
	require.NoError(t, err)
	assert.True(t, reached)
}

func TestStreamingHandler_RequirePermission(t *testing.T) {
	logger := slog.Default()

	reached, _, err := callStream(t, streamToken,
		navigaid.ConnectInterceptor(logger, streamJWKS()),
		navigaid.RequirePermission(logger, "stream:read"))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	assert.False(t, reached)

	reached, _, err = callStream(t, streamToken,
		navigaid.ConnectInterceptor(logger, streamJWKS()),
		navigaid.RequireUnitPermission(logger, "HQ", "stream:read"))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	assert.False(t, reached)

	reached, _, err = callStream(t, streamToken,
		navigaid.ConnectInterceptor(logger, streamJWKS("stream:read")),
		navigaid.RequirePermission(logger, "stream:read"),
		navigaid.RequireUnitPermission(logger, "HQ", "stream:read"))
	require.NoError(t, err)
	assert.True(t, reached)
}
