// Package textutil contains string similarity helpers used by the checks.
package textutil

import (
	"regexp"
	"strings"
	"unicode"
)

// Levenshtein computes the edit distance between two strings (rune based).
func Levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
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
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// OSA computes the optimal string alignment distance (Damerau-Levenshtein
// with adjacent transpositions).
func OSA(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n, m := len(ra), len(rb)
	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := 0; j <= m; j++ {
		d[0][j] = j
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[n][m]
}

// Similarity returns 1 - levenshtein/maxlen in [0,1].
func Similarity(a, b string) float64 {
	la, lb := len([]rune(a)), len([]rune(b))
	mx := max(la, lb)
	if mx == 0 {
		return 1
	}
	return 1 - float64(Levenshtein(a, b))/float64(mx)
}

var trailingPunct = regexp.MustCompile(`[\s.,;:!?]+$`)

// Key normalizes a text for duplicate comparison.
func Key(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	return trailingPunct.ReplaceAllString(s, "")
}

// Words tokenizes text into letter words (including umlauts, hyphenated parts split).
func Words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
}

// WordsWithPos returns words with their byte offsets.
func WordsWithPos(s string) (words []string, pos []int) {
	start := -1
	for i, r := range s {
		if unicode.IsLetter(r) {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			words = append(words, s[start:i])
			pos = append(pos, start)
			start = -1
		}
	}
	if start >= 0 {
		words = append(words, s[start:])
		pos = append(pos, start)
	}
	return
}

// IsUpperWord reports whether all letters are upper case (acronyms).
func IsUpperWord(s string) bool {
	has := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			has = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return has
}

// Sentences splits text into sentences (simple heuristic).
func Sentences(s string) []string {
	var out []string
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		b.WriteRune(r)
		if r == '.' || r == '!' || r == '?' || r == ';' {
			next := ' '
			if i+1 < len(rs) {
				next = rs[i+1]
			}
			prev := ' '
			if i > 0 {
				prev = rs[i-1]
			}
			// do not split decimal numbers or abbreviations like "z.B."
			if unicode.IsDigit(prev) && unicode.IsDigit(next) {
				continue
			}
			if unicode.IsSpace(next) || i+1 == len(rs) {
				if t := strings.TrimSpace(b.String()); t != "" {
					out = append(out, t)
				}
				b.Reset()
			}
		}
	}
	if t := strings.TrimSpace(b.String()); t != "" {
		out = append(out, t)
	}
	return out
}

// Truncate shortens s to n runes.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// Deletes1 returns all strings obtained by deleting one rune.
func Deletes1(s string) []string {
	r := []rune(s)
	out := make([]string, 0, len(r))
	for i := range r {
		out = append(out, string(r[:i])+string(r[i+1:]))
	}
	return out
}

// Majority returns the most frequent key and its count.
func Majority(counts map[string]int) (string, int, int) {
	best, bc, total := "", 0, 0
	for k, c := range counts {
		total += c
		if c > bc || (c == bc && k < best) {
			best, bc = k, c
		}
	}
	return best, bc, total
}
