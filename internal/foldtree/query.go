package foldtree

// SelectedID returns the ID of the currently selected row, or ok=false
// if there is no selection (an empty tree).
func (s *State) SelectedID() (id string, ok bool) {
	if s.cursor < 0 || s.cursor >= len(s.rows) {
		return "", false
	}
	return s.rows[s.cursor].ID, true
}

// SelectedIndex returns the index of the currently selected row into
// Rows, or -1 if there is no selection.
func (s *State) SelectedIndex() int {
	return s.cursor
}

// Offset returns the viewport's current top line.
func (s *State) Offset() int {
	return s.offset
}

// TotalLines returns the full scrollable extent: the sum of all visible
// rows' heights, plus any extra padding set via SetExtraPadding.
func (s *State) TotalLines() int {
	return s.totalLines
}

// Rows returns the currently visible, flattened rows in display order.
// The returned slice must not be mutated.
func (s *State) Rows() []Row {
	return s.rows
}

// VisibleRange returns the inclusive line range currently shown.
func (s *State) VisibleRange() (top, bottom int) {
	top = s.offset
	bottom = s.offset + s.height - 1
	if bottom < top {
		bottom = top
	}
	return top, bottom
}

// RowLineRange returns the inclusive line range the row at index
// occupies, or (0, -1) if index is out of range.
func (s *State) RowLineRange(index int) (start, end int) {
	if index < 0 || index >= len(s.rows) {
		return 0, -1
	}
	start = s.lineStarts[index]
	end = start + effectiveHeight(s.rows[index].Height) - 1
	return start, end
}

// RowVisible reports whether the row at index currently starts within
// the visible line range.
func (s *State) RowVisible(index int) bool {
	return s.rowStartVisible(index)
}
