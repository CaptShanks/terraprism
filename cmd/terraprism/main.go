package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/CaptShanks/terraprism/internal/history"
	"github.com/CaptShanks/terraprism/internal/runner"
	"github.com/CaptShanks/terraprism/internal/tfplan"
	"github.com/CaptShanks/terraprism/internal/tui"
	"github.com/CaptShanks/terraprism/internal/updater"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.14.0"

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
	// Intercept state list/show/rm before passthrough; state mv etc. fall through
	if args[0] == "state" && len(args) >= 2 {
		switch args[1] {
		case "list", "show", "rm":
			runStateMode(args)
			return
		}
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

func updateHistoryApplyResult(historyPath string, success bool) {
	if historyPath == "" {
		return
	}
	status := history.StatusSuccess
	if !success {
		status = history.StatusFailed
	}
	_, _ = history.UpdateFilenameWithStatus(historyPath, status)
}

func saveHistory(commandName, tfCmd string, tfArgs []string, rawJSON []byte) string {
	meta := history.Meta{
		Timestamp:  time.Now(),
		Command:    commandName,
		TFCommand:  tfCmd,
		Args:       tfArgs,
		WorkingDir: history.GetFullWorkingDir(),
	}
	historyPath, err := history.CreateHistoryFile(commandName, meta, rawJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save history: %v\n", err)
	}
	if deleted, _ := history.CleanupOldFiles(); deleted > 0 {
		fmt.Fprintf(os.Stderr, "Cleaned up %d old history files\n", deleted)
	}
	return historyPath
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

	// Plan runs *inside* the TUI itself (tui.Model.startPlanCmd, via
	// runner.PlanStream), streaming its output live into the output pane
	// -- the TUI launches immediately with an empty tree that gets
	// populated once planning completes, rather than main.go running
	// plan synchronously beforehand. Apply, once confirmed, streams the
	// same way (runner.ApplyStream). By the time p.Run() returns, both
	// are guaranteed finished (quitting is blocked mid-run), which is
	// what keeps the plan-file cleanup below safe.
	opts := runner.Options{
		Cmd:          runner.TFCommand(tfCmd),
		Args:         tfArgs,
		KeepPlanFile: true,
	}
	model := tui.NewModelPlanning(opts, version, true)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	m, ok := finalModel.(tui.Model)
	if !ok {
		return // unreachable: finalModel is always the tui.Model this function built
	}
	planFile := m.PlanFile()
	if planFile != "" {
		defer os.Remove(planFile)
	}

	if err := m.PlanErr(); err != nil {
		reportRunError(tfCmd, err)
		if planFile != "" {
			_ = os.Remove(planFile) // os.Exit below skips the deferred cleanup above
		}
		os.Exit(1)
	}

	historyPath := saveHistory(commandName, tfCmd, tfArgs, m.PlanRawJSON())

	if len(m.Plan().DisplayResources()) == 0 {
		fmt.Println("No changes. Infrastructure is up-to-date.")
		if historyPath != "" {
			_, _ = history.UpdateFilenameWithStatus(historyPath, "nochanges")
		}
		return
	}

	switch {
	case !m.ApplyAttempted():
		fmt.Println("\nApply cancelled.")
		if historyPath != "" {
			_, _ = history.UpdateFilenameWithStatus(historyPath, history.StatusCancelled)
		}
	case m.ApplyResult() == nil:
		fmt.Print(m.ApplyOutput())
		fmt.Println("\nApply complete!")
		updateHistoryApplyResult(historyPath, true)
	default:
		fmt.Print(m.ApplyOutput())
		fmt.Fprintf(os.Stderr, "\nApply failed: %v\n", m.ApplyResult())
		updateHistoryApplyResult(historyPath, false)
		if planFile != "" {
			_ = os.Remove(planFile) // os.Exit below skips the deferred cleanup above
		}
		os.Exit(1)
	}
}

// reportRunError prints a runner error in a form matching the CLI's own
// plan/show failure output.
func reportRunError(tfCmd string, err error) {
	var planErr *runner.PlanError
	if errors.As(err, &planErr) {
		fmt.Fprintf(os.Stderr, "\n%s plan failed:\n%s\n", tfCmd, string(planErr.Output))
		return
	}
	fmt.Fprintf(os.Stderr, "\n%v\n", err)
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
	opts := runner.Options{Cmd: runner.TFCommand(tfCmd), Args: tfArgs}

	// Plan streams live into the TUI's output pane -- see runApplyMode's
	// comment for why the TUI launches before plan has even started.
	model := tui.NewModelPlanning(opts, version, false)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	m, ok := finalModel.(tui.Model)
	if !ok {
		return // unreachable: finalModel is always the tui.Model this function built
	}

	if err := m.PlanErr(); err != nil {
		reportRunError(tfCmd, err)
		os.Exit(1)
	}

	saveHistory("plan", tfCmd, tfArgs, m.PlanRawJSON())

	if len(m.Plan().DisplayResources()) == 0 {
		fmt.Println("No changes. Infrastructure is up-to-date.")
		return
	}

	fmt.Print(m.PlanOutput())
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

	// Read the history envelope and decode the plan JSON inside it
	envelope, err := history.ReadEnvelope(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading history file: %v\n", err)
		os.Exit(1)
	}

	plan, err := tfplan.DecodeBytes(envelope.Plan)
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
	return string(runner.DetectCommand(useTofu))
}

// looksLikeJSON reports whether raw's first non-whitespace byte is '{',
// distinguishing `terraform show -json` output from plain plan text or a
// binary `-out=` plan file.
func looksLikeJSON(raw []byte) bool {
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	return len(trimmed) > 0 && trimmed[0] == '{'
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

// runStateMode runs terraform state list, parses addresses, and shows the state TUI
func runStateMode(args []string) {
	tfCmd := detectTFCommand()

	// Extract terraform options from args[2:] (after "state" and subcommand)
	tfStateArgs := []string{}
	if len(args) > 2 {
		tfStateArgs = args[2:]
	}

	// Run terraform state list
	listArgs := append([]string{"state", "list"}, tfStateArgs...)
	output, err := exec.Command(tfCmd, listArgs...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s state list failed:\n%s\n", tfCmd, string(output))
		os.Exit(1)
	}

	// Parse addresses: one per line, trim, filter empty
	lines := strings.Split(string(output), "\n")
	var addresses []string
	for _, line := range lines {
		addr := strings.TrimSpace(line)
		if addr != "" {
			addresses = append(addresses, addr)
		}
	}

	if len(addresses) == 0 {
		fmt.Println("No resources in state.")
		os.Exit(0)
	}

	fmt.Printf("Terra-Prism: %d resources in state\n", len(addresses))

	p := tea.NewProgram(
		tui.NewStateModel(addresses, tfCmd, tfStateArgs, version),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running state TUI: %v\n", err)
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

	raw, err := io.ReadAll(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	if !looksLikeJSON(raw) {
		fmt.Fprintln(os.Stderr, "Error: input does not look like terraform show -json output.")
		fmt.Fprintln(os.Stderr, "terraprism needs a JSON plan, not plain 'terraform plan' text or a binary plan file. Run:")
		fmt.Fprintln(os.Stderr, "    terraform plan -out=plan.bin && terraform show -json plan.bin | terraprism")
		os.Exit(1)
	}

	plan, err := tfplan.DecodeBytes(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing plan: %v\n", err)
		os.Exit(1)
	}

	if len(plan.DisplayResources()) == 0 {
		fmt.Println("No changes detected in the plan.")
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
    terraform plan -out=plan.bin && terraform show -json plan.bin | terraprism
                                                  # Pipe JSON plan output
    terraprism <plan.json>                       # Read a show -json file
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
    state list|show|rm   Interactive state TUI (search, sort, taint, untaint)
    history     View and manage plan/apply history
    version     Show terraprism and terraform/tofu versions
    upgrade     Upgrade terraprism to the latest release
    init, validate, fmt, output, state mv, import, workspace, graph,
    console, login, logout, providers, force-unlock, show, refresh,
    taint, untaint   Pass through to terraform/tofu

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
    a           Apply (only in apply mode); a then y streams live output
    o           Toggle the plan/apply output pane (takes most of the
                screen; has its own j/k/gg/G nav, / search with match
                highlighting, w to toggle word wrap (off by default),
                and h/l or left/right to scroll sideways while open)
    q/Esc       Quit (disabled while a plan or apply is in progress)

HISTORY:
    All plan and apply outputs are saved to ~/.terraprism/
    Use 'terraprism history' to list them.

EXAMPLES:
    # View a piped JSON plan
    terraform plan -out=plan.bin && terraform show -json plan.bin | terraprism

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
    Launches the TUI immediately and runs terraform/tofu plan inside it,
    streaming live output into a toggleable pane ('o') while the tree
    populates. Once you review the plan, press 'a' then 'y' to apply --
    apply streams into the same pane. Neither step exits to the plain
    terminal first.

    All output is saved to ~/.terraprism/ for history.

ENVIRONMENT:
    TERRAPRISM_TOFU   Set to 1, true, or yes to use OpenTofu
    TERRAPRISM_THEME  Set to "light" or "dark" to force theme

TERRAFORM ARGS:
    --          Everything after this is passed to terraform/tofu

CONTROLS IN TUI:
    a           Apply the plan
    y           Confirm apply (starts a live-streamed apply)
    o           Toggle the plan/apply output pane (takes most of the
                screen; has its own j/k/gg/G nav, / search with match
                highlighting, w to toggle word wrap (off by default),
                and h/l or left/right to scroll sideways while open)
    q/Esc       Cancel and quit (disabled while a plan or apply is running)

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
    terraprism history view 2025-01-14_10-30-00_myproj_plan.json

`)
}
