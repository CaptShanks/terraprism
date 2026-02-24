<p align="center">
  <img src="assets/logo-dark.svg?v=2" alt="Terra-Prism Logo" width="400">
</p>

<p align="center">
  <strong>A beautiful terminal UI for viewing Terraform and OpenTofu plans</strong>
</p>

<p align="center">
  <a href="https://github.com/CaptShanks/terraprism/actions/workflows/ci.yml"><img src="https://github.com/CaptShanks/terraprism/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/CaptShanks/terraprism/releases"><img src="https://img.shields.io/github/v/release/CaptShanks/terraprism?include_prereleases" alt="Release"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/CaptShanks/terraprism" alt="Go Version"></a>
  <a href="https://goreportcard.com/report/github.com/CaptShanks/terraprism"><img src="https://goreportcard.com/badge/github.com/CaptShanks/terraprism" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/CaptShanks/terraprism" alt="License"></a>
</p>

<p align="center">
  Collapsible resources • Filter & sort • Syntax-highlighted HCL • Vim-style navigation • Auto light/dark mode
</p>

---

<p align="center">
  <img src="assets/demo.gif" alt="Terra-Prism Demo" width="800">
</p>

## Features

- **Syntax-highlighted HCL** - Full color-coded display of your plan
- **Collapsible resources** - Expand/collapse individual resources or all at once
- **Status filter** - Filter resources by action (create, destroy, update, replace, read, etc.)
- **Sort** - Sort by plan order, action, address, or resource type
- **Search** - Find resources by name, type, or address (works with filters)
- **Vim-style navigation** - j/k/gg/G/d/u and more
- **Auto light/dark mode** - Detects your terminal background
- **Format support** - Works with Terraform 0.11+ and OpenTofu
- **Full-line selection** - Clear visual indicator of selected resource
- **History tracking** - All plans saved with full path, searchable picker
- **Color-coded CLI** - Commands and status colored in history list
- **Environment variables** - Set `TERRAPRISM_TOFU` and `TERRAPRISM_THEME` to avoid passing flags every time

## Installation

### Quick Install (Recommended)

```bash
curl -sSfL https://raw.githubusercontent.com/CaptShanks/terraprism/main/install.sh | sh
```

### Using Go

```bash
go install github.com/CaptShanks/terraprism/cmd/terraprism@latest
```

### From Source

```bash
git clone https://github.com/CaptShanks/terraprism.git
cd terraprism
make build
```

### Manual Download

Download binaries from the [Releases](https://github.com/CaptShanks/terraprism/releases) page.

## Usage

### Apply Mode (Recommended)

Review and apply in one command:

```bash
# Run plan, review interactively, press 'a' to apply
terraprism apply

# With OpenTofu (or set TERRAPRISM_TOFU=1)
terraprism --tofu apply

# Pass arguments to terraform/tofu
terraprism apply -- -target=module.vpc -var="env=prod"
terraprism --tofu apply -- -var="env=prod"
```

### Plan Mode

Run plan and view interactively (no apply):

```bash
terraprism plan
terraprism --tofu plan
```

### Pipe Mode

Pipe plan output for viewing:

```bash
terraform plan -no-color | terraprism
tofu plan -no-color | terraprism
```

### Read from file

```bash
terraform plan -no-color > plan.txt
terraprism plan.txt
```

### Print mode (non-interactive)

```bash
terraform plan -no-color | terraprism -p
```

## Keyboard Controls

### Navigation
| Key | Action |
|-----|--------|
| `j` / `↓` | Move to next resource |
| `k` / `↑` | Move to previous resource |
| `gg` | Jump to first resource |
| `G` | Jump to last resource |
| `d` / `Ctrl+D` | Scroll half page down |
| `u` / `Ctrl+U` | Scroll half page up |

### Expand/Collapse
| Key | Action |
|-----|--------|
| `Enter` / `Space` | Toggle current resource |
| `l` / `→` | Expand current resource |
| `h` / `←` / `⌫` | Collapse current resource |
| `e` | Expand all resources |
| `c` | Collapse all resources |

### Search
| Key | Action |
|-----|--------|
| `/` | Start search |
| `n` | Next match |
| `N` | Previous match |
| `Esc` | Clear search (or clear filters when filters active) |

### Filter
| Key | Action |
|-----|--------|
| `f` | Open filter picker |
| `Esc` | Clear all filters (from main view or picker) |

In the filter picker: **Space** toggle status, **a** select all, **c** clear all, **Enter** apply, **Esc** clear and close.

### Sort
| Key | Action |
|-----|--------|
| `s` | Open sort picker |

Sort options: default (plan order), by action, by address, by type.

### Apply (in apply mode)
| Key | Action |
|-----|--------|
| `a` | Apply the plan |
| `y` | Confirm apply |

### Other
| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit (cancel apply) |

## Color Themes

Terra-Prism automatically detects your terminal background and uses appropriate colors:

### Dark Mode (Catppuccin Mocha)
- Green for resources being created
- Red for resources being destroyed
- Yellow for resources being updated
- Purple for resources being replaced
- Blue for data sources being read

### Light Mode (Catppuccin Latte)
Automatically switches to darker, more visible colors on light backgrounds.

### Force a theme
```bash
terraprism --dark plan.txt   # Force dark mode
terraprism --light plan.txt  # Force light mode
```

## Commands

```
terraprism                     # View piped/file input
terraprism plan                # Run terraform plan and view
terraprism apply               # Run plan, view, and apply
terraprism destroy             # Run destroy plan and apply
terraprism history             # Manage history files
terraprism version             # Show terraprism and terraform/tofu version
```

## Options

```
-h, --help      Show help message
-v, --version   Show version (includes terraform/tofu version)
-p, --print     Print colored output without interactive TUI
--light         Force light color scheme (Catppuccin Latte)
--dark          Force dark color scheme (Catppuccin Mocha)
--tofu          Use OpenTofu instead of Terraform
```

## Environment Variables

Set these to avoid passing flags every time:

```
TERRAPRISM_TOFU    Set to 1, true, or yes to use OpenTofu instead of Terraform
TERRAPRISM_THEME   Set to "light" or "dark" to force color scheme
```

Example: add `export TERRAPRISM_TOFU=1` to your `~/.bashrc` or `~/.zshrc` to always use OpenTofu.

## History

All plan and apply outputs are automatically saved to `~/.terraprism/` for future reference. History includes the full working directory path and is color-coded for easy reading.

### Listing History

```bash
terraprism history list              # List all history files
terraprism history list --plan       # List only plan files
terraprism history list --apply      # List only apply files
terraprism history list --destroy    # List only destroy files
```

Output shows timestamp, command (colored), status (colored), and full path:
```
  #  TIMESTAMP         COMMAND  STATUS        PATH
--------------------------------------------------------------------------------------
  1  2026-01-14 12:52  plan                   .../infrastructure/aws/prod/eks-cluster
  2  2026-01-14 10:30  apply    [SUCCESS]     .../infrastructure/aws/staging/vpc
```

### Viewing History

```bash
terraprism history view              # Interactive picker with search
terraprism history view 1            # View most recent entry
terraprism history view 3            # View 3rd most recent
terraprism history 1                 # Shorthand for 'view 1'
```

The interactive picker supports:
- **j/k** - Navigate up/down
- **/** - Search (fzf-style, multiple terms)
- **Enter** - Select and view in TUI
- **q/Esc** - Cancel

Search by project, command, status, date, or path:
```
/myproject apply success    # Find successful applies for myproject
/2026-01 destroy            # Find January 2026 destroys
```

### File Naming

Files are named: `YYYY-MM-DD_HH-MM-SS_<project>_<command>[_<status>].txt`

- `plan` - Plan-only commands
- `apply` - Apply commands (status: success, failed, cancelled)
- `destroy` - Destroy commands (status: success, failed, cancelled)

### Cleanup

```bash
terraprism history list --clear      # Delete all history files
```

History is automatically cleaned up when exceeding 100 files (oldest removed first).

## Why Terra-Prism?

Large Terraform plans can be difficult to review:

- Hundreds of resources make it hard to find specific changes
- Long attribute values span multiple lines
- No easy way to focus on specific resources
- Color coding from Terraform can be lost when piping

Terra-Prism solves these problems:

- Collapsible sections for high-level overview
- Filter by status to focus on creates, destroys, updates, etc.
- Sort by action, address, or type for organized review
- Consistent syntax highlighting
- Search to find specific resources (works with filters)
- Vim-style navigation for efficiency
- Auto-scrolling keeps selection visible

## Inspired By

- [prettyplan](https://prettyplan.chrislewisdev.com/) - Web-based Terraform plan formatter
- [terraform-landscape](https://github.com/coinbase/terraform-landscape) - Ruby-based plan formatter

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development

```bash
# Clone the repo
git clone https://github.com/CaptShanks/terraprism.git
cd terraprism

# Install dependencies
go mod download

# Run tests
make test

# Build
make build

# Run locally
./bin/terraprism
```

## License

GNU AGPL v3.0 - see [LICENSE](LICENSE) for details.

---

Made with Go
