package dindenault_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/navigacontentlab/dindenault"
)

const textPlain = "text/plain; charset=utf-8"

// TestUnmatchedPathResponseHeaders checks that dindenault's own 404 carries
// headers: an ALB target group with multi-value headers enabled answers 502
// to a Lambda response without multiValueHeaders.
func TestUnmatchedPathResponseHeaders(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	app := dindenault.New(slog.Default(), dindenault.WithService("/api", ok))

	t.Run("ALB", func(t *testing.T) {
		resp, err := app.Handle()(t.Context(), events.ALBTargetGroupRequest{
			HTTPMethod:        http.MethodGet,
			Path:              "/.well-known/openid-configuration",
			MultiValueHeaders: map[string][]string{"host": {"example.com"}},
		})
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}

		if got := resp.MultiValueHeaders["Content-Type"]; len(got) != 1 || got[0] != textPlain {
			t.Errorf("multiValueHeaders Content-Type = %v", got)
		}

		if got := resp.Headers["Content-Type"]; got != textPlain {
			t.Errorf("headers Content-Type = %q", got)
		}
	})

	t.Run("API Gateway", func(t *testing.T) {
		resp, err := app.HandleAPIGateway()(t.Context(), events.APIGatewayV2HTTPRequest{
			Version: "2.0",
			RawPath: "/.well-known/openid-configuration",
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: http.MethodGet},
			},
		})
		if err != nil {
			t.Fatalf("HandleAPIGateway: %v", err)
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}

		if got := resp.Headers["Content-Type"]; got != textPlain {
			t.Errorf("headers Content-Type = %q", got)
		}
	})
}
