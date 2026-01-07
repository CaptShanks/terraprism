# tfplanview

A terminal UI for interactively viewing Terraform and OpenTofu plans with collapsible resources and color-coded changes.

![tfplanview demo](https://user-images.githubusercontent.com/demo/tfplanview-demo.gif)

## Features

- 🎨 **Color-coded changes**: Green for create, red for destroy, yellow for update, pink for replace
- 📁 **Collapsible resources**: Expand/collapse individual resources or all at once
- 🔍 **Search**: Find resources by name, type, or address
- ⌨️ **Keyboard navigation**: Vim-style keybindings (j/k) plus arrow keys
- 📋 **Format support**: Works with both Terraform 0.11 and 0.12+ plan formats
- 🦫 **OpenTofu compatible**: Works seamlessly with `tofu plan` output

## Installation

### Using Go

```bash
go install github.com/tfplanview/tfplanview/cmd/tfplanview@latest
```

### From Source

```bash
git clone https://github.com/tfplanview/tfplanview.git
cd tfplanview
make build
```

### Homebrew (coming soon)

```bash
brew install tfplanview
```

## Usage

### Pipe from Terraform

```bash
terraform plan -no-color | tfplanview
```

### Pipe from OpenTofu

```bash
tofu plan -no-color | tfplanview
```

### Read from file

```bash
terraform plan -no-color > plan.txt
tfplanview plan.txt
```

### Save plan and view

```bash
terraform plan -no-color -out=tfplan && terraform show -no-color tfplan | tfplanview
```

## Keyboard Controls

| Key | Action |
|-----|--------|
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `Enter` / `Space` | Toggle expand/collapse current resource |
| `e` | Expand all resources |
| `c` | Collapse all resources |
| `/` | Start search |
| `n` | Jump to next search result |
| `N` | Jump to previous search result |
| `Esc` | Clear search / Cancel |
| `q` / `Ctrl+C` | Quit |
| `PgUp` / `PgDown` | Scroll viewport |

## Color Legend

| Symbol | Color | Action |
|--------|-------|--------|
| `+` | 🟢 Green | Resource will be created |
| `-` | 🔴 Red | Resource will be destroyed |
| `~` | 🟡 Yellow | Resource will be updated in-place |
| `±` | 🟣 Pink | Resource must be replaced (destroy + create) |
| `≤` | 🔵 Cyan | Data source will be read |

## Examples

### Basic usage

```bash
# Quick view of plan
terraform plan -no-color | tfplanview

# With variable file
terraform plan -var-file=prod.tfvars -no-color | tfplanview
```

### CI/CD Integration

In a CI environment where you want to save plans for review:

```bash
# Generate and save plan
terraform plan -no-color -out=tfplan

# Show plan in human-readable format
terraform show -no-color tfplan > plan.txt

# View interactively (when connected to terminal)
tfplanview plan.txt
```

## Requirements

- Terminal with true color support (most modern terminals)
- Terraform 0.11+ or OpenTofu

## Why tfplanview?

Large Terraform plans can be difficult to review in a terminal:

- Hundreds of resources make it hard to find specific changes
- Long attribute values span multiple lines
- No easy way to focus on specific resources
- Color coding from Terraform can be lost when piping

tfplanview solves these problems by providing:

- Collapsible sections so you can see the big picture first
- Consistent color coding regardless of terminal settings
- Search functionality to find specific resources
- Truncated long values with full details on expand

## Inspired By

- [prettyplan](https://prettyplan.chrislewisdev.com/) - Web-based Terraform plan formatter
- [terraform-landscape](https://github.com/coinbase/terraform-landscape) - Ruby-based plan formatter

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development

```bash
# Clone the repo
git clone https://github.com/tfplanview/tfplanview.git
cd tfplanview

# Install dependencies
go mod download

# Run tests
make test

# Build
make build

# Run locally
./bin/tfplanview
```

## License

MIT License - see [LICENSE](LICENSE) for details.

