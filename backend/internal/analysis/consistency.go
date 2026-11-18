package analysis

import (
	"fmt"
	"sort"
	"strings"

	"scheck/internal/s1000d"
	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

// CheckConsistency covers project-wide content consistency: acronyms,
// warning/caution wording, part number designations, repeated steps,
// empty paragraphs and multiple issues of the same data module.
func CheckConsistency(c *Corpus) []Finding {
	var out []Finding
	out = append(out, checkAcronyms(c)...)
	out = append(out, checkWarningVariants(c)...)
	out = append(out, checkPartNames(c)...)

	byDMC := map[string][]*s1000d.Doc{}
	for _, d := range c.Docs {
		root := d.Root.Root()
		if d.DMC != "" {
			byDMC[d.DMC+"|"+d.Lang] = append(byDMC[d.DMC+"|"+d.Lang], d)
		}
		// consecutive identical procedural steps
		root.Walk(func(n *xmldoc.Node) bool {
			if n.Name != "proceduralStep" {
				return true
			}
			if prev := prevElem(n); prev != nil && prev.Name == "proceduralStep" {
				a := textutil.Key(prev.TextExcluding(s1000d.IsUnit))
				b := textutil.Key(n.TextExcluding(s1000d.IsUnit))
				if a != "" && a == b {
					f := newFinding(d, n, CatConsistency, "STEP_DUPLICATE", SevWarning,
						"Arbeitsschritt ist identisch mit dem vorherigen Schritt.")
					f.Suggestion = "Doppelten Schritt entfernen."
					out = append(out, f)
				}
			}
			return true
		})
		for _, p := range root.FindAll("para", "notePara", "warningAndCautionPara") {
			if p.Ancestor("identAndStatusSection") != nil {
				continue
			}
			if xmldoc.Norm(p.TextContent()) == "" && len(p.Children()) == 0 {
				out = append(out, newFinding(d, p, CatConsistency, "PARA_EMPTY", SevWarning, "Leerer Absatz."))
			}
		}
	}
	for _, group := range byDMC {
		if len(group) < 2 {
			continue
		}
		var files []string
		for _, d := range group {
			files = append(files, d.Path+" (Issue "+d.Issue+")")
		}
		sort.Strings(files)
		for _, d := range group {
			out = append(out, newFinding(d, nil, CatConsistency, "DMC_MULTIPLE_FILES", SevWarning,
				fmt.Sprintf("Datenmodul %s liegt mehrfach im Export vor: %s.", d.DMC, strings.Join(files, "; "))))
		}
	}
	return out
}

func prevElem(n *xmldoc.Node) *xmldoc.Node {
	for p := n.Prev; p != nil; p = p.Prev {
		if p.Kind == xmldoc.Element {
			return p
		}
	}
	return nil
}

type located struct {
	doc  *s1000d.Doc
	node *xmldoc.Node
}

func checkAcronyms(c *Corpus) []Finding {
	defs := map[string]map[string]int{}
	display := map[string]string{}
	var occ []struct {
		located
		term, def string
	}
	for _, d := range c.Docs {
		for _, a := range d.Root.Root().FindAll("acronym") {
			t, df := a.Child("acronymTerm"), a.Child("acronymDefinition")
			if t == nil || df == nil {
				continue
			}
			term := xmldoc.Norm(t.TextContent())
			def := xmldoc.Norm(df.TextContent())
			if term == "" || def == "" {
				continue
			}
			k := textutil.Key(def)
			if defs[term] == nil {
				defs[term] = map[string]int{}
			}
			defs[term][k]++
			display[term+"|"+k] = def
			occ = append(occ, struct {
				located
				term, def string
			}{located{d, a}, term, def})
		}
	}
	var out []Finding
	for _, o := range occ {
		group := defs[o.term]
		if len(group) < 2 {
			continue
		}
		best, bc, total := textutil.Majority(group)
		if textutil.Key(o.def) == best {
			continue
		}
		f := newFinding(o.doc, o.node, CatConsistency, "ACRONYM_CONFLICT", SevWarning,
			fmt.Sprintf("Abkürzung %s wird unterschiedlich definiert: hier %s, sonst %s (%d von %d).",
				q(o.term), q(o.def), q(display[o.term+"|"+best]), bc, total))
		f.Suggestion = display[o.term+"|"+best]
		out = append(out, f)
	}
	return out
}

func checkWarningVariants(c *Corpus) []Finding {
	type wc struct {
		located
		kind, text, key string
	}
	var all []wc
	count := map[string]int{}
	display := map[string]string{}
	for _, d := range c.Docs {
		for _, n := range d.Root.Root().FindAll("warning", "caution") {
			text := xmldoc.Norm(n.TextContent())
			if text == "" {
				continue
			}
			k := n.Name + "|" + textutil.Key(text)
			count[k]++
			display[k] = text
			all = append(all, wc{located{d, n}, n.Name, text, k})
		}
	}
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	better := map[string]string{}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			a, b := keys[i], keys[j]
			if strings.SplitN(a, "|", 2)[0] != strings.SplitN(b, "|", 2)[0] {
				continue
			}
			la, lb := len(a), len(b)
			if la < 20 || lb < 20 || float64(min(la, lb))/float64(max(la, lb)) < 0.85 {
				continue
			}
			if onlyDigitsDiffer(a, b) || textutil.Similarity(a, b) < 0.88 {
				continue
			}
			if count[a] >= count[b] {
				better[b] = a
			} else {
				better[a] = b
			}
		}
	}
	var out []Finding
	for _, w := range all {
		b, ok := better[w.key]
		if !ok {
			continue
		}
		label := map[string]string{"warning": "Warnhinweis (WARNING)", "caution": "Vorsichtshinweis (CAUTION)"}[w.kind]
		f := newFinding(w.doc, w.node, CatConsistency, strings.ToUpper(w.kind)+"_VARIANT", SevInfo,
			fmt.Sprintf("%s weicht nur leicht von einer im Projekt %d× verwendeten Formulierung ab.", label, count[b]))
		f.Suggestion = display[b]
		out = append(out, f)
	}
	return out
}

func checkPartNames(c *Corpus) []Finding {
	names := map[string]map[string]int{}
	display := map[string]string{}
	var occ []struct {
		located
		pn, name string
	}
	for _, d := range c.Docs {
		for _, n := range d.Root.Root().FindAll("supplyDescr", "spareDescr", "supportEquipDescr") {
			nm := n.Child("name")
			id := n.Find("identNumber")
			if nm == nil || id == nil {
				continue
			}
			pnNode := id.Find("partNumber")
			if pnNode == nil {
				continue
			}
			mfc := ""
			if m := id.Child("manufacturerCode"); m != nil {
				mfc = xmldoc.Norm(m.TextContent())
			}
			pn := mfc + "/" + xmldoc.Norm(pnNode.TextContent())
			name := xmldoc.Norm(nm.TextContent())
			k := textutil.Key(name)
			if names[pn] == nil {
				names[pn] = map[string]int{}
			}
			names[pn][k]++
			display[pn+"|"+k] = name
			occ = append(occ, struct {
				located
				pn, name string
			}{located{d, nm}, pn, name})
		}
	}
	var out []Finding
	for _, o := range occ {
		g := names[o.pn]
		if len(g) < 2 {
			continue
		}
		best, bc, total := textutil.Majority(g)
		if textutil.Key(o.name) == best {
			continue
		}
		f := newFinding(o.doc, o.node, CatConsistency, "PARTNO_NAME_CONFLICT", SevWarning,
			fmt.Sprintf("Teilenummer %s hat unterschiedliche Benennungen: hier %s, sonst %s (%d von %d).",
				o.pn, q(o.name), q(display[o.pn+"|"+best]), bc, total))
		f.Suggestion = display[o.pn+"|"+best]
		out = append(out, f)
	}
	return out
}
