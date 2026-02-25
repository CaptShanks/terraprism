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
	"github.com/CaptShanks/terraprism/internal/updater"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.11.0"

var (
	printMode  = false
	forceLight = false
	forceDark  = false
	useTofu    = false
)

var tfPassthroughCommands = map[string]bool{
	"init": true, "validate": true, "fmt": true, "output": true,
	"state": true, "import": true, "workspace": true, "graph": true,
	"console": true, "login": true, "logout": true, "providers": true,
	"force-unlock": true, "show": true, "refresh": true,
	"taint": true, "untaint": true,
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func main() {
	args := os.Args[1:]

	// Load from env vars
	if v := os.Getenv("TERRAPRISM_TOFU"); isTruthy(v) {
		useTofu = true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TERRAPRISM_THEME"))) {
	case "light":
		forceLight = true
	case "dark":
		forceDark = true
	}

	// Apply color scheme
	if forceLight {
		tui.SetLightPalette()
	} else if forceDark {
		tui.SetDarkPalette()
	}

	// Dispatch on args[0]
	if len(args) == 0 {
		runViewMode(nil)
		return
	}
	switch args[0] {
	case "-h", "--help":
		printUsage()
		return
	case "-v", "--version":
		runVersionMode()
		return
	}
	if tfPassthroughCommands[args[0]] {
		runPassthroughMode(args)
		return
	}
	switch args[0] {
	case "apply":
		runApplyMode(args[1:], false)
		return
	case "destroy":
		runApplyMode(args[1:], true)
		return
	case "plan":
		runPlanMode(args[1:])
		return
	case "history":
		runHistoryMode(args[1:])
		return
	case "version":
		runVersionMode()
		return
	case "upgrade":
		runUpgradeMode()
		return
	}
	runViewMode(args)
}

func parseApplyArgs(args []string) []string {
	var tfArgs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printApplyUsage()
			os.Exit(0)
		case "--":
			tfArgs = append(tfArgs, args[i+1:]...)
			return tfArgs
		default:
			tfArgs = append(tfArgs, args[i])
		}
	}
	return tfArgs
}

func ensureDestroyFlag(tfArgs []string) []string {
	for _, arg := range tfArgs {
		if arg == "-destroy" {
			return tfArgs
		}
	}
	return append([]string{"-destroy"}, tfArgs...)
}

func runApplyExecute(tfCmd, planFile, historyPath string) error {
	if historyPath != "" {
		_ = history.AppendToHistoryFile(historyPath, "\n\n--- APPLY OUTPUT ---\n\n")
	}
	applyCmd := exec.Command(tfCmd, "apply", planFile)
	applyCmd.Stdout = os.Stdout
	applyCmd.Stderr = os.Stderr
	applyCmd.Stdin = os.Stdin
	return applyCmd.Run()
}

func updateHistoryApplyResult(historyPath string, success bool, applyErr error) {
	if historyPath == "" {
		return
	}
	if success {
		footer := history.CreateApplyResultFooter(true, nil)
		_ = history.AppendToHistoryFile(historyPath, footer)
		_, _ = history.UpdateFilenameWithStatus(historyPath, history.StatusSuccess)
	} else {
		footer := history.CreateApplyResultFooter(false, applyErr)
		_ = history.AppendToHistoryFile(historyPath, footer)
		_, _ = history.UpdateFilenameWithStatus(historyPath, history.StatusFailed)
	}
}

// runApplyMode runs terraform/tofu plan, shows TUI, and optionally applies
func runApplyMode(args []string, isDestroy bool) {
	tfArgs := parseApplyArgs(args)
	tfCmd := detectTFCommand()
	commandName := "apply"
	if isDestroy {
		commandName = "destroy"
		tfArgs = ensureDestroyFlag(tfArgs)
	}

	planFile := filepath.Join(os.TempDir(), fmt.Sprintf("terraprism-%d.tfplan", os.Getpid()))
	defer os.Remove(planFile)

	fmt.Printf("Terra-Prism: Running %s plan... ", tfCmd)
	planArgs := append([]string{"plan", "-out=" + planFile, "-no-color"}, tfArgs...)
	output, err := exec.Command(tfCmd, planArgs...).CombinedOutput()
	if err != nil {
		fmt.Println("FAILED")
		fmt.Fprintf(os.Stderr, "\n%s plan failed:\n%s\n", tfCmd, string(output))
		os.Exit(1)
	}
	fmt.Println("OK")

	historyHeader := history.CreateHistoryHeader("plan", tfCmd, tfArgs)
	historyPath, historyErr := history.CreateHistoryFile(commandName, historyHeader+string(output))
	if historyErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save history: %v\n", historyErr)
	}
	if deleted, _ := history.CleanupOldFiles(); deleted > 0 {
		fmt.Fprintf(os.Stderr, "Cleaned up %d old history files\n", deleted)
	}

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

	model := tui.NewModelWithApply(plan, planFile, tfCmd, version)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(tui.Model); ok && m.ShouldApply() {
		fmt.Printf("\nApplying plan with %s...\n\n", tfCmd)
		applyErr := runApplyExecute(tfCmd, planFile, historyPath)
		if applyErr != nil {
			fmt.Fprintf(os.Stderr, "\nApply failed: %v\n", applyErr)
			updateHistoryApplyResult(historyPath, false, applyErr)
			os.Exit(1)
		}
		fmt.Println("\nApply complete!")
		updateHistoryApplyResult(historyPath, true, nil)
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

	// Cleanup old history files
	if deleted, _ := history.CleanupOldFiles(); deleted > 0 {
		fmt.Fprintf(os.Stderr, "Cleaned up %d old history files\n", deleted)
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
		tui.NewModel(plan, version),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// runHistoryMode handles history subcommands: list, view
func runHistoryMode(args []string) {
	// Check for help first
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printHistoryUsage()
			os.Exit(0)
		}
	}

	// No args - show history help
	if len(args) == 0 {
		printHistoryUsage()
		os.Exit(0)
	}

	// Handle subcommands
	switch args[0] {
	case "list":
		runHistoryList(args[1:])
	case "view":
		runHistoryView(args[1:])
	case "--clear":
		clearHistory()
	default:
		// Check if it's a number (shorthand for view)
		if isNumeric(args[0]) {
			runHistoryView(args)
		} else {
			fmt.Fprintf(os.Stderr, "Unknown history subcommand: %s\n", args[0])
			printHistoryUsage()
			os.Exit(1)
		}
	}
}

// isNumeric checks if a string is a positive integer
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// runHistoryList lists history files
func runHistoryList(args []string) {
	filterCommand := ""

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
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
				fmt.Fprintln(os.Stderr, "Use 'terraprism history --help' for usage")
				os.Exit(1)
			}
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
	// Header: #(3) + 2 + timestamp(16) + 2 + command(7) + 2 + status(12) + 2 + path(40) = 86
	fmt.Printf("%3s  %-16s  %-7s  %-12s  %-40s\n", "#", "TIMESTAMP", "COMMAND", "STATUS", "PATH")
	fmt.Println(strings.Repeat("-", 86))

	for i, entry := range entries {
		path := entry.WorkingDir
		if path == "" {
			path = "-"
		}
		path = history.TruncatePath(path, 40)

		formatted := tui.FormatHistoryEntryColored(
			entry.Timestamp.Format("2006-01-02 15:04"),
			entry.Command,
			entry.Status,
			path,
		)
		fmt.Printf("%3d  %s\n", i+1, formatted)
	}

	fmt.Printf("\nTotal: %d entries (max: %d)\n", len(entries), history.MaxHistoryFiles)
	fmt.Println("\nUse 'terraprism history view <#>' to view a specific entry")
}

// runHistoryView opens a history file in the TUI
func runHistoryView(args []string) {
	var filePath string

	// No args - interactive picker
	if len(args) == 0 {
		entries, err := history.ListEntries("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading history: %v\n", err)
			os.Exit(1)
		}

		if len(entries) == 0 {
			histDir, _ := history.GetHistoryDir()
			fmt.Printf("No history files found in %s\n", histDir)
			os.Exit(0)
		}

		selectedPath, err := tui.RunPicker(entries)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running picker: %v\n", err)
			os.Exit(1)
		}

		if selectedPath == "" {
			// User cancelled
			os.Exit(0)
		}

		filePath = selectedPath
	} else {
		target := args[0]

		// Check if it's a number (index)
		if isNumeric(target) {
			var index int
			_, _ = fmt.Sscanf(target, "%d", &index)
			if index < 1 {
				fmt.Fprintln(os.Stderr, "Index must be 1 or greater")
				os.Exit(1)
			}

			entries, err := history.ListEntries("")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading history: %v\n", err)
				os.Exit(1)
			}

			if index > len(entries) {
				fmt.Fprintf(os.Stderr, "Index %d out of range (only %d entries)\n", index, len(entries))
				os.Exit(1)
			}

			filePath = entries[index-1].Path
		} else {
			// It's a filename - find the full path
			histDir, err := history.GetHistoryDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting history directory: %v\n", err)
				os.Exit(1)
			}
			filePath = filepath.Join(histDir, target)
		}
	}

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Parse and display in TUI
	plan, err := parser.Parse(string(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing plan: %v\n", err)
		os.Exit(1)
	}

	if printMode {
		tui.PrintPlan(plan)
		return
	}

	p := tea.NewProgram(
		tui.NewModel(plan, version),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
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

// runPassthroughMode runs terraform/tofu with the given args (e.g. init, validate, fmt)
func runPassthroughMode(args []string) {
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}
	tfCmd := detectTFCommand()
	cmd := exec.Command(tfCmd, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
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

	// Check for updates (skip if disabled)
	if !updater.IsSkipUpdateCheck() {
		if latest, hasUpdate, err := updater.CheckLatest(version); err == nil && hasUpdate {
			fmt.Printf("\nUpdate available: v%s. Run 'terraprism upgrade' to update (or re-run the install script).\n", latest)
		}
	}
}

// runUpgradeMode upgrades terraprism to the latest version
func runUpgradeMode() {
	_, hasUpdate, err := updater.CheckLatest(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking for updates: %v\n", err)
		fmt.Println(updater.CurlFallbackMessage(err))
		os.Exit(1)
	}
	if !hasUpdate {
		fmt.Println("Already up to date.")
		return
	}

	newVer, err := updater.Upgrade(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", updater.CurlFallbackMessage(err))
		os.Exit(1)
	}
	fmt.Printf("Upgraded to v%s. Restart terraprism to use the new version.\n", newVer)
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
		tui.NewModel(plan, version),
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
    terraprism plan [-- tf-args]                 # Run plan and view
    terraprism apply [-- tf-args]                # Run plan, view, and apply
    terraprism destroy [-- tf-args]              # Run destroy plan and apply
    terraprism init|validate|fmt|...             # Pass through to terraform/tofu
    terraprism history [options]                 # List history files

DESCRIPTION:
    Terra-Prism provides an interactive terminal UI for viewing Terraform and
    OpenTofu plans with collapsible resources and syntax highlighting.

COMMANDS:
    (none)      View mode - pipe or file input
    plan        Run terraform/tofu plan and view interactively
    apply       Run plan, review in TUI, press 'a' to apply
    destroy     Run destroy plan, review in TUI, press 'a' to destroy
    history     View and manage plan/apply history
    version     Show terraprism and terraform/tofu versions
    upgrade     Upgrade terraprism to the latest release
    init, validate, fmt, output, state, import, workspace, graph,
    console, login, logout, providers, force-unlock, show, refresh,
    taint, untaint   Pass through to terraform/tofu (e.g. state list)

GLOBAL OPTIONS:
    -h, --help      Show this help
    -v, --version   Show version (includes update check)

ENVIRONMENT:
    TERRAPRISM_TOFU   Set to 1, true, or yes to use OpenTofu
    TERRAPRISM_THEME  Set to "light" or "dark" to force theme
    TERRAPRISM_SKIP_UPDATE_CHECK  Set to 1, true, or yes to skip update checks
    TERRAPRISM_UPDATE_CHECK_INTERVAL  Days between TUI update checks (default: 7)

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

    # Use tofu (set TERRAPRISM_TOFU=1 in your shell)
    TERRAPRISM_TOFU=1 terraprism apply

    # Pass extra args to terraform/tofu
    terraprism apply -- -target=module.vpc -var="env=prod"

    # View history
    terraprism history

`, version)
}

func printApplyUsage() {
	fmt.Printf(`terraprism apply - Run plan, review, and apply

USAGE:
    terraprism apply [-- terraform-args]

DESCRIPTION:
    Runs terraform/tofu plan, displays in interactive TUI for review,
    then applies if you press 'a'.

    All output is saved to ~/.terraprism/ for history.

ENVIRONMENT:
    TERRAPRISM_TOFU   Set to 1, true, or yes to use OpenTofu
    TERRAPRISM_THEME  Set to "light" or "dark" to force theme

TERRAFORM ARGS:
    --          Everything after this is passed to terraform/tofu

CONTROLS IN TUI:
    a           Apply the plan
    y           Confirm apply
    q/Esc       Cancel and quit

EXAMPLES:
    terraprism apply
    TERRAPRISM_TOFU=1 terraprism apply
    terraprism apply -- -target=module.vpc
    terraprism apply -- -var="env=prod"

`)
}

func printHistoryUsage() {
	fmt.Printf(`terraprism history - Manage plan/apply history

USAGE:
    terraprism history <subcommand> [options]

DESCRIPTION:
    View and manage plan/apply history files stored in ~/.terraprism/

SUBCOMMANDS:
    list            List all history files
    view            Interactive picker to select and view
    view <#|file>   View a history file in the TUI
                    # = index (1 = most recent)
                    file = exact filename

LIST OPTIONS:
    -p, --plan      Show only plan files
    -a, --apply     Show only apply files
    -d, --destroy   Show only destroy files
    --clear         Delete all history files

EXAMPLES:
    terraprism history list              # List all history
    terraprism history list --plan       # List only plans
    terraprism history list --apply      # List only applies
    terraprism history list --clear      # Clear all history
    terraprism history view              # Interactive picker
    terraprism history view 1            # View most recent entry
    terraprism history view 3            # View 3rd most recent
    terraprism history 1                 # Shorthand for 'view 1'
    terraprism history view 2025-01-14_10-30-00_plan.txt

`)
}
