package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tfplanview/tfplanview/internal/parser"
	"github.com/tfplanview/tfplanview/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.1.0"

var (
	printMode  = false
	forceLight = false
	forceDark  = false
)

func main() {
	// Check for help/version flags
	args := os.Args[1:]
	var inputFile string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			printUsage()
			os.Exit(0)
		case "-v", "--version":
			fmt.Printf("tfplanview %s\n", version)
			os.Exit(0)
		case "-p", "--print":
			printMode = true
		case "--light":
			forceLight = true
		case "--dark":
			forceDark = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				inputFile = args[i]
			}
		}
	}

	// Apply color scheme override
	if forceLight {
		tui.SetLightPalette()
	} else if forceDark {
		tui.SetDarkPalette()
	}

	// Read from stdin or file
	var input io.Reader

	if inputFile != "" && inputFile != "-" {
		// Read from file
		file, err := os.Open(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		input = file
	} else {
		// Check if stdin has data
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			// No piped input, show usage
			printUsage()
			os.Exit(0)
		}
		input = os.Stdin
	}

	// Read all input
	var lines []string
	scanner := bufio.NewScanner(input)
	// Increase buffer size for large plans
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

	// Parse the plan
	plan, err := parser.Parse(planText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing plan: %v\n", err)
		os.Exit(1)
	}

	if len(plan.Resources) == 0 {
		fmt.Println("No resource changes detected in the plan.")
		os.Exit(0)
	}

	// Print mode - just output colored text without TUI
	if printMode {
		tui.PrintPlan(plan)
		os.Exit(0)
	}

	// Create and run the TUI
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
	fmt.Printf(`tfplanview %s - Interactive Terraform/OpenTofu plan viewer

USAGE:
    terraform plan | tfplanview
    tofu plan | tfplanview
    tfplanview <plan-file>
    tfplanview -p <plan-file>    # Print mode (no TUI)

DESCRIPTION:
    tfplanview provides an interactive terminal UI for viewing Terraform and
    OpenTofu plans. It parses plan output and displays resources in a 
    collapsible, color-coded format for easier review.

CONTROLS:
    ↑/k         Move cursor up
    ↓/j         Move cursor down
    Enter/Space Toggle expand/collapse current resource
    e           Expand all resources
    c           Collapse all resources
    /           Search resources
    n           Next search result
    N           Previous search result
    q/Esc       Quit

OPTIONS:
    -h, --help      Show this help message
    -v, --version   Show version
    -p, --print     Print colored output without interactive TUI
    --light         Force light color scheme (Catppuccin Latte)
    --dark          Force dark color scheme (Catppuccin Mocha)

EXAMPLES:
    # Pipe from terraform
    terraform plan -no-color | tfplanview

    # Pipe from tofu
    tofu plan -no-color | tfplanview

    # Read from file
    terraform plan -no-color -out=plan.txt && tfplanview plan.txt

    # Print mode (for piping or non-interactive use)
    terraform plan -no-color | tfplanview -p

`, version)
}
