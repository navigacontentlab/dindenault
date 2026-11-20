package navigaid

import (
	"context"
	"errors"
)

type contextKey int

// authInfoKey is used to retrieve the access token.
const authInfoKey = contextKey(iota)

// AuthInfo holds information about the authenticated user.
type AuthInfo struct {
	AccessToken string
	Claims      Claims
}

// authEntry is the value stored under authInfoKey.
type authEntry struct {
	info AuthInfo
	err  error
}

// GetAuth retrieves authentication information from the context.
func GetAuth(ctx context.Context) (AuthInfo, error) {
	auth, ok := ctx.Value(authInfoKey).(authEntry)
	if !ok {
		return AuthInfo{}, errors.New("no authentication information in context")
	}

	if auth.err != nil {
		return AuthInfo{}, auth.err
	}

	return auth.info, nil
}

// SetAuth adds authentication information to the context.
func SetAuth(ctx context.Context, auth AuthInfo, err error) context.Context {
	return context.WithValue(ctx, authInfoKey, authEntry{
		info: auth,
		err:  err,
	})
}
