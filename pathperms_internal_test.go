package dindenault

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navigacontentlab/dindenault/navigaid"
)

func TestPathPermissionHandlerAnyPermissions(t *testing.T) {
	handler := &PathPermissionHandler{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		configurations: []PathPermissionConfig{
			{PathPrefix: "/any", AnyPermissions: []string{"content:read", "content:admin"}},
			{
				PathPrefix:     "/both",
				Permissions:    []string{"content:write"},
				AnyPermissions: []string{"content:read", "content:admin"},
			},
		},
	}

	tests := []struct {
		name  string
		path  string
		perms []string
		want  int
	}{
		{"any: first permission", "/any", []string{"content:read"}, http.StatusOK},
		{"any: second permission", "/any", []string{"content:admin"}, http.StatusOK},
		{"any: none of them", "/any", []string{"content:write"}, http.StatusForbidden},
		{"both: all and one", "/both", []string{"content:write", "content:admin"}, http.StatusOK},
		{"both: missing required", "/both", []string{"content:read"}, http.StatusForbidden},
		{"both: missing any", "/both", []string{"content:write"}, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := navigaid.SetAuth(context.Background(), navigaid.AuthInfo{
				Claims: navigaid.Claims{
					Permissions: navigaid.PermissionsClaim{Org: tt.perms},
				},
			}, nil)

			req := httptest.NewRequest(http.MethodGet, tt.path, nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("got status %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestPathPermissionHandlerAnyPermissionsIgnoresUnitGrants(t *testing.T) {
	handler := &PathPermissionHandler{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		configurations: []PathPermissionConfig{
			{PathPrefix: "/", AnyPermissions: []string{"content:read", "content:admin"}},
		},
	}

	ctx := navigaid.SetAuth(context.Background(), navigaid.AuthInfo{
		Claims: navigaid.Claims{
			Permissions: navigaid.PermissionsClaim{
				Units: map[string][]string{"unit1": {"content:admin"}},
			},
		},
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("a unit-scoped grant must not satisfy AnyPermissions, got status %d", rec.Code)
	}
}
