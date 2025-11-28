# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2025-01-28

### Added
- Core interfaces in `pkg/ds/app.go`:
  - `IService` - Service interface
  - `Runnable`, `RunnableService`, `OnlyRunnable` - Component lifecycle interfaces
  - `Actor`, `Authorizer` - Authentication/authorization abstractions
  - `ServerBucket`, `AppInfo` - Application metadata
- Empty metrics implementation in `pkg/servicemetrics/metrics.go`
- Repository structure with pkg/ directories
- GitHub Actions CI/CD pipeline
- golangci-lint configuration
- README, LICENSE (MIT), .gitignore

### Notes
- First release with base packages (no template constructions)
- Compatible with generated projects from go-project-starter v1.x
- Go 1.21+ required
