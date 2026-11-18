// Package synth generates synthetic training data for fine-tuning a small
// review model. Real (mostly correct) data modules of a project are
// mutated with typical documentation errors; labels are produced by running
// the deterministic checks on the mutated fragment, so they are consistent
// with the production checks and the LLM review output format.
package synth

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"scheck/internal/analysis"
	"scheck/internal/rules"
	"scheck/internal/s1000d"
	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

type Options struct {
	Count      int     `json:"count"`
	Format     string  `json:"format"` // chat | alpaca
	Input      string  `json:"input"`  // text | xml
	CleanRatio float64 `json:"cleanRatio"`
	Seed       int64   `json:"seed"`
	MaxChars   int     `json:"maxChars"`
	// Focus: semantic (content errors rules cannot find, default),
	// structural (errors the deterministic checks find) or all.
	Focus string `json:"focus"`
}

type Stats struct {
	Written   int            `json:"written"`
	Mutations map[string]int `json:"mutations"`
}

type sample struct {
	mutation string
	fragment string
	injected []analysis.LLMFinding
	related  []string
}

type mutator func(r *rand.Rand, d *s1000d.Doc) (sample, bool)

type legacyMutator func(r *rand.Rand, d *s1000d.Doc, terms []rules.Term) (frag string, injected []analysis.LLMFinding, ok bool)

func wrap(m legacyMutator, terms []rules.Term) mutator {
	return func(r *rand.Rand, d *s1000d.Doc) (sample, bool) {
		frag, inj, ok := m(r, d, terms)
		return sample{fragment: frag, injected: inj}, ok
	}
}

// Generate writes JSONL samples to w.
func Generate(c *analysis.Corpus, rs []rules.Rule, opts Options, w io.Writer) (Stats, error) {
	if opts.Count <= 0 {
		opts.Count = 200
	}
	if opts.Count > 20000 {
		opts.Count = 20000
	}
	if opts.MaxChars <= 0 {
		opts.MaxChars = 3500
	}
	if opts.CleanRatio < 0 || opts.CleanRatio > 0.9 {
		opts.CleanRatio = 0.25
	}
	r := rand.New(rand.NewSource(opts.Seed))
	var dms []*s1000d.Doc
	for _, d := range c.Docs {
		if d.Kind == s1000d.KindDM {
			dms = append(dms, d)
		}
	}
	stats := Stats{Mutations: map[string]int{}}
	if len(dms) == 0 {
		return stats, fmt.Errorf("keine Datenmodule im Snapshot")
	}
	var terms []rules.Term
	var labelRules []rules.Rule
	for _, rl := range rs {
		if !rl.Enabled {
			continue
		}
		switch rl.Type {
		case rules.TypeForbiddenTerms:
			var tc rules.TermsConfig
			if json.Unmarshal(rl.Config, &tc) == nil {
				terms = append(terms, tc.Terms...)
			}
			labelRules = append(labelRules, rl)
		case rules.TypeTitleCase, rules.TypeSentenceLength:
			labelRules = append(labelRules, rl)
		}
	}
	rulesText := rules.Describe(rs)
	gen := &generator{maxChars: opts.MaxChars, corpus: c}
	mutators := map[string]mutator{}
	if opts.Focus != "structural" {
		mutators["step_order"] = gen.stepOrder
		mutators["missing_warning"] = gen.missingWarning
		mutators["value_conflict"] = gen.valueConflict
		mutators["unit_error"] = gen.unitError
		mutators["meaning_inverted"] = gen.meaningFlip
		mutators["procedure_mismatch"] = gen.procedureMismatch
	}
	if opts.Focus == "structural" || opts.Focus == "all" {
		mutators["duplicate_list_item"] = wrap(gen.duplicateListItem, terms)
		mutators["title_typo"] = wrap(gen.titleTypo, terms)
		mutators["text_typo"] = wrap(gen.textTypo, terms)
		mutators["duplicate_sibling_title"] = wrap(gen.duplicateSiblingTitle, terms)
		mutators["duplicate_step"] = wrap(gen.duplicateStep, terms)
		mutators["title_punctuation"] = wrap(gen.titlePunctuation, terms)
		mutators["duplicate_requirement"] = wrap(gen.duplicateRequirement, terms)
		if len(terms) > 0 {
			mutators["forbidden_term"] = wrap(gen.forbiddenTerm, terms)
		}
	}
	names := make([]string, 0, len(mutators))
	for k := range mutators {
		names = append(names, k)
	}
	sortStrings(names)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	attempts := 0
	quota := int(float64(opts.Count)*(1-opts.CleanRatio)/float64(len(names))*1.5) + 1
	for stats.Written < opts.Count && attempts < opts.Count*40 {
		attempts++
		balanced := attempts < opts.Count*20
		d := dms[r.Intn(len(dms))]
		var s sample
		undoInline := inlineWarnings(d)
		if r.Float64() < opts.CleanRatio {
			frag, ok := gen.cleanFragment(r, d)
			undoInline()
			if !ok {
				continue
			}
			s = sample{mutation: "clean", fragment: frag}
		} else {
			name := names[r.Intn(len(names))]
			if balanced && stats.Mutations[name] >= quota {
				undoInline()
				continue
			}
			ms, ok := mutators[name](r, d)
			undoInline()
			if !ok {
				continue
			}
			ms.mutation = name
			s = ms
		}
		labels, input, ok := label(s, d, labelRules, opts.Input)
		if !ok {
			continue
		}
		answer, _ := json.Marshal(map[string]any{"findings": labels})
		user := analysis.ReviewUserPrompt(rulesText, s.related, input)
		var rec any
		if opts.Format == "alpaca" {
			rec = map[string]string{"instruction": analysis.ReviewSystemPrompt, "input": user, "output": string(answer)}
		} else {
			rec = map[string]any{"messages": []map[string]string{
				{"role": "system", "content": analysis.ReviewSystemPrompt},
				{"role": "user", "content": user},
				{"role": "assistant", "content": string(answer)},
			}}
		}
		if err := enc.Encode(rec); err != nil {
			return stats, err
		}
		stats.Written++
		stats.Mutations[s.mutation]++
	}
	if stats.Written == 0 {
		return stats, fmt.Errorf("es konnten keine Beispiele erzeugt werden (zu wenig geeignete Inhalte)")
	}
	return stats, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// label parses the fragment, runs the local deterministic checks and merges
// the injected (non-detectable) errors. Structural mutations must be
// detected by the checks, otherwise the sample is discarded.
func label(s sample, orig *s1000d.Doc, labelRules []rules.Rule, inputMode string) ([]analysis.LLMFinding, string, bool) {
	root, err := xmldoc.Parse([]byte(s.fragment))
	if err != nil {
		return nil, "", false
	}
	mini := &s1000d.Doc{Path: "fragment.xml", Kind: s1000d.KindDM, Root: root, DMC: orig.DMC, Lang: orig.Lang}
	c := &analysis.Corpus{Docs: []*s1000d.Doc{mini}, ICNs: map[string]string{}}
	var fs []analysis.Finding
	fs = append(fs, analysis.CheckLists(c)...)
	fs = append(fs, analysis.CheckTitles(c)...)
	fs = append(fs, analysis.CheckConsistency(c)...)
	fs = append(fs, rules.Evaluate(c, labelRules)...)

	var labels []analysis.LLMFinding
	seen := map[string]bool{}
	for _, f := range fs {
		// corpus-level or not visible to the model in a fragment
		if f.Code == "RULE_CONFIG_ERROR" || strings.HasSuffix(f.Code, "_VARIANT") || strings.Contains(f.Path, "warningsAndCautions") {
			continue
		}
		k := fmt.Sprintf("%s|%d|%s", f.Code, f.Line, f.Message)
		if seen[k] {
			continue
		}
		seen[k] = true
		labels = append(labels, analysis.LLMFinding{Line: f.Line, Code: f.Code, Severity: f.Severity, Message: f.Message, Suggestion: f.Suggestion})
	}
	expected := map[string]string{
		"duplicate_list_item": "LIST_DUPLICATE", "duplicate_sibling_title": "TITLE_DUPLICATE_SIBLING",
		"duplicate_step": "STEP_DUPLICATE", "title_punctuation": "TITLE_FORMAT", "duplicate_requirement": "REQ_DUPLICATE",
		"forbidden_term": "RULE:",
	}
	if want, ok := expected[s.mutation]; ok {
		found := false
		for _, l := range labels {
			if strings.HasPrefix(l.Code, want) {
				found = true
			}
		}
		if !found {
			return nil, "", false
		}
	}
	// injected labels: semantic mutators mark the changed element,
	// structural ones are located via the quoted word
	marks := map[int]int{}
	root.Walk(func(n *xmldoc.Node) bool {
		if v := n.Attr(markAttr); v != "" {
			idx, _ := strconv.Atoi(v)
			marks[idx] = n.Line
			n.RemoveAttr(markAttr)
		}
		return true
	})
	for i, inj := range s.injected {
		if l, ok := marks[i]; ok {
			inj.Line = l
		} else {
			inj.Line = findLine(root, inj.Message, inj.Suggestion)
		}
		if inj.Line == 0 {
			return nil, "", false
		}
		labels = append(labels, inj)
	}
	if s.mutation == "clean" {
		// keep clean samples clean: drop very noisy info-only findings
		var keep []analysis.LLMFinding
		for _, l := range labels {
			if l.Severity != analysis.SevInfo {
				keep = append(keep, l)
			}
		}
		labels = keep
	}
	if labels == nil {
		labels = []analysis.LLMFinding{}
	}
	input := markRe.ReplaceAllString(s.fragment, "")
	if inputMode != "xml" {
		input = "Fragment aus " + orig.Label() + " (" + orig.Title() + ")\n" + s1000d.RenderNode(root.Root(), 0)
		alignLines(labels, input)
	}
	return labels, input, true
}

var markRe = regexp.MustCompile(` ` + markAttr + `="\d+"`)

var lineMarker = regexp.MustCompile(`\[L(\d+)\]`)

// alignLines moves label line numbers to the next [Lnn] marker that is
// visible in the rendered input (elements like proceduralStep start one
// line before their first rendered paragraph).
func alignLines(labels []analysis.LLMFinding, rendered string) {
	var visible []int
	for _, m := range lineMarker.FindAllStringSubmatch(rendered, -1) {
		n, _ := strconv.Atoi(m[1])
		visible = append(visible, n)
	}
	sort.Ints(visible)
	if len(visible) == 0 {
		return
	}
	for i := range labels {
		l := labels[i].Line
		best := visible[len(visible)-1]
		for _, v := range visible {
			if v >= l {
				best = v
				break
			}
		}
		labels[i].Line = best
	}
}

// findLine finds the first line containing the wrong word quoted in the message.
func findLine(root *xmldoc.Node, msg, _ string) int {
	start := strings.Index(msg, "„")
	end := strings.Index(msg, "“")
	if start < 0 || end <= start {
		return 0
	}
	word := msg[start+len("„") : end]
	line := 0
	var rec func(n *xmldoc.Node)
	rec = func(n *xmldoc.Node) {
		if line != 0 {
			return
		}
		if n.Kind == xmldoc.Text && strings.Contains(n.Data, word) {
			if n.Parent != nil {
				line = n.Parent.Line
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.Next {
			rec(c)
		}
	}
	rec(root)
	return line
}

type generator struct {
	maxChars int
	corpus   *analysis.Corpus
}

// unitOf returns the closest enclosing unit (or the content element).
func unitOf(n *xmldoc.Node) *xmldoc.Node {
	for p := n; p != nil; p = p.Parent {
		if p.Kind == xmldoc.Element && (s1000d.IsUnit(p) && p.Name != "warning" && p.Name != "caution" && p.Name != "note") {
			return p
		}
		if p.Name == "mainProcedure" || p.Name == "description" || p.Name == "content" {
			return p
		}
	}
	return n
}

// fragment serializes unit; if too long it keeps only the start tag, the
// title and the branch containing focus.
func (g *generator) fragment(unit, focus *xmldoc.Node) string {
	s := unit.PrettyXML()
	if len(s) <= g.maxChars {
		return s
	}
	return compact(unit, focus, g.maxChars)
}

func compact(n, focus *xmldoc.Node, budget int) string {
	var b strings.Builder
	b.WriteString(n.StartTag())
	for c := n.FirstChild; c != nil; c = c.Next {
		if c.Kind != xmldoc.Element {
			continue
		}
		if c.Name == "title" {
			b.WriteString("\n" + c.OuterXML())
			continue
		}
		if contains(c, focus) {
			s := c.PrettyXML()
			if len(s) > budget {
				s = compact(c, focus, budget)
			}
			b.WriteString("\n" + s)
		}
	}
	b.WriteString("\n" + n.EndTag())
	return b.String()
}

func contains(anc, n *xmldoc.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p == anc {
			return true
		}
	}
	return false
}

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.Intn(len(xs))] }

func content(d *s1000d.Doc) *xmldoc.Node {
	if c := d.Root.Root().Child("content"); c != nil {
		return c
	}
	return d.Root.Root()
}

func (g *generator) cleanFragment(r *rand.Rand, d *s1000d.Doc) (string, bool) {
	units := content(d).FindAll("levelledPara", "proceduralStep", "mainProcedure", "preliminaryRqmts")
	if len(units) == 0 {
		return "", false
	}
	u := pick(r, units)
	return g.fragment(u, u), true
}

func (g *generator) duplicateListItem(r *rand.Rand, d *s1000d.Doc, _ []rules.Term) (string, []analysis.LLMFinding, bool) {
	var lists []*xmldoc.Node
	for _, l := range content(d).FindAll("randomList", "sequentialList") {
		if len(childrenNamed(l, "listItem")) >= 2 {
			lists = append(lists, l)
		}
	}
	if len(lists) == 0 {
		return "", nil, false
	}
	l := pick(r, lists)
	items := childrenNamed(l, "listItem")
	src := pick(r, items)
	after := pick(r, items)
	cp := src.Clone()
	after.InsertAfter(cp)
	frag := g.fragment(unitOf(l), cp)
	cp.Remove()
	return frag, nil, true
}

func (g *generator) duplicateRequirement(r *rand.Rand, d *s1000d.Doc, _ []rules.Term) (string, []analysis.LLMFinding, bool) {
	var items []*xmldoc.Node
	for _, n := range content(d).FindAll("supplyDescr", "spareDescr", "supportEquipDescr") {
		if n.Child("name") != nil {
			items = append(items, n)
		}
	}
	if len(items) == 0 {
		return "", nil, false
	}
	it := pick(r, items)
	cp := it.Clone()
	it.InsertAfter(cp)
	frag := g.fragment(unitOf(it), cp)
	cp.Remove()
	return frag, nil, true
}

func (g *generator) duplicateStep(r *rand.Rand, d *s1000d.Doc, _ []rules.Term) (string, []analysis.LLMFinding, bool) {
	steps := content(d).FindAll("proceduralStep")
	var leaf []*xmldoc.Node
	for _, s := range steps {
		if len(childrenNamed(s, "proceduralStep")) == 0 && xmldoc.Norm(s.TextContent()) != "" {
			leaf = append(leaf, s)
		}
	}
	if len(leaf) == 0 {
		return "", nil, false
	}
	s := pick(r, leaf)
	cp := s.Clone()
	cp.Attrs = withoutID(cp.Attrs)
	s.InsertAfter(cp)
	parent := s.Parent
	frag := g.fragment(unitOf(parent), cp)
	cp.Remove()
	return frag, nil, true
}

func withoutID(a []xmldoc.Attr) []xmldoc.Attr {
	var out []xmldoc.Attr
	for _, x := range a {
		if x.Name != "id" {
			out = append(out, x)
		}
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

func titled(d *s1000d.Doc) []*xmldoc.Node {
	var out []*xmldoc.Node
	for _, u := range content(d).FindAll("levelledPara", "figure", "table") {
		if t := u.Child("title"); t != nil && t.FirstText() != nil && xmldoc.Norm(t.TextContent()) != "" {
			out = append(out, t)
		}
	}
	return out
}

func (g *generator) duplicateSiblingTitle(r *rand.Rand, d *s1000d.Doc, _ []rules.Term) (string, []analysis.LLMFinding, bool) {
	var parents []*xmldoc.Node
	content(d).Walk(func(n *xmldoc.Node) bool {
		if len(childrenNamed(n, "levelledPara")) >= 2 {
			parents = append(parents, n)
		}
		return true
	})
	if len(parents) == 0 {
		return "", nil, false
	}
	p := pick(r, parents)
	kids := childrenNamed(p, "levelledPara")
	a, b := kids[0], kids[1+r.Intn(len(kids)-1)]
	ta, tb := a.Child("title"), b.Child("title")
	if ta == nil || tb == nil || tb.FirstText() == nil {
		return "", nil, false
	}
	txt := tb.FirstText()
	old := txt.Data
	txt.Data = xmldoc.Norm(ta.TextContent())
	// keep fragment small: only titles of the sibling sections
	frag := g.fragmentTitles(p)
	txt.Data = old
	return frag, nil, true
}

// fragmentTitles serializes a parent with its sections, but each section
// shortened to its title and first paragraph.
func (g *generator) fragmentTitles(p *xmldoc.Node) string {
	var b strings.Builder
	b.WriteString(p.StartTag())
	for _, c := range p.Children() {
		if c.Name == "title" {
			b.WriteString("\n" + c.OuterXML())
			continue
		}
		if c.Name != "levelledPara" {
			continue
		}
		b.WriteString("\n" + c.StartTag())
		if t := c.Child("title"); t != nil {
			b.WriteString("\n" + t.OuterXML())
		}
		if para := c.Child("para"); para != nil {
			b.WriteString("\n" + para.OuterXML())
		}
		b.WriteString("\n" + c.EndTag())
		if b.Len() > g.maxChars {
			break
		}
	}
	b.WriteString("\n" + p.EndTag())
	return b.String()
}

func (g *generator) titlePunctuation(r *rand.Rand, d *s1000d.Doc, _ []rules.Term) (string, []analysis.LLMFinding, bool) {
	ts := titled(d)
	if len(ts) == 0 {
		return "", nil, false
	}
	t := pick(r, ts)
	txt := t.LastChild
	if txt == nil || txt.Kind != xmldoc.Text {
		return "", nil, false
	}
	old := txt.Data
	txt.Data = strings.TrimRight(old, " \n\t") + pick(r, []string{".", ":"})
	frag := g.fragment(unitOf(t.Parent), t)
	txt.Data = old
	return frag, nil, true
}

func (g *generator) titleTypo(r *rand.Rand, d *s1000d.Doc, _ []rules.Term) (string, []analysis.LLMFinding, bool) {
	ts := titled(d)
	if len(ts) == 0 {
		return "", nil, false
	}
	t := pick(r, ts)
	return g.typoIn(r, t, "TITLE_TYPO", "warning", "Möglicher Tippfehler im Titel: „%s“ statt „%s“.")
}

func (g *generator) textTypo(r *rand.Rand, d *s1000d.Doc, _ []rules.Term) (string, []analysis.LLMFinding, bool) {
	paras := content(d).FindAll("para")
	if len(paras) == 0 {
		return "", nil, false
	}
	p := pick(r, paras)
	return g.typoIn(r, p, "TEXT_TYPO", "warning", "Tippfehler: „%s“ statt „%s“.")
}

func (g *generator) typoIn(r *rand.Rand, el *xmldoc.Node, code, sev, msgFmt string) (string, []analysis.LLMFinding, bool) {
	txt := el.FirstText()
	if txt == nil {
		return "", nil, false
	}
	words, pos := textutil.WordsWithPos(txt.Data)
	var cand []int
	for i, w := range words {
		if len([]rune(w)) >= 5 && !textutil.IsUpperWord(w) {
			cand = append(cand, i)
		}
	}
	if len(cand) == 0 {
		return "", nil, false
	}
	i := pick(r, cand)
	orig := words[i]
	bad := typo(r, orig)
	if bad == orig {
		return "", nil, false
	}
	old := txt.Data
	txt.Data = old[:pos[i]] + bad + old[pos[i]+len(orig):]
	frag := g.fragment(unitOf(el), el)
	txt.Data = old
	inj := analysis.LLMFinding{Code: code, Severity: sev, Message: fmt.Sprintf(msgFmt, bad, orig), Suggestion: orig}
	return frag, []analysis.LLMFinding{inj}, true
}

var qwertz = map[rune]string{
	'a': "sq", 'b': "vn", 'c': "xv", 'd': "sf", 'e': "wr", 'f': "dg", 'g': "fh", 'h': "gj", 'i': "uo", 'j': "hk",
	'k': "jl", 'l': "k", 'm': "n", 'n': "bm", 'o': "ip", 'p': "o", 'q': "w", 'r': "et", 's': "ad", 't': "rz",
	'u': "zi", 'v': "cb", 'w': "qe", 'x': "yc", 'y': "x", 'z': "tu",
}

// typo applies one realistic keyboard error.
func typo(r *rand.Rand, w string) string {
	rs := []rune(w)
	if len(rs) < 4 {
		return w
	}
	i := 1 + r.Intn(len(rs)-2)
	switch r.Intn(4) {
	case 0: // swap
		if rs[i] == rs[i+1] {
			return w
		}
		rs[i], rs[i+1] = rs[i+1], rs[i]
	case 1: // drop
		rs = append(rs[:i], rs[i+1:]...)
	case 2: // double
		rs = append(rs[:i+1], rs[i:]...)
	default: // neighbour key
		lower := unicode.ToLower(rs[i])
		nb, ok := qwertz[lower]
		if !ok {
			return w
		}
		c := []rune(nb)[r.Intn(len([]rune(nb)))]
		if unicode.IsUpper(rs[i]) {
			c = unicode.ToUpper(c)
		}
		rs[i] = c
	}
	return string(rs)
}

func (g *generator) forbiddenTerm(r *rand.Rand, d *s1000d.Doc, terms []rules.Term) (string, []analysis.LLMFinding, bool) {
	paras := content(d).FindAll("para")
	if len(paras) == 0 || len(terms) == 0 {
		return "", nil, false
	}
	p := pick(r, paras)
	txt := p.FirstText()
	if txt == nil || strings.TrimSpace(txt.Data) == "" {
		return "", nil, false
	}
	t := pick(r, terms)
	if t.Term == "" {
		return "", nil, false
	}
	old := txt.Data
	words, pos := textutil.WordsWithPos(old)
	if len(words) < 3 {
		return "", nil, false
	}
	i := 1 + r.Intn(len(words)-1)
	txt.Data = old[:pos[i]] + t.Term + " " + old[pos[i]:]
	frag := g.fragment(unitOf(p), p)
	txt.Data = old
	return frag, nil, true
}
