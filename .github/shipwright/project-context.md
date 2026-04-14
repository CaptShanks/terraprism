# Terraprism - Project Context

## Project Overview

**Name:** Terraprism
**Repository:** CaptShanks/terraprism
**Purpose:** A terminal UI (TUI) tool that visualizes Terraform plan output in a human-friendly, interactive format. It parses `terraform plan` JSON output and presents resource changes with diffs, filtering, and search capabilities.
**Tech Stack:** Go 1.24, Charm Bubble Tea (TUI framework), Charm Lipgloss (styling), Charm Bubbles (components)
**License:** MIT

## Repository Structure

```
cmd/terraprism/       Entry point (main.go)
internal/
  config/             Configuration management (currently empty, planned)
  history/            Plan history tracking
  parser/             Terraform plan JSON parser (core logic)
  tui/                Terminal UI components (Bubble Tea model, views, styles)
  updater/            Self-update mechanism via GitHub releases
testdata/             Sample Terraform plan JSON files for testing
release-notes/        Per-version release notes (vX.Y.Z.md)
assets/               Screenshots, demo GIFs
bin/                  Build output directory
```

## Development Standards

- **Go version:** 1.24 (ensure CI and local tooling match)
- **Module path:** `github.com/CaptShanks/terraprism`
- **Naming:** Follow standard Go conventions (camelCase for unexported, PascalCase for exported)
- **Error handling:** Wrap errors with `fmt.Errorf("context: %w", err)` for stack traceability
- **Package design:** `internal/` for all non-public packages; accept interfaces, return structs
- **Comments:** Exported functions and types must have godoc comments
- **Formatting:** `gofmt` / `goimports` enforced via CI

## Dependencies

| Package | Purpose |
|---------|---------|
| `charmbracelet/bubbletea` v1.3.10 | TUI framework (Elm architecture) |
| `charmbracelet/bubbles` v0.21.0 | Pre-built TUI components (text input, viewport, etc.) |
| `charmbracelet/lipgloss` v1.1.0 | Terminal styling and layout |
| `muesli/reflow` v0.3.0 | Text wrapping and padding |
| `blang/semver` v4.0.0 | Semantic versioning for self-update |
| `rhysd/go-github-selfupdate` v1.2.3 | GitHub release-based self-update |

## Testing

- **Framework:** Go standard `testing` package
- **Run tests:** `go test -v -race -cover ./...`
- **Test files:** Colocated with source (`*_test.go`)
- **Test data:** `testdata/` directory for sample Terraform plan JSON
- **Patterns:** Table-driven tests with subtests (`t.Run`), `t.Parallel()` where safe
- **Coverage goal:** Increase from current baseline (parser and TUI components)

## Build & Run

```bash
# Build
go build -o terraprism ./cmd/terraprism

# Run (pipe terraform plan output)
terraform plan -json | terraprism

# Run with file
terraprism < plan.json

# Run tests
go test -v -race -cover ./...

# Lint
golangci-lint run ./...

# Build release (cross-platform, handled by CI)
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.version=vX.Y.Z" -o terraprism ./cmd/terraprism
```

## Architecture Decisions

1. **Elm Architecture (TEA):** The TUI follows Bubble Tea's Model-Update-View pattern. All state changes flow through `Update()`, views are pure functions of model state.
2. **Parser separation:** The Terraform plan parser (`internal/parser`) is independent of the TUI. It produces structured data that the TUI consumes. This separation enables testing and future API use.
3. **Self-update:** The binary can update itself via GitHub releases using `go-github-selfupdate`. Version is injected at build time via `-ldflags`.
4. **Internal packages:** All packages are under `internal/` to prevent external imports and allow free refactoring.
5. **Piped input:** Terraprism reads from stdin, making it composable with shell pipelines.

## Security Considerations

- Terraprism processes Terraform plan JSON which may contain sensitive resource attributes (passwords, keys, connection strings)
- Never log or persist raw plan data to disk beyond the current session
- The self-update mechanism downloads binaries from GitHub; checksums should be verified
- No network calls are made except for self-update checks

## PR Guidelines

- Target `dev` branch for feature work
- PR title format: `type: description` (e.g., `feat: add resource filtering`)
- Include tests for new functionality
- Run `golangci-lint` before submitting
- Update CHANGELOG.md for user-facing changes
