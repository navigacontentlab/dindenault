# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
