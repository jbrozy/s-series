package analysis

import (
	"fmt"

	"scheck/internal/s1000d"
	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

var listItemNames = map[string]string{
	"randomList":              "listItem",
	"sequentialList":          "listItem",
	"attentionRandomList":     "attentionRandomListItem",
	"attentionSequentialList": "attentionSequentialListItem",
}

var reqGroups = map[string]struct{ item, label string }{
	"supplyDescrGroup":       {"supplyDescr", "Betriebsstoffe/Verbrauchsmaterial"},
	"spareDescrGroup":        {"spareDescr", "Ersatzteile"},
	"supportEquipDescrGroup": {"supportEquipDescr", "Hilfs- und Prüfmittel"},
}

// CheckLists finds duplicate and near-duplicate entries in lists,
// definition lists, requirement groups and tables.
func CheckLists(c *Corpus) []Finding {
	var out []Finding
	for _, d := range c.Docs {
		root := d.Root.Root()
		root.Walk(func(n *xmldoc.Node) bool {
			if itemName, ok := listItemNames[n.Name]; ok {
				out = append(out, checkListItems(d, n, childrenNamed(n, itemName), "Listeneintrag")...)
				if cnt := len(childrenNamed(n, itemName)); cnt == 1 {
					f := newFinding(d, n, CatLists, "LIST_SINGLE_ITEM", SevInfo,
						"Liste mit nur einem Eintrag – Listen sollten mindestens zwei Einträge haben, sonst als Fließtext formulieren.")
					out = append(out, f)
				}
			}
			if n.Name == "definitionList" {
				var terms []*xmldoc.Node
				for _, it := range childrenNamed(n, "definitionListItem") {
					if t := it.Child("listItemTerm"); t != nil {
						terms = append(terms, t)
					}
				}
				out = append(out, checkListItems(d, n, terms, "Begriff")...)
			}
			if g, ok := reqGroups[n.Name]; ok {
				out = append(out, checkReqGroup(d, n, g.item, g.label)...)
			}
			if n.Name == "tbody" {
				out = append(out, checkTableRows(d, n)...)
			}
			return true
		})
	}
	return out
}

func childrenNamed(n *xmldoc.Node, name string) []*xmldoc.Node {
	var out []*xmldoc.Node
	for _, c := range n.Children() {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// itemText returns the text of a list item without nested lists.
func itemText(n *xmldoc.Node) string {
	return xmldoc.Norm(n.TextExcluding(func(x *xmldoc.Node) bool {
		_, isList := listItemNames[x.Name]
		return isList || x.Name == "definitionList"
	}))
}

func checkListItems(d *s1000d.Doc, list *xmldoc.Node, items []*xmldoc.Node, what string) []Finding {
	var out []Finding
	seen := map[string]int{}
	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = itemText(it)
		k := textutil.Key(texts[i])
		if k == "" {
			continue
		}
		if j, ok := seen[k]; ok {
			f := newFinding(d, it, CatLists, "LIST_DUPLICATE", SevWarning,
				fmt.Sprintf("Doppelter %s %s (identisch mit Eintrag %d).", what, q(texts[i]), j+1))
			f.Suggestion = "Doppelten Eintrag entfernen oder inhaltlich unterscheiden."
			out = append(out, f)
			continue
		}
		seen[k] = i
	}
	// near duplicates (e.g. typo variants of the same entry)
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			a, b := textutil.Key(texts[i]), textutil.Key(texts[j])
			if a == b || len([]rune(a)) < 8 || len([]rune(b)) < 8 {
				continue
			}
			if onlyDigitsDiffer(a, b) {
				continue
			}
			if textutil.Similarity(a, b) >= 0.9 {
				f := newFinding(d, items[j], CatLists, "LIST_NEAR_DUPLICATE", SevInfo,
					fmt.Sprintf("%s %s ist fast identisch mit Eintrag %d %s.", what, q(texts[j]), i+1, q(texts[i])))
				f.Suggestion = "Prüfen, ob es sich um ein Duplikat mit Schreibvariante handelt."
				out = append(out, f)
			}
		}
	}
	return out
}

func onlyDigitsDiffer(a, b string) bool {
	strip := func(s string) string {
		r := []rune{}
		for _, c := range s {
			if c < '0' || c > '9' {
				r = append(r, c)
			}
		}
		return string(r)
	}
	return strip(a) == strip(b)
}

func reqKey(n *xmldoc.Node) (key, label string) {
	name := ""
	if nm := n.Child("name"); nm != nil {
		name = xmldoc.Norm(nm.TextContent())
	}
	pn := ""
	if id := n.Find("identNumber"); id != nil {
		mfc := ""
		if m := id.Child("manufacturerCode"); m != nil {
			mfc = xmldoc.Norm(m.TextContent())
		}
		if p := id.Find("partNumber"); p != nil {
			pn = mfc + "/" + xmldoc.Norm(p.TextContent())
		}
	}
	if pn == "" {
		if nsn := n.Find("natoStockNumber"); nsn != nil {
			pn = "NSN " + xmldoc.Norm(nsn.TextContent())
		}
	}
	label = name
	if pn != "" {
		label += " (" + pn + ")"
	}
	return textutil.Key(name) + "|" + pn, label
}

func checkReqGroup(d *s1000d.Doc, g *xmldoc.Node, itemName, groupLabel string) []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, it := range g.FindAll(itemName) {
		k, label := reqKey(it)
		if k == "|" {
			continue
		}
		if seen[k] {
			f := newFinding(d, it, CatLists, "REQ_DUPLICATE", SevWarning,
				fmt.Sprintf("%s %s ist in den Vorbedingungen mehrfach aufgeführt.", groupLabel, q(label)))
			f.Suggestion = "Eintrag zusammenfassen und ggf. Menge (reqQuantity) anpassen."
			out = append(out, f)
		}
		seen[k] = true
	}
	return out
}

func checkTableRows(d *s1000d.Doc, tbody *xmldoc.Node) []Finding {
	var out []Finding
	seen := map[string]int{}
	for i, row := range childrenNamed(tbody, "row") {
		var cells []string
		for _, e := range row.Children() {
			cells = append(cells, xmldoc.Norm(e.TextContent()))
		}
		k := textutil.Key(fmt.Sprint(cells))
		if k == "[]" || k == "" {
			continue
		}
		if j, ok := seen[k]; ok {
			f := newFinding(d, row, CatLists, "TABLE_ROW_DUPLICATE", SevInfo,
				fmt.Sprintf("Tabellenzeile %d ist identisch mit Zeile %d.", i+1, j+1))
			out = append(out, f)
			continue
		}
		seen[k] = i
	}
	return out
}
