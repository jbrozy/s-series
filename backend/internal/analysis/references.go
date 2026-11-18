package analysis

import (
	"fmt"
	"sort"
	"strings"

	"scheck/internal/s1000d"
	"scheck/internal/xmldoc"
)

var idRefAttrs = []string{"internalRefId", "warningRefs", "cautionRefs", "applicRefId", "reasonForUpdateRefIds"}

// CheckReferences validates cross references inside and between files:
// internal references, data module references, publication module
// references and ICN graphics.
func CheckReferences(c *Corpus) []Finding {
	var out []Finding
	dmcs := map[string]bool{}
	pmcs := map[string]bool{}
	for _, d := range c.Docs {
		if d.Kind == s1000d.KindPM {
			pmcs[d.DMC] = true
		} else if d.DMC != "" {
			dmcs[d.DMC] = true
		}
	}
	usedICN := map[string]bool{}

	for _, d := range c.Docs {
		root := d.Root.Root()
		ids := map[string]bool{}
		root.Walk(func(n *xmldoc.Node) bool {
			if id := n.Attr("id"); id != "" {
				ids[id] = true
			}
			return true
		})
		root.Walk(func(n *xmldoc.Node) bool {
			if n.Name == "dmRefIdent" || n.Name == "dmIdent" {
				return n.Name != "dmIdent"
			}
			for _, attr := range idRefAttrs {
				v := n.Attr(attr)
				if v == "" {
					continue
				}
				for _, ref := range strings.Fields(v) {
					if !ids[ref] {
						sev := SevError
						if attr == "applicRefId" || attr == "reasonForUpdateRefIds" {
							sev = SevWarning
						}
						f := newFinding(d, n, CatReferences, "REF_INTERNAL_MISSING", sev,
							fmt.Sprintf("Verweis %s=%s zeigt auf kein Element in diesem Datenmodul.", attr, q(ref)))
						f.Suggestion = "Ziel-ID prüfen oder Ziel-Element mit passender id anlegen."
						out = append(out, f)
					}
				}
			}
			switch n.Name {
			case "dmRef":
				if n.Attr("scheckMissing") != "" {
					f := newFinding(d, n, CatReferences, "REF_LINK_MISSING", SevWarning,
						fmt.Sprintf("Link auf %s führt ins Leere: Die Seite ist nicht im HTML-Export enthalten.", q(n.Attr("scheckHref"))))
					f.Suggestion = "Seite nachliefern oder Linkziel korrigieren."
					out = append(out, f)
					break
				}
				target := s1000d.RefDMC(n)
				if target == "" {
					break
				}
				if target == d.DMC {
					out = append(out, newFinding(d, n, CatReferences, "REF_SELF", SevInfo,
						"Datenmodul verweist auf sich selbst. Für Ziele im selben Modul internalRef verwenden."))
				} else if !dmcs[target] && n.Ancestor("dmStatus", "identAndStatusSection") == nil {
					f := newFinding(d, n, CatReferences, "REF_DM_MISSING", SevWarning,
						fmt.Sprintf("Referenziertes Datenmodul %s ist nicht im Export enthalten.", target))
					f.Suggestion = "Datenmodul nachliefern oder DMC des Verweises korrigieren."
					out = append(out, f)
				}
			case "pmRef":
				if pc := n.Find("pmCode"); pc != nil && len(pmcs) > 0 {
					target := s1000d.FormatPMC(pc)
					if !pmcs[target] {
						out = append(out, newFinding(d, n, CatReferences, "REF_PM_MISSING", SevWarning,
							fmt.Sprintf("Referenziertes Publikationsmodul %s ist nicht im Export enthalten.", target)))
					}
				}
			}
			if icn := n.Attr("infoEntityIdent"); icn != "" {
				key := strings.ToUpper(icn)
				usedICN[key] = true
				if _, ok := c.ICNs[key]; !ok {
					f := newFinding(d, n, CatReferences, "REF_ICN_MISSING", SevError,
						fmt.Sprintf("Grafik/Medienobjekt %s ist nicht im Export enthalten.", icn))
					f.Suggestion = "ICN-Datei mit exportieren oder infoEntityIdent korrigieren."
					out = append(out, f)
				}
			}
			return true
		})
		// figures without any graphic
		for _, fig := range root.FindAll("figure") {
			if len(fig.FindAll("graphic")) == 0 {
				out = append(out, newFinding(d, fig, CatReferences, "FIGURE_WITHOUT_GRAPHIC", SevWarning,
					fmt.Sprintf("Abbildung %s enthält keine Grafik.", q(s1000d.TitleOf(fig)))))
			}
		}
	}
	var unused []string
	for key := range c.ICNs {
		if !usedICN[key] {
			unused = append(unused, key)
		}
	}
	sort.Strings(unused)
	for _, key := range unused {
		out = append(out, Finding{File: c.ICNs[key], Category: CatReferences, Code: "ICN_UNUSED", Severity: SevInfo,
			Message: fmt.Sprintf("Grafik %s wird in keinem Datenmodul referenziert.", key)})
	}
	return out
}
