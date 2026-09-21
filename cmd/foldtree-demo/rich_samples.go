package main

import "github.com/CaptShanks/terraprism/internal/foldtree"

// taskItem is a synthetic, deliberately non-Terraform payload: a small
// task-board item with a category, used to prove the generalized
// TreeView/Picker stack works with any domain, not just tfplan data.
type taskItem struct {
	title    string
	category string
}

// SearchText implements foldtree.Searchable.
func (t taskItem) SearchText() string { return t.title + " " + t.category }

func taskLeaf(id, title, category string) foldtree.Node {
	return foldtree.Node{ID: id, Height: 1, Payload: taskItem{title: title, category: category}}
}

func taskBlock(id, title, category string, children ...foldtree.Node) foldtree.Node {
	return foldtree.Node{
		ID: id, Height: 1, Collapsible: true,
		Payload:  taskItem{title: title, category: category},
		Children: children,
	}
}

// taskCategories is the fixed set of categories the demo's filter/sort
// pickers offer -- the same role tfplan.Action plays in the real app.
var taskCategories = []string{"bug", "feature", "chore"}

func buildTaskBoard() []foldtree.Node {
	return []foldtree.Node{
		taskBlock("task-1", "Fix crash on empty plan", "bug",
			taskLeaf("task-1.repro", "repro: pipe an empty JSON plan", "bug"),
			taskLeaf("task-1.fix", "fix: guard against zero resources", "bug"),
		),
		taskLeaf("task-2", "Add dark mode toggle", "feature"),
		taskBlock("task-3", "Refactor render pipeline", "chore",
			taskLeaf("task-3.a", "extract RowRenderer interface", "chore"),
			taskLeaf("task-3.b", "delete dead code paths", "chore"),
		),
		taskLeaf("task-4", "Search box loses focus on resize", "bug"),
		taskBlock("task-5", "Support multiple output panes", "feature",
			taskLeaf("task-5.a", "design SplitView layout", "feature"),
			taskLeaf("task-5.b", "wire hideable log pane", "feature"),
		),
		taskLeaf("task-6", "Upgrade bubbletea to latest", "chore"),
		taskLeaf("task-7", "Crash when terminal resized below 3 lines", "bug"),
		taskLeaf("task-8", "Add CSV export", "feature"),
		taskBlock("task-9", "Tidy up test helpers", "chore",
			taskLeaf("task-9.a", "dedupe key() helper across test files", "chore"),
			taskLeaf("task-9.b", "rename ambiguous fixtures", "chore"),
		),
		taskLeaf("task-10", "Flaky test: TestScrollSettles", "bug"),
	}
}
