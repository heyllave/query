// Package nearest finds the name a mistyped one most likely meant, for
// "did you mean" hints in error messages.
package nearest

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Match returns the candidate closest to name, or "" when none is close
// enough to be worth suggesting. A candidate that differs only in case always
// qualifies; otherwise the edits allowed grow with the name: none for a name
// of one or two characters, one up to five, two beyond that. So "totl" finds
// "total" but "x" finds nothing. An edit is an insertion, a deletion, a
// substitution or a swap of two neighbours. Ties go to
// the candidate that sorts first, so the answer does not depend on the order
// of candidates.
func Match(name string, candidates []string) string {
	if name == "" {
		return ""
	}
	sorted := append([]string(nil), candidates...)
	sort.Strings(sorted)

	lower := strings.ToLower(name)
	limit := maxDistance(utf8.RuneCountInString(name))
	best, bestDist := "", limit+1
	for _, c := range sorted {
		if c == name || c == "" {
			continue
		}
		if strings.ToLower(c) == lower {
			return c
		}
		if d := distance(lower, strings.ToLower(c), bestDist); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

func maxDistance(length int) int {
	switch {
	case length <= 2:
		return 0
	case length <= 5:
		return 1
	default:
		return 2
	}
}

// distance is the optimal-string-alignment distance between a and b (edits
// are insert, delete, substitute and swapping two neighbours), or stop when
// the lengths alone put it at stop or beyond.
func distance(a, b string, stop int) int {
	ra, rb := []rune(a), []rune(b)
	if diff := len(ra) - len(rb); diff >= stop || -diff >= stop {
		return stop
	}
	prev2 := make([]int, len(rb)+1)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(rb)]
}
