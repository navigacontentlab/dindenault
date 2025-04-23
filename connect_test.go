package dindenault_test

import (
	"log/slog"
	"testing"

	"github.com/navigacontentlab/dindenault"
)

func TestWithInterceptors(t *testing.T) {
	// Create a test logger
	logger := slog.Default()

	// Create a test app
	app := dindenault.New(logger)

	// Create test interceptors
	testInterceptor1 := dindenault.LoggingInterceptors(logger)
	testInterceptor2 := dindenault.XRayInterceptors("test-service")

	// Apply the WithInterceptors option
	dindenault.WithInterceptors(testInterceptor1, testInterceptor2)(app)

	// Check that interceptors were added
	if len(app.GlobalInterceptors()) != 2 {
		t.Errorf("Expected 2 interceptors, got %d", len(app.GlobalInterceptors()))
	}
}

func TestLoggingInterceptors(t *testing.T) {
	// Create a test logger
	logger := slog.Default()

	// Create the interceptor
	interceptor := dindenault.LoggingInterceptors(logger)

	// Assert it's not nil
	if interceptor == nil {
		t.Error("LoggingInterceptors returned nil")
	}
}

func TestXRayInterceptors(t *testing.T) {
	// Create the interceptor
	interceptor := dindenault.XRayInterceptors("test-service")

	// Assert it's not nil
	if interceptor == nil {
		t.Error("XRayInterceptors returned nil")
	}
}

func TestOpenTelemetryInterceptors(t *testing.T) {
	// Create the interceptor
	interceptor := dindenault.OpenTelemetryInterceptors("test-service")

	// Assert it's not nil
	if interceptor == nil {
		t.Error("OpenTelemetryInterceptors returned nil")
	}
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
	permissions := []string{"test:permission"}
	interceptor := dindenault.AuthInterceptors("https://imas.example.com", permissions)

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
	xrayInterceptor := dindenault.XRayInterceptors("test-service")

	// Create a test app
	app := dindenault.New(logger)

	// Add the interceptors
	dindenault.WithInterceptors(loggingInterceptor, xrayInterceptor)(app)

	// Check the number of interceptors
	if len(app.GlobalInterceptors()) != 2 {
		t.Errorf("Expected 2 interceptors, got %d", len(app.GlobalInterceptors()))
	}
}
