# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.16.0] - 2026-11-25

### Added
- `mcp.Tool.ContentHandler` returns a tools/call result of several content
  blocks, built with `mcp.TextContent` and `mcp.ImageContent` (base64,
  `mimeType`). A tool can send a thumbnail next to its JSON, and clients that
  show images from tools display it. `Handler` works as before, its JSON as
  the one text block; when both are set `ContentHandler` is used. A tool with
  neither now answers with a tool error instead of panicking.

## [1.15.0] - 2026-11-25

### Added
- `mcp.ServerInfo` takes `WebsiteURL` and `Icons` (`mcp.Icon`: `src`,
  `mimeType`, `sizes`, `theme`), sent in `serverInfo` as `websiteUrl` and
  `icons` (spec 2025-11-25) when set. Clients such as ChatGPT show the icon
  and website instead of a generic one.

## [1.14.0] - 2026-11-20

### Security
- `navigaid.ConnectInterceptor` (and so `AuthInterceptors`), `PathInterceptors`,
  `navigaid.RequirePermission` and `navigaid.RequireUnitPermission` now also
  check streaming RPCs (server, client and bidi streaming). They were built on
  `connect.UnaryInterceptorFunc`, which passes streaming handlers through
  untouched, so a streaming RPC ran without authentication or permission
  checks. Unary calls behave as before.
- `navigaid.NewHTTPClient` and `mcp.NewHTTPClient` no longer send the caller's
  token on a redirect to another scheme, host or port than the request started
  with. The transport used to put the token back on every hop, undoing the
  `http.Client` rule that drops `Authorization` on such redirects. Without
  auth in the context they no longer overwrite an `Authorization` header the
  caller set themselves.
- `WithPathPermissionService` checks a path with `..`, `.` or repeated
  slashes against the rules for both the path as received and its cleaned
  form, and requires both. `/api/users/../admin` no longer passes on the
  strength of a `/api/users` rule.
- The App routes on the decoded request path, the one the handlers see. An
  encoded path such as `/%61dmin/` used to skip the `/admin/` registration and
  reach a less specific one.
- `mcp.AuthMiddleware` reads at most `mcp.MaxRequestBytes` (10 MiB) and answers
  413 above that. Lambda's own limits are lower, so Lambda services see no
  change.

### Added
- `AuthInterceptors` takes optional `navigaid.JWKSOption`s, e.g.
  `navigaid.WithExpectedIssuer` or `navigaid.WithExpectedAudience`.
- `WithMCPAuthJWKS`: `WithMCPAuth` with a JWKS you build yourself, for the same
  issuer and audience checks.
- `navigaid.NoTokenError` and `navigaid.NewAccessTokenService`.

### Deprecated
- `navigaid.ErrNoToken` (now an alias of `NoTokenError`) and `navigaid.New`
  (calls `NewAccessTokenService`). Both keep working.

### Changed
- **Go 1.26 is required** (`go 1.26.0`, toolchain go1.26.11) for the root,
  otel and xray modules. Go 1.25 no longer gets security fixes; the standard
  library issues govulncheck reported (GO-2026-6603 to -6617, -6809, -6859)
  are fixed only in Go 1.26. Services must build with Go 1.26.11 or later to
  get those fixes.
- Dependencies held back for Go 1.25 are upgraded: connect v1.21.1,
  aws-lambda-go v1.55.1, OpenTelemetry v1.47.0 / contrib v0.72.0 (otel),
  golang.org/x/net v0.63.0 (otel, xray), plus the rest of each module's
  dependencies. CI, release and govulncheck (now blocking again, v1.10.0)
  run on Go 1.26; golangci-lint v2.12.0.
- Permission checks no longer build a map per call, path prefixes are
  lowercased once instead of per request, and request debug logging (which
  copies the headers) only runs when debug logging is on.
- `mcp.Input.Elicited` documents that answers are not signed and are not a
  security confirmation.
- xray: `github.com/andybalholm/brotli` v1.3.0 and
  `github.com/klauspost/compress` v1.20.4 (fix GO-2026-6888, GO-2026-6865).

## [1.13.0] - 2026-11-19

### Added
- `cors.Options`: `AllowedHosts` (exact hosts, no subdomains),
  `AllowLoopback` (http and https on localhost, 127.0.0.1 and [::1]),
  `AllowHeaders` and `ExposeHeaders` (added to the defaults) and
  `NoCredentials` (no `Access-Control-Allow-Credentials`, for bearer-token
  services). `Options.AllowsOrigin` reports whether an origin is allowed.
- CORS allows the MCP Streamable HTTP headers by default (`MCP-Protocol-Version`,
  `Mcp-Method`, `Mcp-Name`, `Mcp-Session-Id`, `Last-Event-ID`) and exposes
  `WWW-Authenticate`, `MCP-Protocol-Version` and `Mcp-Session-Id`, so an MCP
  client in a browser can read a 401's `resource_metadata`.

### Fixed
- ALB query parameters are decoded once. ALB passes them to Lambda still
  URL-encoded, and they were encoded again, so `r.URL.Query()` returned
  e.g. `https%3A%2F%2Fclaude.ai%2F…`. API Gateway v2 is unchanged. A service
  that decodes them itself (naviga-oc-mcp's `albquery`) must stop doing so
  when it upgrades.
- CORS refuses origins with a path, query or user info, and `null`.
- `WithConnectRPC` falls back to `DefaultDomains` only when neither
  `AllowedDomains` nor `AllowedHosts` is set.

## [1.12.0] - 2026-11-18

### Added
- MCP: tools can ask the user (2026-07-28 multi round-trip requests, SEP-2322).
  A handler returns `mcp.RequireInput(state, elicitations)` and the call is
  answered with an `InputRequiredResult` (`resultType: "input_required"`,
  form-mode `elicitation/create` requests in `inputRequests`, a signed
  `requestState`). On the retry `mcp.InputFromContext(ctx)` gives the state
  back (`State`) and the answers (`Elicited`, an `ElicitResult`).
- `mcp.Server.WithInputRequired(key, ttl)` and
  `dindenault.WithMCPInputRequired(key, ttl)` turn it on. `requestState` is
  signed with HMAC-SHA256 and bound to the tool, the arguments, the caller's
  org and subject and an expiry (`mcp.DefaultRequestStateTTL`, 15 minutes); a
  state that fails is refused with `-32602` and HTTP 400.
- `Input.CanElicit` is true only for 2026-07-28 requests whose client
  capabilities declare form elicitation, on a server with a key; otherwise the
  tool gives its usual answer. `RequireInput` without it becomes the tool error
  `mcp.ErrInputNotSupported`.
- Audit outcome `input_required`.

## [1.11.0] - 2026-11-12

### Added
- MCP: the stateless 2026-07-28 revision, on the same endpoint as the
  initialize-based ones. A request whose `params._meta` carries
  `io.modelcontextprotocol/protocolVersion` is served by the new rules:
  `server/discover`; `MCP-Protocol-Version`, `Mcp-Method` and (for
  `tools/call`) `Mcp-Name` headers must match the body (`-32020`, HTTP 400;
  base64 `Mcp-Name` values are decoded); unknown versions get `-32022` with
  `data.supported`; `_meta.io.modelcontextprotocol/clientCapabilities` is
  required; results carry `resultType: "complete"`, and `tools/list` adds
  `ttlMs` and `cacheScope: "public"`; unknown and removed methods (such as
  `ping`) get `-32601` with HTTP 404.
- MCP: `ping` for initialize-based clients (2025-03-26 to 2025-11-25). It was
  missing, so directories that measure latency with it saw an error.
- MCP: every `notifications/*` message without an id is answered with 202, not
  only `notifications/initialized`.
- `AuthMiddleware` lets `ping` and `server/discover` through without a token,
  like `initialize` and `tools/list`.

### Unchanged
- `initialize` still negotiates 2025-11-25, 2025-06-18 or 2025-03-26; a client
  asking it for 2026-07-28 gets 2025-11-25, as the spec requires (initialize
  always selects the legacy rules).

## [1.10.1] - 2026-10-31

### Removed
- OAuth scopes for MCP tools, added in 1.10.0: `mcp.Tool.RequiredScopes`,
  `mcp.WithScopes`, `mcp.ContextWithGrantedScopes` and
  `mcp.GrantedScopesFromContext`. v1.10.0 is retracted; tools are again
  listed and callable for every authenticated caller.

## [1.10.0] - 2026-10-28

### Added
- OAuth scopes for MCP tools. `mcp.Tool.RequiredScopes` lists the scopes a
  caller's token must all have been granted. `tools/list` leaves out the tools
  the granted scopes do not cover, and a `tools/call` to one gets HTTP 403 with
  `WWW-Authenticate: Bearer error="insufficient_scope", scope="<granted +
  missing>", resource_metadata="…"` (`resource_metadata` only with
  `WithChallenge`), audited as `denied` with `missing scopes: […]`. The
  `scope` value keeps what was already granted, so the client's new login
  does not lose it.
- `mcp.WithScopes(func(r *http.Request) []string)`: tells `AuthMiddleware`
  which scopes the validated token was granted (the claims are already in
  `r`'s context). `mcp.ContextWithGrantedScopes` sets them from another auth
  layer, and `mcp.GrantedScopesFromContext` reads them in a handler.
- A tool with `RequiredScopes` fails closed: without granted scopes in the
  context, for example when `WithScopes` is missing or the tool is exempted
  with `WithPublicTools`, it is hidden and refused. Tools without
  `RequiredScopes` behave as before.

## [1.9.1] - 2026-10-27

### Fixed
- MCP audit: a `tools/call` rejected by `AuthMiddleware` (`unauthenticated`)
  recorded `read_only: false` even for a tool annotated `mcp.ReadOnly()`. The
  middleware now looks the tool up in the wrapped `*Server`, so the field
  matches what an accepted call records. Unknown tools and handlers that are
  not a `*Server` still record `false`.

## [1.9.0] - 2026-10-27

### Added
- `mcp.WithChallenge(metadataURL)`: `AuthMiddleware` requires a valid token
  on every request, `initialize` and `tools/list` included, and answers with
  `WWW-Authenticate: Bearer resource_metadata="…"` (plus
  `error="invalid_token"` for a refused token), as the MCP authorization spec
  asks. MCP clients such as Claude and VS Code only start their OAuth login on
  that challenge. Without the option nothing changes.
- `mcp.WithClaimsCheck(check)`: an extra check on the validated claims, for
  example that the token belongs to the organisation in the hostname. A
  refusal is answered and audited like an invalid token.

## [1.8.2] - 2026-10-15

### Added
- "Any of" permission checks (OR) alongside the existing "all of" lists:
  `PathPermissionConfig.AnyPermissions` (for `PathInterceptors` and
  `WithPathPermissionService`) and `mcp.Tool.AnyPermissions` require at least
  one of the listed organization-level permissions. When combined with
  `Permissions`/`RequiredPermissions`, both conditions must hold.
- `dindenault.HasAnyPermission`, `dindenault.AuthorizeWithAnyPermission`, and
  `navigaid.Claims.HasAnyPermissionInOrganisation`/`HasAnyPermissionInUnit`.
  Unit-scoped grants still do not satisfy organization-level checks.

## [1.8.1] - 2026-10-13

### Changed
- MCP `initialize` negotiates the protocol version instead of always answering
  `2025-03-26`. The server now speaks `2025-11-25`, `2025-06-18` and
  `2025-03-26` and echoes the requested version when it is one of them;
  otherwise it answers with the newest supported version that is not newer than
  the request. Clients that send nothing usable, or an older version such as
  `2024-11-05`, still get `2025-03-26`. `serverInfo.description` only exists in
  spec `2025-11-25`, so registries that read it by negotiated version now see
  it. `Origin` validation and `MCP-Protocol-Version` header checks (both asked
  of servers by the newer specs) are not implemented.

## [1.8.0] - 2026-10-13

### Added
- `WithMCPServerInfo(mcp.ServerInfo{Name, Version, Title, Description})`
  sets what the servers mounted by `WithMCP`/`WithMCPAuth` report in the
  `initialize` response. They were fixed at name `"dindenault"`, version
  `"1.0.0"`, so every service looked the same in MCP registries and clients.
  Order-independent; only non-empty fields apply; without it nothing changes.
- `mcp.ServerInfo` and `(*mcp.Server).WithInfo` for servers built with
  `mcp.NewServer`. `title` and `description` are added to `serverInfo` and
  omitted unless set.

### Changed
- A custom server name (via `WithMCPServerInfo` or `WithInfo`) becomes the
  `service` of `mcp_tool_call` audit events instead of the Lambda function
  name. Servers that keep the default name are unaffected.

## [1.7.1] - 2026-09-29

### Fixed
- MCP: a tools/call's `params._meta.traceparent` is used when the request
  has no `traceparent` header. Strands' `MCPClient` (and other clients that
  propagate OpenTelemetry context per call) send it there, so v1.7.0 audit
  events and downstream requests carried no traceparent for them. The header
  still wins when both are present.

## [1.7.0] - 2026-09-29

### Added
- MCP audit events: every `tools/call` writes one `mcp_tool_call` JSON line
  (caller org and subject, tool, `read_only`, outcome, duration, argument
  fingerprint, correlation ids) to `mcp.DefaultAuditSink` on stdout,
  independent of the log level. Calls rejected by `AuthMiddleware` are
  audited as `unauthenticated`. Arguments and results are never recorded.
  `Server.WithAuditSink`, `mcp.NewJSONAuditSink`, `mcp.DisableAudit` and
  `mcp.WithAuthAuditSink` configure it.
- MCP correlation: `X-Correlation-Id`, `X-Turn-Id` and `traceparent` from the
  incoming request are available via `mcp.CorrelationFromContext` and
  forwarded by `mcp.NewHTTPClient` on downstream requests.

### Changed
- MCP servers now write one audit line per tool call to stdout by default.
  Services that must not, call `WithAuditSink(mcp.DisableAudit)` on a
  server they build themselves.

## [1.6.2] - 2026-09-29

### Fixed
- The 404 for unmatched paths and the generic 500 now carry `Content-Type`
  headers in both `headers` and `multiValueHeaders`. An ALB target group with
  multi-value headers enabled rejected the header-less response and returned
  502 to the client.

## [1.6.0] - 2026-09-29

### Added
- `mcp.Tool.Annotations` and `mcp.ToolAnnotations` — MCP tool behavior
  hints (`readOnlyHint`, `destructiveHint`, …) sent in `tools/list`.
  `mcp.ReadOnly()` builds annotations for read-only tools.

## [1.5.0] - 2026-06-10

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

### Removed (BREAKING)
- `NewConnectHandler`, `ConnectOptions`, `ConnectOption`,
  `WithRequiredPermissions`, `WithUnitPermissions` — pass interceptors at
  handler creation with `connect.WithInterceptors` instead (see
  MIGRATION.md). The old helper silently skipped authentication for
  connect-go generated handlers.
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

[Unreleased]: https://github.com/navigacontentlab/dindenault/compare/v1.14.0...HEAD
[1.14.0]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.14.0
[1.13.0]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.13.0
[1.12.0]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.12.0
[1.11.0]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.11.0
[1.10.1]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.10.1
[1.10.0]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.10.0
[1.9.1]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.9.1
[1.9.0]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.9.0
[1.8.2]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.8.2
[1.8.1]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.8.1
[1.8.0]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.8.0
[1.0.0]: https://github.com/navigacontentlab/dindenault/releases/tag/v1.0.0
