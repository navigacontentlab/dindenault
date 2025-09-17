package lambda

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/navigacontentlab/dindenault/types"
)

// RequestContext combines the relevant fields from ALB and API Gateway contexts.
type RequestContext struct {
	// API Gateway fields
	HTTP struct {
		Method    string            `json:"method"`
		Path      string            `json:"path"`
		Protocol  string            `json:"protocol"`
		SourceIP  string            `json:"sourceIp"`
		UserAgent string            `json:"userAgent"`
		Headers   map[string]string `json:"headers"`
	} `json:"http"`

	// ALB fields
	ELB struct {
		TargetGroupArn string `json:"targetGroupArn"`
	} `json:"elb"`
}

// Request is a generic request that works with both ALB and API Gateway.
type Request struct {
	Version                         string              `json:"version"`
	Path                            string              `json:"path"`
	HTTPMethod                      string              `json:"httpMethod"`
	Headers                         map[string]string   `json:"headers"`
	MultiValueHeaders               map[string][]string `json:"multiValueHeaders"`
	QueryStringParameters           map[string]string   `json:"queryStringParameters"`
	MultiValueQueryStringParameters map[string][]string `json:"multiValueQueryStringParameters"`
	RequestContext                  RequestContext      `json:"requestContext"`
	Body                            string              `json:"body"`
	IsBase64Encoded                 bool                `json:"isBase64Encoded"`
	RawPath                         string              `json:"rawPath"`
	RawQueryString                  string              `json:"rawQueryString"`
}

// FromALBRequest converts an ALB request to a generic Request.
func FromALBRequest(alb types.ALBTargetGroupRequest) Request {
	req := Request{
		Version:               "1.0",
		Path:                  alb.Path,
		HTTPMethod:            alb.HTTPMethod,
		Headers:               alb.Headers,
		QueryStringParameters: alb.QueryStringParams,
		Body:                  alb.Body,
		IsBase64Encoded:       alb.IsBase64Encoded,
	}

	req.RequestContext.ELB.TargetGroupArn = alb.RequestContext.ELB.TargetGroupArn

	return req
}

// FromAPIGatewayRequest converts an API Gateway request to a generic Request.
func FromAPIGatewayRequest(apigw types.APIGatewayV2HTTPRequest) Request {
	req := Request{
		Version:               "2.0",
		Path:                  apigw.RawPath,
		HTTPMethod:            apigw.RequestContext.HTTP.Method,
		Headers:               apigw.Headers,
		RawPath:               apigw.RawPath,
		RawQueryString:        apigw.RawQueryString,
		QueryStringParameters: apigw.QueryStringParameters,
		Body:                  apigw.Body,
		IsBase64Encoded:       apigw.IsBase64Encoded,
	}

	// Manually copy HTTP fields
	req.RequestContext.HTTP.Method = apigw.RequestContext.HTTP.Method
	req.RequestContext.HTTP.Path = apigw.RequestContext.HTTP.Path
	req.RequestContext.HTTP.Protocol = apigw.RequestContext.HTTP.Protocol
	req.RequestContext.HTTP.SourceIP = apigw.RequestContext.HTTP.SourceIP
	req.RequestContext.HTTP.UserAgent = apigw.RequestContext.HTTP.UserAgent

	return req
}

// Response mimics ALBTargetGroupResponse and APIGatewayV2HTTPResponse.
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
		return nil, fmt.Errorf("failed to unescape path %q: %w", u.RawPath, err)
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
		return nil, fmt.Errorf("failed to create HTTP request for method %s and URL %s: %w", HTTPMethod, u.String(), err)
	}

	req.RequestURI = u.RequestURI()
	req.Header = headers

	return req.WithContext(ctx), nil
}
