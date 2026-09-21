package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"

	"github.com/CaptShanks/terraprism/internal/foldtree"
	"github.com/CaptShanks/terraprism/internal/runner"
	"github.com/CaptShanks/terraprism/internal/tfplan"
	"github.com/CaptShanks/terraprism/internal/updater"
)

// Model represents the TUI state
type Model struct {
	plan            *tfplan.Plan
	treeView        foldtree.TreeView // owns nav, viewport, and search
	outputPane      foldtree.LogPane  // hideable pane showing captured/streamed terraform output; outputPane.Visible() is the single source of truth for whether it's shown
	planOutput      string            // captured `plan` output, shown in the output pane via 'o'
	defaultsApplied map[string]bool   // node IDs that have already had a default collapse state applied
	diffContext     int
	revealSensitive bool // 'x' toggle: show real values for sensitive attributes instead of "(sensitive value)"
	ready           bool
	width           int
	height          int

	// spinner animates the "Running <cmd>..." banner while planning or
	// applying; ticking is only kept alive while one of those is true
	// (see the spinner.TickMsg case in Update) so it doesn't run forever
	// in the background once nothing is left running.
	spinner spinner.Model

	// Plan-streaming fields: when planning is true, the model starts with
	// an empty plan and Init() kicks off runner.PlanStream itself,
	// populating the real plan and hiding the output pane once it lands.
	planOptions runner.Options
	planning    bool
	planRawJSON []byte
	planErr     error
	planLines   <-chan runner.PlanLine
	planDone    <-chan runner.PlanStreamResult
	planCmdLine string // the exact `plan` command invoked, once known (see PlanStream); "" until then

	// Apply mode fields
	applyMode      bool   // Whether apply is available
	planFile       string // Path to the plan file
	tfCommand      string // "terraform" or "tofu"
	confirmApply   bool   // Waiting for confirmation
	applying       bool   // apply subprocess is currently running
	applyAttempted bool   // an apply was started at some point, regardless of outcome
	applyResult    error  // nil until applyAttempted && !applying; nil then means success
	applyOutput    string // captured `apply` invocation's own combined stdout+stderr output
	applyCmdLine   string // the exact `apply` command invoked, once known (see ApplyStream); "" until then
	applyLines     <-chan runner.ApplyLine
	applyDone      <-chan runner.ApplyStreamResult

	// applyQuitCountdown counts down the seconds left before the TUI
	// auto-quits after an apply finishes (success or failure); 0 means
	// no countdown is running (either none has started yet, or Esc
	// cancelled it -- see handleKeyEsc).
	applyQuitCountdown int

	// Status filter fields
	filterPicker foldtree.Picker[tfplan.Action]
	filtering    bool // filter picker is open

	// Sort fields
	sortPicker foldtree.Picker[SortOrder]
	sorting    bool // sort picker is open

	// Update nudge
	currentVersion  string // for update check
	updateAvailable string // non-empty when newer version available
}

// UpdateAvailableMsg is sent when an update check finds a newer version.
type UpdateAvailableMsg struct {
	Version string
}

// SortOrder defines how resources are ordered
type SortOrder string

const (
	SortDefault   SortOrder = "default"
	SortByAction  SortOrder = "action"
	SortByAddress SortOrder = "address"
	SortByType    SortOrder = "type"
)

// sortOptions is the ordered list of sort choices for the picker
var sortOptions = []SortOrder{SortDefault, SortByAction, SortByAddress, SortByType}

// actionOrder defines sort order for actions (destructive last)
var actionOrder = map[tfplan.Action]int{
	tfplan.ActionCreate:  0,
	tfplan.ActionRead:    1,
	tfplan.ActionUpdate:  2,
	tfplan.ActionReplace: 3,
	tfplan.ActionForget:  4,
	tfplan.ActionDelete:  5,
	tfplan.ActionOutput:  6,
	tfplan.ActionNoOp:    7,
}

// filterableActions is the ordered list of statuses available for filtering
var filterableActions = []tfplan.Action{
	tfplan.ActionCreate,
	tfplan.ActionDelete,
	tfplan.ActionUpdate,
	tfplan.ActionReplace,
	tfplan.ActionRead,
	tfplan.ActionForget,
	tfplan.ActionOutput,
}

// filteredResources returns indices into plan.Resources that pass the status filter.
// When no filter is active, returns all indices.
func (m *Model) filteredResources() []int {
	if len(m.filterPicker.Selected) == 0 {
		indices := make([]int, len(m.plan.Resources))
		for i := range m.plan.Resources {
			indices[i] = i
		}
		return indices
	}
	var indices []int
	for i, r := range m.plan.Resources {
		if m.filterPicker.Selected[r.Action] {
			indices = append(indices, i)
		}
	}
	return indices
}

// sortedResources returns filtered indices sorted by the current sort order.
func (m *Model) sortedResources() []int {
	filtered := m.filteredResources()
	order := m.sortPicker.Current()
	if order == SortDefault || order == "" {
		return filtered
	}
	sort.Slice(filtered, func(i, j int) bool {
		ri := m.plan.Resources[filtered[i]]
		rj := m.plan.Resources[filtered[j]]
		switch order {
		case SortByAction:
			oi, oki := actionOrder[ri.Action]
			oj, okj := actionOrder[rj.Action]
			if !oki {
				oi = 99
			}
			if !okj {
				oj = 99
			}
			if oi != oj {
				return oi < oj
			}
			return ri.Address < rj.Address
		case SortByAddress:
			return ri.Address < rj.Address
		case SortByType:
			if ri.Type != rj.Type {
				return ri.Type < rj.Type
			}
			return ri.Address < rj.Address
		}
		return false
	})
	return filtered
}

// newFilterPicker builds the multi-select "filter by status" picker,
// with each option's label colored the same way its resource rows are.
func newFilterPicker() foldtree.Picker[tfplan.Action] {
	p := foldtree.NewPicker(filterableActions, func(a tfplan.Action) string {
		return GetResourceStyle(string(a)).Render(filterActionLabel(a))
	})
	p.Multi = true
	return *p
}

// newSortPicker builds the single-select "sort by" picker.
func newSortPicker() foldtree.Picker[SortOrder] {
	p := foldtree.NewPicker(sortOptions, sortOrderLabel)
	p.Hint = sortOrderHint
	p.SetCurrent(SortDefault)
	return *p
}

// treeHeightWithOutputVisible is the most lines the tree view keeps for
// context when the output pane is open -- a cap, not a fixed reservation
// (see treeContentHeight): the pane itself takes the rest of the content
// height, since it's usually the thing being actively read (live
// plan/apply progress, or a captured log to search) while it's open.
const treeHeightWithOutputVisible = 6

// newModel builds the shared TreeView+LogPane plumbing for both NewModel
// and NewModelWithApply.
func newModel(plan *tfplan.Plan, version string, planOutput string) Model {
	m := Model{
		plan:            withDisplayResources(plan),
		defaultsApplied: make(map[string]bool),
		diffContext:     defaultDiffContext,
		filterPicker:    newFilterPicker(),
		sortPicker:      newSortPicker(),
		currentVersion:  version,
		planOutput:      planOutput,
		spinner:         spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	m.treeView = *foldtree.NewTreeView(m)
	m.outputPane = *foldtree.NewLogPane()
	if planOutput != "" {
		m.outputPane.SetLines(strings.Split(planOutput, "\n"))
		m.outputPane.SetTitle("Plan Output")
	}
	return m
}

// NewModel creates a new TUI model (view-only mode)
func NewModel(plan *tfplan.Plan, version string) Model {
	m := newModel(plan, version, "")
	m.applyMode = false
	return m
}

// NewModelWithApply creates a TUI model with apply capability.
// planOutput is the already-captured `plan` invocation's own output
// (runner.PlanResult.Output), shown in the output pane via 'o' before
// any apply has run.
func NewModelWithApply(plan *tfplan.Plan, planFile, tfCommand, version, planOutput string) Model {
	m := newModel(plan, version, planOutput)
	m.applyMode = true
	m.planFile = planFile
	m.tfCommand = tfCommand
	return m
}

// NewModelPlanning creates a TUI model that starts with an empty plan
// and runs runner.PlanStream itself (kicked off from Init()), streaming
// `plan`'s output live into the output pane instead of the caller
// running it synchronously beforehand. Once planning completes
// successfully, the real plan replaces the empty one, the tree is built,
// and the output pane hides itself again (still reachable via 'o').
// applyMode controls whether 'a'/'y' are active once planning finishes,
// exactly as with NewModel vs NewModelWithApply.
func NewModelPlanning(opts runner.Options, version string, applyMode bool) Model {
	m := newModel(&tfplan.Plan{}, version, "")
	m.applyMode = applyMode
	m.tfCommand = string(opts.Cmd)
	m.planOptions = opts
	m.planning = true
	m.outputPane.SetTitle("Plan Output")
	m.outputPane.SetVisible(true)
	return m
}

// withDisplayResources returns a Plan whose Resources includes output
// changes as synthetic entries (see tfplan.Plan.DisplayResources), so the
// rest of the model's rendering/navigation/filtering/sorting code — which
// only ever reads plan.Resources — picks them up automatically without
// needing its own separate output-change code path. The original plan is
// left untouched (callers may still hold onto it, e.g. for history).
func withDisplayResources(plan *tfplan.Plan) *tfplan.Plan {
	if plan == nil || len(plan.OutputChanges) == 0 {
		return plan
	}
	display := *plan
	display.Resources = plan.DisplayResources()
	return &display
}

// ApplyAttempted reports whether an apply was ever started, regardless
// of outcome -- meaningful only after the TUI program has exited (quit
// is blocked while an apply is still in flight, so by then it's settled).
func (m Model) ApplyAttempted() bool {
	return m.applyAttempted
}

// ApplyResult reports the outcome of the apply started during this
// session: nil if it succeeded (or hasn't been attempted -- check
// ApplyAttempted first), non-nil on failure.
func (m Model) ApplyResult() error {
	return m.applyResult
}

// ApplyOutput returns the captured `apply` invocation's own combined
// stdout+stderr output, populated once ApplyAttempted() is true and the
// stream has finished (success or failure). Empty if apply was never
// attempted.
func (m Model) ApplyOutput() string {
	return m.applyOutput
}

// Plan returns the current plan -- for a Model built via NewModelPlanning,
// only meaningful once the program has exited and PlanErr() is nil.
func (m Model) Plan() *tfplan.Plan {
	return m.plan
}

// PlanFile returns the on-disk path of the binary plan file produced by
// a NewModelPlanning run (empty if planning hasn't completed, failed, or
// KeepPlanFile wasn't set).
func (m Model) PlanFile() string {
	return m.planFile
}

// PlanRawJSON returns the raw `show -json` bytes decoded during a
// NewModelPlanning run (nil if planning hasn't completed or failed).
func (m Model) PlanRawJSON() []byte {
	return m.planRawJSON
}

// PlanErr reports why a NewModelPlanning run's plan step failed, or nil
// if it hasn't been attempted, is still running, or succeeded.
func (m Model) PlanErr() error {
	return m.planErr
}

// PlanOutput returns the captured `plan` invocation's own combined
// stdout+stderr output (populated once a NewModelPlanning run's plan
// step has completed successfully, or as supplied directly via
// NewModel/NewModelWithApply).
func (m Model) PlanOutput() string {
	return m.planOutput
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.currentVersion != "" && !updater.IsSkipUpdateCheck() {
		cmds = append(cmds, checkUpdateCmd(m.currentVersion))
	}
	if m.planning {
		cmds = append(cmds, m.startPlanCmd(), m.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

// checkUpdateCmd runs an async update check and sends UpdateAvailableMsg if an update is available.
func checkUpdateCmd(version string) tea.Cmd {
	return func() tea.Msg {
		latest, hasUpdate, err := updater.CheckLatestWithCache(version, updater.UpdateCheckIntervalDays())
		if err != nil || !hasUpdate {
			return nil
		}
		return UpdateAvailableMsg{Version: latest}
	}
}

// chromeHeight returns the header/footer line budget outside the tree
// view's own content area, given the current update-nudge state.
func (m Model) chromeHeight() (header, footer int) {
	header = 4 // Title + summary + blank line
	footer = 3 // Help text
	if m.updateAvailable != "" {
		footer = 4 // +1 for update nudge line
	}
	return header, footer
}

// reflow (re)computes the height split between treeView and the output
// pane from the last known terminal size, and pushes a resize into both
// -- needed on every actual terminal resize, but also whenever the
// output pane's visibility is toggled, since that changes the split
// without a new tea.WindowSizeMsg arriving. The output pane is resized
// even while hidden, so it's immediately ready the instant it's shown.
func (m *Model) reflow() {
	if !m.ready {
		return
	}
	header, footer := m.chromeHeight()
	contentWidth := m.width - 4
	contentHeight := m.height - header - footer

	treeHeight := contentHeight
	if m.outputPane.Visible() {
		treeHeight = m.treeContentHeight()
	}
	if treeHeight > contentHeight {
		treeHeight = contentHeight
	}
	if treeHeight < 1 {
		treeHeight = 1
	}
	auxHeight := contentHeight - treeHeight
	if auxHeight < 0 {
		auxHeight = 0
	}

	newTV, _ := m.treeView.Update(tea.WindowSizeMsg{Width: contentWidth, Height: treeHeight})
	m.treeView = newTV.(foldtree.TreeView)
	newPane, _ := m.outputPane.Update(tea.WindowSizeMsg{Width: contentWidth, Height: auxHeight})
	m.outputPane = newPane.(foldtree.LogPane)
}

// treeContentHeight returns how many lines the tree pane actually needs
// to show its current content -- the empty-state message, or the sum of
// each currently visible row's own height -- capped at
// treeHeightWithOutputVisible so a large tree still leaves the output
// pane most of the screen. Used instead of always reserving the full cap
// so the output pane sits right below the tree/status area rather than
// after a block of wasted blank space when there's little or nothing to
// show above it, e.g. an empty tree while a plan is still streaming.
func (m Model) treeContentHeight() int {
	rows := m.treeView.State().Rows()
	if len(rows) == 0 {
		return 2 // the empty-state message plus its own trailing blank line
	}
	h := 0
	for _, r := range rows {
		h += r.Height
	}
	if h > treeHeightWithOutputVisible {
		h = treeHeightWithOutputVisible
	}
	return h
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case UpdateAvailableMsg:
		m.updateAvailable = msg.Version
		m.reflow()
		return m, nil

	case spinner.TickMsg:
		// Only keep the loop alive while there's actually something to
		// animate for -- otherwise this would tick forever in the
		// background for the rest of the session once planning/applying
		// finished.
		if !m.planning && !m.applying {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.reflow()
		// Width and height both affect the tree: width drives leaf/diff
		// wrapping (so declared node heights change), height drives the
		// viewport's own scroll math -- both need a full rebuild.
		m.rebuildTree()
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			return m.handleFilterKey(msg)
		}
		if m.sorting {
			return m.handleSortKey(msg)
		}
		if m.outputPane.Visible() && m.outputPane.SearchActive() {
			newPane, cmd := m.outputPane.Update(msg)
			m.outputPane = newPane.(foldtree.LogPane)
			return m, cmd
		}
		if m.treeView.SearchActive() {
			newTV, cmd := m.treeView.Update(msg)
			m.treeView = newTV.(foldtree.TreeView)
			return m, cmd
		}
		return m.handleNormalKey(msg)

	case tea.MouseMsg:
		if m.outputPane.Visible() {
			newPane, cmd := m.outputPane.Update(msg)
			m.outputPane = newPane.(foldtree.LogPane)
			return m, cmd
		}
		newTV, cmd := m.treeView.Update(msg)
		m.treeView = newTV.(foldtree.TreeView)
		return m, cmd

	case applyStreamStartedMsg:
		m.applyAttempted = true
		m.applyCmdLine = msg.cmdLine
		m.applyLines = msg.lines
		m.applyDone = msg.done
		m.outputPane.Reset()
		m.outputPane.SetTitle("Apply Output")
		m.outputPane.SetVisible(true)
		m.reflow()
		return m, waitForApplyEvent(msg.lines, msg.done)

	case applyLineMsg:
		m.outputPane.Append(msg.Text)
		return m, waitForApplyEvent(m.applyLines, m.applyDone)

	case applyDoneMsg:
		m.applying = false
		if msg.result != nil {
			m.applyOutput = string(msg.result.Output)
			m.applyResult = nil
		} else if applyErr, ok := msg.err.(*runner.ApplyError); ok {
			m.applyOutput = string(applyErr.Output)
			m.applyResult = applyErr.Err
		} else {
			m.applyResult = msg.err
		}
		m.applyQuitCountdown = applyQuitCountdownStart
		return m, tickApplyQuitCmd()

	case applyQuitTickMsg:
		if m.applyQuitCountdown <= 0 {
			return m, nil // cancelled via Esc since this tick was scheduled
		}
		m.applyQuitCountdown--
		if m.applyQuitCountdown <= 0 {
			return m, tea.Quit
		}
		return m, tickApplyQuitCmd()

	case planStreamStartedMsg:
		m.planCmdLine = msg.cmdLine
		m.planLines = msg.lines
		m.planDone = msg.done
		return m, waitForPlanEvent(msg.lines, msg.done)

	case planLineMsg:
		m.outputPane.Append(msg.Text)
		return m, waitForPlanEvent(m.planLines, m.planDone)

	case planDoneMsg:
		m.planning = false
		if msg.err != nil {
			m.planErr = msg.err
			return m, nil
		}
		m.plan = withDisplayResources(msg.result.Plan)
		m.planFile = msg.result.PlanFile
		m.planRawJSON = msg.result.RawJSON
		m.planOutput = string(msg.result.Output)
		m.outputPane.SetLines(strings.Split(m.planOutput, "\n"))
		m.outputPane.SetVisible(false)
		m.rebuildTree()
		m.reflow()
		if !m.hasApplicableChanges() {
			// Nothing to review or apply -- the live-streamed plan output
			// already showed this while planning ran, and main.go prints
			// "No changes. Infrastructure is up-to-date." right after the
			// TUI exits either way, so there's no reason to make the user
			// press 'q' themselves on an empty tree.
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

const (
	defaultDiffContext = 3
	diffContextStep    = 3
	maxDiffContext     = 30
)

func (m Model) diffContextSize() int {
	return clampDiffContext(m.diffContext)
}

func clampDiffContext(context int) int {
	if context < 0 {
		return 0
	}
	if context > maxDiffContext {
		return maxDiffContext
	}
	return context
}

// normalKeyHandler handles a single key in normal mode. Returns (model, cmd, quit).
type normalKeyHandler func(m Model) (Model, tea.Cmd, bool)

// tuiKeyHandlers covers only terraprism-specific keys (quitting, opening
// pickers, diff context, apply) -- anything not listed here is forwarded
// to treeView.Update, which owns all generic navigation/expand-collapse/
// search key handling.
var tuiKeyHandlers = map[string]normalKeyHandler{
	"q":      handleKeyQuit,
	"ctrl+c": handleKeyQuit,
	"f":      handleKeyFilter,
	"s":      handleKeySort,
	"o":      handleKeyToggleOutput,
	"esc":    handleKeyEsc,
	"+":      handleKeyIncreaseDiffContext,
	"=":      handleKeyIncreaseDiffContext,
	"-":      handleKeyDecreaseDiffContext,
	"x":      handleKeyToggleSensitive,
	"a":      handleKeyApply,
	"y":      handleKeyConfirmApply,
}

// handleKeyQuit quits, unless a plan or apply subprocess is currently
// running -- letting the program exit mid-run would race main.go's
// plan-file handling against the still-running subprocess and orphan
// it, since nothing left running after the TUI exits could still cancel
// it.
func handleKeyQuit(m Model) (Model, tea.Cmd, bool) {
	if m.applying || m.planning {
		return m, nil, true
	}
	return m, tea.Quit, true
}

// handleKeyToggleOutput shows/hides the output pane. There's nothing to
// show until a `plan` has actually produced output (NewModel's pure
// view-only path never runs one).
func handleKeyToggleOutput(m Model) (Model, tea.Cmd, bool) {
	if m.planOutput == "" {
		return m, nil, true
	}
	m.outputPane.Toggle()
	m.reflow()
	return m, nil, true
}

func handleKeyFilter(m Model) (Model, tea.Cmd, bool) {
	if m.planning {
		return m, nil, true
	}
	m.filtering = true
	m.filterPicker.SetCursor(0)
	return m, nil, true
}

func handleKeySort(m Model) (Model, tea.Cmd, bool) {
	if m.planning {
		return m, nil, true
	}
	m.sorting = true
	m.sortPicker.SetCurrent(m.sortPicker.Current()) // reseed cursor to match today's active order
	return m, nil, true
}

func handleKeyEsc(m Model) (Model, tea.Cmd, bool) {
	if m.applyQuitCountdown > 0 {
		m.applyQuitCountdown = 0
		return m, nil, true
	}
	if len(m.filterPicker.Selected) > 0 {
		for k := range m.filterPicker.Selected {
			delete(m.filterPicker.Selected, k)
		}
		m.rebuildTree()
		return m, nil, true
	}
	newTV, cmd := m.treeView.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.treeView = newTV.(foldtree.TreeView)
	return m, cmd, true
}

func handleKeyIncreaseDiffContext(m Model) (Model, tea.Cmd, bool) {
	m.diffContext = clampDiffContext(m.diffContextSize() + diffContextStep)
	m.rebuildTree() // diffContext changes rendered heights
	return m, nil, true
}

func handleKeyDecreaseDiffContext(m Model) (Model, tea.Cmd, bool) {
	m.diffContext = clampDiffContext(m.diffContextSize() - diffContextStep)
	m.rebuildTree()
	return m, nil, true
}

// handleKeyToggleSensitive shows/hides the real value of sensitive
// attributes (redacted as "(sensitive value)" by default, matching
// Terraform CLI). Off by default every time the TUI starts.
func handleKeyToggleSensitive(m Model) (Model, tea.Cmd, bool) {
	m.revealSensitive = !m.revealSensitive
	m.rebuildTree() // reveal state changes rendered row text
	return m, nil, true
}

// handleKeyApply opens the apply confirmation, but only once per plan:
// applyAttempted stays true for the rest of the session once an apply
// has started (successful or not), since the plan file it targets is
// now stale -- either the infrastructure already matches it (success)
// or it's unclear what actually happened (failure/partial apply either
// way). Re-applying the same plan isn't safe; a fresh `terraprism
// apply` (a full re-plan) is required instead.
func handleKeyApply(m Model) (Model, tea.Cmd, bool) {
	if m.applyMode && !m.planning && !m.applyAttempted && m.hasApplicableChanges() {
		if m.confirmApply {
			return m.startApply()
		}
		m.confirmApply = true
	}
	return m, nil, true
}

func handleKeyConfirmApply(m Model) (Model, tea.Cmd, bool) {
	if m.applyMode && m.confirmApply && !m.planning && !m.applyAttempted && m.hasApplicableChanges() {
		return m.startApply()
	}
	return m, nil, true
}

// hasApplicableChanges reports whether the plan has anything for apply
// to actually do -- the same condition main.go uses for its own "No
// changes. Infrastructure is up-to-date." message, reused here so a
// plan with zero changes never offers 'a: APPLY' in the first place.
func (m Model) hasApplicableChanges() bool {
	return len(m.plan.DisplayResources()) > 0
}

// planCommandLine returns the exact `plan` command PlanStream invoked
// (planCmdLine, populated once planStreamStartedMsg arrives), or a
// simplified guess (tf command + "plan" + the user's own extra args,
// omitting internal-only flags like -out=<tempfile>/-no-color since
// they aren't known yet) for the brief window before that -- the very
// first render frame, before the subprocess has actually started.
func (m Model) planCommandLine() string {
	if m.planCmdLine != "" {
		return m.planCmdLine
	}
	parts := append([]string{string(m.planOptions.Cmd), "plan"}, m.planOptions.Args...)
	return strings.Join(parts, " ")
}

// applyCommandLine returns the exact `apply` command ApplyStream
// invoked (applyCmdLine, populated once applyStreamStartedMsg arrives),
// or a simplified "<cmd> apply" guess for the brief window before that.
func (m Model) applyCommandLine() string {
	if m.applyCmdLine != "" {
		return m.applyCmdLine
	}
	return m.tfCommand + " apply"
}

// startApply confirms the apply and kicks off runner.ApplyStream as a
// tea.Cmd -- the TUI keeps running and rendering throughout, unlike the
// old flow where confirming apply quit the program and main.go ran
// terraform/tofu directly on the bare terminal afterward.
func (m Model) startApply() (Model, tea.Cmd, bool) {
	m.confirmApply = false
	m.applying = true
	// Planning's own spinner ticking (if any) already stopped once it
	// finished, so this has to restart the loop rather than assume it's
	// still running.
	return m, tea.Batch(m.startApplyCmd(), m.spinner.Tick), true
}

// applyStreamStartedMsg carries the channels ApplyStream returns, once
// the apply subprocess has actually started.
type applyStreamStartedMsg struct {
	cmdLine string
	lines   <-chan runner.ApplyLine
	done    <-chan runner.ApplyStreamResult
}

// applyLineMsg is one streamed line of `apply` output.
type applyLineMsg runner.ApplyLine

// applyDoneMsg reports the apply subprocess's final result.
type applyDoneMsg struct {
	result *runner.ApplyResult
	err    error
}

// applyQuitCountdownStart is how many seconds the TUI waits after an
// apply finishes (success or failure) before auto-quitting, unless Esc
// cancels it first (see handleKeyEsc).
const applyQuitCountdownStart = 10

// applyQuitTickMsg fires once a second while the post-apply auto-quit
// countdown is running.
type applyQuitTickMsg struct{}

// tickApplyQuitCmd schedules the next applyQuitTickMsg one second out.
func tickApplyQuitCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return applyQuitTickMsg{} })
}

func (m Model) startApplyCmd() tea.Cmd {
	tfCommand, planFile := m.tfCommand, m.planFile
	return func() tea.Msg {
		cmdLine, lines, done := runner.ApplyStream(context.Background(), runner.TFCommand(tfCommand), planFile)
		return applyStreamStartedMsg{cmdLine: cmdLine, lines: lines, done: done}
	}
}

// waitForApplyEvent blocks for exactly one line (or, once the stream is
// exhausted, the final result) and returns it as a tea.Msg; the Update
// case handling applyLineMsg re-arms this itself, so the model keeps
// pumping the channels one message at a time for as long as apply runs.
func waitForApplyEvent(lines <-chan runner.ApplyLine, done <-chan runner.ApplyStreamResult) tea.Cmd {
	return func() tea.Msg {
		if line, ok := <-lines; ok {
			return applyLineMsg(line)
		}
		res := <-done
		return applyDoneMsg{result: res.Result, err: res.Err}
	}
}

// planStreamStartedMsg carries the channels PlanStream returns, once the
// plan subprocess has actually started.
type planStreamStartedMsg struct {
	cmdLine string
	lines   <-chan runner.PlanLine
	done    <-chan runner.PlanStreamResult
}

// planLineMsg is one streamed line of `plan` output.
type planLineMsg runner.PlanLine

// planDoneMsg reports the plan subprocess's final result: either a
// ready-to-display plan, or the error explaining why there isn't one.
type planDoneMsg struct {
	result *runner.PlanResult
	err    error
}

func (m Model) startPlanCmd() tea.Cmd {
	opts := m.planOptions
	return func() tea.Msg {
		cmdLine, lines, done := runner.PlanStream(context.Background(), opts)
		return planStreamStartedMsg{cmdLine: cmdLine, lines: lines, done: done}
	}
}

// waitForPlanEvent mirrors waitForApplyEvent, one message at a time, for
// as long as planning runs.
func waitForPlanEvent(lines <-chan runner.PlanLine, done <-chan runner.PlanStreamResult) tea.Cmd {
	return func() tea.Msg {
		if line, ok := <-lines; ok {
			return planLineMsg(line)
		}
		res := <-done
		return planDoneMsg{result: res.Result, err: res.Err}
	}
}

// handleNormalKey handles key presses in normal (non-search, non-picker)
// mode: terraprism-specific keys (tuiKeyHandlers) are always handled
// directly regardless of what else is visible, so 'o'/'q'/'a'/'y' etc.
// stay reachable even while the output pane is open; everything else
// (navigation, "/") goes to the output pane while it's visible, or
// treeView otherwise. A pending apply confirmation is cancelled by any
// key other than 'a'/'y', whether or not that key was one tui itself
// recognized.
func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	var result Model
	var cmd tea.Cmd
	if handler, ok := tuiKeyHandlers[key]; ok {
		result, cmd, _ = handler(m)
	} else if m.outputPane.Visible() {
		newPane, c := m.outputPane.Update(msg)
		result = m
		result.outputPane = newPane.(foldtree.LogPane)
		cmd = c
	} else {
		newTV, c := m.treeView.Update(msg)
		result = m
		result.treeView = newTV.(foldtree.TreeView)
		cmd = c
	}

	if m.confirmApply && key != "a" && key != "y" {
		result.confirmApply = false
	}
	return result, cmd
}

// handleFilterKey handles key presses in filter picker mode
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.filterPicker.Update(msg) {
	case foldtree.PickerCancel:
		// Esc clears all filters and closes, matching the picker's own
		// footer hint ("Esc: clear all and close").
		for k := range m.filterPicker.Selected {
			delete(m.filterPicker.Selected, k)
		}
		m.filtering = false
		m.rebuildTree()
	case foldtree.PickerApply:
		m.filtering = false
		m.rebuildTree()
	}
	return m, nil
}

// handleSortKey handles key presses in sort picker mode
func (m Model) handleSortKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.sortPicker.Update(msg) {
	case foldtree.PickerCancel:
		m.sorting = false
	case foldtree.PickerApply:
		m.sortPicker.SetCurrent(m.sortPicker.Highlighted())
		m.sorting = false
		m.rebuildTree()
	}
	return m, nil
}

// rebuildTree reconstructs the navigable tree for every currently
// displayed resource and hands it to treeView, applying one-time default
// collapse state (resources start collapsed; blocks whose fully-expanded
// size is large default to collapsed too) for any node ID never seen
// before. Must run whenever membership (filter/sort), size (diffContext),
// or wrap width (resize) changes, since a node's declared Height must
// always match exactly what RenderRow will draw for it. Search narrowing
// is handled entirely inside treeView and needs no rebuild of its own.
func (m *Model) rebuildTree() {
	displayed := m.sortedResources()
	roots := make([]foldtree.Node, len(displayed))
	for i, idx := range displayed {
		roots[i] = m.buildResourceNode(m.plan.Resources[idx])
	}

	m.treeView.SetTree(roots)
	// Trailing chrome appended after the rows in render(): a blank line,
	// the "End of Plan" marker, and a full viewport's worth of padding
	// so the last resource's content can scroll fully into view.
	m.treeView.SetExtraPadding(2 + m.treeView.Height())
	m.applyDefaultCollapse(m.treeView.State(), roots)
}

// applyDefaultCollapse collapses resources and large blocks the first
// time their ID is ever seen, without touching anything the user (or a
// previous default) has already decided. Walks the just-built node
// forest directly (rather than the flattened, visibility-filtered
// Rows()) so a node hidden behind an already-collapsed ancestor still
// gets its default applied.
func (m *Model) applyDefaultCollapse(nav *foldtree.State, roots []foldtree.Node) {
	var walk func(n foldtree.Node)
	walk = func(n foldtree.Node) {
		if !m.defaultsApplied[n.ID] {
			m.defaultsApplied[n.ID] = true
			if info, ok := n.Payload.(rowInfo); ok {
				switch info.kind {
				case rowResource:
					nav.SetCollapsed(n.ID, true)
				case rowContainerHeader, rowMultilineHeader:
					if countRenderedLines(info.attr) >= defaultCollapsedFoldLines {
						nav.SetCollapsed(n.ID, true)
					}
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
}

// RenderRow implements foldtree.RowRenderer. It never reads mutable Model
// state (diffContext-dependent content is already baked into each row's
// Payload at tree-build time in foldtree_adapter.go; row.Collapsed comes
// from the row itself; width/searchQuery are passed in) -- which is what
// makes it safe to hand a Model snapshot to foldtree.NewTreeView once, at
// construction, and never update it again.
func (m Model) RenderRow(row foldtree.Row, selected bool, width int, searchQuery string) string {
	info, _ := row.Payload.(rowInfo)
	switch info.kind {
	case rowResource:
		if selected {
			return m.renderSelectedResourceLine(info.resource, !row.Collapsed, width)
		}
		return m.renderResourceLine(info.resource, !row.Collapsed, searchQuery)

	case rowContainerHeader:
		indent := strings.Repeat("  ", row.Depth)
		open, _ := containerBrackets(info.attr.Kind)
		return m.renderFoldHeader(indent, info.attr, info.keyed, row.Collapsed, selected, width, open, containerFoldSummary(info.attr))

	case rowMultilineHeader:
		indent := strings.Repeat("  ", row.Depth)
		opener := "<<EOT"
		if info.attr.Sensitive && isJSONContainerAttr(info.attr) {
			// Flags that the diff below is against a pretty-printed
			// reformat, not the literal raw value -- relevant if someone
			// copies it out (whitespace/key order can differ).
			opener = "<<EOT (JSON, pretty-printed)"
		}
		return m.renderFoldHeader(indent, info.attr, info.keyed, row.Collapsed, selected, width, opener, multilineFoldSummary(info.attr))

	default: // leaf, sensitive, userdata, multilineBody, closingBracket, eot, unchangedNote
		if selected {
			return m.applySelectedHighlight(info.text, width)
		}
		return info.text
	}
}

// EmptyMessage implements foldtree.RowRenderer.
func (m Model) EmptyMessage(searchQuery string) string {
	var msg string
	if searchQuery != "" {
		msg = fmt.Sprintf("No resources match search '%s'. Press Esc to clear.", searchQuery)
	} else {
		msg = "No resources match the current filters. Press 'f' to change filters."
	}
	return mutedColor.Render(msg) + "\n"
}

// applySelectedHighlight applies the same full-width background
// highlight used for fold headers/resource rows to a row that has no
// selected-specific rendering of its own. Handles multi-line cached text
// (a wrapped leaf, a userdata block, a multiline diff body) by padding
// and highlighting each line individually rather than treating the
// whole blob as one line.
func (m Model) applySelectedHighlight(plain string, width int) string {
	style := lipgloss.NewStyle().Background(selectedBg).Foreground(textColor)

	lines := strings.Split(plain, "\n")
	for i, line := range lines {
		plainLen := utf8.RuneCountInString(stripANSI(line))
		if width > 0 && plainLen < width {
			line += strings.Repeat(" ", width-plainLen)
		}
		lines[i] = style.Render(line)
	}
	return strings.Join(lines, "\n")
}

const defaultCollapsedFoldLines = 30

func foldKey(address, path string) string {
	return address + "#" + path
}

// isContainerAttr reports whether an attribute is a non-empty, non-sensitive
// map or list — i.e. something that renders as a foldable block with
// nested children.
func isContainerAttr(a tfplan.Attribute) bool {
	return (a.Kind == tfplan.KindMap || a.Kind == tfplan.KindList) && len(a.Children) > 0 && !a.Sensitive
}

// countRenderedLines estimates how many lines a foldable attribute would
// take fully expanded — open+close plus recursive descendants for a
// container, or open+close plus diffed line count for a multi-line
// string — used to decide the default collapsed/expanded state.
func countRenderedLines(a tfplan.Attribute) int {
	// sensitiveRevealsAsHeredoc alone would miss a non-sensitive
	// multiline string (it's always false unless a.Sensitive), so both
	// checks stay explicit rather than one subsuming the other.
	if isMultilineStringAttr(a) || sensitiveRevealsAsHeredoc(a) {
		return multilineLineEstimate(a) + 2
	}
	if !isContainerAttr(a) {
		return 1
	}
	n := 2
	for _, c := range a.Children {
		n += countRenderedLines(c)
	}
	return n
}

// multilineLineEstimate returns the larger of the old/new value's line
// count, for sizing the default-collapse heuristic and the collapsed
// "... (N lines)" summary — doesn't need to be exact, just representative.
// For a sensitive attribute, a JSON-shaped value is pretty-printed first
// (see renderMultilineStringBody), so the estimate has to match that or
// the collapsed summary would report the wrong line count.
func multilineLineEstimate(a tfplan.Attribute) int {
	count := func(v any) int {
		s, ok := v.(string)
		if a.Sensitive {
			if pretty, ok2 := TryPrettyPrintJSON(v); ok2 {
				s, ok = pretty, true
			}
		}
		if !ok {
			return 0
		}
		return strings.Count(s, "\n") + 1
	}
	oldLines, newLines := count(a.Old), count(a.New)
	if oldLines > newLines {
		return oldLines
	}
	return newLines
}

// renderFoldHeader renders a fold block's header row: the expand/collapse
// indicator, diff-action symbol, and "name = <opener>" (or just <opener>
// when keyed is false, i.e. a positional list element), with
// collapsedSummary appended when collapsed. Shared between container
// attributes ("{"/"[" ... "N attrs/items }") and multi-line string
// attributes ("<<EOT" ... "N lines") — the two foldable attribute shapes.
func (m Model) renderFoldHeader(indent string, attr tfplan.Attribute, keyed, collapsed, selected bool, maxWidth int, opener, collapsedSummary string) string {
	indicator := expandedIndicator
	if collapsed {
		indicator = collapsedIndicator
	}

	// attr.Sensitive is only ever true here for a revealed multi-line
	// sensitive value (see buildAttributeNodes) -- an ordinary container
	// header never has it set, since isContainerAttr excludes sensitive
	// attributes. The marker mirrors renderRevealedSensitiveValue's flat
	// case, so a revealed secret reads the same way everywhere.
	marker := ""
	if attr.Sensitive {
		marker = fastSensitive.Render("(revealed) ")
	}

	var content string
	if keyed {
		content = marker + fastAttrName.Render(attr.Name) + " = " + fastMuted.Render(opener)
	} else {
		content = marker + fastMuted.Render(opener)
	}

	result := indent + indicator + " " + actionPrefixSymbol(attr.Action) + " " + content
	if collapsed {
		result += fastMuted.Render(" ... " + collapsedSummary)
		if attr.Computed {
			result += " " + fastAttrComputed.Render("(known after apply)")
		}
	}

	if !selected {
		return result
	}

	plainLen := utf8.RuneCountInString(stripANSI(result))
	if maxWidth > 0 && plainLen < maxWidth {
		result += strings.Repeat(" ", maxWidth-plainLen)
	}
	return lipgloss.NewStyle().Background(selectedBg).Foreground(textColor).Render(result)
}

// containerFoldSummary formats the "N attrs }" / "N items ]" text shown
// after a collapsed container's opening bracket.
func containerFoldSummary(attr tfplan.Attribute) string {
	_, closeBracket := containerBrackets(attr.Kind)
	noun := "attrs"
	if attr.Kind == tfplan.KindList {
		noun = "items"
	}
	return fmt.Sprintf("%d %s %s", len(attr.Children), noun, closeBracket)
}

func closingBraceLine(indent string, kind tfplan.ValueKind) string {
	_, closeBracket := containerBrackets(kind)
	return indent + fastMuted.Render(closeBracket)
}

// multilineFoldSummary formats the "N lines" text shown after a
// collapsed multi-line string's opening "<<EOT" marker.
func multilineFoldSummary(attr tfplan.Attribute) string {
	n := multilineLineEstimate(attr)
	noun := "line"
	if n != 1 {
		noun = "lines"
	}
	return fmt.Sprintf("%d %s", n, noun)
}

// unchangedHiddenNote formats the "# (N unchanged attributes hidden)"
// summary line for a run of no-op attributes, matching Terraform CLI's
// own wording (singular "attribute" for exactly one).
func unchangedHiddenNote(count int) string {
	noun := "attribute"
	if count != 1 {
		noun = "attributes"
	}
	return fmt.Sprintf("# (%d unchanged %s hidden)", count, noun)
}

// renderDiffLines writes context-diff lines into a builder, handling all
// DiffOp types including DiffSeparator for collapsed equal runs. Format
// agnostic over []string, so it's reused unchanged from the old
// heredoc/text-diff renderer for both multiline-string and userdata diffs.
func renderDiffLines(b *strings.Builder, diff []DiffLine, indent string, maxWidth int) {
	for _, d := range diff {
		switch d.Op {
		case DiffSeparator:
			b.WriteString(indent)
			b.WriteString(fastMuted.Render("@@ ··· @@"))
			b.WriteString("\n")
		case DiffDelete:
			wrapped := wrapText(d.Text, maxWidth-len(indent)-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				b.WriteString(indent)
				b.WriteString(fastDestroy.Render("- " + wl))
				b.WriteString("\n")
			}
		case DiffInsert:
			wrapped := wrapText(d.Text, maxWidth-len(indent)-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				b.WriteString(indent)
				b.WriteString(fastCreate.Render("+ " + wl))
				b.WriteString("\n")
			}
		case DiffEqual:
			wrapped := wrapText(d.Text, maxWidth-len(indent)-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				b.WriteString(indent)
				b.WriteString(fastMuted.Render("  " + wl))
				b.WriteString("\n")
			}
		}
	}
}

// wrapText word-wraps s to width, used by renderDiffLines for long diff lines.
func wrapText(s string, width int) string {
	if width <= 10 {
		return s
	}
	return wordwrap.String(s, width)
}

// isMultilineStringAttr reports whether a leaf string attribute's old or
// new value spans multiple lines (e.g. a Helm values YAML blob or a
// cloud-init script) — these get line-by-line diffing instead of being
// shown as one opaque blob.
func isMultilineStringAttr(attr tfplan.Attribute) bool {
	if attr.Kind != tfplan.KindString {
		return false
	}
	if s, ok := attr.Old.(string); ok && strings.Contains(s, "\n") {
		return true
	}
	if s, ok := attr.New.(string); ok && strings.Contains(s, "\n") {
		return true
	}
	return false
}

// renderMultilineStringBody renders a multi-line string attribute's
// diffed content — no header, no "EOT" footer; the caller wraps this
// with renderFoldHeader/closingBraceLine-equivalent lines so it folds
// like any other collapsible block. Unlike the old heredoc-marker text
// scanning this replaces, there's no marker detection needed at all:
// JSON strings are just strings.
//
// The returned string has no trailing newline, matching every other
// row-text producer and the RowRenderer.RenderRow contract TreeView
// relies on (it appends exactly one "\n" per row itself) -- each branch
// below writes one line at a time terminated by "\n" for simplicity, so
// the trailing one is trimmed once at the end rather than restructuring
// every branch to join instead of terminate.
func (m Model) renderMultilineStringBody(attr tfplan.Attribute, indent string, maxWidth int) string {
	var b strings.Builder
	contentIndent := indent + "  "

	// buildAttributeNodes only ever routes a sensitive attribute here once
	// revealSensitive is on (see its comment), but this guard stays as a
	// defensive fallback for any other caller -- e.g. a future one that
	// builds a multiline row directly.
	if attr.Sensitive && !m.revealSensitive {
		b.WriteString(contentIndent)
		b.WriteString(fastSensitive.Render("(sensitive value)"))
		return b.String()
	}
	if attr.Computed {
		b.WriteString(contentIndent)
		b.WriteString(attrComputedStyle.Render("(known after apply)"))
		return b.String()
	}

	// A sensitive value's real Old/New is pretty-printed first when it's
	// JSON-shaped -- a minified JSON blob (or a whole-subtree-sensitive
	// container's raw map/list) has no line structure of its own to
	// diff, so without this it would still render as one unreadable
	// line even after being routed here. Scoped to Sensitive so an
	// ordinary multi-line string's existing rendering (already
	// literal-newline text, possibly independently valid JSON) is never
	// affected -- re-marshaling would risk reordering its keys.
	var oldStr, newStr string
	if attr.Sensitive {
		var ok bool
		if oldStr, ok = TryPrettyPrintJSON(attr.Old); !ok {
			oldStr, _ = attr.Old.(string)
		}
		if newStr, ok = TryPrettyPrintJSON(attr.New); !ok {
			newStr, _ = attr.New.(string)
		}
	} else {
		oldStr, _ = attr.Old.(string)
		newStr, _ = attr.New.(string)
	}

	switch attr.Action {
	case tfplan.ActionCreate:
		for _, l := range strings.Split(newStr, "\n") {
			b.WriteString(contentIndent)
			b.WriteString(fastCreate.Render("+ " + l))
			b.WriteString("\n")
		}
	case tfplan.ActionDelete:
		for _, l := range strings.Split(oldStr, "\n") {
			b.WriteString(contentIndent)
			b.WriteString(fastDestroy.Render("- " + l))
			b.WriteString("\n")
		}
	case tfplan.ActionNoOp:
		for _, l := range strings.Split(newStr, "\n") {
			b.WriteString(contentIndent)
			b.WriteString(fastMuted.Render("  " + l))
			b.WriteString("\n")
		}
	default: // update
		diff := ComputeDiff(strings.Split(oldStr, "\n"), strings.Split(newStr, "\n"))
		contextDiff := ContextDiff(diff, m.diffContextSize())
		if contextDiff == nil {
			b.WriteString(contentIndent)
			b.WriteString(fastMuted.Render("  (no changes)"))
			b.WriteString("\n")
		} else {
			renderDiffLines(&b, contextDiff, contentIndent, maxWidth)
		}
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// tryRenderUserdataAttr detects user_data/user_data_base64 attributes and
// renders them decoded, with the decoded content diffed on change. The
// trigger is now a simple name check on structured data, replacing the
// old " = "-splitting text parse.
func (m Model) tryRenderUserdataAttr(attr tfplan.Attribute, indent string, maxWidth int) (string, bool) {
	if attr.Kind != tfplan.KindString || attr.Sensitive {
		return "", false
	}
	if attr.Name != "user_data" && attr.Name != "user_data_base64" {
		return "", false
	}

	oldB64, _ := attr.Old.(string)
	newB64, _ := attr.New.(string)
	oldDecoded, oldOk := TryDecodeUserdata(oldB64)
	newDecoded, newOk := TryDecodeUserdata(newB64)
	if !oldOk && !newOk {
		return "", false
	}

	decodedIndent := indent + "  "
	var b strings.Builder
	b.WriteString(indent + actionPrefixSymbol(attr.Action) + " " + renderKeyValue(attr, true))
	b.WriteString("\n")
	b.WriteString(decodedIndent)
	b.WriteString(fastMuted.Render("┄┄┄ decoded " + attr.Name + " ┄┄┄"))
	b.WriteString("\n")

	switch {
	case oldOk && newOk:
		diff := ComputeDiff(strings.Split(oldDecoded, "\n"), strings.Split(newDecoded, "\n"))
		contextDiff := ContextDiff(diff, m.diffContextSize())
		if contextDiff == nil {
			b.WriteString(decodedIndent)
			b.WriteString(fastMuted.Render("  (no changes in decoded content)"))
			b.WriteString("\n")
		} else {
			renderDiffLines(&b, contextDiff, decodedIndent, maxWidth)
		}
	case oldOk:
		for _, l := range strings.Split(oldDecoded, "\n") {
			b.WriteString(decodedIndent)
			b.WriteString(lipgloss.NewStyle().Foreground(destroyColor).Render("- " + l))
			b.WriteString("\n")
		}
	case newOk:
		for _, l := range strings.Split(newDecoded, "\n") {
			b.WriteString(decodedIndent)
			b.WriteString(lipgloss.NewStyle().Foreground(createColor).Render("+ " + l))
			b.WriteString("\n")
		}
	}
	b.WriteString(decodedIndent)
	b.WriteString(fastMuted.Render("┄┄┄ end " + attr.Name + " ┄┄┄"))
	return b.String(), true
}

// renderLeafRow renders one "name = value" (or bare "value") row for a
// plain scalar attribute, word-wrapping long create/delete/unchanged
// values (update rows, with their old → new arrow, are left unwrapped —
// splitting two colored segments across lines isn't worth the added
// complexity for what's a cosmetic nicety).
func (m Model) renderLeafRow(indent string, attr tfplan.Attribute, keyed bool, maxWidth int) string {
	prefix := actionPrefixSymbol(attr.Action)
	rowPrefix := indent + prefix + " "
	full := rowPrefix + renderKeyValue(attr, keyed)

	if maxWidth <= 0 || attr.Action == tfplan.ActionUpdate || attr.Sensitive || attr.Computed || !keyed {
		return full
	}

	plainValue := renderScalarText(attr.New)
	styled := attrNewValueStyle
	if attr.Action == tfplan.ActionDelete {
		plainValue = renderScalarText(attr.Old)
		styled = attrOldValueStyle
	}

	available := maxWidth - utf8.RuneCountInString(rowPrefix) - utf8.RuneCountInString(attr.Name) - 3
	if available < 20 || utf8.RuneCountInString(plainValue) <= available {
		return full
	}

	wrapped := wordwrap.String(plainValue, available)
	subLines := strings.Split(wrapped, "\n")
	if len(subLines) <= 1 {
		return full
	}

	continuation := indent + strings.Repeat(" ", utf8.RuneCountInString(prefix)+1)
	var b strings.Builder
	for i, sub := range subLines {
		if i > 0 {
			b.WriteString("\n")
			b.WriteString(continuation)
		} else {
			b.WriteString(rowPrefix)
			b.WriteString(fastAttrName.Render(attr.Name))
			b.WriteString(" = ")
		}
		b.WriteString(styled.Render(sub))
	}
	return b.String()
}

// renderSelectedResourceLine renders a resource line with full-width background highlight
func (m Model) renderSelectedResourceLine(r tfplan.Resource, expanded bool, width int) string {
	// Build the line content
	var content strings.Builder

	// Expand/collapse indicator
	if expanded {
		content.WriteString("▼")
	} else {
		content.WriteString("▶")
	}
	content.WriteString(" ")

	// Action symbol
	switch r.Action {
	case tfplan.ActionCreate:
		content.WriteString("+")
	case tfplan.ActionDelete:
		content.WriteString("-")
	case tfplan.ActionUpdate:
		content.WriteString("~")
	case tfplan.ActionReplace:
		content.WriteString("±")
	case tfplan.ActionRead:
		content.WriteString("≤")
	case tfplan.ActionForget:
		content.WriteString("⊘")
	default:
		content.WriteString("~")
	}
	content.WriteString(" ")

	// Resource address
	content.WriteString(r.Address)

	// Action description
	actionDesc := getActionDescription(r)
	content.WriteString(" ")
	content.WriteString(actionDesc)

	// Change count
	if n := r.ChangedAttributeCount(); n > 0 {
		fmt.Fprintf(&content, " (%d changes)", n)
	}

	// Pad to full width and apply selected style with foreground color
	line := content.String()
	if width > 0 && len(line) < width {
		line = line + strings.Repeat(" ", width-len(line))
	}

	// Apply style with both foreground and background
	actionStyle := lipgloss.NewStyle().
		Background(selectedBg).
		Foreground(GetActionColor(string(r.Action))).
		Bold(true)

	return actionStyle.Render(line)
}

func (m Model) renderResourceLine(r tfplan.Resource, expanded bool, searchQuery string) string {
	var b strings.Builder

	// Expand/collapse indicator
	if expanded {
		b.WriteString(expandedIndicator)
	} else {
		b.WriteString(collapsedIndicator)
	}
	b.WriteString(" ")

	// Action symbol
	b.WriteString(GetActionSymbol(string(r.Action)))
	b.WriteString(" ")

	// Resource address
	style := GetResourceStyle(string(r.Action))
	address := r.Address

	if searchQuery != "" {
		// Highlight matching text
		address = highlightMatch(address, searchQuery)
	}

	b.WriteString(style.Render(address))

	// Action description
	actionDesc := getActionDescription(r)
	b.WriteString(" ")
	b.WriteString(fastMuted.Render(actionDesc))

	// Change count for expanded content
	if n := r.ChangedAttributeCount(); n > 0 {
		b.WriteString(fastMuted.Render(fmt.Sprintf(" (%d changes)", n)))
	}

	return b.String()
}

func highlightMatch(text, query string) string {
	lower := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)

	idx := strings.Index(lower, lowerQuery)
	if idx == -1 {
		return text
	}

	before := text[:idx]
	match := text[idx : idx+len(query)]
	after := text[idx+len(query):]

	return before + matchStyle.Render(match) + after
}

// getActionDescription returns the human-readable description shown next
// to a resource's address, distinguishing the two replace orderings via
// ReplacePattern the way the legacy delete-create/create-delete actions
// used to.
func getActionDescription(r tfplan.Resource) string {
	switch r.Action {
	case tfplan.ActionCreate:
		return "will be created"
	case tfplan.ActionDelete:
		return "will be destroyed"
	case tfplan.ActionUpdate:
		return "will be updated"
	case tfplan.ActionReplace:
		if r.ReplacePattern == tfplan.ReplaceCreateBeforeDestroy {
			return "will be created and then destroyed"
		}
		return "will be destroyed and then created"
	case tfplan.ActionRead:
		return "will be read"
	case tfplan.ActionForget:
		return "will be removed from state"
	case tfplan.ActionOutput:
		return "output values will change"
	default:
		return ""
	}
}

// sortOrderLabel returns a display label for a sort option
func sortOrderLabel(opt SortOrder) string {
	switch opt {
	case SortDefault:
		return "default (plan order)"
	case SortByAction:
		return "by action"
	case SortByAddress:
		return "by address"
	case SortByType:
		return "by type"
	default:
		return string(opt)
	}
}

// sortOrderHint returns a one-line hint explaining what a sort option does
func sortOrderHint(opt SortOrder) string {
	switch opt {
	case SortDefault:
		return "— as Terraform outputs them"
	case SortByAction:
		return "— group create, destroy, update, etc."
	case SortByAddress:
		return "— alphabetical by resource address"
	case SortByType:
		return "— group by resource type (aws_instance, etc.)"
	default:
		return ""
	}
}

// filterActionLabel returns a short label for the filter picker
func filterActionLabel(action tfplan.Action) string {
	switch action {
	case tfplan.ActionCreate:
		return "create"
	case tfplan.ActionDelete:
		return "destroy"
	case tfplan.ActionUpdate:
		return "update"
	case tfplan.ActionReplace:
		return "replace"
	case tfplan.ActionRead:
		return "read"
	case tfplan.ActionForget:
		return "forget"
	case tfplan.ActionOutput:
		return "output"
	default:
		return string(action)
	}
}

// viewFilterPicker renders the filter picker overlay (returns full view, caller returns early).
func (m Model) viewFilterPicker() string {
	var b strings.Builder
	b.WriteString(searchStyle.Render("Filter by status (Space: toggle, a: all, c: clear, Enter: apply, Esc: clear all and close)"))
	b.WriteString("\n\n")
	b.WriteString(m.filterPicker.View())
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("j/k: navigate • Space: toggle • a: select all • c: clear all • Enter: apply • Esc: clear all and close"))
	return appStyle.Render(b.String())
}

// viewSortPicker renders the sort picker overlay (returns full view, caller returns early).
func (m Model) viewSortPicker() string {
	var b strings.Builder
	b.WriteString(searchStyle.Render("Sort by (Enter/Space: select, Esc: close)"))
	b.WriteString("\n\n")
	b.WriteString(m.sortPicker.View())
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("j/k: navigate • Enter/Space: select • Esc: close"))
	return appStyle.Render(b.String())
}

// viewHeader renders the header and summary.
func (m Model) viewHeader() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("🔺 Terra-Prism - Terraform Plan Viewer"))
	b.WriteString("\n")
	summary := fmt.Sprintf("  %s to add, %s to change, %s to destroy",
		lipgloss.NewStyle().Foreground(createColor).Render(fmt.Sprintf("%d", m.plan.TotalAdd)),
		lipgloss.NewStyle().Foreground(updateColor).Render(fmt.Sprintf("%d", m.plan.TotalChange)),
		lipgloss.NewStyle().Foreground(destroyColor).Render(fmt.Sprintf("%d", m.plan.TotalDestroy)),
	)
	if m.plan.OutputCount > 0 {
		summary += fmt.Sprintf(", %s output(s) changed",
			lipgloss.NewStyle().Foreground(updateColor).Render(fmt.Sprintf("%d", m.plan.OutputCount)),
		)
	}
	b.WriteString(summaryStyle.Render(summary))
	b.WriteString("\n\n")
	return b.String()
}

// viewFilterStatus renders the filter status line when filters are active.
func (m Model) viewFilterStatus() string {
	if len(m.filterPicker.Selected) == 0 {
		return ""
	}
	var labels []string
	for _, action := range filterableActions {
		if m.filterPicker.Selected[action] {
			labels = append(labels, filterActionLabel(action))
		}
	}
	return searchStyle.Render(fmt.Sprintf("Filter: %s (%d active) • f: change • Esc: clear all", strings.Join(labels, ", "), len(labels))) + "\n\n"
}

// viewSortStatus renders the sort status line when not default.
func (m Model) viewSortStatus() string {
	order := m.sortPicker.Current()
	if order == SortDefault || order == "" {
		return ""
	}
	return searchStyle.Render(fmt.Sprintf("Sort: %s • s: change", sortOrderLabel(order))) + "\n\n"
}

// bannerWidth returns the width available for a status banner's text,
// leaving room for the style's own horizontal padding, so long content
// (e.g. a full plan/apply command line) word-wraps onto multiple lines
// instead of being clipped by the terminal's right edge.
func (m Model) bannerWidth() int {
	w := m.width - 4
	if w < 20 {
		w = 20
	}
	return w
}

// viewConfirmationPrompt renders the apply confirmation prompt, or (once
// confirmed) a status banner for as long as the apply subprocess is
// still running -- mutually exclusive with each other and with the
// pre-confirmation prompt.
func (m Model) viewConfirmationPrompt() string {
	if m.planning {
		style := lipgloss.NewStyle().
			Background(updateColor).
			Foreground(textColor).
			Bold(true).
			Padding(0, 2).
			Width(m.bannerWidth())
		return "\n" + style.Render(fmt.Sprintf("%sRunning %s (quit disabled until it finishes)", m.spinner.View(), m.planCommandLine())) + "\n\n"
	}
	if m.planErr != nil {
		style := lipgloss.NewStyle().
			Background(destroyColor).
			Foreground(textColor).
			Bold(true).
			Padding(0, 2).
			Width(m.bannerWidth())
		return "\n" + style.Render(fmt.Sprintf("✗ Plan failed: %v", m.planErr)) + "\n\n"
	}
	if m.applying {
		style := lipgloss.NewStyle().
			Background(updateColor).
			Foreground(textColor).
			Bold(true).
			Padding(0, 2).
			Width(m.bannerWidth())
		return "\n" + style.Render(fmt.Sprintf("%sRunning %s (quit disabled until it finishes)", m.spinner.View(), m.applyCommandLine())) + "\n\n"
	}
	if m.applyAttempted {
		return m.viewApplyCompletionBanner()
	}
	if !m.confirmApply {
		return ""
	}
	confirmStyle := lipgloss.NewStyle().
		Background(destroyColor).
		Foreground(textColor).
		Bold(true).
		Padding(0, 2).
		Width(m.bannerWidth())
	return "\n" + confirmStyle.Render("⚠️  Apply this plan? Press 'y' to confirm, any other key to cancel") + "\n\n"
}

// viewApplyCompletionBanner renders the post-apply status once the
// subprocess has finished (success or failure), including the auto-quit
// countdown while it's running. Re-applying is never offered again from
// here -- the plan file is stale either way, so getting a fresh one
// means quitting and running `terraprism apply` again.
func (m Model) viewApplyCompletionBanner() string {
	bg := createColor
	msg := "✓ Apply complete!"
	if m.applyResult != nil {
		bg = destroyColor
		msg = fmt.Sprintf("✗ Apply failed: %v", m.applyResult)
	}
	if m.applyQuitCountdown > 0 {
		msg += fmt.Sprintf(" — quitting in %ds (q: now, Esc: stay)", m.applyQuitCountdown)
	} else {
		msg += " — q: quit"
	}
	style := lipgloss.NewStyle().
		Background(bg).
		Foreground(textColor).
		Bold(true).
		Padding(0, 2).
		Width(m.bannerWidth())
	return "\n" + style.Render(msg) + "\n\n"
}

// viewHelpFooter returns the help footer text.
func (m Model) viewHelpFooter() string {
	maxWidth := m.width - 4
	if maxWidth <= 0 {
		maxWidth = m.treeView.Width()
	}

	if m.outputPane.Visible() {
		wrapState := "off"
		if m.outputPane.WordWrap() {
			wrapState = "on"
		}
		navHint := fmt.Sprintf("j/k/gg/G: nav • h/l: scroll sideways • /: search • n/N: cycle • w: wrap (%s)", wrapState)
		switch {
		case m.planning:
			return "Running plan... quit disabled • " + navHint + " • o: hide"
		case m.planErr != nil:
			return "Plan failed • " + navHint + " • o: hide • q: quit"
		case m.applying:
			return "Applying... quit disabled • " + navHint + " • o: hide"
		default:
			return navHint + " • o: hide output"
		}
	}

	sensitiveHint := "x: reveal secrets"
	if m.revealSensitive {
		sensitiveHint = "x: hide secrets"
	}

	if m.applyMode {
		if m.confirmApply {
			return "y: confirm apply • any key: cancel"
		}
		outputHint := ""
		if m.planOutput != "" {
			outputHint = " • o: output"
		}
		// No 'a: APPLY' hint once it's already run (viewApplyCompletionBanner
		// covers that state instead) or if the plan has nothing to apply --
		// in either case there's nothing this key would do.
		if m.applyAttempted || !m.hasApplicableChanges() {
			full := fmt.Sprintf("j/k/↑↓: navigate • e/c: scope • E/C: all • /: search • f: filter • s: sort • %s%s • q: quit", sensitiveHint, outputHint)
			if lipgloss.Width(full) <= maxWidth {
				return full
			}
			medium := fmt.Sprintf("j/k nav • e/c scope • E/C all • / search%s • q", outputHint)
			if lipgloss.Width(medium) <= maxWidth {
				return medium
			}
			return "j/k nav • e/c • / search • q"
		}
		applyHint := lipgloss.NewStyle().Foreground(createColor).Bold(true).Render("a: APPLY")
		full := fmt.Sprintf("%s • j/k/↑↓: navigate • e/c: scope • E/C: all • /: search • f: filter • s: sort • %s%s • q: quit", applyHint, sensitiveHint, outputHint)
		if lipgloss.Width(full) <= maxWidth {
			return full
		}
		medium := fmt.Sprintf("%s • j/k nav • e/c scope • E/C all • / search%s • q", applyHint, outputHint)
		if lipgloss.Width(medium) <= maxWidth {
			return medium
		}
		return fmt.Sprintf("%s • j/k nav • e/c • / search • q", applyHint)
	}

	helpOptions := []string{
		"j/k/↑↓: navigate • l/→: expand • h/←/⌫: collapse • e/c: scope • E/C: all • +/-: diff context • Ctrl+E/Y: line scroll • H/L: sideways • d/u: page scroll • gg/G: top/bottom • /: search • f: filter • s: sort • " + sensitiveHint + " • q: quit",
		"j/k: nav • l/h: fold • e/c: scope • E/C: all • +/-: diff ctx • Ctrl+E/Y: line • d/u: page • /: search • f/s • q",
		"j/k nav • l/h fold • e/c scope • E/C all • +/- diff • Ctrl+E/Y scroll • / search • q",
		"j/k nav • l/h fold • e/c • q",
	}

	if len(m.filterPicker.Selected) > 0 {
		for i, help := range helpOptions {
			helpOptions[i] = help + " • Esc clears filter"
		}
	}

	for _, help := range helpOptions {
		if lipgloss.Width(help) <= maxWidth {
			return help
		}
	}
	return helpOptions[len(helpOptions)-1]
}

// viewUpdateNudge renders the update available nudge.
func (m Model) viewUpdateNudge() string {
	if m.updateAvailable == "" {
		return ""
	}
	nudgeStyle := lipgloss.NewStyle().Foreground(computedColor).Italic(true)
	return "\n" + nudgeStyle.Render(fmt.Sprintf("Update available: v%s. Run 'terraprism upgrade' to update.", m.updateAvailable))
}

// View renders the UI
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}
	if m.filtering {
		return m.viewFilterPicker()
	}
	if m.sorting {
		return m.viewSortPicker()
	}

	var b strings.Builder
	b.WriteString(m.viewHeader())
	b.WriteString(m.viewFilterStatus())
	b.WriteString(m.viewSortStatus())
	b.WriteString(m.treeView.ViewSearchBar(func(s string) string { return searchStyle.Render(s) }))
	b.WriteString(m.viewConfirmationPrompt())
	if m.outputPane.Visible() {
		paneSearchBar := m.outputPane.ViewSearchBar(func(s string) string { return searchStyle.Render(s) })
		b.WriteString(lipgloss.JoinVertical(lipgloss.Left, m.treeView.View(), paneSearchBar+m.outputPane.View()))
	} else {
		b.WriteString(m.treeView.View())
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(m.viewHelpFooter()))
	b.WriteString(m.viewUpdateNudge())
	return appStyle.Render(b.String())
}
