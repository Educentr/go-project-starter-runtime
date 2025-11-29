# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2025-01-29

### Added
- Application core package in `pkg/app/`:
  - `app.go` - Main App struct with lifecycle management for drivers, transports, workers
  - `closer.go` - Graceful shutdown mechanism with timeout handling
  - `metrics/metrics.go` - Metrics initialization via OpenTelemetry
  - `metrics/build_collector.go` - Build info collector for Prometheus
  - `healthstate/service.go` - Health check service implementation
  - `serviceauth/auth.go` - Base authorizer implementation
- ActiveRecord initialization support (always available via `InitActiveRecord` method)

### Changed
- Removed all logger calls from app package (library returns errors instead of logging)
- Template constructions removed:
  - `{{ .Logger.Import }}` → removed
  - `{{ .Logger.InfoMsg }}`, `{{ .Logger.WarnMsg }}`, `{{ .Logger.ErrorMsg }}` → removed
  - `{{ if .UseActiveRecord }}` → removed (ActiveRecord methods always available)
  - `{{ .ProjectPath }}` → hardcoded runtime path

### Notes
- Breaking change: Applications must handle logging themselves - library only returns errors
- ActiveRecord methods are always available regardless of usage
- Compatible with generated projects from go-project-starter v1.x

## [0.3.0] - 2025-01-28

### Added
- Request context management in `pkg/reqctx/`:
  - `context.go` - Context creation, actor/request ID management
  - `cumulative_metric.go` - Cumulative metrics tracking
  - `info.go` - Request processing info
  - `logger.go` - Logger context updater interface (callback pattern)
- Logger-agnostic design with `LoggerContextUpdater` interface
- Support for logger context enrichment via callback

### Changed
- Removed direct logger dependency from reqctx package
- `CreateContext()` no longer wraps logger (caller responsibility)
- Logger context updates now use callback interface instead of direct calls
- Template constructions removed:
  - `{{ .Logger.Import }}` → removed
  - `{{ .Logger.ErrorMsg }}` → removed (library doesn't log)
  - `{{ .Logger.UpdateContext }}` → callback via `LoggerContextUpdater`
  - `{{ .ProjectPath }}` → hardcoded runtime path

### Notes
- Breaking change: Applications must call `reqctx.SetLoggerUpdater()` to enable logger context enrichment
- Example zerolog adapter provided in documentation

## [0.2.0] - 2025-01-28

### Added
- Actor model in `pkg/model/actor/actor.go`:
  - `Actor` struct - ephemeral user entity
  - `New()` - constructor from authorization data
  - `GetID()` - implements `ds.Actor` interface

### Changed
- Template construction `{{ .ProjectPath }}` replaced with hardcoded runtime path
- First package with template transformation

### Notes
- Compatible with generated projects from go-project-starter v1.x

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
