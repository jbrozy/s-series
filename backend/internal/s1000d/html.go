package s1000d

// HTML import: published HTML exports (CSDB/IETP output, static HTML
// manuals) are translated into an S1000D-like structure so that all checks,
// the knowledge graph and the LLM review work on them. Line numbers refer
// to the original HTML source.

import (
	"bytes"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"

	"scheck/internal/xmldoc"
)

// SourceHTML marks documents converted from an HTML export.
const SourceHTML = "html"

var htmlExt = map[string]bool{".htm": true, ".html": true, ".xhtml": true}

func IsHTML(p string) bool { return htmlExt[strings.ToLower(filepath.Ext(p))] }

// IsMarkup reports whether a file is parsed as XML or HTML.
func IsMarkup(p string) bool { return IsXML(p) || IsHTML(p) }

// ---------------------------------------------------------------- tolerant HTML tree with line numbers

type hnode struct {
	tag    string // "" for text
	attrs  map[string]string
	text   string
	kids   []*hnode
	parent *hnode
	line   int
}

var voidTags = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}
var skipTags = map[string]bool{"script": true, "style": true, "noscript": true, "template": true, "svg": true, "math": true}
var closesP = map[string]bool{"p": true, "div": true, "ul": true, "ol": true, "table": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "pre": true, "section": true, "article": true, "figure": true, "blockquote": true, "dl": true, "hr": true}

func parseHTMLTree(data []byte) *hnode {
	root := &hnode{tag: "#root", attrs: map[string]string{}, line: 1}
	stack := []*hnode{root}
	cur := func() *hnode { return stack[len(stack)-1] }
	popTo := func(tag string, stopAt map[string]bool) bool {
		for i := len(stack) - 1; i > 0; i-- {
			t := stack[i].tag
			if t == tag {
				stack = stack[:i]
				return true
			}
			if stopAt[t] {
				return false
			}
		}
		return false
	}
	z := html.NewTokenizer(bytes.NewReader(data))
	line := 1
	skip := ""
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break // io.EOF or a tokenizer error: keep what was parsed
		}
		raw := z.Raw()
		start := line
		line += bytes.Count(raw, []byte("\n"))
		tok := z.Token()
		if skip != "" {
			if tt == html.EndTagToken && tok.Data == skip {
				skip = ""
			}
			continue
		}
		switch tt {
		case html.TextToken:
			if strings.TrimSpace(tok.Data) == "" && len(cur().kids) == 0 {
				continue
			}
			n := &hnode{text: tok.Data, parent: cur(), line: start + leadingNewlines(tok.Data)}
			cur().kids = append(cur().kids, n)
		case html.StartTagToken, html.SelfClosingTagToken:
			t := tok.Data
			if skipTags[t] {
				if tt == html.StartTagToken {
					skip = t
				}
				continue
			}
			// implied end tags
			if closesP[t] {
				popTo("p", map[string]bool{"div": true, "td": true, "th": true, "li": true, "body": true})
			}
			switch t {
			case "li":
				popTo("li", map[string]bool{"ul": true, "ol": true})
			case "dt", "dd":
				popTo("dt", map[string]bool{"dl": true})
				popTo("dd", map[string]bool{"dl": true})
			case "tr":
				popTo("tr", map[string]bool{"table": true, "tbody": true, "thead": true, "tfoot": true})
			case "td", "th":
				popTo("td", map[string]bool{"tr": true, "table": true})
				popTo("th", map[string]bool{"tr": true, "table": true})
			case "tbody", "thead", "tfoot":
				popTo("tbody", map[string]bool{"table": true})
				popTo("thead", map[string]bool{"table": true})
				popTo("tfoot", map[string]bool{"table": true})
			case "option":
				popTo("option", map[string]bool{"select": true})
			}
			n := &hnode{tag: t, attrs: map[string]string{}, parent: cur(), line: start}
			for _, a := range tok.Attr {
				n.attrs[strings.ToLower(a.Key)] = a.Val
			}
			cur().kids = append(cur().kids, n)
			if tt == html.StartTagToken && !voidTags[t] {
				stack = append(stack, n)
			}
		case html.EndTagToken:
			popTo(tok.Data, map[string]bool{"table": tok.Data != "table" && tok.Data != "tr" && tok.Data != "td" && tok.Data != "th" && tok.Data != "tbody" && tok.Data != "thead"})
		}
	}
	return root
}

func leadingNewlines(s string) int {
	n := 0
	for _, r := range s {
		if r == '\n' {
			n++
		} else if r != ' ' && r != '\t' && r != '\r' {
			break
		}
	}
	return n
}

func (h *hnode) find(tag string) *hnode {
	for _, k := range h.kids {
		if k.tag == tag {
			return k
		}
		if f := k.find(tag); f != nil {
			return f
		}
	}
	return nil
}

func (h *hnode) findAll(tag string, out *[]*hnode) {
	for _, k := range h.kids {
		if k.tag == tag {
			*out = append(*out, k)
		}
		k.findAll(tag, out)
	}
}

func (h *hnode) textContent() string {
	if h.tag == "" {
		return h.text
	}
	var b strings.Builder
	for _, k := range h.kids {
		if k.tag == "br" {
			b.WriteString(" ")
			continue
		}
		b.WriteString(k.textContent())
		if blockTags[k.tag] {
			b.WriteString(" ")
		}
	}
	return b.String()
}

func (h *hnode) class() string {
	return strings.ToLower(h.attrs["class"] + " " + h.attrs["id"] + " " + h.attrs["role"])
}

var blockTags = map[string]bool{"p": true, "div": true, "ul": true, "ol": true, "li": true, "table": true, "tr": true, "td": true, "th": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "pre": true, "section": true, "article": true, "figure": true,
	"figcaption": true, "blockquote": true, "dl": true, "dt": true, "dd": true, "img": true, "main": true, "header": true, "footer": true,
	"nav": true, "aside": true, "caption": true, "thead": true, "tbody": true, "tfoot": true, "hr": true, "form": true}

// ---------------------------------------------------------------- metadata

var (
	dmcBodyRe  = `([A-Z0-9]{2,14})-([A-Z0-9]{1,4})-([A-Z0-9]{2,3})-([A-Z0-9])([A-Z0-9])-([A-Z0-9]{2,4})-([A-Z0-9]{2})([A-Z0-9]{1,3})-([A-Z0-9]{3})([A-Z0-9])-([A-Z])`
	dmcFileRe  = regexp.MustCompile(`(?i)DMC-` + dmcBodyRe + `(?:[_.\-]|$)`)
	dmcLabelRe = regexp.MustCompile(`(?i)\bDMC\s*[:\-]?\s*(?:DMC-)?` + dmcBodyRe + `\b`)
	issueRe    = regexp.MustCompile(`(?i)_(\d{3})-(\d{2})(?:_|\.|$)`)
	issueTxtRe = regexp.MustCompile(`(?i)\bIssue\s*(?:No\.?|number|nr\.?)?\s*:?\s*(\d{3})\b`)
	langFileRe = regexp.MustCompile(`(?i)_([A-Z]{2})-([A-Z]{2})(?:\.|_|$)`)
	warnPrefix = regexp.MustCompile(`(?i)^\s*(WARNING|WARNUNG|CAUTION|VORSICHT|ACHTUNG|NOTE|HINWEIS)\s*[:!\-–]?\s*`)
)

var dmcAttrNames = []string{"modelIdentCode", "systemDiffCode", "systemCode", "subSystemCode", "subSubSystemCode", "assyCode",
	"disassyCode", "disassyCodeVariant", "infoCode", "infoCodeVariant", "itemLocationCode"}

// ParseDMC splits a data module code ("DMC-..." or without prefix) into
// dmCode attributes. Returns nil if the string is not a DMC.
func ParseDMC(s string) []xmldoc.Attr {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "DMC-")
	m := regexp.MustCompile(`^` + dmcBodyRe).FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	attrs := make([]xmldoc.Attr, 0, len(dmcAttrNames))
	for i, n := range dmcAttrNames {
		attrs = append(attrs, xmldoc.Attr{Name: n, Value: m[i+1]})
	}
	return attrs
}

func findDMC(name, title string, meta map[string]string, body string) string {
	for _, k := range []string{"dmc", "dmcode", "dc.identifier", "identifier", "s1000d.dmc"} {
		if v := meta[k]; v != "" {
			if a := ParseDMC(v); a != nil {
				return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(v)), "DMC-")
			}
		}
	}
	if m := dmcFileRe.FindStringSubmatch(path.Base(name)); m != nil {
		return strings.Join(m[1:4], "-") + "-" + m[4] + m[5] + "-" + m[6] + "-" + m[7] + m[8] + "-" + m[9] + m[10] + "-" + m[11]
	}
	for _, src := range []string{title, body} {
		if m := dmcLabelRe.FindStringSubmatch(src); m != nil {
			return strings.ToUpper(strings.Join(m[1:4], "-") + "-" + m[4] + m[5] + "-" + m[6] + "-" + m[7] + m[8] + "-" + m[9] + m[10] + "-" + m[11])
		}
		if m := dmcFileRe.FindStringSubmatch(src); m != nil {
			return strings.ToUpper(strings.Join(m[1:4], "-") + "-" + m[4] + m[5] + "-" + m[6] + "-" + m[7] + m[8] + "-" + m[9] + m[10] + "-" + m[11])
		}
	}
	return ""
}

// splitTitle turns "Hydraulic pump – Remove procedure | Site" into tech and info name.
func splitTitle(t, dmc string) (string, string) {
	t = xmldoc.Norm(t)
	if dmc != "" {
		t = strings.ReplaceAll(t, "DMC-"+dmc, "")
		t = strings.ReplaceAll(t, dmc, "")
	}
	if i := strings.LastIndex(t, " | "); i > 0 {
		t = t[:i]
	}
	t = strings.Trim(t, " -–:|")
	for _, sep := range []string{" – ", " — ", " - "} {
		if i := strings.Index(t, sep); i > 0 {
			return strings.TrimSpace(t[:i]), strings.TrimSpace(t[i+len(sep):])
		}
	}
	return t, ""
}

// ---------------------------------------------------------------- conversion

type conv struct {
	page       string // path of the page within the export
	procedural bool
	ids        map[string]bool
}

func el(name string, line int, attrs ...xmldoc.Attr) *xmldoc.Node {
	return &xmldoc.Node{Kind: xmldoc.Element, Name: name, Line: line, Attrs: attrs}
}

func txt(s string, line int) *xmldoc.Node {
	return &xmldoc.Node{Kind: xmldoc.Text, Data: s, Line: line}
}

func add(parent *xmldoc.Node, kids ...*xmldoc.Node) *xmldoc.Node {
	for _, k := range kids {
		if k == nil {
			continue
		}
		k.Parent = parent
		if parent.LastChild == nil {
			parent.FirstChild, parent.LastChild = k, k
		} else {
			k.Prev = parent.LastChild
			parent.LastChild.Next = k
			parent.LastChild = k
		}
	}
	return parent
}

func (c *conv) withID(n *xmldoc.Node, h *hnode) *xmldoc.Node {
	if id := h.attrs["id"]; id != "" && !c.ids[id] {
		c.ids[id] = true
		n.Attrs = append(n.Attrs, xmldoc.Attr{Name: "id", Value: id})
	}
	return n
}

// inline converts inline content into text nodes, references and
// collects images found inline.
func (c *conv) inline(h *hnode, out *xmldoc.Node, imgs *[]*hnode, skip func(*hnode) bool) {
	for _, k := range h.kids {
		switch {
		case k.tag == "":
			add(out, txt(k.text, k.line))
		case skip != nil && skip(k):
			continue
		case k.tag == "br":
			add(out, txt(" ", k.line))
		case k.tag == "img":
			*imgs = append(*imgs, k)
		case k.tag == "a" && k.attrs["href"] != "":
			c.inline(k, out, imgs, skip)
			if ref := c.link(k); ref != nil {
				add(out, ref)
			}
		default:
			c.inline(k, out, imgs, skip)
			if blockTags[k.tag] {
				add(out, txt(" ", k.line))
			}
		}
	}
}

// link converts an anchor into internalRef or a dmRef placeholder that is
// resolved once all pages are known (ResolveHTMLLinks).
func (c *conv) link(a *hnode) *xmldoc.Node {
	href := strings.TrimSpace(a.attrs["href"])
	low := strings.ToLower(href)
	switch {
	case strings.HasPrefix(href, "#") && len(href) > 1:
		return el("internalRef", a.line, xmldoc.Attr{Name: "internalRefId", Value: href[1:]})
	case strings.Contains(low, "://"), strings.HasPrefix(low, "mailto:"), strings.HasPrefix(low, "javascript:"), strings.HasPrefix(low, "data:"):
		return nil
	}
	target := href
	if i := strings.IndexAny(target, "#?"); i >= 0 {
		target = target[:i]
	}
	if target == "" || !IsHTML(target) && !IsXML(target) {
		return nil
	}
	resolved := path.Clean(path.Join(path.Dir(c.page), target))
	ref := el("dmRef", a.line, xmldoc.Attr{Name: "scheckHref", Value: resolved})
	if dmc := findDMC(target, "", nil, ""); dmc != "" {
		add(ref, add(el("dmRefIdent", a.line), el("dmCode", a.line, ParseDMC(dmc)...)))
	}
	return ref
}

func (c *conv) para(h *hnode, name string) (*xmldoc.Node, []*hnode) {
	p := c.withID(el(name, h.line), h)
	var imgs []*hnode
	c.inline(h, p, &imgs, func(k *hnode) bool { return k.tag == "ul" || k.tag == "ol" || k.tag == "dl" || k.tag == "table" })
	return p, imgs
}

func (c *conv) figure(h *hnode, imgs []*hnode, caption string) *xmldoc.Node {
	f := el("figure", h.line)
	c.withID(f, h)
	title := caption
	if title == "" && len(imgs) > 0 {
		title = imgs[0].attrs["title"]
		if title == "" {
			title = imgs[0].attrs["alt"]
		}
	}
	add(f, add(el("title", h.line), txt(xmldoc.Norm(title), h.line)))
	for _, im := range imgs {
		src := im.attrs["src"]
		if src == "" || strings.HasPrefix(src, "data:") {
			continue
		}
		if i := strings.IndexAny(src, "?#"); i >= 0 {
			src = src[:i]
		}
		base := path.Base(src)
		if i := strings.LastIndex(base, "."); i > 0 {
			base = base[:i]
		}
		add(f, el("graphic", im.line, xmldoc.Attr{Name: "infoEntityIdent", Value: base}))
	}
	return f
}

// admonition detects warnings, cautions and notes by class or leading keyword.
func admonition(h *hnode) string {
	cl := h.class()
	switch {
	case strings.Contains(cl, "warning") || strings.Contains(cl, "warnung") || strings.Contains(cl, "danger"):
		return "warning"
	case strings.Contains(cl, "caution") || strings.Contains(cl, "vorsicht"):
		return "caution"
	case strings.Contains(cl, "note") || strings.Contains(cl, "hinweis"):
		return "note"
	}
	if h.tag == "p" || h.tag == "div" || h.tag == "li" || h.tag == "td" {
		if m := warnPrefix.FindStringSubmatch(h.textContent()); m != nil {
			switch strings.ToUpper(m[1]) {
			case "WARNING", "WARNUNG":
				return "warning"
			case "CAUTION", "VORSICHT", "ACHTUNG":
				return "caution"
			default:
				return "note"
			}
		}
	}
	return ""
}

func (c *conv) admonitionNode(h *hnode, kind string) *xmldoc.Node {
	w := c.withID(el(kind, h.line), h)
	pname := "warningAndCautionPara"
	if kind == "note" {
		pname = "notePara"
	}
	var paras []*hnode
	for _, k := range h.kids {
		if k.tag == "p" || k.tag == "div" {
			paras = append(paras, k)
		}
	}
	if len(paras) == 0 {
		paras = []*hnode{h}
	}
	for i, p := range paras {
		pn := el(pname, p.line)
		var imgs []*hnode
		c.inline(p, pn, &imgs, nil)
		if i == 0 && pn.FirstChild != nil && pn.FirstChild.Kind == xmldoc.Text {
			pn.FirstChild.Data = warnPrefix.ReplaceAllString(pn.FirstChild.Data, "")
		}
		add(w, pn)
	}
	return w
}

func (c *conv) list(h *hnode, ordered bool) *xmldoc.Node {
	name := "randomList"
	if ordered {
		name = "sequentialList"
	}
	l := c.withID(el(name, h.line), h)
	for _, li := range h.kids {
		if li.tag != "li" {
			continue
		}
		item := c.withID(el("listItem", li.line), li)
		p, _ := c.para(li, "para")
		for _, k := range li.kids {
			switch k.tag {
			case "ul", "ol":
				add(p, c.list(k, k.tag == "ol"))
			}
		}
		add(l, add(item, p))
	}
	return l
}

func (c *conv) steps(h *hnode) []*xmldoc.Node {
	var out []*xmldoc.Node
	for _, li := range h.kids {
		if li.tag != "li" {
			continue
		}
		st := c.withID(el("proceduralStep", li.line), li)
		// admonitions inside the step come first
		for _, k := range li.kids {
			if k.tag != "" {
				if kind := admonition(k); kind != "" && kind != "note" {
					add(st, c.admonitionNode(k, kind))
				}
			}
		}
		// step text without nested lists and without the admonitions above
		p := el("para", li.line)
		var imgs []*hnode
		c.inline(li, p, &imgs, func(k *hnode) bool {
			if k.tag == "ul" || k.tag == "ol" || k.tag == "table" {
				return true
			}
			kind := admonition(k)
			return kind == "warning" || kind == "caution"
		})
		add(st, p)
		for _, k := range li.kids {
			switch k.tag {
			case "ol":
				for _, s := range c.steps(k) {
					add(st, s)
				}
			case "ul":
				add(p, c.list(k, false))
			}
		}
		if len(imgs) > 0 {
			add(st, c.figure(li, imgs, ""))
		}
		out = append(out, st)
	}
	return out
}

func (c *conv) table(h *hnode) *xmldoc.Node {
	t := c.withID(el("table", h.line), h)
	if cap := h.find("caption"); cap != nil {
		add(t, add(el("title", cap.line), txt(xmldoc.Norm(cap.textContent()), cap.line)))
	}
	var rows []*hnode
	h.findAll("tr", &rows)
	cols := 0
	for _, r := range rows {
		n := 0
		for _, k := range r.kids {
			if k.tag == "td" || k.tag == "th" {
				n++
			}
		}
		cols = max(cols, n)
	}
	tg := el("tgroup", h.line, xmldoc.Attr{Name: "cols", Value: itoa(max(1, cols))})
	tb := el("tbody", h.line)
	for _, r := range rows {
		row := el("row", r.line)
		for _, k := range r.kids {
			if k.tag != "td" && k.tag != "th" {
				continue
			}
			e := el("entry", k.line)
			p := el("para", k.line)
			var imgs []*hnode
			c.inline(k, p, &imgs, nil)
			add(row, add(e, p))
		}
		add(tb, row)
	}
	return add(t, add(tg, tb))
}

func (c *conv) dl(h *hnode) *xmldoc.Node {
	d := c.withID(el("definitionList", h.line), h)
	var item *xmldoc.Node
	for _, k := range h.kids {
		switch k.tag {
		case "dt":
			item = el("definitionListItem", k.line)
			add(d, item)
			add(item, add(el("listItemTerm", k.line), txt(xmldoc.Norm(k.textContent()), k.line)))
		case "dd":
			if item == nil {
				item = el("definitionListItem", k.line)
				add(d, item)
			}
			p := el("para", k.line)
			var imgs []*hnode
			c.inline(k, p, &imgs, nil)
			add(item, add(el("listItemDefinition", k.line), p))
		}
	}
	return d
}

var chromeHints = []string{"nav", "menu", "breadcrumb", "footer", "header", "sidebar", "toolbar", "skip", "cookie"}

func isChrome(h *hnode) bool {
	switch h.tag {
	case "nav", "header", "footer", "aside", "form", "button", "select", "input", "head", "title":
		return true
	}
	cl := h.class()
	for _, x := range chromeHints {
		if strings.Contains(cl, x) {
			return true
		}
	}
	return false
}

// flow walks the page in document order and builds sections from headings.
type section struct {
	level int
	node  *xmldoc.Node
}

func (c *conv) body(h *hnode, content *xmldoc.Node, skipTitle string) {
	stack := []section{{0, content}}
	target := func() *xmldoc.Node { return stack[len(stack)-1].node }
	var walk func(n *hnode)
	emitBlock := func(n *xmldoc.Node) { add(target(), n) }
	walk = func(n *hnode) {
		for _, k := range n.kids {
			if k.tag == "" {
				if t := xmldoc.Norm(k.text); t != "" {
					emitBlock(add(el("para", k.line), txt(t, k.line)))
				}
				continue
			}
			if isChrome(k) {
				continue
			}
			if kind := admonition(k); kind != "" {
				emitBlock(c.admonitionNode(k, kind))
				continue
			}
			switch k.tag {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				lvl := int(k.tag[1] - '0')
				title := xmldoc.Norm(k.textContent())
				if lvl == 1 && skipTitle != "" && (title == skipTitle || strings.Contains(skipTitle, title) || strings.Contains(title, skipTitle)) {
					skipTitle = ""
					continue
				}
				for len(stack) > 1 && stack[len(stack)-1].level >= lvl {
					stack = stack[:len(stack)-1]
				}
				lp := c.withID(el("levelledPara", k.line), k)
				add(lp, add(el("title", k.line), txt(title, k.line)))
				emitBlock(lp)
				stack = append(stack, section{lvl, lp})
			case "p", "pre", "blockquote":
				p, imgs := c.para(k, "para")
				if xmldoc.Norm(p.TextContent()) != "" || p.FirstChild != nil && len(imgs) == 0 {
					emitBlock(p)
				}
				if len(imgs) > 0 {
					emitBlock(c.figure(k, imgs, ""))
				}
				for _, sub := range k.kids {
					if sub.tag == "ul" || sub.tag == "ol" {
						emitBlock(add(el("para", sub.line), c.list(sub, sub.tag == "ol")))
					}
				}
			case "ul":
				emitBlock(add(el("para", k.line), c.list(k, false)))
			case "ol":
				if c.procedural {
					for _, s := range c.steps(k) {
						emitBlock(s)
					}
				} else {
					emitBlock(add(el("para", k.line), c.list(k, true)))
				}
			case "table":
				emitBlock(c.table(k))
			case "dl":
				emitBlock(add(el("para", k.line), c.dl(k)))
			case "figure":
				var imgs []*hnode
				k.findAll("img", &imgs)
				cap := ""
				if fc := k.find("figcaption"); fc != nil {
					cap = fc.textContent()
				}
				emitBlock(c.figure(k, imgs, cap))
			case "img":
				emitBlock(c.figure(k, []*hnode{k}, ""))
			case "hr", "br":
			default:
				// containers (div, section, article, main, span …)
				if hasBlockChild(k) {
					walk(k)
				} else if t := xmldoc.Norm(k.textContent()); t != "" || k.find("img") != nil {
					p, imgs := c.para(k, "para")
					if t != "" {
						emitBlock(p)
					}
					if len(imgs) > 0 {
						emitBlock(c.figure(k, imgs, ""))
					}
				}
			}
		}
	}
	walk(h)
}

func hasBlockChild(h *hnode) bool {
	for _, k := range h.kids {
		if blockTags[k.tag] && k.tag != "br" {
			return true
		}
	}
	return false
}

// ParseHTML converts an HTML page into an S1000D-like document.
func ParseHTML(fileID, p string, data []byte) (*Doc, error) {
	if !utf8.Valid(data) {
		out := make([]byte, 0, len(data)+len(data)/8)
		for _, b := range data {
			out = utf8.AppendRune(out, rune(b))
		}
		data = out
	}
	root := parseHTMLTree(data)
	title := ""
	if t := root.find("title"); t != nil {
		title = xmldoc.Norm(t.textContent())
	}
	meta := map[string]string{}
	var metas []*hnode
	root.findAll("meta", &metas)
	for _, m := range metas {
		k := strings.ToLower(m.attrs["name"] + m.attrs["property"])
		if k != "" {
			meta[k] = m.attrs["content"]
		}
	}
	body := root.find("body")
	if body == nil {
		body = root
	}
	main := body
	if m := body.find("main"); m != nil {
		main = m
	} else {
		for _, k := range body.kids {
			if k.tag != "" && (strings.Contains(k.class(), "content") || k.attrs["role"] == "main") && !isChrome(k) {
				main = k
				break
			}
		}
	}
	bodyText := xmldoc.Norm(main.textContent())
	if len(bodyText) > 4000 {
		bodyText = bodyText[:4000]
	}
	dmc := findDMC(p, title, meta, bodyText)
	h1 := ""
	if h := main.find("h1"); h != nil {
		h1 = xmldoc.Norm(h.textContent())
	}
	lang := strings.ToLower(root.find("html").attrOr("lang"))
	if m := langFileRe.FindStringSubmatch(path.Base(p)); m != nil {
		lang = strings.ToLower(m[1]) + "-" + strings.ToUpper(m[2])
	}

	doc := &xmldoc.Node{Kind: xmldoc.Document}
	if dmc == "" {
		// start/contents page: many links to other pages -> publication module
		var links []*hnode
		body.findAll("a", &links)
		c := &conv{page: p, ids: map[string]bool{}}
		var refs []*xmldoc.Node
		for _, a := range links {
			if r := c.link(a); r != nil && r.Name == "dmRef" {
				refs = append(refs, r)
			}
		}
		if len(refs) >= 2 {
			name := strings.ToUpper(strings.TrimSuffix(path.Base(p), path.Ext(p)))
			pmTitle := title
			if pmTitle == "" {
				pmTitle = h1
			}
			pm := el("pm", 1)
			pmc := el("pmCode", 1, xmldoc.Attr{Name: "modelIdentCode", Value: "HTML"}, xmldoc.Attr{Name: "pmIssuer", Value: "00000"},
				xmldoc.Attr{Name: "pmNumber", Value: name}, xmldoc.Attr{Name: "pmVolume", Value: "00"})
			add(pm, add(el("identAndStatusSection", 1), add(el("pmAddress", 1), add(el("pmIdent", 1), pmc),
				add(el("pmAddressItems", 1), add(el("pmTitle", 1), txt(pmTitle, 1))))))
			entry := el("pmEntry", 1)
			add(entry, refs...)
			add(pm, add(el("content", 1), entry))
			add(doc, pm)
			d := &Doc{FileID: fileID, Path: p, Root: doc, Kind: KindPM, DMC: FormatPMC(pmc), TechName: pmTitle, Lang: lang, Source: SourceHTML}
			return d, nil
		}
	}

	tech, info := splitTitle(firstNonEmpty(title, h1), dmc)
	if dmc != "" && tech == "" {
		tech, info = splitTitle(h1, dmc)
	}
	var code []xmldoc.Attr
	if dmc != "" {
		code = ParseDMC(dmc)
	}
	procedural := false
	if len(code) > 0 {
		ic := code[8].Value
		procedural = ic[0] >= '1' && ic[0] <= '8'
	}
	issue, inwork := "", ""
	if m := issueRe.FindStringSubmatch(path.Base(p)); m != nil {
		issue, inwork = m[1], m[2]
	} else if m := issueTxtRe.FindStringSubmatch(bodyText); m != nil {
		issue, inwork = m[1], "00"
	}
	dm := el("dmodule", 1)
	ident := el("dmIdent", 1)
	if code != nil {
		add(ident, el("dmCode", 1, code...))
	}
	if lang != "" {
		parts := strings.SplitN(lang, "-", 2)
		la := []xmldoc.Attr{{Name: "languageIsoCode", Value: parts[0]}}
		if len(parts) == 2 {
			la = append(la, xmldoc.Attr{Name: "countryIsoCode", Value: strings.ToUpper(parts[1])})
		}
		add(ident, el("language", 1, la...))
	}
	if issue != "" {
		add(ident, el("issueInfo", 1, xmldoc.Attr{Name: "issueNumber", Value: issue}, xmldoc.Attr{Name: "inWork", Value: inwork}))
	}
	dmTitle := add(el("dmTitle", 1), add(el("techName", 1), txt(tech, 1)))
	if info != "" {
		add(dmTitle, add(el("infoName", 1), txt(info, 1)))
	}
	add(dm, add(el("identAndStatusSection", 1), add(el("dmAddress", 1), ident, add(el("dmAddressItems", 1), dmTitle))))
	content := el("content", main.line)
	var flow *xmldoc.Node
	if procedural {
		flow = el("mainProcedure", main.line)
		add(content, add(el("procedure", main.line), flow))
	} else {
		flow = el("description", main.line)
		add(content, flow)
	}
	add(dm, content)
	add(doc, dm)
	c := &conv{page: p, procedural: procedural, ids: map[string]bool{}}
	c.body(main, flow, firstNonEmpty(h1, tech))

	d := &Doc{FileID: fileID, Path: p, Root: doc, Kind: KindXML, Lang: lang, TechName: tech, InfoName: info, Source: SourceHTML}
	if dmc != "" {
		d.Kind = KindDM
		dc := dm.Find("dmCode")
		d.DMC = FormatDMC(dc)
		d.InfoCode = dc.Attr("infoCode") + dc.Attr("infoCodeVariant")
		d.SNSKey = snsKey(dc)
		a := func(n string) string { return strings.ToUpper(dc.Attr(n)) }
		d.Code = DMCode{MIC: a("modelIdentCode"), SDC: a("systemDiffCode"), SC: a("systemCode"), SSC: a("subSystemCode"),
			SSSC: a("subSubSystemCode"), Assy: a("assyCode"), DC: a("disassyCode"), DCV: a("disassyCodeVariant"),
			IC: a("infoCode"), ICV: a("infoCodeVariant"), ILC: a("itemLocationCode")}
		if issue != "" {
			d.Issue = issue + "-" + inwork
		}
	}
	if procedural {
		d.Content = "procedure"
	} else {
		d.Content = "description"
	}
	return d, nil
}

func (h *hnode) attrOr(k string) string {
	if h == nil {
		return ""
	}
	return h.attrs[k]
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

// ResolveHTMLLinks turns links between HTML pages into data module
// references once all pages of an export are known. Links to pages that do
// not exist are marked (reported by the reference check).
func ResolveHTMLLinks(docs []*Doc) {
	byPath := map[string]*Doc{}
	for _, d := range docs {
		byPath[strings.ToLower(path.Clean(d.Path))] = d
	}
	for _, d := range docs {
		if d.Source != SourceHTML {
			continue
		}
		for _, r := range d.Root.FindAll("dmRef") {
			href := r.Attr("scheckHref")
			if href == "" || r.Find("dmCode") != nil {
				continue
			}
			t := byPath[strings.ToLower(href)]
			switch {
			case t != nil && t.Kind == KindDM && t.DMC != "":
				add(r, add(el("dmRefIdent", r.Line), el("dmCode", r.Line, ParseDMC(t.DMC)...)))
			case t != nil:
				r.SetAttr("scheckTarget", t.Kind) // existing page without data module code
			default:
				r.SetAttr("scheckMissing", "1")
			}
		}
	}
}

// MergeHTML resolves links between HTML pages and drops HTML pages whose
// data module is also present as XML (exports often ship both).
func MergeHTML(docs []*Doc) []*Doc {
	hasHTML := false
	xmlDMC := map[string]bool{}
	for _, d := range docs {
		if d.Source == SourceHTML {
			hasHTML = true
		} else if d.DMC != "" {
			xmlDMC[d.DMC] = true
		}
	}
	if !hasHTML {
		return docs
	}
	ResolveHTMLLinks(docs)
	out := docs[:0]
	for _, d := range docs {
		if d.Source == SourceHTML && d.DMC != "" && xmlDMC[d.DMC] {
			continue
		}
		out = append(out, d)
	}
	return out
}
