# Terraprism - Claude Code Configuration

## Project Overview

Terraprism is a terminal UI (TUI) tool that visualizes Terraform plan output in a human-friendly, interactive format. It decodes `terraform show -json` plan output and presents resource changes with diffs, filtering, and search capabilities.

- **Tech Stack:** Go 1.24, Charm Bubble Tea, Charm Lipgloss, Charm Bubbles
- **Module path:** `github.com/CaptShanks/terraprism`
- **License:** MIT

## Repository Structure

```
cmd/terraprism/       Entry point (main.go)
internal/
  history/            Plan/apply history tracking (JSON envelope files)
  runner/             Shells out to terraform/tofu plan/show/apply
  tfplan/             Decodes `terraform show -json` into the tui view model (core logic)
  tui/                Terminal UI components (Bubble Tea model, views, styles)
  updater/            Self-update mechanism via GitHub releases
docs/architecture/    Architecture documents for issues (auto-generated)
```

## Development Standards

- Follow standard Go conventions (camelCase unexported, PascalCase exported)
- Wrap errors: `fmt.Errorf("context: %w", err)`
- All packages under `internal/` — no external imports allowed
- Exported functions and types must have godoc comments
- `gofmt` / `goimports` enforced

## Build & Test

```bash
go build -o terraprism ./cmd/terraprism
go test -v -race -cover ./...
golangci-lint run ./...
```

## Testing Conventions

- Go standard `testing` package
- Table-driven tests with subtests (`t.Run`), `t.Parallel()` where safe
- Test data colocated per-package (e.g. `internal/tfplan/testdata/*.json`)
- Test files colocated with source (`*_test.go`)
- Prefer real stand-in implementations over mocks (e.g. a fake shell
  script for subprocess/streaming code, real `tea.Model` update cycles
  for TUI components) so tests exercise the same path production does
- Reduce real-world bugs to a minimal synthetic fixture rather than
  checking in real/sensitive data
- Redirect fixed filesystem paths (e.g. `$HOME`) to a temp dir via
  `t.Setenv` instead of mocking the filesystem
- Strip ANSI codes before asserting on rendered text
- Name regression tests after the failure scenario they guard, not just
  the function under test
- Verify performance fixes with a benchmark, not just code inspection
- Fuzz/property-test core invariants where inputs are hard to enumerate
  by hand (e.g. tree navigation boundaries)

## Dependencies

| Package | Purpose |
|---------|---------|
| `charmbracelet/bubbletea` v1.3.10 | TUI framework (Elm architecture) |
| `charmbracelet/bubbles` v0.21.0 | Pre-built TUI components |
| `charmbracelet/lipgloss` v1.1.0 | Terminal styling and layout |
| `muesli/reflow` v0.3.0 | Text wrapping and padding |
| `blang/semver` v4.0.0 | Semantic versioning |
| `rhysd/go-github-selfupdate` v1.2.3 | GitHub release-based self-update |
| `hashicorp/terraform-json` | Canonical types for decoding `terraform show -json` plan output |

## Architecture Decisions

See [ARCHITECTURE.md](ARCHITECTURE.md) for diagrams of package
dependencies, data flow, and the plan data model.

1. **Elm Architecture (TEA):** TUI follows Bubble Tea's Model-Update-View pattern
2. **JSON-only, no regex parsing:** `internal/tfplan` decodes `terraform show -json` (via `hashicorp/terraform-json`) into a pre-diffed attribute tree; `internal/tui` renders that tree directly rather than re-parsing text
3. **Runner separation:** `internal/runner` is the only place that shells out to `terraform`/`tofu` plan/show/apply, independent of TUI and decoding
4. **Self-update:** Binary self-updates via GitHub releases with `-ldflags` version injection
5. **Internal packages:** All under `internal/` for free refactoring
6. **Piped input:** Reads `terraform show -json` output from stdin, composable with shell pipelines

## Security Considerations

- Terraform plan JSON may contain sensitive attributes (passwords, keys, connection strings)
- Never log or persist raw plan data beyond the current session
- Self-update downloads must verify checksums
- No network calls except self-update checks

## PR Guidelines

- Target `dev` branch for all feature work
- PR title format: `type: description` (e.g., `feat: add resource filtering`)
- Include tests for new functionality
- Run `golangci-lint` before submitting

## Branching

- `main` — production releases only
- `dev` — integration branch for feature work
- `shipwright/issue-N` — feature branches created by automation
