// Package graph builds a knowledge graph of an S-Series project: the
// product breakdown (SNS), data and publication modules, graphics, parts and
// acronyms, connected by typed relations.
package graph

import (
	"sort"
	"strings"

	"scheck/internal/analysis"
	"scheck/internal/s1000d"
	"scheck/internal/xmldoc"
)

// Node types.
const (
	TModel   = "model"
	TSNS     = "sns"
	TDM      = "dm"
	TPM      = "pm"
	TICN     = "icn"
	TPart    = "part"
	TAcronym = "acronym"
)

// Edge types (direction: Src -> Dst).
const (
	EPartOf       = "part_of"       // dm/sns -> sns/model
	EReferences   = "references"    // dm -> dm
	EContains     = "contains"      // pm -> dm/pm
	EUsesGraphic  = "uses_graphic"  // dm -> icn
	ERequiresPart = "requires_part" // dm (procedure requirements) -> part
	EListsPart    = "lists_part"    // dm (illustrated parts data) -> part
	EDefines      = "defines"       // dm -> acronym
)

type Node struct {
	Key    string         `json:"key"`
	Type   string         `json:"type"`
	Label  string         `json:"label"`
	FileID string         `json:"fileId,omitempty"`
	Props  map[string]any `json:"props"`
}

type Edge struct {
	Src    string `json:"src"`
	Dst    string `json:"dst"`
	Type   string `json:"type"`
	Weight int    `json:"weight"`
}

type Graph struct {
	nodes map[string]*Node
	order []string
	edges map[[3]string]*Edge
}

func newGraph() *Graph {
	return &Graph{nodes: map[string]*Node{}, edges: map[[3]string]*Edge{}}
}

func (g *Graph) node(key, typ, label string) *Node {
	if n, ok := g.nodes[key]; ok {
		return n
	}
	n := &Node{Key: key, Type: typ, Label: label, Props: map[string]any{}}
	g.nodes[key] = n
	g.order = append(g.order, key)
	return n
}

func (g *Graph) edge(src, dst, typ string) {
	if src == dst {
		return
	}
	k := [3]string{src, dst, typ}
	if e, ok := g.edges[k]; ok {
		e.Weight++
		return
	}
	g.edges[k] = &Edge{Src: src, Dst: dst, Type: typ, Weight: 1}
}

// Nodes returns all nodes in insertion order.
func (g *Graph) Nodes() []*Node {
	out := make([]*Node, 0, len(g.order))
	for _, k := range g.order {
		out = append(out, g.nodes[k])
	}
	return out
}

// Edges returns all edges in a stable order.
func (g *Graph) Edges() []*Edge {
	out := make([]*Edge, 0, len(g.edges))
	for _, e := range g.edges {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Src != b.Src {
			return a.Src < b.Src
		}
		if a.Dst != b.Dst {
			return a.Dst < b.Dst
		}
		return a.Type < b.Type
	})
	return out
}

// Build derives the knowledge graph from a parsed snapshot. icnFiles maps
// ICN keys (upper case, without extension) to file ids.
func Build(c *analysis.Corpus, icnFiles map[string]string) *Graph {
	g := newGraph()
	dmKey := map[string]string{}   // DMC -> node key of the first module with that code
	fileKey := map[string]string{} // file id -> node key

	// 1) modules and product breakdown
	for _, d := range c.Docs {
		switch d.Kind {
		case s1000d.KindDM, s1000d.KindBREX:
			if d.DMC == "" {
				continue
			}
			key := "dm:" + d.DMC
			if _, dup := g.nodes[key]; dup {
				key += "@" + d.Lang + "#" + d.Issue // further languages/issues of the same module
			} else {
				dmKey[d.DMC] = key
			}
			n := g.node(key, TDM, d.Title())
			n.FileID = d.FileID
			fileKey[d.FileID] = key
			n.Props["dmc"] = d.DMC
			n.Props["title"] = d.Title()
			n.Props["infoCode"] = d.InfoCode
			n.Props["infoName"] = d.InfoName
			n.Props["issue"] = d.Issue
			n.Props["lang"] = d.Lang
			n.Props["content"] = d.Content
			n.Props["path"] = d.Path
			if d.Kind == s1000d.KindBREX {
				n.Props["brex"] = true
			}
			if parent := g.sns(d); parent != "" {
				g.edge(key, parent, EPartOf)
			}
		case s1000d.KindPM:
			if d.DMC == "" {
				continue
			}
			n := g.node("pm:"+d.DMC, TPM, d.TechName)
			n.FileID = d.FileID
			n.Props["pmc"] = d.DMC
			n.Props["title"] = d.TechName
			n.Props["path"] = d.Path
		}
	}
	g.nameAssemblies()

	// 2) relations found inside the modules
	for _, d := range c.Docs {
		var src string
		switch d.Kind {
		case s1000d.KindDM, s1000d.KindBREX:
			src = fileKey[d.FileID]
		case s1000d.KindPM:
			src = "pm:" + d.DMC
		}
		if src == "" {
			continue
		}
		root := d.Root.Root()
		root.Walk(func(n *xmldoc.Node) bool {
			switch n.Name {
			case "identAndStatusSection":
				return d.Kind == s1000d.KindPM // pm titles only; skip DM status refs (BREX etc.)
			case "dmRef":
				target := s1000d.RefDMC(n)
				if target == "" {
					return false
				}
				tk, ok := dmKey[target]
				if !ok {
					tk = "dm:" + target
					m := g.node(tk, TDM, target)
					m.Props["dmc"] = target
					m.Props["missing"] = true
				}
				if d.Kind == s1000d.KindPM {
					g.edge(src, tk, EContains)
				} else {
					g.edge(src, tk, EReferences)
				}
				return false
			case "pmRef":
				if pc := n.Find("pmCode"); pc != nil && d.Kind == s1000d.KindPM {
					t := s1000d.FormatPMC(pc)
					m := g.node("pm:"+t, TPM, t)
					m.Props["pmc"] = t
					if m.FileID == "" {
						m.Props["missing"] = true
					}
					g.edge(src, "pm:"+t, EContains)
				}
				return false
			case "acronym":
				term, def := n.Child("acronymTerm"), n.Child("acronymDefinition")
				if term != nil && def != nil {
					t := xmldoc.Norm(term.TextContent())
					if t != "" {
						a := g.node("acr:"+t, TAcronym, t)
						addUnique(a.Props, "definitions", xmldoc.Norm(def.TextContent()))
						g.edge(src, a.Key, EDefines)
					}
				}
				return false
			case "spareDescr", "supplyDescr", "supportEquipDescr":
				if k := g.partFromDescr(n); k != "" {
					g.edge(src, k, ERequiresPart)
				}
				return false
			case "partRef":
				if k := g.partFromRef(n); k != "" {
					g.edge(src, k, EListsPart)
				}
				return true
			}
			if icn := n.Attr("infoEntityIdent"); icn != "" && d.Kind != s1000d.KindPM {
				k := strings.ToUpper(icn)
				in := g.node("icn:"+k, TICN, icn)
				if fid, ok := icnFiles[k]; ok {
					in.FileID = fid
				} else {
					in.Props["missing"] = true
				}
				if fig := n.Ancestor("figure"); fig != nil {
					addUnique(in.Props, "figures", s1000d.TitleOf(fig))
				}
				g.edge(src, in.Key, EUsesGraphic)
			}
			return true
		})
	}
	// graphics in the export that nothing references
	for k, fid := range icnFiles {
		if _, ok := g.nodes["icn:"+k]; !ok {
			n := g.node("icn:"+k, TICN, k)
			n.FileID = fid
			n.Props["unused"] = true
		}
	}
	return g
}

// sns creates the breakdown chain of a module and returns the assembly key.
func (g *Graph) sns(d *s1000d.Doc) string {
	c := d.Code
	if c.MIC == "" {
		return ""
	}
	model := "model:" + c.MIC
	m := g.node(model, TModel, c.MIC)
	m.Props["code"] = c.MIC
	if c.SC == "" {
		return model
	}
	base := c.MIC + "-" + c.SDC
	levels := []struct{ key, label, level string }{
		{base + "-" + c.SC, c.SC, "system"},
		{base + "-" + c.SC + "-" + c.SSC + c.SSSC, c.SC + "-" + c.SSC + c.SSSC, "subsystem"},
		{base + "-" + c.SC + "-" + c.SSC + c.SSSC + "-" + c.Assy, c.SC + "-" + c.SSC + c.SSSC + "-" + c.Assy, "assembly"},
	}
	parent := model
	for _, l := range levels {
		k := "sns:" + l.key
		n := g.node(k, TSNS, l.label)
		n.Props["level"] = l.level
		n.Props["code"] = l.label
		if c.SDC != "" {
			n.Props["sdc"] = c.SDC
		}
		g.edge(k, parent, EPartOf)
		parent = k
	}
	return parent
}

// nameAssemblies labels breakdown nodes with the most frequent techName of
// the modules below them ("29-10-00 Hydraulic pump").
func (g *Graph) nameAssemblies() {
	names := map[string]map[string]int{}
	for _, e := range g.edges {
		if e.Type != EPartOf {
			continue
		}
		n := g.nodes[e.Src]
		if n.Type != TDM {
			continue
		}
		title, _ := n.Props["title"].(string)
		tech := strings.SplitN(title, " - ", 2)[0]
		if tech == "" {
			continue
		}
		if names[e.Dst] == nil {
			names[e.Dst] = map[string]int{}
		}
		names[e.Dst][strings.TrimSpace(tech)]++
	}
	for k, m := range names {
		best, bc := "", 0
		for name, c := range m {
			if c > bc || (c == bc && name < best) {
				best, bc = name, c
			}
		}
		if n := g.nodes[k]; n != nil && best != "" {
			n.Props["name"] = best
			n.Label = n.Props["code"].(string) + " " + best
		}
	}
}

func (g *Graph) partFromDescr(n *xmldoc.Node) string {
	name := ""
	if nm := n.Child("name"); nm != nil {
		name = xmldoc.Norm(nm.TextContent())
	}
	mfc, pn := "", ""
	if id := n.Find("identNumber"); id != nil {
		if m := id.Child("manufacturerCode"); m != nil {
			mfc = xmldoc.Norm(m.TextContent())
		}
		if p := id.Find("partNumber"); p != nil {
			pn = xmldoc.Norm(p.TextContent())
		}
	}
	return g.part(mfc, pn, name, n.Name)
}

func (g *Graph) partFromRef(n *xmldoc.Node) string {
	mfc, pn := n.Attr("manufacturerCodeValue"), n.Attr("partNumberValue")
	name := ""
	if item := n.Ancestor("itemSeqNumber", "catalogSeqNumber"); item != nil {
		if dsc := item.Find("descrForPart"); dsc != nil {
			name = xmldoc.Norm(dsc.TextContent())
		}
	}
	return g.part(mfc, pn, name, "ipd")
}

func (g *Graph) part(mfc, pn, name, role string) string {
	if pn == "" && name == "" {
		return ""
	}
	key := "part:" + strings.ToUpper(mfc+"/"+pn)
	if pn == "" {
		key = "part:name:" + strings.ToLower(name)
	}
	label := name
	if label == "" {
		label = pn
	}
	p := g.node(key, TPart, label)
	if pn != "" {
		p.Props["partNumber"] = pn
	}
	if mfc != "" {
		p.Props["manufacturer"] = mfc
	}
	if name != "" {
		addUnique(p.Props, "names", name)
	}
	roleDE := map[string]string{"spareDescr": "Ersatzteil", "supplyDescr": "Betriebsstoff", "supportEquipDescr": "Hilfsmittel", "ipd": "Katalogteil"}[role]
	addUnique(p.Props, "roles", roleDE)
	return key
}

func addUnique(props map[string]any, key, val string) {
	if val == "" {
		return
	}
	list, _ := props[key].([]string)
	for _, v := range list {
		if v == val {
			return
		}
	}
	if len(list) < 20 {
		props[key] = append(list, val)
	}
}
