package dindenault_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault"
)

// writeName returns a handler that answers with name.
func writeName(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, name)
	})
}

// TestRoutingUsesDecodedPath checks that an encoded path is routed like the
// decoded path the handler sees, not to a less specific catch-all.
func TestRoutingUsesDecodedPath(t *testing.T) {
	app := dindenault.New(slog.Default(),
		dindenault.WithPlainService("/", writeName("root")),
		dindenault.WithPlainService("/admin/", writeName("admin")),
	)

	paths := map[string]string{
		"/admin/x":    "admin",
		"/%61dmin/x":  "admin",
		"/ADMIN/x":    "admin",
		"/other":      "root",
		"/adm%69n/%2": "", // invalid escape: generic 500
	}

	for path, expected := range paths {
		t.Run("ALB "+path, func(t *testing.T) {
			resp, err := app.Handle()(t.Context(), events.ALBTargetGroupRequest{
				HTTPMethod: http.MethodGet,
				Path:       path,
			})
			require.NoError(t, err)

			if expected == "" {
				assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

				return
			}

			assert.Equal(t, expected, resp.Body)
		})

		t.Run("API Gateway "+path, func(t *testing.T) {
			resp, err := app.HandleAPIGateway()(t.Context(), events.APIGatewayV2HTTPRequest{
				Version: "2.0",
				RawPath: path,
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: http.MethodGet},
				},
			})
			require.NoError(t, err)

			if expected == "" {
				assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

				return
			}

			assert.Equal(t, expected, resp.Body)
		})
	}
}

func TestWithMCPAuthJWKS_NilPanics(t *testing.T) {
	assert.Panics(t, func() {
		dindenault.WithMCPAuthJWKS("/mcp", slog.Default(), nil, nil)
	})
}
