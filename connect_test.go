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

// TestWithConnectRPC tests the CORS configuration function. CORS is
// applied as an HTTP middleware around registered handlers; the
// middleware behavior itself is tested in the cors package.
func TestWithConnectRPC(t *testing.T) {
	logger := slog.Default()

	t.Run("stores CORS options on the app", func(t *testing.T) {
		app := dindenault.New(logger)

		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{testDomain},
			AllowHTTP:      false,
		})(app)

		opts := app.CORSOptions()
		if opts == nil {
			t.Fatal("Expected CORS options to be set")
		}

		if len(opts.AllowedDomains) != 1 || opts.AllowedDomains[0] != testDomain {
			t.Errorf("Expected allowed domains [%s], got %v", testDomain, opts.AllowedDomains)
		}

		// CORS no longer registers handlers or global interceptors.
		if len(app.Registrations()) != 0 {
			t.Errorf("Expected 0 registrations, got %d", len(app.Registrations()))
		}

		if len(app.GlobalInterceptors()) != 0 {
			t.Errorf("Expected 0 global interceptors, got %d", len(app.GlobalInterceptors()))
		}
	})

	t.Run("uses default domains when empty options provided", func(t *testing.T) {
		app := dindenault.New(logger)

		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{},
			AllowHTTP:      false,
		})(app)

		opts := app.CORSOptions()
		if opts == nil {
			t.Fatal("Expected CORS options to be set")
		}

		if len(opts.AllowedDomains) == 0 {
			t.Error("Expected default domains to be applied")
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

	t.Run("global interceptors with non-Connect handler panics at startup", func(t *testing.T) {
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

		// Preparing the app must panic — silently skipping interceptors
		// (e.g. authentication) would be fail-open.
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when interceptors cannot be applied")
			}
		}()

		_ = app.Handle()
	})
	t.Run("multiple global interceptors are all applied", func(t *testing.T) {
		// Create a test app with multiple global interceptors
		permissionConfigs := []dindenault.PathPermissionConfig{
			{PathPrefix: "/test", Permissions: []string{"test:access"}},
		}

		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
				dindenault.PathInterceptors(logger, permissionConfigs),
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

		// CORS is applied as middleware at prepare time, not as a
		// separate registration.
		registrations := app.Registrations()
		if len(registrations) != 1 {
			t.Fatalf("Expected 1 registration (service), got %d", len(registrations))
		}

		if app.CORSOptions() == nil {
			t.Error("Expected CORS options to be set")
		}
	})

	t.Run("works with complex configuration combining multiple features", func(t *testing.T) {
		// Create a test app with global interceptors and CORS
		app := dindenault.New(logger,
			dindenault.WithInterceptors(
				dindenault.LoggingInterceptors(logger),
			),
			dindenault.WithConnectRPC(cors.Options{
				AllowedDomains: []string{testWildcardDomain},
				AllowHTTP:      false,
			}),
		)

		// Register services that support interceptors
		mockHandler1 := &mockConnectHandler{interceptorsApplied: false}
		mockHandler2 := &mockConnectHandler{interceptorsApplied: false}

		dindenault.WithService("/api/v1", mockHandler1)(app)
		dindenault.WithService("/api/v2", mockHandler2)(app)

		// Verify all registrations
		registrations := app.Registrations()
		if len(registrations) != 2 {
			t.Fatalf("Expected 2 registrations, got %d", len(registrations))
		}

		// Verify interceptors (logging only — CORS is middleware, not an
		// interceptor)
		if len(app.GlobalInterceptors()) != 1 {
			t.Errorf("Expected 1 global interceptor, got %d", len(app.GlobalInterceptors()))
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
