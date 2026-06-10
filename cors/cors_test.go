package cors_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navigacontentlab/dindenault/cors"
)

const (
	exampleDomain    = "example.com"
	exampleDotDomain = ".example.com"
	exampleAppOrigin = "https://app.example.com"
)

func TestStandardAllowOriginFunc(t *testing.T) {
	tests := []struct {
		name      string
		allowHTTP bool
		domains   []string
		origin    string
		want      bool
	}{
		{"exact domain", false, []string{exampleDomain}, "https://example.com", true},
		{"subdomain", false, []string{exampleDomain}, exampleAppOrigin, true},
		{"dot-prefixed domain matches subdomain", false, []string{exampleDotDomain}, exampleAppOrigin, true},
		{"dot-prefixed domain matches apex", false, []string{exampleDotDomain}, "https://example.com", true},
		{"suffix attack is rejected", false, []string{exampleDomain}, "https://evilexample.com", false},
		{"suffix attack with dot config is rejected", false, []string{exampleDotDomain}, "https://evil-example.com", false},
		{"unrelated domain", false, []string{exampleDomain}, "https://malicious.com", false},
		{"http rejected by default", false, []string{exampleDomain}, "http://example.com", false},
		{"http allowed when configured", true, []string{exampleDomain}, "http://example.com", true},
		{"non-http scheme rejected", true, []string{exampleDomain}, "ftp://example.com", false},
		{"wildcard allows https origin", false, []string{"*"}, "https://anything.com", true},
		{"wildcard rejects http by default", false, []string{"*"}, "http://anything.com", false},
		{"wildcard allows http when configured", true, []string{"*"}, "http://anything.com", true},
		{"empty origin rejected", false, []string{exampleDomain}, "", false},
		{"garbage origin rejected", false, []string{exampleDomain}, "not a url", false},
		{"origin with port matches", false, []string{exampleDomain}, "https://app.example.com:8443", true},
		{"case insensitive host", false, []string{exampleDomain}, "https://APP.EXAMPLE.COM", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := cors.StandardAllowOriginFunc(tt.allowHTTP, tt.domains)
			if got := fn(tt.origin); got != tt.want {
				t.Errorf("origin %q with domains %v: got %v, want %v",
					tt.origin, tt.domains, got, tt.want)
			}
		})
	}
}

func TestMiddlewarePreflight(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("handler"))
	})

	handler := cors.Middleware(cors.Options{
		AllowedDomains: []string{exampleDotDomain},
	}, next)

	t.Run("allowed preflight is answered without reaching handler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/svc", nil)
		req.Header.Set("Origin", exampleAppOrigin)
		req.Header.Set("Access-Control-Request-Method", "POST")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", rec.Code)
		}

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != exampleAppOrigin {
			t.Errorf("expected origin to be reflected, got %q", got)
		}

		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("expected credentials to be allowed, got %q", got)
		}

		if rec.Body.String() == "handler" {
			t.Error("preflight should not reach the handler")
		}
	})

	t.Run("forbidden preflight gets 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/svc", nil)
		req.Header.Set("Origin", "https://malicious.com")
		req.Header.Set("Access-Control-Request-Method", "POST")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rec.Code)
		}

		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("forbidden origin must not get CORS headers")
		}
	})
}

func TestMiddlewareRequests(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("allowed origin gets CORS headers", func(t *testing.T) {
		handler := cors.Middleware(cors.Options{
			AllowedDomains: []string{exampleDotDomain},
		}, next)

		req := httptest.NewRequest(http.MethodPost, "/svc", nil)
		req.Header.Set("Origin", exampleAppOrigin)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != exampleAppOrigin {
			t.Errorf("expected origin to be reflected, got %q", got)
		}

		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("expected credentials header, got %q", got)
		}
	})

	t.Run("disallowed origin passes through without CORS headers", func(t *testing.T) {
		handler := cors.Middleware(cors.Options{
			AllowedDomains: []string{exampleDotDomain},
		}, next)

		req := httptest.NewRequest(http.MethodPost, "/svc", nil)
		req.Header.Set("Origin", "https://malicious.com")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}

		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("disallowed origin must not get CORS headers")
		}
	})

	t.Run("no origin header passes through untouched", func(t *testing.T) {
		handler := cors.Middleware(cors.Options{
			AllowedDomains: []string{exampleDotDomain},
		}, next)

		req := httptest.NewRequest(http.MethodPost, "/svc", nil)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("requests without Origin must not get CORS headers")
		}
	})

	t.Run("wildcard never allows credentials", func(t *testing.T) {
		handler := cors.Middleware(cors.Options{
			AllowedDomains: []string{"*"},
		}, next)

		req := httptest.NewRequest(http.MethodPost, "/svc", nil)
		req.Header.Set("Origin", "https://anything.com")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://anything.com" {
			t.Errorf("expected origin to be allowed with wildcard, got %q", got)
		}

		if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Error("wildcard configuration must not allow credentials")
		}
	})
}
