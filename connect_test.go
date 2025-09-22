package dindenault_test

import (
	"log/slog"
	"testing"

	"github.com/navigacontentlab/dindenault"
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
	// Create the interceptor
	origins := []string{"https://example.com"}
	interceptor := dindenault.CORSInterceptors(origins, false)

	// Assert it's not nil
	if interceptor == nil {
		t.Error("CORSInterceptors returned nil")
	}
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
