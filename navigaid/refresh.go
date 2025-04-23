package navigaid

import (
	"context"
	"errors"
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

	// Check if we have a valid cached token
	if cached, ok := tr.tokenCache[navigaIDToken]; ok {
		// If token is still valid with a 30-second buffer, return it
		if time.Now().Add(30 * time.Second).Before(cached.expiresAt) {
			return cached.accessToken, nil
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
		expiresAt:   time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	return tokenResp.AccessToken, nil
}

// WithTokenRefresh wraps a function to ensure it has a valid access token.
func WithTokenRefresh(ctx context.Context, refresher *TokenRefresher, fn func(ctx context.Context) error) error {
	// Get current auth info
	_, err := GetAuth(ctx)
	if err != nil {
		return err
	}

	// Function to refresh the token if needed during execution
	refreshToken := func() (context.Context, error) {
		if refresher == nil {
			return ctx, errors.New("token refresher not configured")
		}

		// This would require storing the original Naviga ID token
		// For simplicity, we're assuming the access token is refreshable directly
		// In a real implementation, you'd store the original token or use a refresh token

		// This is a simplified implementation
		return ctx, errors.New("token refresh not implemented")
	}

	// Execute the function with token refresh capability
	err = fn(ctx)
	if err != nil {
		// If the error is due to an expired token, try to refresh and retry
		// This is a simplified check - in real implementation, check for specific auth errors
		// Note: This is a placeholder check - implement actual token expiry detection
		if err.Error() == "token expired" {
			newCtx, refreshErr := refreshToken()
			if refreshErr != nil {
				return refreshErr
			}

			// Retry with the new token
			return fn(newCtx)
		}

		return err
	}

	return nil
}
