package navigaid

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// TokenRefresher manages access token refreshing.
type TokenRefresher struct {
	service    *AccessTokenService
	logger     *slog.Logger
	mu         sync.Mutex
	tokenCache map[string]*cachedToken
}

type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// NewTokenRefresher creates a new token refresher.
func NewTokenRefresher(logger *slog.Logger, tokenEndpoint string) *TokenRefresher {
	return &TokenRefresher{
		service:    New(tokenEndpoint),
		logger:     logger,
		tokenCache: make(map[string]*cachedToken),
	}
}

// GetAccessToken gets a valid access token, refreshing if necessary.
// Context parameter is currently unused but kept for API consistency
// and for potential future use with context-based operations.
func (tr *TokenRefresher) GetAccessToken(_ context.Context, navigaIDToken string) (string, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	now := time.Now()

	// Check if we have a valid cached token
	if cached, ok := tr.tokenCache[navigaIDToken]; ok {
		// If token is still valid with a 30-second buffer, return it
		if now.Add(30 * time.Second).Before(cached.expiresAt) {
			return cached.accessToken, nil
		}
	}

	// Evict expired entries so the cache cannot grow without bound in
	// long-lived processes.
	for key, cached := range tr.tokenCache {
		if now.After(cached.expiresAt) {
			delete(tr.tokenCache, key)
		}
	}

	// We need to get a new token
	tokenResp, err := tr.service.NewAccessToken(navigaIDToken)
	if err != nil {
		return "", err
	}

	// Cache the new token
	tr.tokenCache[navigaIDToken] = &cachedToken{
		accessToken: tokenResp.AccessToken,
		expiresAt:   now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	return tokenResp.AccessToken, nil
}
