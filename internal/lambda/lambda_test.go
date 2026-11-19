package lambda_test

import (
	"context"
	"net/http"
	"testing"

	awsevents "github.com/aws/aws-lambda-go/events"

	"github.com/navigacontentlab/dindenault/internal/lambda"
)

const (
	redirectURI   = "https://claude.ai/api/mcp/auth_callback"
	authorizePath = "/authorize"
	badEncoding   = "100%"
)

// ALB hands Lambda the query values still URL-encoded; the request must
// carry them decoded exactly once.
func TestALBQueryIsDecodedOnce(t *testing.T) {
	for name, alb := range map[string]awsevents.ALBTargetGroupRequest{
		"single value": {
			HTTPMethod: http.MethodGet,
			Path:       authorizePath,
			QueryStringParameters: map[string]string{
				"redirect_uri": "https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback",
				"scope":        "read+write",
				"odd":          badEncoding,
			},
		},
		"multi value": {
			HTTPMethod: http.MethodGet,
			Path:       authorizePath,
			MultiValueQueryStringParameters: map[string][]string{
				"redirect_uri": {"https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback"},
				"scope":        {"read+write"},
				"odd":          {badEncoding},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			req, err := lambda.AWSRequestToHTTPRequest(context.Background(), lambda.FromALBRequest(alb))
			if err != nil {
				t.Fatal(err)
			}

			q := req.URL.Query()

			if got := q.Get("redirect_uri"); got != redirectURI {
				t.Errorf("redirect_uri = %q, want %q", got, redirectURI)
			}

			if got := q.Get("scope"); got != "read write" {
				t.Errorf("scope = %q, want %q", got, "read write")
			}

			if got := q.Get("odd"); got != badEncoding {
				t.Errorf("odd = %q, want the invalid encoding kept", got)
			}
		})
	}
}

// API Gateway v2 passes the raw query string, which is used as it is.
func TestAPIGatewayQueryIsUsedAsSent(t *testing.T) {
	req, err := lambda.AWSRequestToHTTPRequest(context.Background(), lambda.FromAPIGatewayRequest(awsevents.APIGatewayV2HTTPRequest{
		RawPath:        authorizePath,
		RawQueryString: "redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback",
		RequestContext: awsevents.APIGatewayV2HTTPRequestContext{
			HTTP: awsevents.APIGatewayV2HTTPRequestContextHTTPDescription{Method: http.MethodGet},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	if got := req.URL.Query().Get("redirect_uri"); got != redirectURI {
		t.Errorf("redirect_uri = %q, want %q", got, redirectURI)
	}
}
