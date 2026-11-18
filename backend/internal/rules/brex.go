package rules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"scheck/internal/s1000d"
	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

// FromBREX converts the structure object rules of a BREX data module into
// project business rules.
func FromBREX(d *s1000d.Doc) []Rule {
	var out []Rule
	root := d.Root.Root()
	for i, sor := range root.FindAll("structureObjectRule") {
		pathEl := sor.Child("objectPath")
		if pathEl == nil {
			continue
		}
		xp := strings.TrimSpace(pathEl.TextContent())
		if xp == "" {
			continue
		}
		use := ""
		if u := sor.Child("objectUse"); u != nil {
			use = xmldoc.Norm(u.TextContent())
		}
		flag := pathEl.Attr("allowedObjectFlag")
		cfg := XPathConfig{XPath: xp, Message: use}
		for _, v := range sor.FindAll("objectValue") {
			allowed := v.Attr("valueAllowed")
			switch v.Attr("valueForm") {
			case "pattern":
				cfg.Pattern = joinPattern(cfg.Pattern, xsdToGo(allowed))
			case "range":
				cfg.Ranges = append(cfg.Ranges, allowed)
			default:
				cfg.Allowed = append(cfg.Allowed, allowed)
			}
		}
		ruleType := ""
		switch flag {
		case "0":
			ruleType = TypeXPathForbidden
		case "1":
			ruleType = TypeXPathRequired
		default:
			if len(cfg.Allowed) > 0 || cfg.Pattern != "" || len(cfg.Ranges) > 0 {
				ruleType = TypeXPathValues
			}
		}
		if ruleType == "" {
			continue
		}
		if flag == "1" && (len(cfg.Allowed) > 0 || cfg.Pattern != "" || len(cfg.Ranges) > 0) {
			// required AND restricted: emit both rules
			vc := cfg
			b, _ := json.Marshal(vc)
			out = append(out, Rule{Name: brexName(d, sor, i, "Werte"), Type: TypeXPathValues, Severity: "warning",
				Enabled: true, Source: "brex", Config: b})
			cfg.Allowed, cfg.Pattern, cfg.Ranges = nil, "", nil
		}
		b, _ := json.Marshal(cfg)
		r := Rule{Name: brexName(d, sor, i, ""), Type: ruleType, Severity: "warning", Enabled: true, Source: "brex", Config: b}
		if Validate(r) != nil {
			r.Enabled = false
		}
		out = append(out, r)
	}
	return out
}

func brexName(d *s1000d.Doc, sor *xmldoc.Node, i int, suffix string) string {
	id := sor.Attr("id")
	if id == "" {
		id = fmt.Sprintf("#%d", i+1)
	}
	name := "BREX " + id
	if use := sor.Child("objectUse"); use != nil {
		name += ": " + textutil.Truncate(xmldoc.Norm(use.TextContent()), 70)
	}
	if suffix != "" {
		name += " (" + suffix + ")"
	}
	return name
}

func joinPattern(a, b string) string {
	if a == "" {
		return b
	}
	return a + "|" + b
}

var xsdClass = regexp.MustCompile(`\\[cCiI]`)

// xsdToGo maps XSD regex specifics to Go RE2 syntax where possible.
func xsdToGo(p string) string {
	return xsdClass.ReplaceAllStringFunc(p, func(s string) string {
		switch s {
		case `\c`, `\i`:
			return `[A-Za-z_:.\-0-9]`
		default:
			return `[^A-Za-z_:.\-0-9]`
		}
	})
}
