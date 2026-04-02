# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.11.0] - 2026-02-25

### Added

- `terraprism upgrade` — upgrade to the latest release with a single command
- Version check — `terraprism version` shows an update hint when a newer version is available
- TUI footer nudge — when a newer version exists, a timely check (7-day cache) displays an update prompt at the bottom of the screen
- Curl fallback — if self-update fails (e.g. read-only install location), the command prints the manual upgrade instruction
- `TERRAPRISM_SKIP_UPDATE_CHECK` — set to 1, true, or yes to skip all update checks
- `TERRAPRISM_UPDATE_CHECK_INTERVAL` — days between TUI update checks (default: 7)

**Upgrade from v0.10.0:** Run `terraprism upgrade` or re-run the install script.

## [0.10.0] - 2026-02-24

### Added

- Terraform/OpenTofu command passthrough — run any terraform/tofu command through terraprism
- `terraprism init`, `terraprism validate`, `terraprism fmt`, `terraprism output`
- `terraprism state list`, `terraprism import`, `terraprism workspace`, `terraprism graph`
- `terraprism console`, `terraprism show`, `terraprism providers`, and more

### Removed

- `--tofu`, `--light`, `--dark` flags — use `TERRAPRISM_TOFU` and `TERRAPRISM_THEME` env vars instead

**Note:** If you relied on these flags, set the corresponding env vars or update your scripts.

### Changed

- Cleaner dispatch: `terraprism init -h` shows terraform help, not terraprism help

## [0.9.0] - 2026-02-24

### Added

- `TERRAPRISM_TOFU` — set to 1, true, or yes to use OpenTofu instead of Terraform
- `TERRAPRISM_THEME` — set to "light" or "dark" to force color scheme
- CLI flags override env vars when both are specified

## [0.8.0] - 2026-02-24

### Added

- Status filter — filter resources by action (create, destroy, update, etc.)
- Sort — sort resource list by default, action, address, or type
- Sort picker includes hints for each option
- Esc key clears filters from main view or filter picker

## [0.7.0] - 2026-02-23

### Added

- Diff engine with LCS and git-style context diffs
- Word wrap for plan output
- Base64 userdata decoding — decodes `user_data_base64` blocks in diffs for readability
- Granular diff highlighting (additions, deletions, context)
- History picker with scrolling and alt screen
- VHS demo script and README updates with demo GIF

## [0.6.0] - 2026-01-14

### Added

- Full working directory path in history list and picker
- Color-coded output: plan (blue), apply (green), destroy (red)
- Color-coded status: SUCCESS (green), FAILED (red), CANCELLED (yellow)
- Auto-cleanup: removes old files when count exceeds 100
- fzf-style search now includes path in history picker

### Fixed

- Separator width alignment (86 chars)
- Header PATH column padding
- Path column padding for consistent line width
- Unknown option error handling in history list
- Underscore sanitization in project names
- Parser edge case handling for projects named 'plan/apply/destroy'

## [0.5.0] - 2026-01-14

### Added

- `terraprism history list` — list all history with index numbers
- `terraprism history view` — interactive picker with j/k navigation
- `terraprism history view <#>` — view by index (1 = most recent)
- fzf-style multi-term search in picker (`/` to search)
- Project name included in history filenames

### Fixed

- Fix separator width alignment (70 chars)
- Sanitize underscores in project names (filename delimiter)
- Handle edge cases in filename parser (projects named 'plan' etc.)
- Error on unknown options in history list
- Fix header column alignment with printf formatting

## [0.4.0] - 2026-01-14

### Added

- `terraprism version` command to show terraform/tofu version

## [0.3.0] - 2026-01-10

### Fixed

- Remove ineffectual assignment (linter fix)

## [0.2.1] - 2026-01-08

### Fixed

- Fix gofmt formatting issues
- Remove emojis from README

## [0.2.0] - 2026-01-08

### Added

- Streamlined apply workflow — `terraprism plan` and `terraprism apply` go straight to TUI after running `terraform plan` (no intermediate prompt)

### Changed

- Remove intermediate prompt in apply mode

### Fixed

- Fix duplicate model declaration build error

## [0.1.0] - 2026-01-07

### Added

- TUI for Terraform plan review and apply — interactive terminal UI to review plan output and run apply
- Curl installer for quick setup
- CI/CD workflows for releases

### Fixed

- Remove unused code and fix deprecations
- Remove `borderColor`, fix deprecated viewport methods
- CI: use Go 1.22 and add golangci config

[Unreleased]: https://github.com/CaptShanks/terraprism/compare/v0.10.0...HEAD
[0.11.0]: https://github.com/CaptShanks/terraprism/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/CaptShanks/terraprism/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/CaptShanks/terraprism/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/CaptShanks/terraprism/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/CaptShanks/terraprism/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/CaptShanks/terraprism/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/CaptShanks/terraprism/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/CaptShanks/terraprism/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/CaptShanks/terraprism/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/CaptShanks/terraprism/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/CaptShanks/terraprism/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/CaptShanks/terraprism/releases/tag/v0.1.0
