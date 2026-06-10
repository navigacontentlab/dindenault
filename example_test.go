package dindenault_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/cors"
	"github.com/navigacontentlab/dindenault/mcp"
	"github.com/navigacontentlab/dindenault/navigaid"
)

// Example shows a minimal app with a single Connect service.
// Interceptors are passed at handler creation, which works with any
// connect-go generated handler.
func Example() {
	logger := slog.Default()

	// In a real service:
	//
	//	path, handler := servicev1connect.NewServiceHandler(
	//	    impl,
	//	    connect.WithInterceptors(
	//	        dindenault.LoggingInterceptors(logger),
	//	        dindenault.AuthInterceptors(logger, "https://imas.example.com"),
	//	    ),
	//	)
	var (
		path    = "/service.v1.Service/"
		handler http.Handler
	)

	app := dindenault.New(logger,
		dindenault.WithService(path, handler),
	)

	// lambda.Start(app.Handle())
	_ = app
}

// ExampleWithConnectRPC enables CORS for browser-facing services.
// CORS is applied as HTTP middleware around all registered handlers,
// including preflight handling.
func ExampleWithConnectRPC() {
	logger := slog.Default()

	var handler http.Handler

	app := dindenault.New(logger,
		dindenault.WithConnectRPC(cors.Options{
			AllowedDomains: []string{".mycompany.com"},
			AllowHTTP:      false,
		}),
		dindenault.WithService("/service.v1.Service/", handler),
	)

	_ = app
}

// ExampleWithMCPAuth exposes MCP tools to AI agents with JWT
// authentication and a per-tool permission requirement.
func ExampleWithMCPAuth() {
	logger := slog.Default()

	app := dindenault.New(logger,
		dindenault.WithMCPAuth("/mcp", logger, "https://imas.example.com", nil,
			mcp.Tool{
				Name:                "write_document",
				Description:         "Create or update a document",
				RequiredPermissions: []string{"content:write"},
				Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
					// Forward the caller's token to downstream services.
					client := mcp.NewHTTPClient(ctx, nil)
					_ = client

					return json.Marshal(map[string]any{"ok": true})
				},
			},
		),
	)

	_ = app
}

// ExampleWithPlainService registers a handler that intentionally does
// not receive app-level interceptors, authenticated with the HTTP
// middleware instead.
func ExampleWithPlainService() {
	logger := slog.Default()
	jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint("https://imas.example.com"))

	webhook := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, err := navigaid.GetAuth(r.Context())
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)

			return
		}

		_ = auth.Claims.Org
		w.WriteHeader(http.StatusOK)
	})

	app := dindenault.New(logger,
		dindenault.WithPlainService("/webhook", navigaid.HTTPMiddleware(logger, jwks, webhook)),
	)

	_ = app
}

// ExampleHasPermissionInUnit checks a unit-scoped permission. Org-level
// checks (HasPermission) are not satisfied by unit-scoped grants.
func ExampleHasPermissionInUnit() {
	ctx := context.Background() // carries auth info in a real handler

	if dindenault.HasPermissionInUnit(ctx, "HQ", "admin:access") {
		// unit-scoped admin operation
		_ = ctx
	}
}
