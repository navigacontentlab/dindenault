package types

// ALBTargetGroupRequest contains data originating from the ALB Lambda target group integration
type ALBTargetGroupRequest struct {
	RequestContext    ALBTargetGroupRequestContext `json:"requestContext"`
	HTTPMethod        string                       `json:"httpMethod"`
	Path              string                       `json:"path"`
	QueryStringParams map[string]string            `json:"queryStringParameters"`
	Headers           map[string]string            `json:"headers"`
	Body              string                       `json:"body"`
	IsBase64Encoded   bool                         `json:"isBase64Encoded"`
}

// ALBTargetGroupRequestContext contains the information to identify the load balancer invoking the lambda
type ALBTargetGroupRequestContext struct {
	ELB ELBContext `json:"elb"`
}

// ELBContext contains the information to identify the ARN invoking the lambda
type ELBContext struct {
	TargetGroupArn string `json:"targetGroupArn"`
}

// ALBTargetGroupResponse configures the response to be returned by the ALB Lambda target group for the request
type ALBTargetGroupResponse struct {
	StatusCode        int                 `json:"statusCode"`
	StatusDescription string              `json:"statusDescription,omitempty"`
	Headers           map[string]string   `json:"headers"`
	MultiValueHeaders map[string][]string `json:"multiValueHeaders"`
	Body              string              `json:"body"`
	IsBase64Encoded   bool                `json:"isBase64Encoded"`
}

// APIGatewayV2HTTPRequest contains data coming from the API Gateway V2 HTTP integration
type APIGatewayV2HTTPRequest struct {
	Version               string                        `json:"version"`
	RouteKey              string                        `json:"routeKey"`
	RawPath               string                        `json:"rawPath"`
	RawQueryString        string                        `json:"rawQueryString"`
	Cookies               []string                      `json:"cookies"`
	Headers               map[string]string             `json:"headers"`
	QueryStringParameters map[string]string             `json:"queryStringParameters"`
	RequestContext        APIGatewayV2HTTPRequestContext `json:"requestContext"`
	Body                  string                        `json:"body"`
	PathParameters        map[string]string             `json:"pathParameters"`
	IsBase64Encoded       bool                          `json:"isBase64Encoded"`
	StageVariables        map[string]string             `json:"stageVariables"`
}

// APIGatewayV2HTTPRequestContext contains the information to identify the AWS account and resources invoking the Lambda function
type APIGatewayV2HTTPRequestContext struct {
	RouteKey    string                             `json:"routeKey"`
	AccountID   string                             `json:"accountId"`
	Stage       string                             `json:"stage"`
	RequestID   string                             `json:"requestId"`
	Authorizer  APIGatewayV2HTTPRequestContextAuth `json:"authorizer,omitempty"`
	APIID       string                             `json:"apiId"`
	DomainName  string                             `json:"domainName"`
	DomainPrefix string                            `json:"domainPrefix"`
	Time        string                             `json:"time"`
	TimeEpoch   int64                              `json:"timeEpoch"`
	HTTP        APIGatewayV2HTTPRequestContextHTTP `json:"http"`
}

// APIGatewayV2HTTPRequestContextAuth contains the information about the authorizer
type APIGatewayV2HTTPRequestContextAuth struct {
	JWT map[string]interface{} `json:"jwt,omitempty"`
}

// APIGatewayV2HTTPRequestContextHTTP contains the information about the HTTP request
type APIGatewayV2HTTPRequestContextHTTP struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Protocol  string `json:"protocol"`
	SourceIP  string `json:"sourceIp"`
	UserAgent string `json:"userAgent"`
}

// APIGatewayV2HTTPResponse configures the response to be returned by API Gateway V2 for the request
type APIGatewayV2HTTPResponse struct {
	StatusCode        int                 `json:"statusCode"`
	Headers           map[string]string   `json:"headers"`
	MultiValueHeaders map[string][]string `json:"multiValueHeaders"`
	Body              string              `json:"body"`
	IsBase64Encoded   bool                `json:"isBase64Encoded"`
	Cookies           []string            `json:"cookies"`
}
