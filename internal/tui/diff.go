package tui

// DiffOp represents the type of a diff operation
type DiffOp int

const (
	DiffEqual     DiffOp = iota
	DiffInsert                   // line exists only in the new version
	DiffDelete                   // line exists only in the old version
	DiffSeparator                // context separator ("@@" line)
)

// DiffLine pairs an operation with its text content
type DiffLine struct {
	Op   DiffOp
	Text string
}

const maxLCSLines = 800

// ComputeDiff computes a line-level diff between old and new using LCS.
// For inputs exceeding maxLCSLines total, it trims the common prefix/suffix
// and only diffs the changed core to avoid O(m*n) blowup.
func ComputeDiff(oldLines, newLines []string) []DiffLine {
	m, n := len(oldLines), len(newLines)

	if m+n > maxLCSLines {
		return computeDiffLargeInput(oldLines, newLines)
	}

	return lcs(oldLines, newLines)
}

func lcs(oldLines, newLines []string) []DiffLine {
	m, n := len(oldLines), len(newLines)

	table := make([][]int, m+1)
	for i := range table {
		table[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				table[i][j] = table[i-1][j-1] + 1
			} else if table[i-1][j] >= table[i][j-1] {
				table[i][j] = table[i-1][j]
			} else {
				table[i][j] = table[i][j-1]
			}
		}
	}

	var result []DiffLine
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			result = append(result, DiffLine{Op: DiffEqual, Text: oldLines[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || table[i][j-1] >= table[i-1][j]) {
			result = append(result, DiffLine{Op: DiffInsert, Text: newLines[j-1]})
			j--
		} else {
			result = append(result, DiffLine{Op: DiffDelete, Text: oldLines[i-1]})
			i--
		}
	}

	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}

	return result
}

// computeDiffLargeInput strips common prefix/suffix lines and only diffs the
// changed middle portion, keeping memory and CPU reasonable for large files.
func computeDiffLargeInput(oldLines, newLines []string) []DiffLine {
	m, n := len(oldLines), len(newLines)

	prefixLen := 0
	limit := m
	if n < limit {
		limit = n
	}
	for prefixLen < limit && oldLines[prefixLen] == newLines[prefixLen] {
		prefixLen++
	}

	suffixLen := 0
	for suffixLen < limit-prefixLen &&
		oldLines[m-1-suffixLen] == newLines[n-1-suffixLen] {
		suffixLen++
	}

	var result []DiffLine
	for i := 0; i < prefixLen; i++ {
		result = append(result, DiffLine{Op: DiffEqual, Text: oldLines[i]})
	}

	oldCore := oldLines[prefixLen : m-suffixLen]
	newCore := newLines[prefixLen : n-suffixLen]

	if len(oldCore)+len(newCore) <= maxLCSLines {
		result = append(result, lcs(oldCore, newCore)...)
	} else {
		for _, l := range oldCore {
			result = append(result, DiffLine{Op: DiffDelete, Text: l})
		}
		for _, l := range newCore {
			result = append(result, DiffLine{Op: DiffInsert, Text: l})
		}
	}

	for i := 0; i < suffixLen; i++ {
		result = append(result, DiffLine{Op: DiffEqual, Text: oldLines[m-suffixLen+i]})
	}

	return result
}

// ContextDiff collapses runs of DiffEqual lines, keeping only contextSize
// lines around each change. Collapsed regions are replaced by a single
// DiffSeparator entry. If the entire diff is equal, returns nil.
func ContextDiff(diff []DiffLine, contextSize int) []DiffLine {
	if contextSize < 0 {
		contextSize = 3
	}

	hasChanges := false
	for _, d := range diff {
		if d.Op != DiffEqual {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return nil
	}

	keep := make([]bool, len(diff))
	for i, d := range diff {
		if d.Op != DiffEqual {
			lo := i - contextSize
			if lo < 0 {
				lo = 0
			}
			hi := i + contextSize
			if hi >= len(diff) {
				hi = len(diff) - 1
			}
			for k := lo; k <= hi; k++ {
				keep[k] = true
			}
		}
	}

	var result []DiffLine
	inGap := false
	for i, d := range diff {
		if keep[i] {
			if inGap {
				result = append(result, DiffLine{Op: DiffSeparator, Text: "@@"})
				inGap = false
			}
			result = append(result, d)
		} else {
			inGap = true
		}
	}

	return result
}
