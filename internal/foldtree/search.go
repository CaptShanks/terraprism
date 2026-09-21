package foldtree

import "strings"

// FuzzyMatch reports whether every character of query appears in text in
// order (not necessarily consecutively), case-insensitively. E.g. "lmbda"
// matches "lambda", "inst" matches "instance". An empty query always
// matches.
func FuzzyMatch(text, query string) bool {
	text = strings.ToLower(text)
	query = strings.ToLower(query)
	if query == "" {
		return true
	}
	qi := 0
	for i := 0; i < len(text) && qi < len(query); i++ {
		if text[i] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}
