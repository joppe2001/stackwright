package registry

import (
	"sort"
	"strings"
)

// ByCategory returns all entries in the given category, preserving bundle order.
// Used by the layer navigator when the user opens the sub-list for a layer.
func (b Bundle) ByCategory(c Category) []Entry {
	out := make([]Entry, 0, 8)
	for _, e := range b.Entries {
		if e.Category == c {
			out = append(out, e)
		}
	}
	return out
}

// BySlug returns the entry with the given slug, or (Entry{}, false) if missing.
func (b Bundle) BySlug(slug string) (Entry, bool) {
	for _, e := range b.Entries {
		if e.Slug == slug {
			return e, true
		}
	}
	return Entry{}, false
}

// SearchResult is one hit with its relevance score.
// Higher score = better match. Used for fuzzy ranking in the design TUI.
type SearchResult struct {
	Entry Entry
	Score int
}

// Search does a simple fuzzy match over Name and Slug, optionally filtered
// by category. Scoring rewards prefix matches and exact matches heavily;
// subsequence matches get partial credit so typos still surface the right tech.
//
// If query is empty, all entries (optionally filtered) are returned in bundle order.
// Category == "" means "all categories."
func (b Bundle) Search(query string, category Category) []SearchResult {
	q := strings.ToLower(strings.TrimSpace(query))
	results := make([]SearchResult, 0, 8)

	for _, e := range b.Entries {
		if category != "" && e.Category != category {
			continue
		}
		if q == "" {
			results = append(results, SearchResult{Entry: e, Score: 0})
			continue
		}
		score := scoreEntry(e, q)
		if score > 0 {
			results = append(results, SearchResult{Entry: e, Score: score})
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		// Higher score first. Stable keeps bundle order for ties (keeps
		// the navigator predictable when the user is just browsing).
		return results[i].Score > results[j].Score
	})
	return results
}

// scoreEntry is a quick-and-dirty relevance score. Tuned so exact and
// prefix matches always outrank substring or subsequence matches.
func scoreEntry(e Entry, q string) int {
	name := strings.ToLower(e.Name)
	slug := strings.ToLower(e.Slug)

	switch {
	case name == q, slug == q:
		return 1000
	case strings.HasPrefix(name, q), strings.HasPrefix(slug, q):
		return 500
	case strings.Contains(name, q), strings.Contains(slug, q):
		return 250
	}
	if subsequenceMatch(name, q) || subsequenceMatch(slug, q) {
		// A subsequence match means every query char appears in order,
		// but not contiguously — useful for typos like "nxjs" → "next.js".
		return 100
	}
	return 0
}

// subsequenceMatch reports whether needle is a subsequence of haystack.
// Both assumed lower-cased by the caller.
func subsequenceMatch(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	i := 0
	for _, r := range haystack {
		if rune(needle[i]) == r {
			i++
			if i == len(needle) {
				return true
			}
		}
	}
	return false
}
