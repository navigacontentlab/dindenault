package lambda

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

type RequestContext struct {
	events.ALBTargetGroupRequestContext
	events.APIGatewayV2HTTPRequestContext
}

// Request wraps ALBTargetGroupRequest and APIGatewayV2HTTPRequest
// into a generic request struct.
type Request struct {
	events.ALBTargetGroupRequest
	events.APIGatewayV2HTTPRequest //nolint:govet

	// Added to resolve "ambiguous selectors" error
	Headers               map[string]string `json:"headers"`
	QueryStringParameters map[string]string `json:"queryStringParameters"`
	RequestContext        RequestContext    `json:"requestContext"`
	Body                  string            `json:"body"`
	IsBase64Encoded       bool              `json:"isBase64Encoded"`
}

// Request mimics ALBTargetGroupResponse and APIGatewayV2HTTPResponse
// into a generic response struct.
type Response struct {
	StatusCode        int                 `json:"statusCode"`
	Headers           map[string]string   `json:"headers"`
	MultiValueHeaders map[string][]string `json:"multiValueHeaders"`
	Body              string              `json:"body"`
	IsBase64Encoded   bool                `json:"isBase64Encoded"`
	Cookies           []string            `json:"cookies"`
}

func AWSRequestToHTTPRequest(ctx context.Context, event Request) (*http.Request, error) {
	HTTPMethod := event.HTTPMethod
	if event.Version == "2.0" {
		HTTPMethod = event.RequestContext.HTTP.Method
	}

	params := url.Values{}
	for k, v := range event.QueryStringParameters {
		params.Set(k, v)
	}

	for k, vals := range event.MultiValueQueryStringParameters {
		for _, v := range vals {
			params.Add(k, v)
		}
	}

	headers := make(http.Header)
	for k, v := range event.Headers {
		headers.Set(k, v)
	}

	for k, vals := range event.MultiValueHeaders {
		for _, v := range vals {
			headers.Add(k, v)
		}
	}

	u := url.URL{
		Host:     headers.Get("Host"),
		RawPath:  event.Path,
		RawQuery: params.Encode(),
	}
	if event.Version == "2.0" {
		u.RawPath = event.RawPath
		u.RawQuery = event.RawQueryString
	}

	p, err := url.PathUnescape(u.RawPath)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	u.Path = p

	if u.Path == u.RawPath {
		u.RawPath = ""
	}

	var body io.Reader = strings.NewReader(event.Body)
	if event.IsBase64Encoded {
		body = base64.NewDecoder(base64.StdEncoding, body)
	}

	req, err := http.NewRequest(HTTPMethod, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("could not convert to request: %w", err)
	}

	req.RequestURI = u.RequestURI()
	req.Header = headers

	return req.WithContext(ctx), nil
}
