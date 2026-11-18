package xmldoc

import (
	"fmt"
	"strings"
	"sync"

	"github.com/antchfx/xpath"
)

// Nav implements xpath.NodeNavigator for Node trees.
type Nav struct {
	root *Node
	cur  *Node
	attr int
}

func NewNav(n *Node) *Nav {
	root := n
	for root.Parent != nil {
		root = root.Parent
	}
	return &Nav{root: root, cur: n, attr: -1}
}

func (x *Nav) Current() *Node { return x.cur }

// AttrIndex is >=0 if the navigator points to an attribute.
func (x *Nav) AttrIndex() int { return x.attr }

func (x *Nav) NodeType() xpath.NodeType {
	if x.attr >= 0 {
		return xpath.AttributeNode
	}
	switch x.cur.Kind {
	case Document:
		return xpath.RootNode
	case Text:
		return xpath.TextNode
	case Comment:
		return xpath.CommentNode
	}
	return xpath.ElementNode
}

func (x *Nav) LocalName() string {
	if x.attr >= 0 {
		return x.cur.Attrs[x.attr].Name
	}
	return x.cur.Name
}

func (x *Nav) Prefix() string {
	if x.attr >= 0 {
		return x.cur.Attrs[x.attr].Prefix
	}
	return x.cur.Prefix
}

func (x *Nav) Value() string {
	if x.attr >= 0 {
		return x.cur.Attrs[x.attr].Value
	}
	switch x.cur.Kind {
	case Text, Comment:
		return x.cur.Data
	}
	return x.cur.TextContent()
}

func (x *Nav) Copy() xpath.NodeNavigator { c := *x; return &c }

func (x *Nav) MoveToRoot() { x.cur, x.attr = x.root, -1 }

func (x *Nav) MoveToParent() bool {
	if x.attr >= 0 {
		x.attr = -1
		return true
	}
	if x.cur.Parent == nil {
		return false
	}
	x.cur = x.cur.Parent
	return true
}

func (x *Nav) MoveToNextAttribute() bool {
	if x.cur.Kind != Element {
		return false
	}
	for i := x.attr + 1; i < len(x.cur.Attrs); i++ {
		a := x.cur.Attrs[i]
		if a.Prefix == "xmlns" || (a.Prefix == "" && a.Name == "xmlns") {
			continue
		}
		x.attr = i
		return true
	}
	return false
}

func (x *Nav) MoveToChild() bool {
	if x.attr >= 0 {
		return false
	}
	if x.cur.FirstChild == nil {
		return false
	}
	x.cur = x.cur.FirstChild
	return true
}

func (x *Nav) MoveToFirst() bool {
	if x.attr >= 0 || x.cur.Parent == nil {
		return false
	}
	x.cur = x.cur.Parent.FirstChild
	return true
}

func (x *Nav) MoveToNext() bool {
	if x.attr >= 0 || x.cur.Next == nil {
		return false
	}
	x.cur = x.cur.Next
	return true
}

func (x *Nav) MoveToPrevious() bool {
	if x.attr >= 0 || x.cur.Prev == nil {
		return false
	}
	x.cur = x.cur.Prev
	return true
}

func (x *Nav) MoveTo(other xpath.NodeNavigator) bool {
	o, ok := other.(*Nav)
	if !ok || o.root != x.root {
		return false
	}
	x.cur, x.attr = o.cur, o.attr
	return true
}

// Match is one XPath result.
type Match struct {
	Node     *Node // element (or owner element for attributes / text)
	AttrName string
	Value    string
}

var exprCache sync.Map

// Compile compiles (and caches) an XPath expression.
func Compile(expr string) (*xpath.Expr, error) {
	if v, ok := exprCache.Load(expr); ok {
		return v.(*xpath.Expr), nil
	}
	e, err := compileSafe(expr)
	if err != nil {
		return nil, err
	}
	exprCache.Store(expr, e)
	return e, nil
}

func compileSafe(expr string) (e *xpath.Expr, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ungültiger XPath-Ausdruck: %v", r)
		}
	}()
	return xpath.Compile(strings.TrimSpace(expr))
}

// Select evaluates an XPath expression relative to n.
// Non-node-set results are returned as a single Match with Value set
// (booleans as "true"/"false"); the second return reports whether a
// boolean expression evaluated to true or a node set was non-empty.
func Select(n *Node, expr string) (matches []Match, truthy bool, err error) {
	e, err := Compile(expr)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("XPath-Auswertung fehlgeschlagen: %v", r)
		}
	}()
	res := e.Evaluate(NewNav(n))
	switch v := res.(type) {
	case *xpath.NodeIterator:
		for v.MoveNext() {
			nav := v.Current().(*Nav)
			m := Match{Node: nav.cur, Value: nav.Value()}
			if nav.attr >= 0 {
				m.AttrName = nav.cur.Attrs[nav.attr].Name
			}
			if m.Node.Kind == Text && m.Node.Parent != nil {
				m.Node = m.Node.Parent
			}
			matches = append(matches, m)
		}
		return matches, len(matches) > 0, nil
	case bool:
		return []Match{{Node: n, Value: fmt.Sprint(v)}}, v, nil
	case float64:
		return []Match{{Node: n, Value: fmt.Sprint(v)}}, v != 0, nil
	case string:
		return []Match{{Node: n, Value: v}}, v != "", nil
	}
	return nil, false, nil
}
