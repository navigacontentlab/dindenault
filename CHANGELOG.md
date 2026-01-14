# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- GitHub Actions workflows for CI and automated releases
- Makefile for manual release management
- This changelog

### Changed
- Documentation improvements in README.md

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

[Unreleased]: https://github.com/navigacontentlab/dindenault/compare/HEAD...HEAD
