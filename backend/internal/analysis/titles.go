package analysis

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"scheck/internal/s1000d"
	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

type titleOcc struct {
	doc  *s1000d.Doc
	node *xmldoc.Node // the <title> (or techName) element
	text string
	kind string
}

var titledUnits = map[string]string{
	"levelledPara": "Abschnitt", "proceduralStep": "Schritt", "figure": "Abbildung",
	"table": "Tabelle", "isolationStep": "Fehlersuchschritt",
}

func collectTitles(c *Corpus) []titleOcc {
	var out []titleOcc
	for _, d := range c.Docs {
		d.Root.Root().Walk(func(n *xmldoc.Node) bool {
			if n.Name == "title" && n.Parent != nil {
				if kind, ok := titledUnits[n.Parent.Name]; ok {
					out = append(out, titleOcc{d, n, xmldoc.Norm(n.TextContent()), kind})
				}
			}
			return true
		})
	}
	return out
}

// CheckTitles validates titles: formatting, duplicates within a module,
// duplicate data module titles, spelling variants and inconsistent
// techName/infoName usage across the project.
func CheckTitles(c *Corpus) []Finding {
	var out []Finding
	titles := collectTitles(c)

	// formatting
	for _, t := range titles {
		raw := t.node.TextContent()
		switch {
		case t.text == "":
			out = append(out, newFinding(t.doc, t.node, CatTitles, "TITLE_EMPTY", SevError, t.kind+"-Titel ist leer."))
		case strings.HasSuffix(t.text, ".") || strings.HasSuffix(t.text, ":"):
			f := newFinding(t.doc, t.node, CatTitles, "TITLE_FORMAT", SevWarning,
				fmt.Sprintf("%s-Titel %s endet mit Satzzeichen.", t.kind, q(t.text)))
			f.Suggestion = strings.TrimRight(t.text, ".:")
			out = append(out, f)
		case !strings.Contains(raw, "\n") && (raw != strings.TrimSpace(raw) || strings.Contains(raw, "  ")):
			f := newFinding(t.doc, t.node, CatTitles, "TITLE_WHITESPACE", SevInfo,
				fmt.Sprintf("%s-Titel %s enthält überflüssige Leerzeichen.", t.kind, q(t.text)))
			f.Suggestion = t.text
			out = append(out, f)
		}
	}

	// duplicates among siblings and within a data module
	type key struct {
		parent *xmldoc.Node
		k      string
	}
	sibling := map[key]*titleOcc{}
	inDoc := map[string]*titleOcc{}
	for i := range titles {
		t := &titles[i]
		if t.text == "" || t.kind == "Schritt" {
			continue
		}
		unit := t.node.Parent
		k := textutil.Key(t.text)
		sk := key{unit.Parent, unit.Name + "|" + k}
		if first, ok := sibling[sk]; ok {
			f := newFinding(t.doc, t.node, CatTitles, "TITLE_DUPLICATE_SIBLING", SevWarning,
				fmt.Sprintf("%s-Titel %s existiert auf derselben Ebene bereits (Zeile %d).", t.kind, q(t.text), first.node.Line))
			f.Suggestion = "Abschnitte zusammenführen oder Titel eindeutig formulieren."
			out = append(out, f)
			continue
		}
		sibling[sk] = t
		dk := t.doc.FileID + "|" + unit.Name + "|" + k
		if first, ok := inDoc[dk]; ok {
			f := newFinding(t.doc, t.node, CatTitles, "TITLE_DUPLICATE_IN_DM", SevInfo,
				fmt.Sprintf("%s-Titel %s kommt in diesem Datenmodul mehrfach vor (zuerst Zeile %d).", t.kind, q(t.text), first.node.Line))
			out = append(out, f)
			continue
		}
		inDoc[dk] = t
	}

	out = append(out, checkDMTitles(c)...)
	out = append(out, checkTitleVariants(c, titles)...)
	return out
}

func checkDMTitles(c *Corpus) []Finding {
	var out []Finding
	dms := c.DMs()
	byTitle := map[string][]*s1000d.Doc{}
	for _, d := range dms {
		if d.TechName == "" {
			continue
		}
		k := d.Lang + "|" + textutil.Key(d.Title())
		byTitle[k] = append(byTitle[k], d)
	}
	for _, group := range byTitle {
		dmcs := map[string]bool{}
		for _, d := range group {
			dmcs[d.DMC] = true
		}
		if len(dmcs) < 2 {
			continue
		}
		for _, d := range group {
			var others []string
			sameSNS := true
			for _, o := range group {
				if o.DMC != d.DMC {
					others = append(others, o.DMC)
					if o.SNSKey != d.SNSKey {
						sameSNS = false
					}
				}
			}
			sort.Strings(others)
			sev := SevWarning
			if sameSNS {
				sev = SevInfo // variants of the same hardware item
			}
			n := d.Root.Root().Find("dmTitle")
			f := newFinding(d, n, CatTitles, "TITLE_DUPLICATE_DM", sev,
				fmt.Sprintf("Datenmodul-Titel %s existiert bereits in %s.", q(d.Title()), strings.Join(dedup(others), ", ")))
			f.Suggestion = "Titel eindeutig formulieren (z. B. Variante, Einbauort oder Baugruppe ergänzen)."
			out = append(out, f)
		}
	}

	// consistency of infoName per info code and techName per SNS/hardware
	out = append(out, majorityCheck(dms, func(d *s1000d.Doc) (string, string) {
		if d.InfoCode == "" || d.InfoName == "" {
			return "", ""
		}
		return d.Lang + "|" + d.InfoCode, d.InfoName
	}, "INFONAME_INCONSISTENT", "infoName", func(d *s1000d.Doc) string { return "Infocode " + d.InfoCode })...)
	out = append(out, majorityCheck(dms, func(d *s1000d.Doc) (string, string) {
		if d.SNSKey == "" || d.TechName == "" {
			return "", ""
		}
		return d.Lang + "|" + d.SNSKey, d.TechName
	}, "TECHNAME_INCONSISTENT", "techName", func(d *s1000d.Doc) string { return "SNS/Baugruppe " + d.SNSKey })...)
	return out
}

func majorityCheck(dms []*s1000d.Doc, keyVal func(*s1000d.Doc) (string, string), code, field string, groupLabel func(*s1000d.Doc) string) []Finding {
	var out []Finding
	groups := map[string]map[string]int{}
	for _, d := range dms {
		k, v := keyVal(d)
		if k == "" {
			continue
		}
		if groups[k] == nil {
			groups[k] = map[string]int{}
		}
		groups[k][v]++
	}
	for _, d := range dms {
		k, v := keyVal(d)
		if k == "" || len(groups[k]) < 2 {
			continue
		}
		best, bc, total := textutil.Majority(groups[k])
		if v == best || bc < 2 || float64(bc)/float64(total) < 0.6 {
			continue
		}
		node := d.Root.Root().Find(field)
		f := newFinding(d, node, CatTitles, code, SevWarning,
			fmt.Sprintf("%s %s weicht von der im Projekt üblichen Bezeichnung für %s ab (%d von %d Datenmodulen: %s).",
				field, q(v), groupLabel(d), bc, total, q(best)))
		f.Suggestion = best
		out = append(out, f)
	}
	return out
}

func dedup(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// checkTitleVariants finds titles that differ only in capitalization,
// spacing/hyphenation or in one typo-like word.
func checkTitleVariants(c *Corpus, titles []titleOcc) []Finding {
	all := append([]titleOcc{}, titles...)
	for _, d := range c.DMs() {
		if d.TechName != "" {
			if n := d.Root.Root().Find("techName"); n != nil {
				all = append(all, titleOcc{d, n, d.TechName, "techName"})
			}
		}
	}
	exact := map[string]int{}
	byLower := map[string]map[string]int{}
	for _, t := range all {
		if t.text == "" {
			continue
		}
		exact[t.text]++
		lk := strings.ToLower(t.text)
		if byLower[lk] == nil {
			byLower[lk] = map[string]int{}
		}
		byLower[lk][t.text]++
	}
	var out []Finding
	reported := map[*xmldoc.Node]bool{}

	// 1) capitalization variants
	for _, t := range all {
		forms := byLower[strings.ToLower(t.text)]
		if len(forms) < 2 {
			continue
		}
		best, bc, _ := textutil.Majority(forms)
		if t.text == best || bc <= forms[t.text] {
			continue
		}
		f := newFinding(t.doc, t.node, CatTitles, "TITLE_CASE_VARIANT", SevWarning,
			fmt.Sprintf("Titel %s weicht in der Groß-/Kleinschreibung von %s ab (%d× im Projekt).", q(t.text), q(best), bc))
		f.Suggestion = best
		out = append(out, f)
		reported[t.node] = true
	}

	// 2) spacing / hyphenation variants and 3) one-word typo variants
	lowerCount := map[string]int{}
	for lk, forms := range byLower {
		for _, n := range forms {
			lowerCount[lk] += n
		}
	}
	squash := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) || r == '-' || r == '/' {
				return -1
			}
			return r
		}, s)
	}
	bySquash := map[string]map[string]int{}
	buckets := map[string]map[string]bool{}
	for lk := range byLower {
		sq := squash(lk)
		if bySquash[sq] == nil {
			bySquash[sq] = map[string]int{}
		}
		bySquash[sq][lk] = lowerCount[lk]
		words := strings.Fields(lk)
		for i := range words {
			sig := make([]string, len(words))
			copy(sig, words)
			sig[i] = "*"
			bk := fmt.Sprintf("%d|%s", i, strings.Join(sig, " "))
			if buckets[bk] == nil {
				buckets[bk] = map[string]bool{}
			}
			buckets[bk][lk] = true
		}
	}
	suggestFor := map[string]string{} // lower key -> better lower key
	reason := map[string]string{}
	for _, group := range bySquash {
		if len(group) < 2 {
			continue
		}
		best, bc, _ := textutil.Majority(group)
		for lk, n := range group {
			if lk != best && n < bc {
				suggestFor[lk] = best
				reason[lk] = "Getrennt-/Zusammenschreibung"
			}
		}
	}
	for bk, members := range buckets {
		if len(members) < 2 {
			continue
		}
		var pos int
		fmt.Sscanf(bk, "%d|", &pos)
		keys := make([]string, 0, len(members))
		for k := range members {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				a, b := keys[i], keys[j]
				wa, wb := strings.Fields(a)[pos], strings.Fields(b)[pos]
				if !typoLike(wa, wb) {
					continue
				}
				ca, cb := lowerCount[a], lowerCount[b]
				switch {
				case ca > cb:
					suggestFor[b], reason[b] = a, fmt.Sprintf("„%s“ statt „%s“", wb, wa)
				case cb > ca:
					suggestFor[a], reason[a] = b, fmt.Sprintf("„%s“ statt „%s“", wa, wb)
				default:
					if _, ok := suggestFor[b]; !ok {
						suggestFor[b], reason[b] = a, fmt.Sprintf("„%s“ / „%s“", wb, wa)
					}
				}
			}
		}
	}
	for _, t := range all {
		lk := strings.ToLower(t.text)
		better, ok := suggestFor[lk]
		if !ok || reported[t.node] {
			continue
		}
		display := better
		if forms := byLower[better]; forms != nil {
			display, _, _ = textutil.Majority(forms)
		}
		sev := SevWarning
		if lowerCount[lk] == lowerCount[better] {
			sev = SevInfo
		}
		f := newFinding(t.doc, t.node, CatTitles, "TITLE_VARIANT", sev,
			fmt.Sprintf("Mögliche Schreibvariante: %s ähnelt %s (%d× im Projekt) – %s.", q(t.text), q(display), lowerCount[better], reason[lk]))
		f.Suggestion = display
		out = append(out, f)
	}
	return out
}

// typoLike reports whether two words look like spelling variants rather
// than intentionally different words.
func typoLike(a, b string) bool {
	if a == b || len([]rune(a)) < 4 || len([]rune(b)) < 4 {
		return false
	}
	if hasDigit(a) || hasDigit(b) {
		return false
	}
	// inflections, prefixes (pump/pumps, install/uninstall) are not typos
	if strings.HasPrefix(a, b) || strings.HasPrefix(b, a) || strings.HasSuffix(a, b) || strings.HasSuffix(b, a) {
		return false
	}
	d := textutil.OSA(a, b)
	l := min(len([]rune(a)), len([]rune(b)))
	return d == 1 || (d == 2 && l >= 8)
}

func hasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
