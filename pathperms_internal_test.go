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
		matcher: newPathMatcher([]PathPermissionConfig{
			{PathPrefix: "/any", AnyPermissions: []string{"content:read", "content:admin"}},
			{
				PathPrefix:     "/both",
				Permissions:    []string{"content:write"},
				AnyPermissions: []string{"content:read", "content:admin"},
			},
		}),
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
		matcher: newPathMatcher([]PathPermissionConfig{
			{PathPrefix: "/", AnyPermissions: []string{"content:read", "content:admin"}},
		}),
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

func TestPathPermissionHandlerChecksCleanedPath(t *testing.T) {
	handler := &PathPermissionHandler{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		matcher: newPathMatcher([]PathPermissionConfig{
			{PathPrefix: "/api/users", Permissions: []string{"users:read"}},
			{PathPrefix: "/api/admin", Permissions: []string{"admin:access"}},
		}),
	}

	tests := []struct {
		name     string
		path     string
		perms    []string
		expected int
	}{
		{"plain users path", "/api/users/1", []string{"users:read"}, http.StatusOK},
		{"trailing slash kept", "/api/users/", []string{"users:read"}, http.StatusOK},
		{"dot dot into admin", "/api/users/../admin", []string{"users:read"}, http.StatusForbidden},
		{"double slash admin", "/api//admin", []string{"users:read"}, http.StatusForbidden},
		{"dot segment admin", "/api/./admin", []string{"users:read"}, http.StatusForbidden},
		{"raw admin cleaned users", "/api/admin/../users", []string{"users:read"}, http.StatusForbidden},
		{"both rules held", "/api/admin/../users", []string{"users:read", "admin:access"}, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := navigaid.SetAuth(context.Background(), navigaid.AuthInfo{
				Claims: navigaid.Claims{Permissions: navigaid.PermissionsClaim{Org: tc.perms}},
			}, nil)

			req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil).WithContext(ctx)
			req.URL.Path = tc.path

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.expected {
				t.Errorf("got status %d, want %d", rec.Code, tc.expected)
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	for input, expected := range map[string]string{
		"":              "/",
		"api":           "/api",
		"/a//b":         "/a/b",
		"/a/./b/":       "/a/b/",
		"/a/b/../c":     "/a/c",
		"/../../etc":    "/etc",
		"/":             "/",
		"/a/b/":         "/a/b/",
		"/a/b/..":       "/a",
		"/a/b/../../..": "/",
	} {
		if got := cleanPath(input); got != expected {
			t.Errorf("cleanPath(%q) = %q, want %q", input, got, expected)
		}
	}
}
