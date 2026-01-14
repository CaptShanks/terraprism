package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CaptShanks/terraprism/internal/history"
	"github.com/CaptShanks/terraprism/internal/parser"
	"github.com/CaptShanks/terraprism/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.4.0"

var (
	printMode  = false
	forceLight = false
	forceDark  = false
	useTofu    = false
)

func main() {
	args := os.Args[1:]

	// Parse global flags first (before subcommand)
	// Don't consume -h/--help if it comes after a subcommand
	var remaining []string
	for i := 0; i < len(args); i++ {
		// Check if this is a subcommand - pass remaining args as-is
		if args[i] == "apply" || args[i] == "destroy" || args[i] == "plan" || args[i] == "history" || args[i] == "version" {
			remaining = append(remaining, args[i:]...)
			break
		}

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
			runVersionMode()
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
			runApplyMode(remaining[1:], false)
			return
		case "destroy":
			runApplyMode(remaining[1:], true)
			return
		case "plan":
			runPlanMode(remaining[1:])
			return
		case "history":
			runHistoryMode(remaining[1:])
			return
		case "version":
			runVersionMode()
			return
		}
	}

	// Default: view mode (pipe or file input)
	runViewMode(remaining)
}

// runApplyMode runs terraform/tofu plan, shows TUI, and optionally applies
func runApplyMode(args []string, isDestroy bool) {
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

	// Determine command name for history
	commandName := "apply"
	if isDestroy {
		commandName = "destroy"
		// Add -destroy flag if not already present
		hasDestroy := false
		for _, arg := range tfArgs {
			if arg == "-destroy" {
				hasDestroy = true
				break
			}
		}
		if !hasDestroy {
			tfArgs = append([]string{"-destroy"}, tfArgs...)
		}
	}

	// Create temp file for plan
	tmpDir := os.TempDir()
	planFile := filepath.Join(tmpDir, fmt.Sprintf("terraprism-%d.tfplan", os.Getpid()))
	defer os.Remove(planFile)

	fmt.Printf("Terra-Prism: Running %s plan... ", tfCmd)

	// Run terraform/tofu plan
	planArgs := append([]string{"plan", "-out=" + planFile, "-no-color"}, tfArgs...)
	cmd := exec.Command(tfCmd, planArgs...)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("FAILED")
		fmt.Fprintf(os.Stderr, "\n%s plan failed:\n%s\n", tfCmd, string(output))
		os.Exit(1)
	}

	fmt.Println("OK")

	// Save plan output to history
	historyHeader := history.CreateHistoryHeader("plan", tfCmd, tfArgs)
	historyPath, historyErr := history.CreateHistoryFile(commandName, historyHeader+string(output))
	if historyErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save history: %v\n", historyErr)
	}

	// Parse the plan
	plan, err := parser.Parse(string(output))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing plan: %v\n", err)
		os.Exit(1)
	}

	if len(plan.Resources) == 0 {
		fmt.Println("No changes. Infrastructure is up-to-date.")
		if historyPath != "" {
			_, _ = history.UpdateFilenameWithStatus(historyPath, "nochanges")
		}
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
		fmt.Printf("\nApplying plan with %s...\n\n", tfCmd)

		// Append apply start to history
		if historyPath != "" {
			_ = history.AppendToHistoryFile(historyPath, "\n\n--- APPLY OUTPUT ---\n\n")
		}

		applyCmd := exec.Command(tfCmd, "apply", planFile)
		applyCmd.Stdout = os.Stdout
		applyCmd.Stderr = os.Stderr
		applyCmd.Stdin = os.Stdin

		applyErr := applyCmd.Run()

		if applyErr != nil {
			fmt.Fprintf(os.Stderr, "\nApply failed: %v\n", applyErr)
			if historyPath != "" {
				footer := history.CreateApplyResultFooter(false, applyErr)
				_ = history.AppendToHistoryFile(historyPath, footer)
				_, _ = history.UpdateFilenameWithStatus(historyPath, history.StatusFailed)
			}
			os.Exit(1)
		}

		fmt.Println("\nApply complete!")
		if historyPath != "" {
			footer := history.CreateApplyResultFooter(true, nil)
			_ = history.AppendToHistoryFile(historyPath, footer)
			_, _ = history.UpdateFilenameWithStatus(historyPath, history.StatusSuccess)
		}
	} else {
		fmt.Println("\nApply cancelled.")
		if historyPath != "" {
			_, _ = history.UpdateFilenameWithStatus(historyPath, history.StatusCancelled)
		}
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

	fmt.Printf("Terra-Prism: Running %s plan... ", tfCmd)

	planArgs := append([]string{"plan", "-no-color"}, tfArgs...)
	cmd := exec.Command(tfCmd, planArgs...)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("FAILED")
		fmt.Fprintf(os.Stderr, "\n%s plan failed:\n%s\n", tfCmd, string(output))
		os.Exit(1)
	}

	fmt.Println("OK")

	// Save plan output to history
	historyHeader := history.CreateHistoryHeader("plan", tfCmd, tfArgs)
	_, historyErr := history.CreateHistoryFile("plan", historyHeader+string(output))
	if historyErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save history: %v\n", historyErr)
	}

	plan, err := parser.Parse(string(output))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing plan: %v\n", err)
		os.Exit(1)
	}

	if len(plan.Resources) == 0 {
		fmt.Println("No changes. Infrastructure is up-to-date.")
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

// runHistoryMode lists or manages history files
func runHistoryMode(args []string) {
	filterCommand := ""

	// Check for help first
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printHistoryUsage()
			os.Exit(0)
		}
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--plan", "-p":
			filterCommand = "plan"
		case "--apply", "-a":
			filterCommand = "apply"
		case "--destroy", "-d":
			filterCommand = "destroy"
		case "--clear":
			clearHistory()
			return
		}
	}

	entries, err := history.ListEntries(filterCommand)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading history: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		histDir, _ := history.GetHistoryDir()
		fmt.Printf("No history files found in %s\n", histDir)
		if filterCommand != "" {
			fmt.Printf("(filtered by: %s)\n", filterCommand)
		}
		return
	}

	histDir, _ := history.GetHistoryDir()
	fmt.Printf("History files in %s:\n\n", histDir)
	fmt.Println("TIMESTAMP            COMMAND   STATUS       FILENAME")
	fmt.Println(strings.Repeat("-", 80))

	for _, entry := range entries {
		fmt.Println(history.FormatEntry(entry))
	}

	fmt.Printf("\nTotal: %d entries\n", len(entries))
}

// clearHistory removes all history files
func clearHistory() {
	histDir, err := history.GetHistoryDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting history directory: %v\n", err)
		os.Exit(1)
	}

	entries, err := history.ListEntries("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading history: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("No history files to clear.")
		return
	}

	fmt.Printf("This will delete %d history files from %s\n", len(entries), histDir)
	fmt.Print("Are you sure? (y/N): ")

	var response string
	_, _ = fmt.Scanln(&response)

	if strings.ToLower(response) != "y" {
		fmt.Println("Cancelled.")
		return
	}

	deleted := 0
	for _, entry := range entries {
		if err := os.Remove(entry.Path); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete %s: %v\n", entry.Filename, err)
		} else {
			deleted++
		}
	}

	fmt.Printf("Deleted %d history files.\n", deleted)
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

// runVersionMode displays terraprism version and terraform/tofu version
func runVersionMode() {
	fmt.Printf("terraprism v%s\n\n", version)

	tfCmd := detectTFCommand()
	fmt.Printf("%s version:\n", tfCmd)

	cmd := exec.Command(tfCmd, "version")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  %s not found or failed to run\n", tfCmd)
	}
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
	fmt.Printf(`terraprism %s - Interactive Terraform/OpenTofu plan viewer

USAGE:
    terraform plan -no-color | terraprism        # Pipe plan output
    terraprism <plan-file>                       # Read from file
    terraprism [options] plan [-- tf-args]       # Run plan and view
    terraprism [options] apply [-- tf-args]      # Run plan, view, and apply
    terraprism [options] destroy [-- tf-args]    # Run destroy plan and apply
    terraprism history [options]                 # List history files

DESCRIPTION:
    Terra-Prism provides an interactive terminal UI for viewing Terraform and
    OpenTofu plans with collapsible resources and syntax highlighting.

COMMANDS:
    (none)      View mode - pipe or file input
    plan        Run terraform/tofu plan and view interactively
    apply       Run plan, review in TUI, press 'a' to apply
    destroy     Run destroy plan, review in TUI, press 'a' to destroy
    history     List plan/apply history files
    version     Show terraprism and terraform/tofu versions

GLOBAL OPTIONS:
    -h, --help      Show this help
    -v, --version   Show version
    --tofu          Use tofu instead of terraform
    --light         Force light theme
    --dark          Force dark theme

VIEW OPTIONS:
    -p, --print     Print mode (no TUI)

CONTROLS:
    j/k         Move cursor up/down
    Enter/Space Toggle expand/collapse
    l/h         Expand/collapse current resource
    d/u         Half page down/up
    gg/G        Go to first/last resource
    e/c         Expand/collapse all
    /           Search resources
    n/N         Next/previous match
    a           Apply (only in apply mode)
    q/Esc       Quit

HISTORY:
    All plan and apply outputs are saved to ~/.terraprism/
    Use 'terraprism history' to list them.

EXAMPLES:
    # View piped plan
    terraform plan -no-color | terraprism

    # Run plan and view
    terraprism plan

    # Run plan, review, and apply
    terraprism apply

    # Destroy resources
    terraprism destroy

    # Use tofu instead of terraform
    terraprism --tofu apply

    # Pass extra args to terraform/tofu
    terraprism apply -- -target=module.vpc -var="env=prod"

    # View history
    terraprism history

`, version)
}

func printApplyUsage() {
	fmt.Printf(`terraprism apply - Run plan, review, and apply

USAGE:
    terraprism [--tofu] apply [-- terraform-args]

DESCRIPTION:
    Runs terraform/tofu plan, displays in interactive TUI for review,
    then applies if you press 'a'.

    All output is saved to ~/.terraprism/ for history.

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

func printHistoryUsage() {
	fmt.Printf(`terraprism history - List plan/apply history

USAGE:
    terraprism history [options]

DESCRIPTION:
    Lists all plan and apply history files stored in ~/.terraprism/

OPTIONS:
    -h, --help      Show this help
    -p, --plan      Show only plan files
    -a, --apply     Show only apply files
    -d, --destroy   Show only destroy files
    --clear         Delete all history files

EXAMPLES:
    terraprism history              # List all history
    terraprism history --plan       # List only plans
    terraprism history --apply      # List only applies
    terraprism history --clear      # Clear all history

`)
}
