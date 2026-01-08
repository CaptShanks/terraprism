package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CaptShanks/terraprism/internal/parser"
	"github.com/CaptShanks/terraprism/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.1.0"

var (
	printMode  = false
	forceLight = false
	forceDark  = false
	useTofu    = false
)

func main() {
	args := os.Args[1:]

	// Parse global flags first (before subcommand)
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tofu":
			useTofu = true
		case "--light":
			forceLight = true
		case "--dark":
			forceDark = true
		case "-h", "--help":
			printUsage()
			os.Exit(0)
		case "-v", "--version":
			fmt.Printf("terraprism %s\n", version)
			os.Exit(0)
		default:
			remaining = append(remaining, args[i])
		}
	}

	// Apply color scheme
	if forceLight {
		tui.SetLightPalette()
	} else if forceDark {
		tui.SetDarkPalette()
	}

	// Check for subcommands
	if len(remaining) > 0 {
		switch remaining[0] {
		case "apply":
			runApplyMode(remaining[1:])
			return
		case "plan":
			runPlanMode(remaining[1:])
			return
		}
	}

	// Default: view mode (pipe or file input)
	runViewMode(remaining)
}

// runApplyMode runs terraform/tofu plan, shows TUI, and optionally applies
func runApplyMode(args []string) {
	var tfArgs []string

	// Parse args (only -- and help at this point, global flags already parsed)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printApplyUsage()
			os.Exit(0)
		case "--":
			// Everything after -- is passed to terraform/tofu
			tfArgs = append(tfArgs, args[i+1:]...)
			i = len(args) // break loop
		default:
			tfArgs = append(tfArgs, args[i])
		}
	}

	// Detect terraform or tofu
	tfCmd := detectTFCommand()

	// Create temp file for plan
	tmpDir := os.TempDir()
	planFile := filepath.Join(tmpDir, fmt.Sprintf("terraprism-%d.tfplan", os.Getpid()))
	defer os.Remove(planFile)

	fmt.Printf("🔺 Terra-Prism: Running %s plan... ", tfCmd)

	// Run terraform/tofu plan
	planArgs := append([]string{"plan", "-out=" + planFile, "-no-color"}, tfArgs...)
	cmd := exec.Command(tfCmd, planArgs...)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("❌")
		fmt.Fprintf(os.Stderr, "\n%s plan failed:\n%s\n", tfCmd, string(output))
		os.Exit(1)
	}

	fmt.Println("✅")

	// Parse the plan
	plan, err := parser.Parse(string(output))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing plan: %v\n", err)
		os.Exit(1)
	}

	if len(plan.Resources) == 0 {
		fmt.Println("✅ No changes. Infrastructure is up-to-date.")
		os.Exit(0)
	}

	// Go straight to TUI
	model := tui.NewModelWithApply(plan, planFile, tfCmd)

	// Run TUI
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	// Check if user wants to apply
	if m, ok := finalModel.(tui.Model); ok && m.ShouldApply() {
		fmt.Printf("\n🚀 Applying plan with %s...\n\n", tfCmd)

		applyCmd := exec.Command(tfCmd, "apply", planFile)
		applyCmd.Stdout = os.Stdout
		applyCmd.Stderr = os.Stderr
		applyCmd.Stdin = os.Stdin

		if err := applyCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "\n❌ Apply failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n✅ Apply complete!")
	} else {
		fmt.Println("\n👋 Apply cancelled.")
	}
}

// runPlanMode runs terraform/tofu plan and shows in TUI (read-only)
func runPlanMode(args []string) {
	var tfArgs []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		case "--":
			tfArgs = append(tfArgs, args[i+1:]...)
			i = len(args)
		default:
			tfArgs = append(tfArgs, args[i])
		}
	}

	tfCmd := detectTFCommand()

	fmt.Printf("🔺 Terra-Prism: Running %s plan... ", tfCmd)

	planArgs := append([]string{"plan", "-no-color"}, tfArgs...)
	cmd := exec.Command(tfCmd, planArgs...)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("❌")
		fmt.Fprintf(os.Stderr, "\n%s plan failed:\n%s\n", tfCmd, string(output))
		os.Exit(1)
	}

	fmt.Println("✅")

	plan, err := parser.Parse(string(output))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing plan: %v\n", err)
		os.Exit(1)
	}

	if len(plan.Resources) == 0 {
		fmt.Println("✅ No changes. Infrastructure is up-to-date.")
		os.Exit(0)
	}

	// Go straight to TUI
	p := tea.NewProgram(
		tui.NewModel(plan),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// detectTFCommand returns "terraform" or "tofu" based on flags and availability
func detectTFCommand() string {
	if useTofu {
		return "tofu"
	}
	// Auto-detect: prefer terraform, fall back to tofu
	if _, err := exec.LookPath("terraform"); err == nil {
		return "terraform"
	}
	if _, err := exec.LookPath("tofu"); err == nil {
		return "tofu"
	}
	return "terraform" // Default, will error if not found
}

// runViewMode is the default pipe/file view mode
func runViewMode(args []string) {
	var inputFile string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--print":
			printMode = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				inputFile = args[i]
			}
		}
	}

	var input io.Reader

	if inputFile != "" && inputFile != "-" {
		file, err := os.Open(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		input = file
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			printUsage()
			os.Exit(0)
		}
		input = os.Stdin
	}

	var lines []string
	scanner := bufio.NewScanner(input)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	planText := strings.Join(lines, "\n")

	plan, err := parser.Parse(planText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing plan: %v\n", err)
		os.Exit(1)
	}

	if len(plan.Resources) == 0 {
		fmt.Println("No resource changes detected in the plan.")
		os.Exit(0)
	}

	if printMode {
		tui.PrintPlan(plan)
		os.Exit(0)
	}

	p := tea.NewProgram(
		tui.NewModel(plan),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`terraprism %s - Interactive Terraform/OpenTofu plan viewer 🔺✨

USAGE:
    terraform plan -no-color | terraprism        # Pipe plan output
    terraprism <plan-file>                       # Read from file
    terraprism [options] plan [-- tf-args]       # Run plan and view
    terraprism [options] apply [-- tf-args]      # Run plan, view, and apply

DESCRIPTION:
    Terra-Prism provides an interactive terminal UI for viewing Terraform and
    OpenTofu plans with collapsible resources and syntax highlighting.

COMMANDS:
    (none)      View mode - pipe or file input
    plan        Run terraform/tofu plan and view interactively
    apply       Run plan, review in TUI, press 'a' to apply

GLOBAL OPTIONS:
    -h, --help      Show this help
    -v, --version   Show version
    --tofu          Use tofu instead of terraform
    --light         Force light theme
    --dark          Force dark theme

VIEW OPTIONS:
    -p, --print     Print mode (no TUI)

CONTROLS:
    ↑/k         Move cursor up
    ↓/j         Move cursor down
    Enter/Space Toggle expand/collapse
    l/→         Expand current resource
    h/←/⌫       Collapse current resource
    d/u         Half page down/up
    gg/G        Go to first/last resource
    e/c         Expand/collapse all
    /           Search resources
    n/N         Next/previous match
    a           Apply (only in apply mode)
    q/Esc       Quit

EXAMPLES:
    # View piped plan
    terraform plan -no-color | terraprism

    # Run plan and view
    terraprism plan

    # Run plan, review, and apply
    terraprism apply

    # Use tofu instead of terraform
    terraprism --tofu apply

    # Pass extra args to terraform/tofu
    terraprism --tofu apply -- -target=module.vpc -var="env=prod"

`, version)
}

func printApplyUsage() {
	fmt.Printf(`terraprism apply - Run plan, review, and apply 🔺✨

USAGE:
    terraprism [--tofu] apply [-- terraform-args]

DESCRIPTION:
    Runs terraform/tofu plan, displays in interactive TUI for review,
    then applies if you press 'a'.

GLOBAL OPTIONS:
    --tofu      Use tofu instead of terraform
    --light     Force light color scheme
    --dark      Force dark color scheme

TERRAFORM ARGS:
    --          Everything after this is passed to terraform/tofu

CONTROLS IN TUI:
    a           Apply the plan
    y           Confirm apply
    q/Esc       Cancel and quit

EXAMPLES:
    terraprism apply
    terraprism --tofu apply
    terraprism apply -- -target=module.vpc
    terraprism --tofu apply -- -var="env=prod"

`)
}
