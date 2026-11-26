// Package project finds what a board's form can point at: git worktrees,
// local branches and files inside the project root, plus the fuzzy matcher
// the pickers filter them with.
package project

import (
	"sort"
	"strings"
	"unicode"
)

// Match is one item that contains the query as a case-insensitive
// subsequence. Index points into the items given to Fuzzy.
type Match struct {
	Index int
	Score int
}

// Score tiers: a prefix beats a word-boundary substring, which beats any
// contiguous substring, which beats a scattered subsequence.
const (
	tierPrefix     = 40000
	tierBoundary   = 30000
	tierContiguous = 20000
	tierScattered  = 10000
)

// Fuzzy ranks the items that contain query as a subsequence, best first;
// ties keep the shorter item, then the original order. An empty query
// matches everything in its original order.
func Fuzzy(query string, items []string) []Match {
	q := []rune(strings.ToLower(strings.TrimSpace(query)))
	out := make([]Match, 0, len(items))
	for i, item := range items {
		if len(q) == 0 {
			out = append(out, Match{Index: i})
			continue
		}
		if s, ok := score(q, []rune(strings.ToLower(item))); ok {
			out = append(out, Match{Index: i, Score: s})
		}
	}
	if len(q) == 0 {
		return out
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		return len(items[out[a].Index]) < len(items[out[b].Index])
	})
	return out
}

// isBoundary reports whether position i of s starts a word: the start of
// the string, or right after a separator such as / - _ . or a space.
func isBoundary(s []rune, i int) bool {
	if i == 0 {
		return true
	}
	p := s[i-1]
	return !unicode.IsLetter(p) && !unicode.IsDigit(p)
}

// score rates q against s (both lower-cased); ok is false when q is not a
// subsequence of s. Within a tier shorter items score higher.
func score(q, s []rune) (int, bool) {
	if len(q) > len(s) {
		return 0, false
	}
	length := min(len(s), 999)
	if hasPrefix(s, q) {
		return tierPrefix - length, true
	}
	contiguous, boundary := false, false
	for i := 0; i+len(q) <= len(s); i++ {
		if hasPrefix(s[i:], q) {
			contiguous = true
			if isBoundary(s, i) {
				boundary = true
				break
			}
		}
	}
	if boundary {
		return tierBoundary - length, true
	}
	if contiguous {
		return tierContiguous - length, true
	}
	// Scattered: greedy left-to-right, rewarding letters that start words
	// and penalising the gaps between matched letters.
	bonus, gaps, last := 0, 0, -1
	j := 0
	for i := 0; i < len(s) && j < len(q); i++ {
		if s[i] != q[j] {
			continue
		}
		if isBoundary(s, i) {
			bonus += 10
		}
		if last >= 0 {
			gaps += i - last - 1
		}
		last = i
		j++
	}
	if j < len(q) {
		return 0, false
	}
	return tierScattered + min(bonus, 500) - min(gaps, 400) - length/10, true
}

func hasPrefix(s, p []rune) bool {
	if len(p) > len(s) {
		return false
	}
	for i := range p {
		if s[i] != p[i] {
			return false
		}
	}
	return true
}
