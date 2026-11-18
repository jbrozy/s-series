// Package s1000d extracts the semantic model of S-Series (primarily S1000D)
// XML exports: data module identity, titles, references, lists and text
// units that are later used for checks and retrieval.
package s1000d

import (
	"path/filepath"
	"regexp"
	"strings"

	"scheck/internal/xmldoc"
)

// File kinds.
const (
	KindDM    = "dm"
	KindPM    = "pm"
	KindDML   = "dml"
	KindBREX  = "brex"
	KindICN   = "icn"
	KindXML   = "xml"
	KindOther = "other"
)

var rasterExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".tif": true, ".tiff": true, ".bmp": true}
var vectorExt = map[string]bool{".svg": true, ".cgm": true, ".eps": true, ".wmf": true, ".emf": true}
var xmlExt = map[string]bool{".xml": true, ".sgm": true, ".sgml": true}

func IsRaster(path string) bool { return rasterExt[strings.ToLower(filepath.Ext(path))] }
func IsVector(path string) bool { return vectorExt[strings.ToLower(filepath.Ext(path))] }
func IsXML(path string) bool    { return xmlExt[strings.ToLower(filepath.Ext(path))] }

// KindByName classifies a file by name only (XML files are refined after parsing).
func KindByName(path string) string {
	switch {
	case IsRaster(path) || IsVector(path):
		return KindICN
	case IsXML(path) || IsHTML(path):
		return KindXML
	}
	return KindOther
}

// ICNKey normalizes a file name to the information entity identifier.
func ICNKey(name string) string {
	base := filepath.Base(name)
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	return strings.ToUpper(base)
}

// Doc is a parsed data module (or other S-Series XML file).
type Doc struct {
	FileID   string
	Path     string
	Kind     string
	Root     *xmldoc.Node // document node
	DMC      string
	Issue    string
	Lang     string
	TechName string
	InfoName string
	InfoCode string
	SNSKey   string
	Content  string // first element below <content>, e.g. description, procedure
	Code     DMCode
	Source   string // "" for XML, SourceHTML for converted HTML pages
}

// DMCode holds the parts of a data module code that describe the product
// breakdown (standard numbering system).
type DMCode struct {
	MIC, SDC, SC, SSC, SSSC, Assy, DC, DCV, IC, ICV, ILC string
}

func (d *Doc) Title() string {
	if d.InfoName != "" {
		return d.TechName + " - " + d.InfoName
	}
	return d.TechName
}

// Label is a human-readable identification of the document.
func (d *Doc) Label() string {
	if d.DMC != "" {
		return d.DMC
	}
	return d.Path
}

// Parse parses an XML file and extracts identity information.
func Parse(fileID, path string, data []byte) (*Doc, error) {
	if IsHTML(path) {
		return ParseHTML(fileID, path, data)
	}
	root, err := xmldoc.Parse(data)
	if err != nil {
		return nil, err
	}
	d := &Doc{FileID: fileID, Path: path, Root: root, Kind: KindXML}
	el := root.Root()
	switch el.Name {
	case "dmodule":
		d.Kind = KindDM
		if dc := el.Find("dmIdent"); dc != nil {
			if code := dc.Child("dmCode"); code != nil {
				d.DMC = FormatDMC(code)
				d.InfoCode = code.Attr("infoCode") + code.Attr("infoCodeVariant")
				a := func(n string) string { return strings.ToUpper(code.Attr(n)) }
				d.Code = DMCode{MIC: a("modelIdentCode"), SDC: a("systemDiffCode"), SC: a("systemCode"), SSC: a("subSystemCode"),
					SSSC: a("subSubSystemCode"), Assy: a("assyCode"), DC: a("disassyCode"), DCV: a("disassyCodeVariant"),
					IC: a("infoCode"), ICV: a("infoCodeVariant"), ILC: a("itemLocationCode")}
				d.SNSKey = snsKey(code)
			}
			if iss := dc.Child("issueInfo"); iss != nil {
				d.Issue = iss.Attr("issueNumber") + "-" + iss.Attr("inWork")
			}
			if l := dc.Child("language"); l != nil {
				d.Lang = strings.ToLower(l.Attr("languageIsoCode"))
				if c := l.Attr("countryIsoCode"); c != "" {
					d.Lang += "-" + strings.ToUpper(c)
				}
			}
		}
		if t := el.Find("dmTitle"); t != nil {
			if tn := t.Child("techName"); tn != nil {
				d.TechName = xmldoc.Norm(tn.TextContent())
			}
			if in := t.Child("infoName"); in != nil {
				d.InfoName = xmldoc.Norm(in.TextContent())
			}
		}
		if c := el.Child("content"); c != nil {
			for _, ch := range c.Children() {
				if ch.Name != "refs" && ch.Name != "warningsAndCautions" && ch.Name != "warningsAndCautionsRef" {
					d.Content = ch.Name
					break
				}
			}
			if d.Content == "brex" {
				d.Kind = KindBREX
			}
		}
	case "pm":
		d.Kind = KindPM
		if pc := el.Find("pmCode"); pc != nil {
			d.DMC = FormatPMC(pc)
		}
		if t := el.Find("pmTitle"); t != nil {
			d.TechName = xmldoc.Norm(t.TextContent())
		}
	case "dml":
		d.Kind = KindDML
	}
	return d, nil
}

// FormatDMC renders a dmCode element as DMC string.
func FormatDMC(c *xmldoc.Node) string {
	a := c.Attr
	s := "DMC-" + a("modelIdentCode") + "-" + a("systemDiffCode") + "-" + a("systemCode") + "-" +
		a("subSystemCode") + a("subSubSystemCode") + "-" + a("assyCode") + "-" +
		a("disassyCode") + a("disassyCodeVariant") + "-" + a("infoCode") + a("infoCodeVariant") + "-" +
		a("itemLocationCode")
	if lc := a("learnCode"); lc != "" {
		s += "-" + lc + a("learnEventCode")
	}
	return strings.ToUpper(s)
}

func snsKey(c *xmldoc.Node) string {
	a := c.Attr
	return strings.ToUpper(a("modelIdentCode") + "-" + a("systemDiffCode") + "-" + a("systemCode") + "-" +
		a("subSystemCode") + a("subSubSystemCode") + "-" + a("assyCode") + "-" + a("disassyCode") + a("disassyCodeVariant"))
}

// FormatPMC renders a pmCode element.
func FormatPMC(c *xmldoc.Node) string {
	a := c.Attr
	return strings.ToUpper("PMC-" + a("modelIdentCode") + "-" + a("pmIssuer") + "-" + a("pmNumber") + "-" + a("pmVolume"))
}

// RefDMC extracts the DMC of a dmRef element.
func RefDMC(ref *xmldoc.Node) string {
	if c := ref.Find("dmCode"); c != nil {
		return FormatDMC(c)
	}
	return ""
}

// TitleOf returns the normalized title child text.
func TitleOf(n *xmldoc.Node) string {
	if t := n.Child("title"); t != nil {
		return xmldoc.Norm(t.TextContent())
	}
	return ""
}

// Unit elements form the retrieval and rendering units of a data module.
var unitElements = map[string]bool{
	"levelledPara": true, "proceduralStep": true, "isolationStep": true, "isolationProcedureEnd": true,
	"warning": true, "caution": true, "note": true, "figure": true, "table": true,
	"crewDrillStep": true, "commonInfo": true, "preliminaryRqmts": true, "closeRqmts": true,
	"faultDescr": true, "taskDefinition": true, "subTask": true,
}

func IsUnit(n *xmldoc.Node) bool { return unitElements[n.Name] }

// Chunk is a text unit of a data module used for search and RAG.
type Chunk struct {
	Breadcrumb string
	Element    string
	Path       string
	Line       int
	Text       string
}

// Chunks splits a document into text units with hierarchical context, so
// that XML structure becomes usable for retrieval.
func Chunks(d *Doc) []Chunk {
	root := d.Root.Root()
	var out []Chunk
	head := d.Label()
	if d.Title() != "" {
		head += " | " + d.Title()
	}
	if d.Kind == KindDM || d.Kind == KindPM {
		out = append(out, Chunk{Breadcrumb: head, Element: "dmTitle", Path: "/", Line: 1, Text: d.Title()})
	}
	content := root.Child("content")
	if content == nil {
		content = root
	}
	content.Walk(func(n *xmldoc.Node) bool {
		if !IsUnit(n) {
			return true
		}
		text := xmldoc.Norm(n.TextExcluding(IsUnit))
		if n.Name == "figure" {
			for _, g := range n.FindAll("graphic") {
				if id := g.Attr("infoEntityIdent"); id != "" {
					text += " [" + id + "]"
				}
			}
			text = strings.TrimSpace(text)
		}
		if text == "" {
			return true
		}
		var crumbs []string
		for p := n.Parent; p != nil; p = p.Parent {
			if IsUnit(p) {
				if t := TitleOf(p); t != "" {
					crumbs = append([]string{t}, crumbs...)
				}
			}
		}
		bc := head
		if len(crumbs) > 0 {
			bc += " > " + strings.Join(crumbs, " > ")
		}
		if len(text) > 4000 {
			text = text[:4000]
		}
		out = append(out, Chunk{Breadcrumb: bc, Element: n.Name, Path: n.Path(), Line: n.Line, Text: text})
		return true
	})
	return out
}

// Render produces a readable, line-referenced text form of a data module for
// language models. Every line starts with [Lnn] referring to the XML source line.
func Render(d *Doc, maxChars int) string {
	head := "Datenmodul " + d.Label() + "\nTitel: " + d.Title() + "\n"
	if d.Lang != "" {
		head += "Sprache: " + d.Lang + "\n"
	}
	root := d.Root.Root()
	content := root.Child("content")
	if content == nil {
		content = root
	}
	return head + RenderNode(content, maxChars)
}

// RenderNode renders an element subtree (see Render). Referenced warnings
// and cautions (warningRefs/cautionRefs) are resolved and shown before the
// referencing step; internal references show their target.
func RenderNode(content *xmldoc.Node, maxChars int) string {
	var b strings.Builder
	ids := xmldoc.IDIndex(content.Root())
	isList := func(x *xmldoc.Node) bool {
		return x.Name == "randomList" || x.Name == "sequentialList" || x.Name == "definitionList"
	}
	refText := func(x *xmldoc.Node) (string, bool) {
		switch x.Name {
		case "internalRef":
			id := x.Attr("internalRefId")
			if t := ids[id]; t != nil {
				kind := map[string]string{"figure": "Abb.", "table": "Tab.", "levelledPara": "Abschnitt", "proceduralStep": "Schritt",
					"supplyDescr": "Betriebsstoff", "spareDescr": "Ersatzteil", "supportEquipDescr": "Hilfsmittel"}[t.Name]
				if title := TitleOf(t); title != "" {
					return " [→ " + strings.TrimSpace(kind+" „"+title+"“") + "] ", true
				}
				if nm := t.Child("name"); nm != nil {
					return " [→ " + strings.TrimSpace(kind+" „"+xmldoc.Norm(nm.TextContent())+"“") + "] ", true
				}
			}
			return " [→ " + id + "] ", true
		case "dmRef":
			if dmc := RefDMC(x); dmc != "" {
				return " [→ " + dmc + "] ", true
			}
			if href := x.Attr("scheckHref"); href != "" {
				return " [→ " + href + "] ", true
			}
		}
		return "", false
	}
	var rec func(n *xmldoc.Node, depth int)
	emit := func(n *xmldoc.Node, depth int, prefix, text string) {
		if text == "" {
			return
		}
		b.WriteString("[L")
		b.WriteString(itoa(n.Line))
		b.WriteString("] ")
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString(prefix)
		b.WriteString(text)
		b.WriteString("\n")
	}
	rec = func(n *xmldoc.Node, depth int) {
		if maxChars > 0 && b.Len() > maxChars {
			return
		}
		// resolved warning/caution references
		for _, ra := range []struct{ attr, label string }{{"warningRefs", "WARNUNG: "}, {"cautionRefs", "VORSICHT: "}} {
			for _, id := range strings.Fields(n.Attr(ra.attr)) {
				if t := ids[id]; t != nil {
					emit(n, depth, ra.label, xmldoc.Norm(t.TextContent()))
				}
			}
		}
		switch n.Name {
		case "warningsAndCautions":
			return // shown where they are referenced
		case "title":
			emit(n, depth, "## ", xmldoc.Norm(n.TextWith(nil, refText)))
			return
		case "para", "notePara", "warningAndCautionPara", "listItemTerm", "listItemDefinition", "entry":
			prefix := ""
			if p := n.Parent; p != nil {
				switch p.Name {
				case "warning":
					prefix = "WARNUNG: "
				case "caution":
					prefix = "VORSICHT: "
				case "note":
					prefix = "HINWEIS: "
				case "listItem":
					prefix = "- "
				case "proceduralStep", "isolationStep":
					prefix = "SCHRITT: "
				}
			}
			if n.Name == "entry" {
				prefix = "| "
			}
			emit(n, depth, prefix, xmldoc.Norm(n.TextWith(isList, refText)))
			for _, c := range n.Children() {
				if isList(c) {
					rec(c, depth+1)
				}
			}
			return
		case "graphic":
			emit(n, depth, "[Grafik] ", n.Attr("infoEntityIdent"))
			return
		case "dmRef":
			emit(n, depth, "[Verweis] ", firstNonEmpty(RefDMC(n), n.Attr("scheckHref")))
			return
		case "name":
			if p := n.Parent; p != nil && (p.Name == "supplyDescr" || p.Name == "spareDescr" || p.Name == "supportEquipDescr") {
				emit(n, depth, "- ", xmldoc.Norm(p.TextContent()))
				return
			}
		}
		for _, c := range n.Children() {
			nd := depth
			if IsUnit(c) {
				nd = depth + 1
			}
			rec(c, nd)
		}
	}
	rec(content, 0)
	s := b.String()
	if maxChars > 0 && len(s) > maxChars {
		s = s[:maxChars] + "\n[... gekürzt]"
	}
	return s
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	p := len(buf)
	for i > 0 {
		p--
		buf[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		buf[p] = '-'
	}
	return string(buf[p:])
}

var lineRefRe = regexp.MustCompile(`^\[L(\d+)\]`)

// ParseLineRef extracts a line number from a "[L12]" reference.
func ParseLineRef(s string) int {
	m := lineRefRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	return n
}
