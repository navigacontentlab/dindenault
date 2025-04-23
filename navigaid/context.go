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

type ai struct {
	Ac  AuthInfo
	Err error
}

// GetAuth retrieves authentication information from the context.
func GetAuth(ctx context.Context) (AuthInfo, error) {
	auth, ok := ctx.Value(authInfoKey).(ai)
	if !ok {
		return AuthInfo{}, errors.New("no authentication information in context")
	}

	if auth.Err != nil {
		return AuthInfo{}, auth.Err
	}

	return auth.Ac, nil
}

// SetAuth adds authentication information to the context.
func SetAuth(ctx context.Context, auth AuthInfo, err error) context.Context {
	return context.WithValue(ctx, authInfoKey, ai{
		Ac:  auth,
		Err: err,
	})
}

// AddAnnotation adds an annotation to the XRay segment in the current context.
func AddAnnotation(ctx context.Context, key string, value string) {
	// Add XRay annotation if available
	addXRayAnnotation(ctx, key, value)
}

// AddUserAnnotation adds a user annotation to the XRay segment in the current context.
func AddUserAnnotation(ctx context.Context, user string) {
	AddAnnotation(ctx, "user", user)
}
