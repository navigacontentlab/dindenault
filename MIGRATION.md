# Migration Guide

## 1.3.x → 1.4.0

Version 1.4.0 contains security hardening with behavior changes. Most
services — in particular MCP services built on `WithMCPAuth` — need **no
code changes**. Read through the sections below and update the patterns
that apply to you.

### Quick checklist

- [ ] Do you call `dindenault.WithInterceptors`? → see [App-level interceptors fail fast](#app-level-interceptors-fail-fast)
- [ ] Do you call `WithPathPermissionService`? → see [WithPathPermissionService](#withpathpermissionservice-authenticates-itself)
- [ ] Do you call `HasPermission` or `AuthorizeWithDetails`? → see [Permission scope](#permission-checks-are-organization-scoped)
- [ ] Do you read `AuthResult.Permissions`? → see [AuthResult](#authresult-exposes-unit-permissions-separately)
- [ ] Do you import `golang-jwt/jwt` directly or craft test tokens? → see [JWT v5](#jwt-library-upgraded-to-v5)
- [ ] Do you use `navigaid.WithTokenRefresh`, `navigaid.AddAnnotation`, or `navigaid.AddUserAnnotation`? → see [Removed APIs](#removed-apis)
- [ ] Is `IMAS_URL` (or equivalent) set in every environment? `WithMCPAuth` and `AuthInterceptors` now panic at startup on an empty URL.

### App-level interceptors fail fast

`WithInterceptors` previously logged a warning and **silently skipped**
interceptors for handlers that don't implement
`ConnectHandlerWithInterceptor` — which standard connect-go generated
handlers don't. If your auth interceptor was configured this way, your
service was running **without authentication**.

The app now panics at startup instead.

Before (silently unauthenticated):

```go
path, handler := servicev1connect.NewServiceHandler(impl)

app := dindenault.New(logger,
    dindenault.WithInterceptors(
        dindenault.AuthInterceptors(logger, imasURL), // never applied!
    ),
    dindenault.WithService(path, handler),
)
```

After (interceptors at handler creation):

```go
path, handler := servicev1connect.NewServiceHandler(
    impl,
    connect.WithInterceptors(
        dindenault.AuthInterceptors(logger, imasURL),
    ),
)

app := dindenault.New(logger,
    dindenault.WithService(path, handler),
)
```

Handlers that intentionally should not receive interceptors (health
checks, webhooks) are registered with the new `WithPlainService`:

```go
app := dindenault.New(logger,
    dindenault.WithInterceptors(...),
    dindenault.WithPlainService("/health", healthHandler),
)
```

### CORS is HTTP middleware

`WithConnectRPC` keeps its signature but now wraps every registered
handler in HTTP-level CORS middleware instead of registering a global
Connect interceptor plus a catch-all OPTIONS handler. No code changes
are needed, but behavior differs:

- Preflight responses are `204 No Content` (previously `200 OK`).
- An OPTIONS request without an `Origin` header is passed to your
  handler (previously `400 Bad Request`).
- With wildcard `"*"` domains, `Access-Control-Allow-Credentials` is no
  longer sent.
- Origin matching is host-based with label boundaries: `example.com`
  matches `app.example.com` but no longer matches `evilexample.com`.
  Verify your `AllowedDomains` entries are real domain suffixes.

### WithPathPermissionService authenticates itself

The old design asked you to combine `WithPathPermissionService` with
`WithInterceptors(AuthInterceptors(...))` — which could never work,
since Connect interceptors don't run on plain HTTP handlers. The
function now takes a JWKS and authenticates every request itself:

```go
jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint(imasURL))

app := dindenault.New(logger,
    dindenault.WithPathPermissionService("/api/", jwks, apiHandler,
        []dindenault.PathPermissionConfig{
            {PathPrefix: "/api/admin", Permissions: []string{"admin:access"}},
        },
    ),
)
```

Path matching now picks the longest (most specific) matching prefix
instead of the first match, and is case-insensitive.

### Permission checks are organization-scoped

`HasPermission` and `AuthorizeWithDetails` previously accepted a
permission granted in *any* unit as if it were organization-wide. They
now check organization-level permissions only. For unit-scoped checks,
use the new function:

```go
// Org-level
if dindenault.HasPermission(ctx, "content:write") { ... }

// Unit-scoped (direct grant or inherited from org)
if dindenault.HasPermissionInUnit(ctx, "HQ", "content:write") { ... }
```

### AuthResult exposes unit permissions separately

`AuthResult.Permissions` no longer flattens unit permissions into the
list — it contains organization-level permissions only. Unit
permissions are available with their unit context preserved:

```go
result, _ := dindenault.AuthorizeWithDetails(ctx, "")
orgPerms := result.Permissions              // org-level only
hqPerms := result.UnitPermissions["HQ"]     // per unit
```

### JWT library upgraded to v5

dindenault now uses `github.com/golang-jwt/jwt/v5` internally.

- `navigaid.Claims` no longer has a `Valid()` method.
- Tokens **must** carry an `exp` claim; tokens without one are rejected.
  IMAS always issues `exp`, but hand-crafted test tokens may need
  updating.
- Issuer/audience validation is available (opt-in):

```go
jwks := navigaid.NewJWKS(navigaid.ImasJWKSEndpoint(imasURL),
    navigaid.WithExpectedIssuer("https://imas.example.com"),
    navigaid.WithExpectedAudience("my-service"),
)
```

If you don't import the jwt library yourself, `go get
github.com/navigacontentlab/dindenault@v1.4.0 && go mod tidy` (plus
`go mod vendor` if you vendor) is all that's needed.

### Removed APIs

| Removed | Replacement |
|---|---|
| `navigaid.WithTokenRefresh` | Was a non-functional placeholder. Use `TokenRefresher.GetAccessToken` directly. |
| `navigaid.AddAnnotation` | Was a no-op. Use your telemetry provider's API (e.g. `awsxray.AddAnnotation`). |
| `navigaid.AddUserAnnotation` | Same as above. |

### Operational changes (no code changes)

- JWKS and token-endpoint HTTP clients default to a 10 s timeout.
- A failed JWKS refresh falls back to previously fetched keys with a
  30 s retry backoff, instead of failing all authentication.
- HTTP 500 bodies no longer include internal error details (check logs
  instead).
- `Authorization`, `x-imid-token`, and `Cookie` headers are redacted in
  debug logging.
- `WithMCPAuth` and `AuthInterceptors` panic at startup when the IMAS
  URL is empty — a misconfigured environment now fails the deploy
  instead of every request.
