package rules

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Describe renders the enabled rules as natural language for LLM prompts.
func Describe(rs []Rule) string {
	var lines []string
	for _, r := range rs {
		if !r.Enabled {
			continue
		}
		switch r.Type {
		case TypeForbiddenTerms:
			var c TermsConfig
			if json.Unmarshal(r.Config, &c) != nil {
				continue
			}
			var parts []string
			for _, t := range c.Terms {
				if t.Replacement != "" {
					parts = append(parts, fmt.Sprintf("„%s“ (stattdessen „%s“)", t.Term, t.Replacement))
				} else {
					parts = append(parts, "„"+t.Term+"“")
				}
			}
			if len(parts) > 40 {
				parts = parts[:40]
			}
			lines = append(lines, "Nicht zulässige Begriffe: "+strings.Join(parts, ", "))
		case TypeTitleCase:
			var c TitleCaseConfig
			if json.Unmarshal(r.Config, &c) == nil {
				style := map[string]string{"sentence": "Satzschreibung (nur erstes Wort groß, außer Eigennamen/Abkürzungen)",
					"title": "Titelschreibung (bedeutende Wörter groß)", "upper": "durchgehend Großbuchstaben"}[c.Style]
				lines = append(lines, "Überschriften in "+style)
			}
		case TypeSentenceLength:
			var c SentenceConfig
			if json.Unmarshal(r.Config, &c) == nil {
				p, d := c.Procedural, c.Descriptive
				if p == 0 {
					p = 20
				}
				if d == 0 {
					d = 25
				}
				lines = append(lines, fmt.Sprintf("Sätze höchstens %d Wörter in Arbeitsschritten, %d Wörter in beschreibendem Text (ASD-STE100)", p, d))
			}
		case TypeDictionary:
			var c DictionaryConfig
			if json.Unmarshal(r.Config, &c) == nil && len(c.Words) > 0 {
				w := c.Words
				if len(w) > 60 {
					w = w[:60]
				}
				lines = append(lines, "Zulässige Fachbegriffe (nicht als Fehler melden): "+strings.Join(w, ", "))
			}
		case TypeXPathForbidden, TypeXPathRequired, TypeXPathValues:
			var c XPathConfig
			if json.Unmarshal(r.Config, &c) == nil && c.Message != "" && r.Source != "brex" {
				lines = append(lines, c.Message)
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "- " + strings.Join(lines, "\n- ")
}
