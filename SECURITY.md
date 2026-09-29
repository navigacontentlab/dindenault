# Security Policy

## Supported versions

Security fixes are released for the latest minor version of each module:

| Module | Supported |
|---|---|
| `github.com/navigacontentlab/dindenault` | latest `v1.x` |
| `github.com/navigacontentlab/dindenault/otel` | latest `otel/v1.x` |
| `github.com/navigacontentlab/dindenault/xray` | latest `xray/v1.x` |

Upgrade to the latest release before reporting an issue in an older version.

## Reporting a vulnerability

Please do not open a public issue for security problems.

Report it privately through GitHub instead: go to the repository's
**Security** tab and choose **Report a vulnerability**. Include the affected
module and version, a description of the issue, and steps to reproduce if
you have them.

We acknowledge reports within five working days and keep you updated until
a fix is released. Once fixed, we publish a GitHub security advisory and
credit you unless you prefer otherwise.

## Scope

dindenault validates Naviga ID JWTs and forwards caller tokens to downstream
services. Issues in token validation, permission checks
(`RequiredPermissions`, path permissions) or token forwarding are in scope.
See the "Security Model" section in README.md for the guarantees the library
makes and what remains the caller's responsibility.
