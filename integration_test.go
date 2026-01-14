package dindenault_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/cors"
)

// TestCompleteServiceRegistrationFlow tests the complete service registration flow
// with various handler types, CORS configuration, and PathInterceptors.
// This validates Requirements 1.1, 1.4, and 2.3.
//
//nolint:gocognit,funlen // Comprehensive integration test with multiple scenarios
func TestCompleteServiceRegistrationFlow(t *testing.T) {
	logger := slog.Default()

	t.Run("simple service registration without CORS", func(t *testing.T) {
		// Create a simple handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("simple service"))
		})

		// Create app with just service registration
		app := dindenault.New(logger,
			dindenault.WithService("/api/simple", handler),
		)

		// Verify registration
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		if registrations[0].Path != "/api/simple" {
			t.Errorf("Expected path '/api/simple', got '%s'", registrations[0].Path)
		}

		// Test the handler works
		req := httptest.NewRequest("GET", "/api/simple", nil)
		recorder := httptest.NewRecorder()
		registrations[0].Handler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		body := recorder.Body.String()
		if body != "simple service" {
			t.Errorf("Expected body 'simple service', got '%s'", body)
		}
	})

	t.Run("service registration with default CORS", func(t *testing.T) {
		// Create a handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("service with CORS"))
		})

		// Create app with CORS and service
		app := dindenault.New(logger,
			dindenault.WithConnectRPC(cors.Options{}), // Empty options use defaults
			dindenault.WithService("/api/cors", handler),
		)

		// Verify registrations (CORS OPTIONS handler + service)
		registrations := app.Registrations()
		if len(registrations) != 2 {
			t.Fatalf("Expected 2 registrations (CORS + service), got %d", len(registrations))
		}

		// Verify CORS interceptor was added
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor (CORS), got %d", len(app.GlobalInterceptors()))
		}
	})

	t.Run("service registration with custom CORS configuration", func(t *testing.T) {
		// Create a handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Create app with custom CORS
		app := dindenault.New(logger,
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{".example.com", ".test.com"},
				AllowHTTP:      true,
			}),
			dindenault.WithService("/api/custom", handler),
		)

		// Verify registrations
		registrations := app.Registrations()
		if len(registrations) != 2 {
			t.Fatalf("Expected 2 registrations, got %d", len(registrations))
		}

		// Test OPTIONS handler with allowed origin
		optionsHandler := registrations[0].Handler
		req := httptest.NewRequest("OPTIONS", "/api/custom", nil)
		req.Header.Set("Origin", "https://app.example.com")

		recorder := httptest.NewRecorder()
		optionsHandler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200 for allowed origin, got %d", recorder.Code)
		}

		// Test OPTIONS handler with HTTP origin (should be allowed)
		req2 := httptest.NewRequest("OPTIONS", "/api/custom", nil)
		req2.Header.Set("Origin", "http://app.example.com")

		recorder2 := httptest.NewRecorder()
		optionsHandler.ServeHTTP(recorder2, req2)

		if recorder2.Code != http.StatusOK {
			t.Errorf("Expected status 200 for HTTP origin when AllowHTTP=true, got %d", recorder2.Code)
		}
	})

	t.Run("multiple services with different handler types", func(t *testing.T) {
		// Create different handler types
		handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mux := http.NewServeMux()
		mux.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mockConnect := &mockConnectHandler{interceptorsApplied: false}

		// Create app with multiple services
		app := dindenault.New(logger,
			dindenault.WithService("/api/func", handlerFunc),
			dindenault.WithService("/api/mux", mux),
			dindenault.WithService("/api/connect", mockConnect),
		)

		// Verify all services were registered
		registrations := app.Registrations()
		if len(registrations) != 3 {
			t.Fatalf("Expected 3 registrations, got %d", len(registrations))
		}

		// Verify paths
		expectedPaths := []string{"/api/func", "/api/mux", "/api/connect"}
		for i, expected := range expectedPaths {
			if registrations[i].Path != expected {
				t.Errorf("Expected path '%s' at index %d, got '%s'", expected, i, registrations[i].Path)
			}
		}
	})

	t.Run("service with global interceptors", func(t *testing.T) {
		// Create a Connect handler that supports interceptors
		mockHandler := &mockConnectHandler{interceptorsApplied: false}

		// Create app with global interceptors
		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
				dindenault.CORSInterceptors([]string{".example.com"}, false),
			),
			dindenault.WithService("/api/intercepted", mockHandler),
		)

		// Verify global interceptors were set
		if len(app.GlobalInterceptors()) != 2 {
			t.Errorf("Expected 2 global interceptors, got %d", len(app.GlobalInterceptors()))
		}

		// Verify service was registered
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}
	})

	t.Run("complex configuration with CORS, interceptors, and multiple services", func(t *testing.T) {
		// Create handlers
		publicHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("public"))
		})

		privateHandler := &mockConnectHandler{interceptorsApplied: false}

		// Create app with complex configuration
		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
			),
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{".company.com"},
				AllowHTTP:      false,
			}),
			dindenault.WithService("/api/public", publicHandler),
			dindenault.WithService("/api/private", privateHandler),
		)

		// Verify all components
		registrations := app.Registrations()
		if len(registrations) != 3 { // OPTIONS + 2 services
			t.Fatalf("Expected 3 registrations, got %d", len(registrations))
		}

		interceptors := app.GlobalInterceptors()
		if len(interceptors) != 2 { // Logging + CORS
			t.Errorf("Expected 2 global interceptors, got %d", len(interceptors))
		}

		// Test that services work
		req := httptest.NewRequest("GET", "/api/public", nil)
		recorder := httptest.NewRecorder()
		registrations[1].Handler.ServeHTTP(recorder, req) // Skip OPTIONS handler

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}
	})

	t.Run("service registration with PathInterceptors", func(t *testing.T) {
		// Create permission configs
		permissionConfigs := []dindenault.PathPermissionConfig{
			{PathPrefix: "/api/admin", Permissions: []string{"admin:access"}},
			{PathPrefix: "/api/user", Permissions: []string{"user:read"}},
		}

		// Create a mock Connect handler
		mockHandler := &mockConnectHandler{interceptorsApplied: false}

		// Create app with PathInterceptors
		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.PathInterceptors(logger, permissionConfigs),
			),
			dindenault.WithService("/api", mockHandler),
		)

		// Verify interceptor was added
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor (PathInterceptors), got %d", len(app.GlobalInterceptors()))
		}

		// Verify service was registered
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}
	})

	t.Run("optional CORS can be omitted", func(t *testing.T) {
		// Create handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Create app without CORS (internal service)
		app := dindenault.New(logger,
			dindenault.WithService("/internal/health", handler),
		)

		// Verify only service was registered (no CORS)
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration (no CORS), got %d", len(registrations))
		}

		// Verify no CORS interceptor
		if len(app.GlobalInterceptors()) != 0 {
			t.Errorf("Expected 0 global interceptors (no CORS), got %d", len(app.GlobalInterceptors()))
		}
	})
}

// TestBackwardCompatibilityScenarios tests that existing functionality continues to work.
// This validates Requirements 5.1 and 5.4.
//
//nolint:funlen // Long test function with comprehensive backward compatibility scenarios
func TestBackwardCompatibilityScenarios(t *testing.T) {
	logger := slog.Default()

	t.Run("existing WithService usage continues to work", func(t *testing.T) {
		// This is the existing pattern that should continue working
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("existing pattern"))
		})

		app := dindenault.New(logger,
			dindenault.WithService("/api/existing", handler),
		)

		// Verify it works
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		// Test the handler
		req := httptest.NewRequest("GET", "/api/existing", nil)
		recorder := httptest.NewRecorder()
		registrations[0].Handler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		body := recorder.Body.String()
		if body != "existing pattern" {
			t.Errorf("Expected body 'existing pattern', got '%s'", body)
		}
	})

	t.Run("PathInterceptors behavior is unchanged", func(t *testing.T) {
		// Create permission configs (existing pattern)
		permissionConfigs := []dindenault.PathPermissionConfig{
			{PathPrefix: "/api/secure", Permissions: []string{"service:access"}},
		}

		// Create interceptor
		interceptor := dindenault.PathInterceptors(logger, permissionConfigs)

		// Verify interceptor is created
		if interceptor == nil {
			t.Fatal("Expected PathInterceptors to return non-nil interceptor")
		}

		// Create a mock Connect handler with the interceptor
		mockHandler := &mockConnectHandler{interceptorsApplied: false}

		app := dindenault.New(logger,
			dindenault.WithInterceptors(interceptor),
			dindenault.WithService("/api", mockHandler),
		)

		// Verify interceptor was added
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor, got %d", len(app.GlobalInterceptors()))
		}
	})

	t.Run("CORS functionality with new name works identically", func(t *testing.T) {
		// Test that WithConnectRPC works the same as the old WithConnectRPCCORS
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		app := dindenault.New(logger,
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{".example.com"},
				AllowHTTP:      false,
			}),
			dindenault.WithService("/api/test", handler),
		)

		// Verify CORS was configured
		registrations := app.Registrations()
		if len(registrations) != 2 { // OPTIONS + service
			t.Fatalf("Expected 2 registrations, got %d", len(registrations))
		}

		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor (CORS), got %d", len(app.GlobalInterceptors()))
		}

		// Test OPTIONS handler works
		optionsHandler := registrations[0].Handler
		req := httptest.NewRequest("OPTIONS", "/api/test", nil)
		req.Header.Set("Origin", "https://app.example.com")

		recorder := httptest.NewRecorder()
		optionsHandler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		// Verify CORS headers
		if origin := recorder.Header().Get("Access-Control-Allow-Origin"); origin != "https://app.example.com" {
			t.Errorf("Expected Access-Control-Allow-Origin header, got '%s'", origin)
		}
	})

	t.Run("multiple services with interceptors work as before", func(t *testing.T) {
		// Existing pattern with multiple services and interceptors
		handler1 := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		handler2 := &mockConnectHandler{interceptorsApplied: false}

		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
			),
			dindenault.WithService("/api/v1", handler1),
			dindenault.WithService("/api/v2", handler2),
		)

		// Verify everything works
		if len(app.Registrations()) != 2 {
			t.Errorf("Expected 2 registrations, got %d", len(app.Registrations()))
		}

		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor, got %d", len(app.GlobalInterceptors()))
		}
	})

	t.Run("complex existing patterns continue to work", func(t *testing.T) {
		// Complex existing pattern combining multiple features
		permissionConfigs := []dindenault.PathPermissionConfig{
			{PathPrefix: "/api/admin", Permissions: []string{"admin:access"}},
		}

		handler := &mockConnectHandler{interceptorsApplied: false}

		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
				dindenault.PathInterceptors(logger, permissionConfigs),
			),
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{".company.com"},
				AllowHTTP:      false,
			}),
			dindenault.WithService("/api", handler),
		)

		// Verify all components
		if len(app.Registrations()) != 2 { // OPTIONS + service
			t.Errorf("Expected 2 registrations, got %d", len(app.Registrations()))
		}

		if len(app.GlobalInterceptors()) != 3 { // Logging + PathInterceptors + CORS
			t.Errorf("Expected 3 global interceptors, got %d", len(app.GlobalInterceptors()))
		}
	})
}

// TestErrorHandlingAndEdgeCases tests error handling and edge cases.
// This validates Requirement 6.4.
//
//nolint:gocognit,funlen // Comprehensive error handling test with multiple edge cases
func TestErrorHandlingAndEdgeCases(t *testing.T) {
	logger := slog.Default()

	t.Run("nil handler panics or fails gracefully", func(t *testing.T) {
		// Test that registering a nil handler is handled
		// Note: The current implementation doesn't explicitly check for nil,
		// but we should verify the behavior
		app := dindenault.New(logger)

		// Register with nil handler - this should not panic during registration
		dindenault.WithService("/api/nil", nil)(app)

		// Verify registration was added (even with nil handler)
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}
		// The handler will be nil, which will panic when called
		// This is expected behavior - we don't prevent nil handlers at registration time
	})
	t.Run("empty CORS options use defaults", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Create app with empty CORS options
		app := dindenault.New(logger,
			dindenault.WithConnectRPC(cors.Options{}),
			dindenault.WithService("/api/test", handler),
		)

		// Verify CORS was configured with defaults
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor (CORS with defaults), got %d", len(app.GlobalInterceptors()))
		}

		registrations := app.Registrations()
		if len(registrations) != 2 { // OPTIONS + service
			t.Fatalf("Expected 2 registrations, got %d", len(registrations))
		}

		// Test that OPTIONS handler works with default domains
		optionsHandler := registrations[0].Handler
		req := httptest.NewRequest("OPTIONS", "/api/test", nil)
		req.Header.Set("Origin", "https://app.infomaker.io")

		recorder := httptest.NewRecorder()
		optionsHandler.ServeHTTP(recorder, req)

		// Should work with default domains (which include .infomaker.io)
		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200 with default domains, got %d", recorder.Code)
		}
	})

	t.Run("complex PathInterceptor configurations", func(t *testing.T) {
		// Test with multiple overlapping path prefixes
		permissionConfigs := []dindenault.PathPermissionConfig{
			{PathPrefix: "/api", Permissions: []string{"api:access"}},
			{PathPrefix: "/api/admin", Permissions: []string{"admin:access"}},
			{PathPrefix: "/api/admin/users", Permissions: []string{"users:manage"}},
			{PathPrefix: "/api/public", Permissions: []string{}}, // No permissions required
		}

		mockHandler := &mockConnectHandler{interceptorsApplied: false}

		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.PathInterceptors(logger, permissionConfigs),
			),
			dindenault.WithService("/api", mockHandler),
		)

		// Verify interceptor was added
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor, got %d", len(app.GlobalInterceptors()))
		}

		// Verify service was registered
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}
	})

	t.Run("empty PathInterceptor configurations", func(t *testing.T) {
		// Test with empty permission configs
		emptyConfigs := []dindenault.PathPermissionConfig{}

		mockHandler := &mockConnectHandler{interceptorsApplied: false}

		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.PathInterceptors(logger, emptyConfigs),
			),
			dindenault.WithService("/api", mockHandler),
		)

		// Verify interceptor was still added (even if empty)
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor, got %d", len(app.GlobalInterceptors()))
		}
	})

	t.Run("CORS with invalid origin", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		app := dindenault.New(logger,
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{".example.com"},
				AllowHTTP:      false,
			}),
			dindenault.WithService("/api/test", handler),
		)

		// Get OPTIONS handler
		registrations := app.Registrations()
		optionsHandler := registrations[0].Handler

		// Test with invalid origin
		req := httptest.NewRequest("OPTIONS", "/api/test", nil)
		req.Header.Set("Origin", "https://malicious.com")

		recorder := httptest.NewRecorder()
		optionsHandler.ServeHTTP(recorder, req)

		// Should return 403 Forbidden
		if recorder.Code != http.StatusForbidden {
			t.Errorf("Expected status 403 for invalid origin, got %d", recorder.Code)
		}
	})

	t.Run("CORS with missing origin header", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		app := dindenault.New(logger,
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{".example.com"},
				AllowHTTP:      false,
			}),
			dindenault.WithService("/api/test", handler),
		)

		// Get OPTIONS handler
		registrations := app.Registrations()
		optionsHandler := registrations[0].Handler

		// Test without Origin header
		req := httptest.NewRequest("OPTIONS", "/api/test", nil)
		// No Origin header set
		recorder := httptest.NewRecorder()
		optionsHandler.ServeHTTP(recorder, req)

		// Should return 400 Bad Request
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for missing origin, got %d", recorder.Code)
		}
	})

	t.Run("multiple CORS configurations", func(t *testing.T) {
		// Test applying WithConnectRPC multiple times
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		app := dindenault.New(logger,
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{".example.com"},
				AllowHTTP:      false,
			}),
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{".test.com"},
				AllowHTTP:      true,
			}),
			dindenault.WithService("/api/test", handler),
		)

		// Both CORS configurations should be added
		if len(app.GlobalInterceptors()) != 2 {
			t.Errorf("Expected 2 global interceptors (2 CORS), got %d", len(app.GlobalInterceptors()))
		}

		// Both OPTIONS handlers should be added
		registrations := app.Registrations()
		if len(registrations) != 3 { // 2 OPTIONS + 1 service
			t.Fatalf("Expected 3 registrations, got %d", len(registrations))
		}
	})

	t.Run("service with empty path", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Register service with empty path
		app := dindenault.New(logger,
			dindenault.WithService("", handler),
		)

		// Verify registration was added
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		if registrations[0].Path != "" {
			t.Errorf("Expected empty path, got '%s'", registrations[0].Path)
		}
	})

	t.Run("service with root path", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Register service at root path
		app := dindenault.New(logger,
			dindenault.WithService("/", handler),
		)

		// Verify registration
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		if registrations[0].Path != "/" {
			t.Errorf("Expected path '/', got '%s'", registrations[0].Path)
		}
	})

	t.Run("PathInterceptors with nil logger", func(t *testing.T) {
		// Test PathInterceptors with nil logger (should not panic)
		permissionConfigs := []dindenault.PathPermissionConfig{
			{PathPrefix: "/api", Permissions: []string{"api:access"}},
		}

		// This should not panic
		interceptor := dindenault.PathInterceptors(nil, permissionConfigs)

		if interceptor == nil {
			t.Error("Expected non-nil interceptor even with nil logger")
		}
	})

	t.Run("handler that doesn't implement ConnectHandlerWithInterceptor", func(t *testing.T) {
		// Test with a regular HTTP handler that doesn't support interceptors
		regularHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
			),
			dindenault.WithService("/api/regular", regularHandler),
		)

		// Verify service was registered
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		// Global interceptors should still be set
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor, got %d", len(app.GlobalInterceptors()))
		}

		// The handler should still work (interceptors just won't be applied)
		req := httptest.NewRequest("GET", "/api/regular", nil)
		recorder := httptest.NewRecorder()
		registrations[0].Handler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}
	})
}
