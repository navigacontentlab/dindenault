// Package navigaid provides authentication and authorization functionality for Naviga ID.
package navigaid

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AccessTokenEndpoint is a helper function that returns the v1 token
// endpoint URL given an URL that points to the access token service.
func AccessTokenEndpoint(serviceURL string) string {
	return fmt.Sprintf("%s/v1/token", strings.TrimSuffix(serviceURL, "/"))
}

// AccessTokenServiceOption configures the AccessTokenService.
type AccessTokenServiceOption func(ats *AccessTokenService)

// WithAccessTokenClient sets the HTTP client that should be used for
// access token requests.
func WithAccessTokenClient(client *http.Client) AccessTokenServiceOption {
	return func(ats *AccessTokenService) {
		ats.client = client
	}
}

// AccessTokenService can validate access tokens and create access tokens from
// naviga-id tokens.
type AccessTokenService struct {
	client        *http.Client
	tokenEndpoint string
}

// NewAccessTokenService creates a new access token service with given options.
func NewAccessTokenService(tokenEndpoint string, options ...AccessTokenServiceOption) *AccessTokenService {
	ats := AccessTokenService{
		tokenEndpoint: tokenEndpoint,
	}

	for _, o := range options {
		o(&ats)
	}

	if ats.client == nil {
		ats.client = &http.Client{Timeout: defaultHTTPTimeout}
	}

	return &ats
}

// New creates a new access token service with given options.
//
// Deprecated: use NewAccessTokenService; navigaid has several types, and New
// does not say which one it makes.
func New(tokenEndpoint string, options ...AccessTokenServiceOption) *AccessTokenService {
	return NewAccessTokenService(tokenEndpoint, options...)
}

// AccessTokenResponse is the response retrieved from navigaID.
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// NewAccessToken takes an navigaID token and returns an access token.
func (ats *AccessTokenService) NewAccessToken(navigaIDToken string) (*AccessTokenResponse, error) {
	req, err := http.NewRequest(http.MethodPost, ats.tokenEndpoint, strings.NewReader(""))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+navigaIDToken)

	res, err := ats.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request access token: %w", err)
	}

	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint responded with: %s", res.Status)
	}

	bytes, err := io.ReadAll(io.LimitReader(res.Body, maxTokenResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	var atr AccessTokenResponse
	err = json.Unmarshal(bytes, &atr)

	if err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	if atr.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned an empty access token")
	}

	return &atr, nil
}

// maxTokenResponseBytes bounds how much of a token endpoint response we
// are willing to read.
const maxTokenResponseBytes = 1 << 20 // 1 MiB

// NoTokenError is used to communicate that no bearer token was included
// in the request.
type NoTokenError struct{}

func (NoTokenError) Error() string {
	return "no token found"
}

// ErrNoToken is the old name of NoTokenError.
//
// Deprecated: use NoTokenError. The Err prefix is for error values, not types.
type ErrNoToken = NoTokenError

// GetAuthToken extracts the bearer token from the Authorization header.
func GetAuthToken(header http.Header) (string, error) {
	auth := header.Get("Authorization")

	authType, token, _ := strings.Cut(auth, " ")
	if token == "" {
		return "", NoTokenError{}
	}

	if strings.ToLower(authType) != "bearer" {
		return "", NoTokenError{}
	}

	return token, nil
}
