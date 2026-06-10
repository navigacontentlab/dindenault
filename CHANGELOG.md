# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `mcp.Tool.RequiredPermissions` — per-tool organization-level permission
  checks enforced by the MCP server before the handler runs. Tools with
  requirements reject unauthenticated calls.
- `MIGRATION.md` with a 1.3 → 1.4 upgrade guide.
- "Security Model" section in README documenting JWT validation rules,
  fail-fast behavior, and the caller's authorization responsibilities.
- Godoc examples (`example_test.go`).

### Changed
- `mcp.AuthorizationFromContext` falls back to reconstructing the bearer
  token from validated `navigaid` auth info in the context, unifying the
  two token channels — it now works with any of dindenault's
  authentication entry points.
- `WithTelemetry` now actually applies the provider's interceptor to
  handlers that support interceptors (previously the stored provider was
  never used by the App). Best-effort: a warning is logged for handlers
  that cannot receive it.

### Deprecated
- `NewConnectHandler`, `ConnectOptions`, `ConnectOption`,
  `WithRequiredPermissions`, `WithUnitPermissions` — pass interceptors at
  handler creation with `connect.WithInterceptors` instead.
- `CORSInterceptors` — use `WithConnectRPC` (HTTP-level CORS middleware
  with preflight handling) or `cors.Middleware`.

## [1.4.0] - 2026-06-10

### Security (BREAKING)
- App-level interceptors (`WithInterceptors`) now fail fast: the app panics at
  startup if a registered handler cannot receive the configured interceptors,
  instead of silently running the handler without them (previously this could
  leave services completely unauthenticated). Use `connect.WithInterceptors`
  at handler creation, or the new `WithPlainService` for handlers that should
  not receive interceptors.
- CORS validation now parses the Origin header and matches the hostname on
  label boundaries — `example.com` no longer matches `evilexample.com`.
- CORS no longer sets `Access-Control-Allow-Credentials` when the wildcard
  `"*"` is configured.
- `HasPermission`/`AuthorizeWithDetails` now only accept organization-level
  permissions; a permission granted in a single unit no longer grants
  organization-wide access. Use the new `HasPermissionInUnit` for unit-scoped
  checks.
- JWT validation requires an expiration claim (`exp`) and supports optional
  issuer/audience validation via `navigaid.WithExpectedIssuer` and
  `navigaid.WithExpectedAudience`.
- Fixed panic (DoS) when validating a JWT without a `kid` header.
- Internal error details are no longer returned in HTTP 500 response bodies.
- `Authorization`, `x-imid-token`, and `Cookie` headers are redacted in debug
  logging.
- `AccessTokenService.NewAccessToken` now verifies the HTTP status code and
  rejects empty tokens.

### Changed (BREAKING)
- Migrated from `github.com/golang-jwt/jwt/v4` to `v5`. `navigaid.Claims` no
  longer has a `Valid()` method.
- `WithConnectRPC` applies CORS as HTTP middleware around all registered
  handlers (including preflight) instead of registering a catch-all OPTIONS
  handler and a Connect interceptor.
- `WithPathPermissionService` now takes a `*navigaid.JWKS` and authenticates
  requests itself (the previous design could never authenticate, since
  Connect interceptors cannot run on plain HTTP handlers).
- Path permission matching uses the longest (most specific) matching prefix
  instead of first match, and is case-insensitive to match request routing.
- `AuthResult.Permissions` now contains organization-level permissions only;
  unit permissions are exposed in the new `AuthResult.UnitPermissions` map.
- Removed dead code: `navigaid.WithTokenRefresh`, `navigaid.AddAnnotation`,
  `navigaid.AddUserAnnotation`.

### Added
- `WithPlainService` for registering plain HTTP handlers that opt out of
  app-level interceptors.
- `navigaid.HTTPMiddleware` — JWT authentication middleware for plain HTTP
  handlers.
- `dindenault.HasPermissionInUnit` for unit-scoped permission checks.
- `cors.Middleware` — HTTP-level CORS middleware with preflight handling.

### Fixed
- JWKS and access-token HTTP clients now have a 10 s timeout (previously no
  timeout — a hung IMAS endpoint could stall requests until the Lambda
  timed out).
- JWKS refresh failures fall back to previously fetched keys with a 30 s
  retry backoff instead of failing all authentication.
- `TokenRefresher` evicts expired tokens so its cache cannot grow without
  bound.
- `Handle`/`HandleAPIGateway` can both be called on the same App without
  applying interceptors twice; handler sorting now happens once at startup
  instead of on every request.
- `WithMCPAuth` panics on empty `imasURL` instead of silently configuring a
  broken JWKS endpoint.

## [1.0.0] - 2026-01-14

### Added
- GitHub Actions workflows for CI and automated releases
- CI workflow runs tests and linting on all branches
- Release workflow for automated version tagging via GitHub UI
- Makefile for manual release management with semantic versioning
- CHANGELOG.md following Keep a Changelog format
- Comprehensive integration tests for service registration and CORS
- Documentation for release process and contributing guidelines
- Support for golangci-lint v2.8.0
- asdf version manager setup instructions in README

### Changed
- Upgraded golangci-lint from v1.64 to v2.8.0
- Updated `.golangci.yml` to v2 configuration format
  - Moved formatters (gofmt, goimports) to separate `formatters` section
  - Moved linter settings to `linters.settings`
  - Removed deprecated linters (typecheck, gosimple, stylecheck, tenv)
  - Disabled deprecated `wsl` linter (replaced by wsl_v5 in future)
- Updated GitHub Actions to use golangci-lint-action v7 for v2 support
- Enhanced README.md with comprehensive documentation
  - golangci-lint v2.8.0 usage and configuration
  - Release process (automated and manual)
  - Contributing guidelines with testing and linting instructions
  - Version manager setup for managing multiple golangci-lint versions

### Fixed
- All linting issues resolved (0 issues)
- Added missing package and function comments
- Fixed unused parameter warnings
- Added nolint directives for acceptable test complexity
- Proper formatting with gofmt and goimports

## Guidelines

### For Maintainers

When making changes, add them under the `[Unreleased]` section using these categories:

- **Added** for new features
- **Changed** for changes in existing functionality
- **Deprecated** for soon-to-be removed features
- **Removed** for now removed features
- **Fixed** for any bug fixes
- **Security** for vulnerability fixes

When creating a release:
1. Change `[Unreleased]` to the new version number with date: `[1.2.3] - 2026-01-14`
2. Add a new `[Unreleased]` section at the top
3. Update the version comparison links at the bottom

### Version Links

[Unreleased]: https://github.com/navigacontentlab/dindenault/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.0.0
