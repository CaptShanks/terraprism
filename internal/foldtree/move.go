package foldtree

// MoveUp moves the cursor to the previous row and brings it into view.
// Once already at the first row, further calls free-scroll upward
// instead (revealing any content above), or snap back to the cursor if
// the viewport had been scrolled away from it (e.g. by MoveMouse).
func (s *State) MoveUp() {
	if s.cursor > 0 {
		s.cursor--
		s.ensureVisible()
		return
	}
	s.freeScroll(-1)
}

// MoveDown is the symmetric counterpart of MoveUp.
func (s *State) MoveDown() {
	if s.cursor >= 0 && s.cursor < len(s.rows)-1 {
		s.cursor++
		s.ensureVisible()
		return
	}
	s.freeScroll(1)
}

// MoveToTop selects the first row.
func (s *State) MoveToTop() {
	if len(s.rows) == 0 {
		return
	}
	s.cursor = 0
	s.ensureVisible()
}

// MoveToBottom selects the last row.
func (s *State) MoveToBottom() {
	if len(s.rows) == 0 {
		return
	}
	s.cursor = len(s.rows) - 1
	s.ensureVisible()
}

// SelectIndex moves the selection directly to row i, clamping into the
// valid range, and ensures it's visible. A no-op if there are no rows.
func (s *State) SelectIndex(i int) {
	if len(s.rows) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(s.rows) {
		i = len(s.rows) - 1
	}
	s.cursor = i
	s.ensureVisible()
}

// PageDown moves the cursor forward by roughly one viewport height's
// worth of lines (at least one row).
func (s *State) PageDown() { s.pageBy(1) }

// PageUp is the symmetric counterpart of PageDown.
func (s *State) PageUp() { s.pageBy(-1) }

func (s *State) pageBy(dir int) {
	if len(s.rows) == 0 {
		return
	}
	if s.height <= 0 {
		if dir > 0 {
			s.MoveDown()
		} else {
			s.MoveUp()
		}
		return
	}

	cur := s.clampedCursor()
	targetLine := s.lineStarts[cur] + dir*s.height
	next := s.rowAtLine(targetLine)

	if dir > 0 && next <= cur {
		next = min(cur+1, len(s.rows)-1)
	}
	if dir < 0 && next >= cur {
		next = max(cur-1, 0)
	}
	s.cursor = next
	s.ensureVisible()
}

// rowAtLine returns the index of the row that contains the given line
// (clamped to the valid row range).
func (s *State) rowAtLine(line int) int {
	if len(s.lineStarts) == 0 {
		return -1
	}
	if line <= 0 {
		return 0
	}
	lo, hi := 0, len(s.lineStarts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if s.lineStarts[mid] <= line {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// SelectParent moves the selection to the immediate parent of the
// currently selected row, if it has one and it's currently visible (not
// itself hidden behind a further-collapsed ancestor). No-op if there's
// no selection or no visible parent (e.g. already on a root row).
func (s *State) SelectParent() {
	id, ok := s.SelectedID()
	if !ok {
		return
	}
	parentID, ok := s.parent[id]
	if !ok {
		return
	}
	if idx, ok := s.rowIndex[parentID]; ok {
		s.cursor = idx
		s.ensureVisible()
	}
}

// MoveMouse scrolls the viewport by delta lines without moving the
// cursor — this is what a mouse wheel does, and it's exactly how cursor
// and scroll position end up disagreeing until a subsequent cursor
// movement reconciles them.
func (s *State) MoveMouse(delta int) {
	s.offset += delta
	s.clampOffset()
}

func (s *State) ensureVisible() {
	s.clampCursor()
	s.clampOffset()
	if s.cursor < 0 || s.height <= 0 {
		return
	}
	start := s.lineStarts[s.cursor]
	// A zero-height row still occupies its start line as a selectable
	// slot even though it renders nothing, so end is clamped to at
	// least start — otherwise a raw end of start-1 would make the
	// bottom-alignment branch below target one line short of the row's
	// actual position.
	end := start + effectiveHeight(s.rows[s.cursor].Height) - 1
	if end < start {
		end = start
	}
	rowHeight := end - start + 1

	switch {
	case start < s.offset:
		s.offset = start
	case rowHeight > s.height:
		// Row taller than the viewport: it can never be made to fit
		// entirely, so align to its top rather than its bottom, so at
		// least the start is visible.
		s.offset = start
	case end > s.offset+s.height-1:
		s.offset = end - s.height + 1
	}
	s.clampOffset()
}

func (s *State) clampOffset() {
	maxOffset := s.totalLines - s.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if s.offset > maxOffset {
		s.offset = maxOffset
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

func (s *State) rowStartVisible(i int) bool {
	if i < 0 || i >= len(s.rows) {
		return false
	}
	line := s.lineStarts[i]
	return line >= s.offset && line <= s.offset+s.height-1
}

// freeScroll implements "scroll one line further past a cursor that
// can't move," used once the cursor is already at the first/last row.
//
// It stops for good once the selected row's start line reaches the near
// edge of the viewport in the direction of travel, rather than
// continuing until the row is pushed just out of view: rowStartVisible
// treats a row sitting exactly on the boundary as still visible, so
// scrolling one more line would make it invisible, which would make the
// very next call's "not visible -> snap back" branch immediately undo
// the scroll — an unstable two-step oscillation instead of a settled
// state. This is the exact bug this package was built to eliminate.
func (s *State) freeScroll(dir int) {
	if s.cursor < 0 {
		s.offset += dir
		s.clampOffset()
		return
	}
	if !s.rowStartVisible(s.cursor) {
		s.ensureVisible()
		return
	}

	start := s.lineStarts[s.cursor]
	bottom := s.offset + s.height - 1
	if dir > 0 {
		if start > s.offset {
			s.offset++
		}
	} else {
		if start < bottom {
			s.offset--
		}
	}
	s.clampOffset()
}

func (s *State) clampedCursor() int {
	if s.cursor < 0 {
		return 0
	}
	if s.cursor >= len(s.rows) {
		return len(s.rows) - 1
	}
	return s.cursor
}
