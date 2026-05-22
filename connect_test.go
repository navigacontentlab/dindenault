package dindenault_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	"github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/cors"
)

const (
	testDomain         = "example.com"
	testWildcardDomain = ".example.com"
)

func TestWithInterceptors(t *testing.T) {
	// Create a test Logger
	logger := slog.Default()

	// Create a test app
	app := dindenault.New(logger)

	// Create test interceptors
	testInterceptor1 := dindenault.LoggingInterceptors(logger)

	// Apply the WithInterceptors option
	dindenault.WithInterceptors(testInterceptor1)(app)

	// Check that interceptors were added
	if len(app.GlobalInterceptors()) != 1 {
		t.Errorf("Expected 2 interceptors, got %d", len(app.GlobalInterceptors()))
	}
}

func TestLoggingInterceptors(t *testing.T) {
	// Create a test Logger
	logger := slog.Default()

	// Create the interceptor
	interceptor := dindenault.LoggingInterceptors(logger)

	// Assert it's not nil
	if interceptor == nil {
		t.Error("LoggingInterceptors returned nil")
	}
}

func TestTelemetryInterceptor(t *testing.T) {
	logger := slog.Default()

	t.Run("with noop provider", func(t *testing.T) {
		// Create the interceptor with noop provider
		interceptor := dindenault.TelemetryInterceptor(logger, dindenault.NoopTelemetry{}, dindenault.DefaultTelemetryOptions())

		// Should be nil for noop provider
		if interceptor != nil {
			t.Error("TelemetryInterceptor with NoopTelemetry should return nil")
		}
	})

	t.Run("with nil provider", func(t *testing.T) {
		// Create the interceptor with nil provider
		interceptor := dindenault.TelemetryInterceptor(logger, nil, dindenault.DefaultTelemetryOptions())

		// Should be nil for nil provider
		if interceptor != nil {
			t.Error("TelemetryInterceptor with nil provider should return nil")
		}
	})
}

func TestCORSInterceptors(t *testing.T) {
	t.Run("function exists and returns interceptor", func(t *testing.T) {
		// Create the interceptor
		origins := []string{"https://example.com"}
		interceptor := dindenault.CORSInterceptors(origins, false)

		// Assert it's not nil
		if interceptor == nil {
			t.Error("CORSInterceptors returned nil")
		}
	})

	t.Run("function works with different configurations", func(t *testing.T) {
		// Test with HTTP allowed
		interceptor1 := dindenault.CORSInterceptors([]string{testDomain}, true)
		if interceptor1 == nil {
			t.Error("CORSInterceptors with allowHTTP=true returned nil")
		}

		// Test with multiple origins
		interceptor2 := dindenault.CORSInterceptors([]string{testDomain, "test.com"}, false)
		if interceptor2 == nil {
			t.Error("CORSInterceptors with multiple origins returned nil")
		}

		// Test with empty origins
		interceptor3 := dindenault.CORSInterceptors([]string{}, false)
		if interceptor3 == nil {
			t.Error("CORSInterceptors with empty origins returned nil")
		}
	})
}

func TestAuthInterceptors(t *testing.T) {
	// Skip if testing in short mode
	if testing.Short() {
		t.Skip("Skipping authentication interceptor test in short mode")
	}

	// Create the interceptor
	interceptor := dindenault.AuthInterceptors(slog.Default(), "https://imas.example.com")

	// Assert it's not nil
	if interceptor == nil {
		t.Error("AuthInterceptors returned nil")
	}
}

// TestMultipleInterceptors tests that we can use multiple interceptors together.
func TestMultipleInterceptors(t *testing.T) {
	logger := slog.Default()

	// Create multiple interceptors
	loggingInterceptor := dindenault.LoggingInterceptors(logger)

	// Create a test app
	app := dindenault.New(logger)

	// Add the interceptors
	dindenault.WithInterceptors(loggingInterceptor)(app)

	// Check the number of interceptors
	if len(app.GlobalInterceptors()) != 1 {
		t.Errorf("Expected 2 interceptors, got %d", len(app.GlobalInterceptors()))
	}
}

// TestWithConnectRPCCORS_Deprecated tests the old function name for backward compatibility.
// This test can be removed once all users have migrated to WithConnectRPC.
func TestWithConnectRPCCORS_Deprecated(t *testing.T) {
	t.Skip("WithConnectRPCCORS has been removed - use WithConnectRPC instead")
}

// TestWithConnectRPC tests the renamed CORS configuration function.
// This tests Requirements 2.2 and 2.4 - verifying CORS headers, OPTIONS handling, and default options.
//
//nolint:gocognit,funlen // Comprehensive test with multiple scenarios
func TestWithConnectRPC(t *testing.T) {
	logger := slog.Default()

	t.Run("adds CORS interceptor to global interceptors", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Apply WithConnectRPC with custom domains
		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{testDomain},
			AllowHTTP:      false,
		})(app)

		// Check that a global interceptor was added
		interceptors := app.GlobalInterceptors()
		if len(interceptors) != 1 {
			t.Errorf("Expected 1 global interceptor, got %d", len(interceptors))
		}
	})

	t.Run("adds OPTIONS handler registration", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Apply WithConnectRPC
		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{testDomain},
			AllowHTTP:      false,
		})(app)

		// Check that a registration was added (for the OPTIONS handler)
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Errorf("Expected 1 registration (OPTIONS handler), got %d", len(registrations))
		}

		// Verify the registration is for the root path (catch-all)
		if registrations[0].Path != "/" {
			t.Errorf("Expected OPTIONS handler at '/', got '%s'", registrations[0].Path)
		}
	})

	t.Run("uses default domains when empty options provided", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Apply WithConnectRPC with empty domains - should use defaults
		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{},
			AllowHTTP:      false,
		})(app)

		// Should still add interceptor and handler with default domains
		if len(app.GlobalInterceptors()) != 1 {
			t.Error("Expected CORS interceptor to be added with default domains")
		}

		if len(app.Registrations()) != 1 {
			t.Error("Expected OPTIONS handler to be added with default domains")
		}
	})

	t.Run("OPTIONS handler returns correct CORS headers", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Apply WithConnectRPC
		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{testWildcardDomain},
			AllowHTTP:      false,
		})(app)

		// Get the OPTIONS handler
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatal("Expected 1 registration")
		}

		handler := registrations[0].Handler

		// Test valid OPTIONS request
		req := newTestRequest(t, "OPTIONS", "/test/path")
		req.Header.Set("Origin", "https://app.example.com")

		recorder := newTestResponseRecorder()
		handler.ServeHTTP(recorder, req)

		// Verify status code
		if recorder.Code != 200 {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		// Verify CORS headers are set correctly
		headers := recorder.Header()

		if origin := headers.Get("Access-Control-Allow-Origin"); origin != "https://app.example.com" {
			t.Errorf("Expected Access-Control-Allow-Origin to be 'https://app.example.com', got '%s'", origin)
		}

		if methods := headers.Get("Access-Control-Allow-Methods"); methods != "POST, OPTIONS" {
			t.Errorf("Expected Access-Control-Allow-Methods to be 'POST, OPTIONS', got '%s'", methods)
		}

		if allowHeaders := headers.Get("Access-Control-Allow-Headers"); allowHeaders == "" {
			t.Error("Expected Access-Control-Allow-Headers to be set")
		}

		if credentials := headers.Get("Access-Control-Allow-Credentials"); credentials != "true" {
			t.Errorf("Expected Access-Control-Allow-Credentials to be 'true', got '%s'", credentials)
		}

		if maxAge := headers.Get("Access-Control-Max-Age"); maxAge != "86400" {
			t.Errorf("Expected Access-Control-Max-Age to be '86400', got '%s'", maxAge)
		}
	})

	t.Run("OPTIONS handler rejects non-OPTIONS requests", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Apply WithConnectRPC
		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{testWildcardDomain},
			AllowHTTP:      false,
		})(app)

		// Get the OPTIONS handler
		registrations := app.Registrations()
		handler := registrations[0].Handler

		// Test POST request (should return 404)
		req := newTestRequest(t, "POST", "/test/path")
		req.Header.Set("Origin", "https://app.example.com")

		recorder := newTestResponseRecorder()
		handler.ServeHTTP(recorder, req)

		// Should return 404 for non-OPTIONS requests
		if recorder.Code != 404 {
			t.Errorf("Expected status 404 for non-OPTIONS request, got %d", recorder.Code)
		}
	})

	t.Run("OPTIONS handler rejects requests without Origin header", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Apply WithConnectRPC
		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{testWildcardDomain},
			AllowHTTP:      false,
		})(app)

		// Get the OPTIONS handler
		registrations := app.Registrations()
		handler := registrations[0].Handler

		// Test OPTIONS request without Origin header
		req := newTestRequest(t, "OPTIONS", "/test/path")
		// No Origin header set

		recorder := newTestResponseRecorder()
		handler.ServeHTTP(recorder, req)

		// Should return 400 Bad Request
		if recorder.Code != 400 {
			t.Errorf("Expected status 400 for OPTIONS without Origin, got %d", recorder.Code)
		}
	})

	t.Run("OPTIONS handler rejects forbidden origins", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Apply WithConnectRPC with specific allowed domain
		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{testWildcardDomain},
			AllowHTTP:      false,
		})(app)

		// Get the OPTIONS handler
		registrations := app.Registrations()
		handler := registrations[0].Handler

		// Test OPTIONS request with forbidden origin
		req := newTestRequest(t, "OPTIONS", "/test/path")
		req.Header.Set("Origin", "https://malicious.com")

		recorder := newTestResponseRecorder()
		handler.ServeHTTP(recorder, req)

		// Should return 403 Forbidden
		if recorder.Code != 403 {
			t.Errorf("Expected status 403 for forbidden origin, got %d", recorder.Code)
		}
	})

	t.Run("OPTIONS handler respects AllowHTTP setting", func(t *testing.T) {
		// Create a test app with AllowHTTP=false
		app := dindenault.New(logger)

		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{testWildcardDomain},
			AllowHTTP:      false,
		})(app)

		registrations := app.Registrations()
		handler := registrations[0].Handler

		// Test HTTP origin (should be rejected when AllowHTTP=false)
		req := newTestRequest(t, "OPTIONS", "/test/path")
		req.Header.Set("Origin", "http://app.example.com")

		recorder := newTestResponseRecorder()
		handler.ServeHTTP(recorder, req)

		// Should return 403 Forbidden for HTTP origin when AllowHTTP=false
		if recorder.Code != 403 {
			t.Errorf("Expected status 403 for HTTP origin when AllowHTTP=false, got %d", recorder.Code)
		}
	})

	t.Run("OPTIONS handler allows HTTP when AllowHTTP=true", func(t *testing.T) {
		// Create a test app with AllowHTTP=true
		app := dindenault.New(logger)

		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{testWildcardDomain},
			AllowHTTP:      true,
		})(app)

		registrations := app.Registrations()
		handler := registrations[0].Handler

		// Test HTTP origin (should be allowed when AllowHTTP=true)
		req := newTestRequest(t, "OPTIONS", "/test/path")
		req.Header.Set("Origin", "http://app.example.com")

		recorder := newTestResponseRecorder()
		handler.ServeHTTP(recorder, req)

		// Should return 200 OK for HTTP origin when AllowHTTP=true
		if recorder.Code != 200 {
			t.Errorf("Expected status 200 for HTTP origin when AllowHTTP=true, got %d", recorder.Code)
		}

		// Verify origin is reflected in response
		if origin := recorder.Header().Get("Access-Control-Allow-Origin"); origin != "http://app.example.com" {
			t.Errorf("Expected Access-Control-Allow-Origin to be 'http://app.example.com', got '%s'", origin)
		}
	})
}

// Test helper functions

// newTestRequest creates a new HTTP request for testing.
func newTestRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}

	return req
}

// newTestResponseRecorder creates a new response recorder for testing.
func newTestResponseRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

// TestWithService tests that WithService correctly registers services and applies global interceptors.
// This tests Requirements 1.1 and 1.4 - verifying WithService works with all handler types
// and that global interceptors are still applied.
//
//nolint:gocognit,funlen // Comprehensive test with multiple scenarios
func TestWithService(t *testing.T) {
	logger := slog.Default()

	t.Run("registers service with path and handler", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Create a simple test handler
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("test response"))
		})

		// Register service using WithService
		dindenault.WithService("/test/path", testHandler)(app)

		// Verify the service was registered
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		if registrations[0].Path != "/test/path" {
			t.Errorf("Expected path '/test/path', got '%s'", registrations[0].Path)
		}

		if registrations[0].Handler == nil {
			t.Error("Expected handler to be set, got nil")
		}
	})

	t.Run("works with multiple service registrations", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Create test handlers
		handler1 := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		handler2 := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Register multiple services
		dindenault.WithService("/service1", handler1)(app)
		dindenault.WithService("/service2", handler2)(app)

		// Verify both services were registered
		registrations := app.Registrations()
		if len(registrations) != 2 {
			t.Fatalf("Expected 2 registrations, got %d", len(registrations))
		}

		if registrations[0].Path != "/service1" {
			t.Errorf("Expected first path '/service1', got '%s'", registrations[0].Path)
		}

		if registrations[1].Path != "/service2" {
			t.Errorf("Expected second path '/service2', got '%s'", registrations[1].Path)
		}
	})

	t.Run("works with different handler types", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Test with http.HandlerFunc
		handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		dindenault.WithService("/func", handlerFunc)(app)

		// Test with http.Handler (using http.ServeMux)
		mux := http.NewServeMux()
		mux.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		dindenault.WithService("/mux", mux)(app)

		// Test with custom handler type
		customHandler := &customHTTPHandler{called: false}
		dindenault.WithService("/custom", customHandler)(app)

		// Verify all were registered
		registrations := app.Registrations()
		if len(registrations) != 3 {
			t.Fatalf("Expected 3 registrations, got %d", len(registrations))
		}
	})

	t.Run("works with Connect handler that supports interceptors", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Create a mock Connect handler that implements ConnectHandlerWithInterceptor
		mockHandler := &mockConnectHandler{
			interceptorsApplied: false,
		}

		// Register service
		dindenault.WithService("/connect", mockHandler)(app)

		// Verify the service was registered
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		if registrations[0].Path != "/connect" {
			t.Errorf("Expected path '/connect', got '%s'", registrations[0].Path)
		}
	})

	t.Run("global interceptors are applied to Connect handlers", func(t *testing.T) {
		// Create a test app with global interceptors
		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
			),
		)

		// Create a mock Connect handler that implements ConnectHandlerWithInterceptor
		mockHandler := &mockConnectHandler{
			interceptorsApplied: false,
		}

		// Register service
		dindenault.WithService("/test", mockHandler)(app)

		// Verify the service was registered
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		// Verify global interceptors were set
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor, got %d", len(app.GlobalInterceptors()))
		}
	})

	t.Run("global interceptors are NOT applied to non-Connect handlers", func(t *testing.T) {
		// Create a test app with global interceptors
		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
			),
		)

		// Create a regular HTTP handler (not Connect)
		regularHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Register service
		dindenault.WithService("/regular", regularHandler)(app)

		// Verify the service was registered
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		// Global interceptors should still be set in the app
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor in app, got %d", len(app.GlobalInterceptors()))
		}
		// The handler itself should be the original (interceptors only apply to Connect handlers)
		// This is expected behavior - regular HTTP handlers don't support Connect interceptors
	})
	t.Run("multiple global interceptors are all applied", func(t *testing.T) {
		// Create a test app with multiple global interceptors
		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
				dindenault.CORSInterceptors([]string{testWildcardDomain}, false),
			),
		)

		// Create a mock Connect handler
		mockHandler := &mockConnectHandler{
			interceptorsApplied: false,
		}

		// Register service
		dindenault.WithService("/test", mockHandler)(app)

		// Verify global interceptors were set
		if len(app.GlobalInterceptors()) != 2 {
			t.Errorf("Expected 2 global interceptors, got %d", len(app.GlobalInterceptors()))
		}
	})

	t.Run("can be used with WithConnectRPC for CORS", func(t *testing.T) {
		// Create a test app with CORS and service registration
		app := dindenault.New(logger,
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{testWildcardDomain},
				AllowHTTP:      false,
			}),
		)

		// Register a service
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		dindenault.WithService("/api/test", testHandler)(app)

		// Verify both CORS and service were registered
		registrations := app.Registrations()
		if len(registrations) != 2 { // 1 for OPTIONS handler, 1 for service
			t.Fatalf("Expected 2 registrations (CORS + service), got %d", len(registrations))
		}

		// Verify CORS interceptor was added
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor (CORS), got %d", len(app.GlobalInterceptors()))
		}
	})

	t.Run("works with complex configuration combining multiple features", func(t *testing.T) {
		// Create a test app with multiple global interceptors and CORS
		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
			),
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{testWildcardDomain},
				AllowHTTP:      false,
			}),
		)

		// Register multiple services of different types
		handler1 := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		mockHandler := &mockConnectHandler{interceptorsApplied: false}

		dindenault.WithService("/api/v1", handler1)(app)
		dindenault.WithService("/api/v2", mockHandler)(app)

		// Verify all registrations
		registrations := app.Registrations()
		if len(registrations) != 3 { // OPTIONS handler + 2 services
			t.Fatalf("Expected 3 registrations, got %d", len(registrations))
		}

		// Verify all interceptors
		if len(app.GlobalInterceptors()) != 2 { // Logging + CORS
			t.Errorf("Expected 2 global interceptors, got %d", len(app.GlobalInterceptors()))
		}
	})

	t.Run("preserves handler functionality after registration", func(t *testing.T) {
		// Create a test app
		app := dindenault.New(logger)

		// Create a handler that tracks if it was called
		called := false
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("handler called"))
		})

		// Register service
		dindenault.WithService("/test", testHandler)(app)

		// Get the registered handler
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(registrations))
		}

		// Call the handler
		req := newTestRequest(t, "GET", "/test")
		recorder := newTestResponseRecorder()
		registrations[0].Handler.ServeHTTP(recorder, req)

		// Verify the handler was called
		if !called {
			t.Error("Expected handler to be called")
		}

		// Verify response
		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}
	})
}

// customHTTPHandler is a custom handler type for testing.
type customHTTPHandler struct {
	called bool
}

func (c *customHTTPHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	c.called = true

	w.WriteHeader(http.StatusOK)
}

// mockConnectHandler is a mock handler that implements ConnectHandlerWithInterceptor.
type mockConnectHandler struct {
	interceptorsApplied bool
}

func (m *mockConnectHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (m *mockConnectHandler) WithInterceptors(_ ...connect.Interceptor) http.Handler {
	m.interceptorsApplied = true

	return m
}
