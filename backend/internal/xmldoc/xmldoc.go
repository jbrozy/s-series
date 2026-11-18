// Package xmldoc implements a small DOM for S-Series XML files with line
// numbers, stable element paths and an XPath 1.0 navigator.
package xmldoc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Kind int

const (
	Document Kind = iota
	Element
	Text
	Comment
)

type Attr struct {
	Prefix string
	Name   string
	Value  string
}

type Node struct {
	Kind       Kind
	Prefix     string
	Name       string
	Attrs      []Attr
	Data       string
	Line       int
	Parent     *Node
	FirstChild *Node
	LastChild  *Node
	Prev       *Node
	Next       *Node
}

func (n *Node) appendChild(c *Node) {
	c.Parent = n
	if n.LastChild == nil {
		n.FirstChild, n.LastChild = c, c
		return
	}
	c.Prev = n.LastChild
	n.LastChild.Next = c
	n.LastChild = c
}

// Attr returns the value of the attribute with the given local name.
func (n *Node) Attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value
		}
	}
	return ""
}

func (n *Node) HasAttr(name string) bool {
	for _, a := range n.Attrs {
		if a.Name == name {
			return true
		}
	}
	return false
}

// SetAttr sets (or adds) an attribute.
func (n *Node) SetAttr(name, value string) {
	for i, a := range n.Attrs {
		if a.Name == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, Attr{Name: name, Value: value})
}

// Children returns the element children.
func (n *Node) Children() []*Node {
	var out []*Node
	for c := n.FirstChild; c != nil; c = c.Next {
		if c.Kind == Element {
			out = append(out, c)
		}
	}
	return out
}

// Child returns the first element child with the given name.
func (n *Node) Child(name string) *Node {
	for c := n.FirstChild; c != nil; c = c.Next {
		if c.Kind == Element && c.Name == name {
			return c
		}
	}
	return nil
}

// ChildPath follows a chain of child names.
func (n *Node) ChildPath(names ...string) *Node {
	cur := n
	for _, nm := range names {
		if cur == nil {
			return nil
		}
		cur = cur.Child(nm)
	}
	return cur
}

// Walk visits all element descendants (including n if element) depth-first.
func (n *Node) Walk(fn func(*Node) bool) {
	if n.Kind == Element || n.Kind == Document {
		if n.Kind == Element && !fn(n) {
			return
		}
		for c := n.FirstChild; c != nil; c = c.Next {
			c.Walk(fn)
		}
	}
}

// FindAll returns all descendant elements with one of the given names.
func (n *Node) FindAll(names ...string) []*Node {
	set := map[string]bool{}
	for _, nm := range names {
		set[nm] = true
	}
	var out []*Node
	n.Walk(func(e *Node) bool {
		if set[e.Name] {
			out = append(out, e)
		}
		return true
	})
	return out
}

// Find returns the first descendant element with the given name.
func (n *Node) Find(name string) *Node {
	var found *Node
	n.Walk(func(e *Node) bool {
		if found != nil {
			return false
		}
		if e.Name == name {
			found = e
			return false
		}
		return true
	})
	return found
}

// Ancestor returns the closest ancestor with one of the names.
func (n *Node) Ancestor(names ...string) *Node {
	for p := n.Parent; p != nil; p = p.Parent {
		for _, nm := range names {
			if p.Name == nm {
				return p
			}
		}
	}
	return nil
}

// Root returns the document element.
func (n *Node) Root() *Node {
	cur := n
	for cur.Parent != nil {
		cur = cur.Parent
	}
	if cur.Kind == Document {
		for c := cur.FirstChild; c != nil; c = c.Next {
			if c.Kind == Element {
				return c
			}
		}
	}
	return cur
}

// TextContent returns all descendant text.
func (n *Node) TextContent() string {
	var b strings.Builder
	var rec func(*Node)
	rec = func(x *Node) {
		if x.Kind == Text {
			b.WriteString(x.Data)
			return
		}
		for c := x.FirstChild; c != nil; c = c.Next {
			rec(c)
		}
	}
	rec(n)
	return b.String()
}

// TextExcluding returns descendant text but skips subtrees for which skip returns true.
func (n *Node) TextExcluding(skip func(*Node) bool) string {
	var b strings.Builder
	var rec func(*Node)
	rec = func(x *Node) {
		if x.Kind == Text {
			b.WriteString(x.Data)
			return
		}
		if x != n && x.Kind == Element && skip(x) {
			b.WriteString(" ")
			return
		}
		for c := x.FirstChild; c != nil; c = c.Next {
			rec(c)
			if c.Kind == Element && blockElements[c.Name] {
				b.WriteString(" ")
			}
		}
	}
	rec(n)
	return b.String()
}

var blockElements = map[string]bool{
	"para": true, "title": true, "listItem": true, "warningAndCautionPara": true,
	"notePara": true, "entry": true, "listItemTerm": true, "listItemDefinition": true,
}

var wsRe = regexp.MustCompile(`\s+`)

// Norm collapses whitespace and trims.
func Norm(s string) string {
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
}

// Path returns an XPath-like location of the element, e.g.
// /dmodule/content/description/levelledPara[2]/title
func (n *Node) Path() string {
	if n == nil || n.Kind == Document {
		return "/"
	}
	var parts []string
	for cur := n; cur != nil && cur.Kind == Element; cur = cur.Parent {
		seg := cur.qname()
		if cur.Parent != nil {
			idx, total := 0, 0
			for s := cur.Parent.FirstChild; s != nil; s = s.Next {
				if s.Kind == Element && s.Name == cur.Name && s.Prefix == cur.Prefix {
					total++
					if s == cur {
						idx = total
					}
				}
			}
			if total > 1 {
				seg += "[" + strconv.Itoa(idx) + "]"
			}
		}
		parts = append(parts, seg)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return "/" + strings.Join(parts, "/")
}

func (n *Node) qname() string {
	if n.Prefix != "" {
		return n.Prefix + ":" + n.Name
	}
	return n.Name
}

// OuterXML serializes the node.
func (n *Node) OuterXML() string {
	var b strings.Builder
	n.writeXML(&b)
	return b.String()
}

// StartTag renders only the start tag of an element.
func (n *Node) StartTag() string {
	var b strings.Builder
	b.WriteString("<" + n.qname())
	for _, a := range n.Attrs {
		name := a.Name
		if a.Prefix != "" {
			name = a.Prefix + ":" + a.Name
		}
		b.WriteString(" " + name + "=\"" + escapeAttr(a.Value) + "\"")
	}
	b.WriteString(">")
	return b.String()
}

func (n *Node) EndTag() string { return "</" + n.qname() + ">" }

func (n *Node) writeXML(b *strings.Builder) {
	switch n.Kind {
	case Text:
		xml.EscapeText(writerFunc(func(p []byte) (int, error) { b.Write(p); return len(p), nil }), []byte(n.Data))
	case Comment:
		b.WriteString("<!--" + n.Data + "-->")
	case Document:
		for c := n.FirstChild; c != nil; c = c.Next {
			c.writeXML(b)
		}
	case Element:
		if n.FirstChild == nil {
			s := n.StartTag()
			b.WriteString(s[:len(s)-1] + "/>")
			return
		}
		b.WriteString(n.StartTag())
		for c := n.FirstChild; c != nil; c = c.Next {
			c.writeXML(b)
		}
		b.WriteString(n.EndTag())
	}
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func escapeAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", "\"", "&quot;")
	return r.Replace(s)
}

var (
	entityDeclRe = regexp.MustCompile(`<!ENTITY\s+([A-Za-z_][\w.\-]*)\s+(?:"([^"]*)"|'([^']*)')\s*>`)
	entityRefRe  = regexp.MustCompile(`&([A-Za-z_][\w.\-]*);`)
	xmlDeclEnc   = regexp.MustCompile(`(?i)^\s*<\?xml[^>]*encoding=["']([^"']+)["']`)
)

// Parse parses an XML document. Unknown entity references (frequent in
// S1000D exports with external DTD subsets) are replaced by their name in
// brackets instead of failing the parse.
func Parse(data []byte) (*Node, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if m := xmlDeclEnc.FindSubmatch(data[:min(len(data), 200)]); m != nil {
		enc := strings.ToLower(string(m[1]))
		if enc == "iso-8859-1" || enc == "latin1" || enc == "windows-1252" || enc == "cp1252" {
			data = latin1ToUTF8(data)
			data = xmlDeclEnc.ReplaceAllFunc(data, func(b []byte) []byte {
				return bytes.Replace(b, m[1], []byte("UTF-8"), 1)
			})
		}
	}
	entities := map[string]string{}
	for k, v := range xml.HTMLEntity {
		entities[k] = v
	}
	for _, m := range entityDeclRe.FindAllSubmatch(data, -1) {
		val := string(m[2])
		if val == "" {
			val = string(m[3])
		}
		entities[string(m[1])] = val
	}
	data = entityRefRe.ReplaceAllFunc(data, func(b []byte) []byte {
		name := string(b[1 : len(b)-1])
		switch name {
		case "amp", "lt", "gt", "quot", "apos":
			return b
		}
		if _, ok := entities[name]; ok {
			return b
		}
		return []byte("[" + name + "]")
	})

	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	dec.Entity = entities
	dec.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	doc := &Node{Kind: Document}
	cur := doc
	for {
		line, _ := dec.InputPos()
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML-Fehler: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el := &Node{Kind: Element, Prefix: t.Name.Space, Name: t.Name.Local, Line: line}
			for _, a := range t.Attr {
				el.Attrs = append(el.Attrs, Attr{Prefix: a.Name.Space, Name: a.Name.Local, Value: a.Value})
			}
			cur.appendChild(el)
			cur = el
		case xml.EndElement:
			if cur.Parent == nil {
				return nil, fmt.Errorf("XML-Fehler: unerwartetes Ende-Tag </%s>", t.Name.Local)
			}
			cur = cur.Parent
		case xml.CharData:
			s := string(t)
			if cur == doc && strings.TrimSpace(s) == "" {
				continue
			}
			if cur.LastChild != nil && cur.LastChild.Kind == Text {
				cur.LastChild.Data += s
			} else {
				cur.appendChild(&Node{Kind: Text, Data: s, Line: line})
			}
		case xml.Comment:
			cur.appendChild(&Node{Kind: Comment, Data: string(t), Line: line})
		}
	}
	if doc.Root() == doc {
		return nil, fmt.Errorf("XML-Fehler: kein Wurzelelement")
	}
	return doc, nil
}

func latin1ToUTF8(b []byte) []byte {
	if utf8.Valid(b) {
		return b
	}
	out := make([]byte, 0, len(b)+len(b)/8)
	for _, c := range b {
		out = utf8.AppendRune(out, rune(c))
	}
	return out
}

// Clone deep-copies a subtree (without parent/sibling links).
func (n *Node) Clone() *Node {
	c := &Node{Kind: n.Kind, Prefix: n.Prefix, Name: n.Name, Data: n.Data, Line: n.Line}
	c.Attrs = append([]Attr(nil), n.Attrs...)
	for ch := n.FirstChild; ch != nil; ch = ch.Next {
		c.appendChild(ch.Clone())
	}
	return c
}

// InsertAfter inserts c after ref (same parent).
func (ref *Node) InsertAfter(c *Node) {
	p := ref.Parent
	c.Parent = p
	c.Prev = ref
	c.Next = ref.Next
	if ref.Next != nil {
		ref.Next.Prev = c
	} else if p != nil {
		p.LastChild = c
	}
	ref.Next = c
}

// Remove detaches the node from its parent.
func (n *Node) Remove() {
	p := n.Parent
	if n.Prev != nil {
		n.Prev.Next = n.Next
	} else if p != nil {
		p.FirstChild = n.Next
	}
	if n.Next != nil {
		n.Next.Prev = n.Prev
	} else if p != nil {
		p.LastChild = n.Prev
	}
	n.Parent, n.Prev, n.Next = nil, nil, nil
}

// FirstText returns the first text node descendant.
func (n *Node) FirstText() *Node {
	if n.Kind == Text {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.Next {
		if t := c.FirstText(); t != nil {
			return t
		}
	}
	return nil
}

// PrettyXML serializes the subtree and puts element-only children on their
// own lines (mixed content stays untouched), so that line numbers are
// meaningful after re-parsing.
func (n *Node) PrettyXML() string {
	var b strings.Builder
	n.writePretty(&b)
	return b.String()
}

func (n *Node) elementOnly() bool {
	for c := n.FirstChild; c != nil; c = c.Next {
		if c.Kind == Text && strings.TrimSpace(c.Data) != "" {
			return false
		}
	}
	return true
}

func (n *Node) writePretty(b *strings.Builder) {
	if n.Kind != Element {
		n.writeXML(b)
		return
	}
	if n.FirstChild == nil || !n.elementOnly() {
		n.writeXML(b)
		return
	}
	b.WriteString(n.StartTag())
	for c := n.FirstChild; c != nil; c = c.Next {
		if c.Kind == Text {
			continue
		}
		b.WriteString("\n")
		c.writePretty(b)
	}
	b.WriteString("\n" + n.EndTag())
}

// PrependChild inserts c as first child of n.
func (n *Node) PrependChild(c *Node) {
	c.Parent = n
	c.Prev = nil
	c.Next = n.FirstChild
	if n.FirstChild != nil {
		n.FirstChild.Prev = c
	} else {
		n.LastChild = c
	}
	n.FirstChild = c
}

// RemoveAttr deletes an attribute.
func (n *Node) RemoveAttr(name string) {
	out := n.Attrs[:0]
	for _, a := range n.Attrs {
		if a.Name != name {
			out = append(out, a)
		}
	}
	n.Attrs = out
}

// TextWith returns descendant text; subtrees for which skip returns true
// are left out, elements for which repl returns ok are replaced by the
// returned string.
func (n *Node) TextWith(skip func(*Node) bool, repl func(*Node) (string, bool)) string {
	var b strings.Builder
	var rec func(*Node)
	rec = func(x *Node) {
		if x.Kind == Text {
			b.WriteString(x.Data)
			return
		}
		if x != n && x.Kind == Element {
			if skip != nil && skip(x) {
				b.WriteString(" ")
				return
			}
			if repl != nil {
				if s, ok := repl(x); ok {
					b.WriteString(s)
					return
				}
			}
		}
		for c := x.FirstChild; c != nil; c = c.Next {
			rec(c)
			if c.Kind == Element && blockElements[c.Name] {
				b.WriteString(" ")
			}
		}
	}
	rec(n)
	return b.String()
}

// IDIndex maps all id attributes below n to their elements.
func IDIndex(n *Node) map[string]*Node {
	m := map[string]*Node{}
	n.Walk(func(e *Node) bool {
		if id := e.Attr("id"); id != "" {
			m[id] = e
		}
		return true
	})
	return m
}
