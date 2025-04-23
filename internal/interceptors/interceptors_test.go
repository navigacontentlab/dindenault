package interceptors_test

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/navigacontentlab/dindenault/internal/interceptors"
)

func TestInterceptors(t *testing.T) {
	// This test is just checking that the interceptors can be created without panicking
	t.Helper()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Test that we can create interceptors without errors
	_ = interceptors.Logging(logger)
	_ = interceptors.XRay("test-service")
	_ = interceptors.OpenTelemetry("test-service")
	_ = interceptors.CORS([]string{"example.com"}, false)
}

// MockHandler implements http.Handler.
type MockHandler struct{}

func (h *MockHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// MockInterceptor is a simple Connect interceptor for testing.
type MockInterceptor struct {
	called bool
}

// Intercept implements connect.Interceptor.
func (i *MockInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		i.called = true

		return next(ctx, req)
	}
}
