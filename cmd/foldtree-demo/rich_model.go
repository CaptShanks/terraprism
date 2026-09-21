package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/CaptShanks/terraprism/internal/foldtree"
)

// richRenderer implements foldtree.RowRenderer for taskItem payloads --
// the demo's stand-in for terraprism's own colorize.go, proving RenderRow
// works with any Payload type, not just tfplan.Attribute/Resource.
type richRenderer struct{}

func (richRenderer) RenderRow(row foldtree.Row, selected bool, width int, searchQuery string) string {
	item, _ := row.Payload.(taskItem)
	indent := strings.Repeat("  ", row.Depth)
	marker := "  "
	if row.HasChildren {
		if row.Collapsed {
			marker = "▶ "
		} else {
			marker = "▼ "
		}
	}
	line := fmt.Sprintf("%s%s[%s] %s", indent, marker, item.category, item.title)
	if width > 0 {
		line = padTo(line, width)
	}
	if selected {
		line = selectedStyle.Render(line)
	}
	return line
}

func (richRenderer) EmptyMessage(query string) string {
	if query != "" {
		return "no tasks match search '" + query + "'. Press Esc to clear."
	}
	return "no tasks match the current filters. Press 'f' to change filters."
}

const richSortDefault = "default"

var richSortOptions = []string{richSortDefault, "title", "category"}

// richModel wires TreeView plus two Picker[string] instances (filter by
// category, sort by field) against the synthetic task board -- the demo
// counterpart of terraprism's own tui.Model, minus anything
// Terraform-specific (no apply flow, no diff context, no update nudge).
type richModel struct {
	allRoots []foldtree.Node
	treeView foldtree.TreeView

	statusFilters map[string]bool
	filterPicker  foldtree.Picker[string]
	filtering     bool

	sortOrder  string
	sortPicker foldtree.Picker[string]
	sorting    bool

	width, height int
	ready         bool
}

func newRichModel() richModel {
	m := richModel{
		allRoots:      buildTaskBoard(),
		treeView:      *foldtree.NewTreeView(richRenderer{}),
		statusFilters: make(map[string]bool),
		sortOrder:     richSortDefault,
	}
	m.filterPicker = *foldtree.NewPicker(taskCategories, func(c string) string { return c })
	m.filterPicker.Multi = true
	m.sortPicker = *foldtree.NewPicker(richSortOptions, func(s string) string { return s })
	m.sortPicker.SetCurrent(richSortDefault)
	m.rebuildTree()
	return m
}

// rebuildTree filters allRoots by the active category filters and sorts
// them by the active sort field, then hands the result to treeView --
// mirroring tui.Model's own filteredResources/sortedResources/SetTree
// split, just over taskItem instead of tfplan.Resource.
func (m *richModel) rebuildTree() {
	var roots []foldtree.Node
	for _, r := range m.allRoots {
		item := r.Payload.(taskItem)
		if len(m.statusFilters) == 0 || m.statusFilters[item.category] {
			roots = append(roots, r)
		}
	}
	if m.sortOrder != richSortDefault {
		sort.SliceStable(roots, func(i, j int) bool {
			a := roots[i].Payload.(taskItem)
			b := roots[j].Payload.(taskItem)
			if m.sortOrder == "category" {
				return a.category < b.category
			}
			return a.title < b.title
		})
	}
	m.treeView.SetTree(roots)
}

func (m richModel) Init() tea.Cmd { return nil }

func (m richModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		const chromeLines = 3 // header + blank + help line
		height := msg.Height - chromeLines
		if height < 1 {
			height = 1
		}
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		newTV, _ := m.treeView.Update(tea.WindowSizeMsg{Width: msg.Width, Height: height})
		m.treeView = newTV.(foldtree.TreeView)
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			return m.handleFilterKey(msg), nil
		}
		if m.sorting {
			return m.handleSortKey(msg), nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "f":
			m.filtering = true
			return m, nil
		case "s":
			m.sorting = true
			return m, nil
		}
		newTV, cmd := m.treeView.Update(msg)
		m.treeView = newTV.(foldtree.TreeView)
		return m, cmd

	case tea.MouseMsg:
		newTV, _ := m.treeView.Update(msg)
		m.treeView = newTV.(foldtree.TreeView)
		return m, nil
	}
	return m, nil
}

func (m richModel) handleFilterKey(msg tea.KeyMsg) richModel {
	switch msg.String() {
	case "esc":
		m.statusFilters = make(map[string]bool)
		m.filtering = false
		m.rebuildTree()
		return m
	case "enter":
		m.filtering = false
		m.rebuildTree()
		return m
	}
	m.filterPicker.Update(msg)
	m.statusFilters = m.filterPicker.Selected
	return m
}

func (m richModel) handleSortKey(msg tea.KeyMsg) richModel {
	switch msg.String() {
	case "esc":
		m.sorting = false
		return m
	}
	action := m.sortPicker.Update(msg)
	if action == foldtree.PickerApply {
		m.sortPicker.SetCurrent(m.sortPicker.Highlighted())
		m.sortOrder = m.sortPicker.Current()
		m.sorting = false
		m.rebuildTree()
	}
	return m
}

func (m richModel) View() string {
	if !m.ready {
		return "initializing..."
	}
	if m.filtering {
		var b strings.Builder
		b.WriteString(headerStyle.Render(" Filter by category (Space: toggle, a: all, c: clear, Enter: apply, Esc: clear+close) "))
		b.WriteString("\n\n")
		b.WriteString(m.filterPicker.View())
		return b.String()
	}
	if m.sorting {
		var b strings.Builder
		b.WriteString(headerStyle.Render(" Sort by (Enter/Space: select, Esc: close) "))
		b.WriteString("\n\n")
		b.WriteString(m.sortPicker.View())
		return b.String()
	}

	header := headerStyle.Render(fmt.Sprintf(" foldtree-demo --rich  |  %d tasks shown  |  filter=%v sort=%s ",
		len(m.treeView.State().Rows()), categoryList(m.statusFilters), m.sortOrder))
	searchBar := m.treeView.ViewSearchBar(func(s string) string { return s })
	help := helpStyle.Render("j/k move  enter/space toggle  e/c expand/collapse branch  E/C all  " +
		"/ search  n/N cycle  f filter  s sort  q quit")
	return header + "\n" + searchBar + m.treeView.View() + "\n" + help
}

func categoryList(filters map[string]bool) []string {
	if len(filters) == 0 {
		return []string{"all"}
	}
	var out []string
	for _, c := range taskCategories {
		if filters[c] {
			out = append(out, c)
		}
	}
	return out
}

func runRichDemo() {
	p := tea.NewProgram(newRichModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
