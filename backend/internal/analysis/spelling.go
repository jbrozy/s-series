package analysis

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

var titleContainers = map[string]bool{"title": true, "techName": true, "infoName": true, "pmEntryTitle": true}

// metadata elements that should not be spell-checked
var skipText = map[string]bool{
	"dmCode": true, "identNumber": true, "partNumber": true, "manufacturerCode": true,
	"natoStockNumber": true, "shortName": true, "acronymTerm": true, "verbatimText": true,
	"identAndStatusSection": true, "dmAddress": true, "dmStatus": true, "inlineSignificantData": true,
	"internalRef": true, "dmRef": true, "externalPubRef": true, "symbol": true,
}

type wordOcc struct {
	word  string
	node  *xmldoc.Node
	title bool
	ctx   string
}

// CheckSpelling is a language-independent, corpus-based spelling check:
// a word that occurs only once in the whole project but is one edit away
// from a word used frequently is very likely a typo ("Hydarulic" vs
// "Hydraulic"). Project dictionaries from the business rules suppress
// accepted words.
func CheckSpelling(c *Corpus) []Finding {
	freq := map[string]int{}
	var occs []wordOcc
	docOf := map[*xmldoc.Node]int{}
	for di, d := range c.Docs {
		root := d.Root.Root()
		var rec func(n *xmldoc.Node, inTitle bool)
		rec = func(n *xmldoc.Node, inTitle bool) {
			for ch := n.FirstChild; ch != nil; ch = ch.Next {
				switch ch.Kind {
				case xmldoc.Element:
					if skipText[ch.Name] {
						continue
					}
					rec(ch, inTitle || titleContainers[ch.Name])
				case xmldoc.Text:
					for _, w := range textutil.Words(ch.Data) {
						lw := strings.ToLower(w)
						if utf8.RuneCountInString(lw) < 4 {
							continue
						}
						freq[lw]++
						if utf8.RuneCountInString(lw) >= 5 && !textutil.IsUpperWord(w) {
							occs = append(occs, wordOcc{word: w, node: n, title: inTitle, ctx: ch.Data})
							docOf[n] = di
						}
					}
				}
			}
		}
		if is := root.Child("identAndStatusSection"); is != nil {
			if t := is.Find("dmTitle"); t != nil {
				rec(t, true)
			}
		}
		if content := root.Child("content"); content != nil {
			rec(content, false)
		} else if root.Name != "dmodule" {
			rec(root, false)
		}
	}
	for w := range c.Dictionary {
		freq[strings.ToLower(w)] += 1000
	}

	// delete-1 index over frequent words (SymSpell-like)
	index := map[string][]string{}
	for w, n := range freq {
		if n < 3 {
			continue
		}
		index[w] = append(index[w], w)
		for _, d := range textutil.Deletes1(w) {
			index[d] = append(index[d], w)
		}
	}

	var out []Finding
	perDoc := map[int]int{}
	cache := map[string]string{}
	for _, o := range occs {
		lw := strings.ToLower(o.word)
		if freq[lw] > 1 || c.Dictionary[lw] {
			continue
		}
		best, ok := cache[lw]
		if !ok {
			best = bestCorrection(lw, index, freq)
			cache[lw] = best
		}
		if best == "" {
			continue
		}
		di := docOf[o.node]
		d := c.Docs[di]
		suggestion := matchCase(o.word, best)
		if o.title {
			f := newFinding(d, o.node, CatTitles, "TITLE_TYPO", SevWarning,
				fmt.Sprintf("Möglicher Tippfehler im Titel: %s – im Projekt wird %s %d× verwendet.", q(o.word), q(suggestion), freq[best]))
			f.Suggestion = suggestion
			out = append(out, f)
			continue
		}
		if perDoc[di] >= 25 {
			continue
		}
		perDoc[di]++
		f := newFinding(d, o.node, CatTitles, "TEXT_TYPO", SevInfo,
			fmt.Sprintf("Möglicher Tippfehler: %s – im Projekt wird %s %d× verwendet.", q(o.word), q(suggestion), freq[best]))
		f.Suggestion = suggestion
		out = append(out, f)
	}
	return out
}

func bestCorrection(w string, index map[string][]string, freq map[string]int) string {
	cands := map[string]bool{}
	for _, c := range index[w] {
		cands[c] = true
	}
	for _, d := range textutil.Deletes1(w) {
		for _, c := range index[d] {
			cands[c] = true
		}
	}
	best, bf := "", 0
	for c := range cands {
		if c == w || !typoLike(w, c) {
			continue
		}
		if textutil.OSA(w, c) > 1 && utf8.RuneCountInString(w) < 9 {
			continue
		}
		if freq[c] > bf {
			best, bf = c, freq[c]
		}
	}
	return best
}

func matchCase(orig, lower string) string {
	r := []rune(orig)
	if len(r) > 0 && strings.ToUpper(string(r[0])) == string(r[0]) {
		lr := []rune(lower)
		return strings.ToUpper(string(lr[0])) + string(lr[1:])
	}
	return lower
}
