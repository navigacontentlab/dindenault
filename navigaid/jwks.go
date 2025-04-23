package navigaid

import (
	"bytes"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const defaultJwksTTL = 10 * time.Minute

// ImasJWKSEndpoint is a helper function that returns the v1 JWKS
// endpoint URL given an URL that points to the IMAS service.
func ImasJWKSEndpoint(serviceURL string) string {
	return fmt.Sprintf("%s/v1/jwks", strings.TrimSuffix(serviceURL, "/"))
}

// JWKS can validate access tokens using published JWKS.
type JWKS struct {
	client       *http.Client
	jwksEndpoint string
	ttl          time.Duration

	m              sync.Mutex
	jwksStaleAfter time.Time
	jwks           *jwksResponse

	// For testing purposes
	validate ValidateFunc
}

// JWKSOption is a function that controls the JWKS configuration.
type JWKSOption func(j *JWKS)

// WithJwksTTL can be used to change the default JWKS refresh rate.
func WithJwksTTL(ttl time.Duration) JWKSOption {
	return func(j *JWKS) {
		j.ttl = ttl
	}
}

// WithJwksClient sets the HTTP client that should be used for
// requests.
func WithJwksClient(client *http.Client) JWKSOption {
	return func(j *JWKS) {
		j.client = client
	}
}

// SetValidationFunc sets a custom validation function for testing.
func (j *JWKS) SetValidationFunc(fn ValidateFunc) {
	j.validate = fn
}

// NewJWKS creates a new access token validator.
func NewJWKS(jwksEndpoint string, options ...JWKSOption) *JWKS {
	j := JWKS{
		jwksEndpoint: jwksEndpoint,
		ttl:          defaultJwksTTL,
	}

	for _, o := range options {
		o(&j)
	}

	if j.client == nil {
		j.client = http.DefaultClient
	}

	return &j
}

func (j *JWKS) fetchJWKS() (*jwksResponse, error) {
	req, err := http.NewRequest(http.MethodGet, j.jwksEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create jwks fetch request: %w", err)
	}

	res, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with: %s", res.Status)
	}

	dec := json.NewDecoder(res.Body)

	var jwks jwksResponse

	err = dec.Decode(&jwks)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWKS response: %w", err)
	}

	return &jwks, nil
}

func (j *JWKS) getKey(kid string) (*jwksKey, error) {
	j.m.Lock()
	defer j.m.Unlock()

	// ensure up-to-date version of our jwks
	if time.Now().After(j.jwksStaleAfter) {
		res, err := j.fetchJWKS()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to fetch jwks: %w", err)
		}

		j.jwks = res
		j.jwksStaleAfter = time.Now().Add(j.ttl)
	}

	// find the correct key
	for _, key := range j.jwks.Keys {
		if key.Kid == kid {
			return &key, nil
		}
	}

	return nil, errors.New("key not found")
}

// ValidateFunc is the type of the validation function.
type ValidateFunc func(accessToken string) (Claims, error)

// Validate tries to validate a given access token by first parsing it and then
// looking up the "kid" to match with a jwk (which are cached locally).
func (j *JWKS) Validate(accessToken string) (Claims, error) {
	if j.validate != nil {
		return j.validate(accessToken)
	}

	return j.ValidateToken(accessToken, TokenTypeAccessToken)
}

// ValidateToken tries to validate a given JWT token by first parsing
// it and then looking up the "kid" to match with a jwk (which are
// cached locally).
func (j *JWKS) ValidateToken(token string, tokenType string) (Claims, error) {
	var claims Claims

	t, err := jwt.ParseWithClaims(token, &claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return Claims{}, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		if claims.TokenType != tokenType {
			return Claims{}, fmt.Errorf("unexpected token type %q", claims.TokenType)
		}

		jwk, err := j.getKey(token.Header["kid"].(string))
		if err != nil {
			return Claims{}, errors.New("unknown key id")
		}

		// ensure we have the same algorithm
		if token.Method.Alg() != jwk.Alg {
			return Claims{}, errors.New("algorithm is not the same")
		}

		return jwk.publicKey()
	})
	if err != nil {
		return Claims{}, fmt.Errorf("failed to parse token: %w", err)
	}

	if !t.Valid {
		return Claims{}, errors.New("token is invalid")
	}

	return claims, nil
}

type jwksKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (j *jwksKey) nAsBigInt() (*big.Int, error) {
	data, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	n := big.NewInt(0)
	n.SetBytes(data)

	return n, nil
}

func (j *jwksKey) publicKey() (*rsa.PublicKey, error) {
	var public rsa.PublicKey

	n, err := j.nAsBigInt()
	if err != nil {
		return nil, err
	}

	e, err := j.eAsInt()
	if err != nil {
		return nil, err
	}

	public.E = e
	public.N = n

	return &public, nil
}

func (j *jwksKey) eAsInt() (int, error) {
	data, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return -1, fmt.Errorf("%w", err)
	}

	var ebytes []byte
	// ensure we have padding if needed
	if len(data) < 8 {
		ebytes = make([]byte, 8-len(data), 8)
		ebytes = append(ebytes, data...)
	} else {
		ebytes = data
	}

	reader := bytes.NewReader(ebytes)

	var e uint64
	err = binary.Read(reader, binary.BigEndian, &e)

	if err != nil {
		return -1, fmt.Errorf("%w", err)
	}

	// Make sure the value fits in an int to avoid overflow
	if e > (1<<31 - 1) {
		return -1, fmt.Errorf("exponent value too large: %d", e)
	}

	return int(e), nil
}

type jwksKeyMetadata struct {
	At int `json:"at"`
}

type jwksResponse struct {
	Keys         []jwksKey                  `json:"keys"`
	KeysMetadata map[string]jwksKeyMetadata `json:"keysMeta"`
	MaxTokenTTL  int                        `json:"maxTokenTTL"`
}
