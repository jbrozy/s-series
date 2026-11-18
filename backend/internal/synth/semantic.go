package synth

import (
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"scheck/internal/analysis"
	"scheck/internal/s1000d"
	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

// Semantic mutators inject errors that cannot be found by rules: the
// generator knows what it changed, so the label is certain even though
// detection needs understanding of the content.

const markAttr = "scheckmark"

func mark(n *xmldoc.Node, idx int) func() {
	n.SetAttr(markAttr, strconv.Itoa(idx))
	return func() { n.RemoveAttr(markAttr) }
}

// inlineWarnings copies referenced warnings/cautions into the referencing
// elements, so fragments are self-contained. Returns an undo function.
func inlineWarnings(d *s1000d.Doc) func() {
	ids := xmldoc.IDIndex(d.Root)
	var undos []func()
	content(d).Walk(func(n *xmldoc.Node) bool {
		for _, attr := range []string{"cautionRefs", "warningRefs"} { // prepend: warnings end up first
			v := n.Attr(attr)
			if v == "" {
				continue
			}
			refs := strings.Fields(v)
			for k := len(refs) - 1; k >= 0; k-- {
				t := ids[refs[k]]
				if t == nil || (t.Name != "warning" && t.Name != "caution") {
					continue
				}
				c := t.Clone()
				c.Attrs = withoutID(c.Attrs)
				n.PrependChild(c)
				undos = append(undos, c.Remove)
			}
			node, a, old := n, attr, v
			node.RemoveAttr(a)
			undos = append(undos, func() { node.SetAttr(a, old) })
		}
		return true
	})
	return func() {
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i]()
		}
	}
}

func leafSteps(d *s1000d.Doc) []*xmldoc.Node {
	var out []*xmldoc.Node
	for _, s := range content(d).FindAll("proceduralStep") {
		if len(childrenNamed(s, "proceduralStep")) == 0 && stepText(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func stepText(s *xmldoc.Node) string {
	return xmldoc.Norm(s.TextExcluding(func(x *xmldoc.Node) bool {
		return x.Name == "warning" || x.Name == "caution" || x.Name == "note" || x.Name == "proceduralStep"
	}))
}

// rangeFragment serializes the siblings from..to (inclusive) of a parent.
func (g *generator) rangeFragment(parent *xmldoc.Node, from, to *xmldoc.Node) string {
	var b strings.Builder
	b.WriteString(parent.StartTag())
	if t := parent.Child("title"); t != nil {
		b.WriteString("\n" + t.OuterXML())
	}
	in := false
	for c := parent.FirstChild; c != nil; c = c.Next {
		if c == from {
			in = true
		}
		if in && c.Kind == xmldoc.Element {
			b.WriteString("\n" + c.PrettyXML())
		}
		if c == to {
			break
		}
	}
	b.WriteString("\n" + parent.EndTag())
	return b.String()
}

func (g *generator) stepWindow(step *xmldoc.Node, before, after int) string {
	parent := step.Parent
	sibs := childrenNamed(parent, "proceduralStep")
	idx := 0
	for i, s := range sibs {
		if s == step {
			idx = i
		}
	}
	from := sibs[max(0, idx-before)]
	to := sibs[min(len(sibs)-1, idx+after)]
	frag := g.rangeFragment(parent, from, to)
	if len(frag) > g.maxChars*2 {
		return g.fragment(step, step)
	}
	return frag
}

// ---------------------------------------------------------------- step order

type orderRule struct {
	first, second *regexp.Regexp
	why           string
}

func rx(s string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + s) }

var (
	reDepress  = rx(`depressuri|release (the )?([a-z]+ ){0,2}pressure|bleed (the )?([a-z]+ ){0,2}pressure|relieve (the )?([a-z]+ ){0,2}pressure|druck (ab|ent)lassen|drucklos|druckentlast`)
	reDisconn  = rx(`\bdisconnect|\bremove\b|\bloosen|\bopen the (drain|line|valve|cap)|\btrennen|\babschrauben|\blösen|\bausbauen|\babziehen|\bentfernen`)
	rePowerOff = rx(`open the circuit breaker|de-?energi|switch off|turn off|disconnect (the )?(electrical )?power|isolate (the )?power|abschalten|ausschalten|spannungsfrei|sicherung (ziehen|öffnen)|stromlos`)
	reDrain    = rx(`\bdrain\b|\bablassen\b|\bentleeren`)
	reInstall  = rx(`\binstall\b|\binsert\b|\bput .* (on|in|into)\b|\bposition\b|\battach\b|\beinbauen|\beinsetzen|\banbringen|\bmontieren|\baufsetzen`)
	reFasten   = rx(`\btorque\b|\btighten|\bsafety (the|with)|\blockwire|\bsafety wire|\banziehen|\bsichern\b|\bfestziehen|\bverdrahten`)
	reTest     = rx(`\b(do|make) (a|an|the) .*(test|check)|\bleak (test|check)|\boperational (test|check)|\bfunctional test|\bprüfen\b.*(dicht|funktion)|\bfunktionsprüfung|\bdichtheitsprüfung`)
	reRemOld   = rx(`remove (and discard )?(the )?(old|used)|\bdiscard\b|alte .* entfernen|\bentsorgen`)
	reInstNew  = rx(`install (a |the )?new|new .* (on|in)\b|neue[nrs]? .* (einsetzen|einbauen|anbringen)`)
	reClean    = rx(`\bclean\b|\breinigen\b|\bsäubern\b`)
	reApply    = rx(`\bapply\b|\blubricate\b|\bauftragen\b|\bschmieren\b`)
)

var orderRules = []orderRule{
	{reDepress, reDisconn, "Das System muss drucklos sein, bevor Leitungen oder Bauteile gelöst werden."},
	{rePowerOff, reDisconn, "Die Spannungsversorgung muss getrennt sein, bevor Bauteile gelöst werden."},
	{reDrain, reDisconn, "Die Flüssigkeit muss abgelassen sein, bevor Bauteile gelöst werden."},
	{reInstall, reFasten, "Ein Bauteil kann erst angezogen/gesichert werden, nachdem es eingebaut ist."},
	{reInstall, reTest, "Die Prüfung kann erst nach dem Einbau erfolgen."},
	{reFasten, reTest, "Die Prüfung erfolgt erst, wenn alles angezogen und gesichert ist."},
	{reRemOld, reInstNew, "Das alte Teil muss entfernt sein, bevor das neue eingesetzt wird."},
	{reClean, reApply, "Erst reinigen, dann Mittel auftragen."},
}

// swapContent exchanges the content (attributes, children) of two elements.
func swapContent(a, b *xmldoc.Node) {
	a.Attrs, b.Attrs = b.Attrs, a.Attrs
	a.FirstChild, b.FirstChild = b.FirstChild, a.FirstChild
	a.LastChild, b.LastChild = b.LastChild, a.LastChild
	for c := a.FirstChild; c != nil; c = c.Next {
		c.Parent = a
	}
	for c := b.FirstChild; c != nil; c = c.Next {
		c.Parent = b
	}
}

func (g *generator) stepOrder(r *rand.Rand, d *s1000d.Doc) (sample, bool) {
	type cand struct {
		a, b *xmldoc.Node
		rule orderRule
	}
	var cands []cand
	parents := map[*xmldoc.Node]bool{}
	for _, s := range leafSteps(d) {
		parents[s.Parent] = true
	}
	for p := range parents {
		sibs := childrenNamed(p, "proceduralStep")
		for i := 0; i < len(sibs); i++ {
			ta := stepText(sibs[i])
			for j := i + 1; j < len(sibs) && j <= i+3; j++ {
				tb := stepText(sibs[j])
				for _, rl := range orderRules {
					if rl.first.MatchString(ta) && !rl.second.MatchString(ta) && rl.second.MatchString(tb) && !rl.first.MatchString(tb) {
						cands = append(cands, cand{sibs[i], sibs[j], rl})
					}
				}
			}
		}
	}
	if len(cands) == 0 {
		return sample{}, false
	}
	c := cands[r.Intn(len(cands))]
	ta, tb := stepText(c.a), stepText(c.b)
	swapContent(c.a, c.b)
	defer swapContent(c.a, c.b)
	undo := mark(c.a, 0) // position of a now holds the step that must come later
	frag := g.rangeFragment(c.a.Parent, c.a, c.b)
	undo()
	inj := analysis.LLMFinding{Code: "STEP_ORDER", Severity: analysis.SevError,
		Message:    fmt.Sprintf("Falsche Reihenfolge: „%s“ steht vor „%s“. %s", textutil.Truncate(tb, 90), textutil.Truncate(ta, 90), c.rule.why),
		Suggestion: "Schritte in die richtige Reihenfolge bringen."}
	return sample{fragment: frag, injected: []analysis.LLMFinding{inj}}, true
}

// ---------------------------------------------------------------- missing warning

func (g *generator) missingWarning(r *rand.Rand, d *s1000d.Doc) (sample, bool) {
	var steps []*xmldoc.Node
	for _, s := range leafSteps(d) {
		for _, c := range s.Children() {
			if c.Name == "warning" || c.Name == "caution" {
				steps = append(steps, s)
				break
			}
		}
	}
	if len(steps) == 0 {
		return sample{}, false
	}
	s := steps[r.Intn(len(steps))]
	var removed []*xmldoc.Node
	var texts []string
	kind := "Warnhinweis (WARNUNG)"
	for _, c := range s.Children() {
		if c.Name == "warning" || c.Name == "caution" {
			if c.Name == "caution" && len(removed) == 0 {
				kind = "Vorsichtshinweis (VORSICHT)"
			}
			removed = append(removed, c)
			texts = append(texts, xmldoc.Norm(c.TextContent()))
		}
	}
	// remember positions to restore
	type pos struct{ n, prev *xmldoc.Node }
	var ps []pos
	for _, c := range removed {
		ps = append(ps, pos{c, c.Prev})
	}
	for _, c := range removed {
		c.Remove()
	}
	defer func() {
		for i := 0; i < len(ps); i++ {
			if ps[i].prev == nil {
				s.PrependChild(ps[i].n)
			} else {
				ps[i].prev.InsertAfter(ps[i].n)
			}
		}
	}()
	undo := mark(s, 0)
	frag := g.stepWindow(s, 1, 0)
	undo()
	inj := analysis.LLMFinding{Code: "MISSING_WARNING", Severity: analysis.SevError,
		Message: fmt.Sprintf("Sicherheitshinweis fehlt: Vor dem Schritt „%s“ ist ein %s erforderlich.",
			textutil.Truncate(stepText(s), 90), kind),
		Suggestion: "Ergänzen: „" + textutil.Truncate(strings.Join(texts, " "), 160) + "“"}
	return sample{fragment: frag, injected: []analysis.LLMFinding{inj}}, true
}

// ---------------------------------------------------------------- values & units

var valueRe = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s?(N·m|N\.m|Nm|lbf\.in|lbf\.ft|lb\.in|lb\.ft|psi|bar|kPa|MPa|°C|°F|mm|cm|ml|kg|rpm|V|A|l|g|%)`)

type valueOcc struct {
	doc   *s1000d.Doc
	text  *xmldoc.Node // text node
	start int          // byte offsets of the whole match within text.Data
	end   int
	num   string
	unit  string
}

func (v valueOcc) key() string { return strings.ReplaceAll(v.num, ",", ".") + " " + v.unit }

func findValues(docs []*s1000d.Doc) []valueOcc {
	var out []valueOcc
	for _, d := range docs {
		var rec func(n *xmldoc.Node)
		rec = func(n *xmldoc.Node) {
			for c := n.FirstChild; c != nil; c = c.Next {
				if c.Kind == xmldoc.Element {
					if c.Name == "identAndStatusSection" || c.Name == "dmCode" || c.Name == "identNumber" {
						continue
					}
					rec(c)
					continue
				}
				if c.Kind != xmldoc.Text {
					continue
				}
				for _, m := range valueRe.FindAllStringSubmatchIndex(c.Data, -1) {
					end := m[1]
					if end < len(c.Data) {
						if rn, _ := utf8.DecodeRuneInString(c.Data[end:]); unicode.IsLetter(rn) {
							continue
						}
					}
					if m[0] > 0 {
						if rn, _ := utf8.DecodeLastRuneInString(c.Data[:m[0]]); unicode.IsLetter(rn) || rn == '-' {
							continue // part numbers like MS20995C32
						}
					}
					out = append(out, valueOcc{doc: d, text: c, start: m[0], end: end, num: c.Data[m[2]:m[3]], unit: c.Data[m[4]:m[5]]})
				}
			}
		}
		rec(content(d))
	}
	return out
}

func sentenceAround(s string, start, end int) string {
	from := strings.LastIndexAny(s[:start], ".!?;") + 1
	to := strings.IndexAny(s[end:], ".!?;")
	if to < 0 {
		to = len(s)
	} else {
		to += end + 1
	}
	return xmldoc.Norm(s[from:to])
}

func changedNumber(r *rand.Rand, num string) string {
	comma := strings.Contains(num, ",")
	v, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", "."), 64)
	if err != nil || v == 0 {
		return ""
	}
	for tries := 0; tries < 6; tries++ {
		f := []float64{0.5, 0.8, 1.2, 1.4, 1.5, 2}[r.Intn(6)]
		nv := v * f
		var s string
		switch {
		case v >= 50:
			s = strconv.Itoa(int(math.Round(nv/5) * 5))
		case v >= 10 || !strings.ContainsAny(num, ".,"):
			s = strconv.Itoa(int(math.Round(nv)))
		default:
			s = strconv.FormatFloat(math.Round(nv*10)/10, 'f', 1, 64)
		}
		if comma {
			s = strings.ReplaceAll(s, ".", ",")
		}
		if s != num && s != "0" {
			return s
		}
	}
	return ""
}

func (g *generator) valueConflict(r *rand.Rand, d *s1000d.Doc) (sample, bool) {
	// related documents: same hardware item (SNS) or the same module
	var related []*s1000d.Doc
	for _, o := range g.corpus.Docs {
		if o.Kind == s1000d.KindDM && (o == d || (o.SNSKey != "" && o.SNSKey == d.SNSKey && o.Lang == d.Lang)) {
			related = append(related, o)
		}
	}
	occ := findValues(related)
	byKey := map[string][]valueOcc{}
	for _, o := range occ {
		byKey[o.key()] = append(byKey[o.key()], o)
	}
	var mine []valueOcc
	for _, o := range occ {
		if o.doc == d && len(byKey[o.key()]) >= 2 {
			mine = append(mine, o)
		}
	}
	if len(mine) == 0 {
		return sample{}, false
	}
	a := mine[r.Intn(len(mine))]
	var b valueOcc
	for _, o := range byKey[a.key()] {
		if o.text != a.text || o.start != a.start {
			b = o
			break
		}
	}
	nv := changedNumber(r, a.num)
	if nv == "" || b.text == nil {
		return sample{}, false
	}
	ctxLine := b.doc.Label() + ": " + sentenceAround(b.text.Data, b.start, b.end)
	old := a.text.Data
	newMatch := strings.Replace(old[a.start:a.end], a.num, nv, 1)
	a.text.Data = old[:a.start] + newMatch + old[a.end:]
	defer func() { a.text.Data = old }()
	el := a.text.Parent
	undo := mark(el, 0)
	frag := g.fragment(unitOf(el), el)
	undo()
	inj := analysis.LLMFinding{Code: "VALUE_CONFLICT", Severity: analysis.SevError,
		Message:    fmt.Sprintf("Widersprüchlicher Wert: hier „%s“, in %s dagegen „%s %s“.", newMatch, b.doc.Label(), b.num, b.unit),
		Suggestion: fmt.Sprintf("Wert prüfen und vereinheitlichen (vermutlich %s %s).", a.num, a.unit)}
	return sample{fragment: frag, injected: []analysis.LLMFinding{inj}, related: []string{ctxLine}}, true
}

// unit swaps that change the magnitude by at least a factor of ten
var unitSwap = map[string]string{"psi": "bar", "bar": "psi", "Nm": "kNm", "N·m": "kN·m", "mm": "m", "ml": "l", "kPa": "MPa", "MPa": "kPa", "g": "kg", "lbf.in": "lbf.ft"}

func (g *generator) unitError(r *rand.Rand, d *s1000d.Doc) (sample, bool) {
	var cands []valueOcc
	for _, o := range findValues([]*s1000d.Doc{d}) {
		if _, ok := unitSwap[o.unit]; ok {
			cands = append(cands, o)
		}
	}
	if len(cands) == 0 {
		return sample{}, false
	}
	a := cands[r.Intn(len(cands))]
	nu := unitSwap[a.unit]
	old := a.text.Data
	orig := old[a.start:a.end]
	repl := orig[:len(orig)-len(a.unit)] + nu
	a.text.Data = old[:a.start] + repl + old[a.end:]
	defer func() { a.text.Data = old }()
	el := a.text.Parent
	undo := mark(el, 0)
	frag := g.fragment(unitOf(el), el)
	undo()
	inj := analysis.LLMFinding{Code: "UNIT_ERROR", Severity: analysis.SevError,
		Message:    fmt.Sprintf("Unplausible Einheit: „%s“ – die Größenordnung passt nicht.", repl),
		Suggestion: fmt.Sprintf("Einheit prüfen (vermutlich „%s“).", orig)}
	return sample{fragment: frag, injected: []analysis.LLMFinding{inj}}, true
}

// ---------------------------------------------------------------- meaning flip

type flip struct {
	re   *regexp.Regexp
	repl string
}

var flips = []flip{
	{regexp.MustCompile(`\b([Dd])o not ([a-z])`), "${2}"},
	{regexp.MustCompile(`\b(is|are|must|can|will|could|should) not\b`), "$1"},
	{regexp.MustCompile(`\bbefore\b`), "after"},
	{regexp.MustCompile(`\bafter\b`), "before"},
	{regexp.MustCompile(`\bnicht\s`), ""},
	{regexp.MustCompile(`\bvor dem\b`), "nach dem"},
	{regexp.MustCompile(`\bnach dem\b`), "vor dem"},
}

func (g *generator) meaningFlip(r *rand.Rand, d *s1000d.Doc) (sample, bool) {
	type cand struct {
		t *xmldoc.Node
		f flip
	}
	var cands []cand
	for _, el := range content(d).FindAll("warningAndCautionPara", "para") {
		if el.Name == "para" && el.Ancestor("proceduralStep") == nil {
			continue
		}
		for c := el.FirstChild; c != nil; c = c.Next {
			if c.Kind != xmldoc.Text {
				continue
			}
			for _, f := range flips {
				if f.re.MatchString(c.Data) {
					cands = append(cands, cand{c, f})
				}
			}
		}
	}
	if len(cands) == 0 {
		return sample{}, false
	}
	c := cands[r.Intn(len(cands))]
	old := c.t.Data
	loc := c.f.re.FindStringSubmatchIndex(old)
	var dst []byte
	dst = c.f.re.ExpandString(dst, c.f.repl, old, loc)
	repl := string(dst)
	if strings.HasPrefix(c.f.re.String(), `\b([Dd])o not`) && len(repl) > 0 && old[loc[0]] == 'D' {
		repl = strings.ToUpper(repl[:1]) + repl[1:]
	}
	c.t.Data = old[:loc[0]] + repl + old[loc[1]:]
	defer func() { c.t.Data = old }()
	el := c.t.Parent
	sentenceNew := sentenceAround(c.t.Data, loc[0], loc[0]+len(repl))
	sentenceOld := sentenceAround(old, loc[0], loc[1])
	undo := mark(el, 0)
	var frag string
	if st := el.Ancestor("proceduralStep"); st != nil {
		frag = g.stepWindow(st, 1, 0)
	} else {
		frag = g.fragment(unitOf(el), el)
	}
	undo()
	inj := analysis.LLMFinding{Code: "MEANING_INVERTED", Severity: analysis.SevError,
		Message:    fmt.Sprintf("Aussage ist sinnverkehrt: „%s“.", textutil.Truncate(sentenceNew, 140)),
		Suggestion: "Richtig vermutlich: „" + textutil.Truncate(sentenceOld, 140) + "“"}
	return sample{fragment: frag, injected: []analysis.LLMFinding{inj}}, true
}

// ---------------------------------------------------------------- procedure mismatch

var (
	installVerbs = [][2]string{{"Install", "Remove"}, {"install", "remove"}, {"Connect", "Disconnect"}, {"connect", "disconnect"},
		{"Tighten", "Loosen"}, {"tighten", "loosen"}, {"einbauen", "ausbauen"}, {"Einbauen", "Ausbauen"}, {"anschließen", "trennen"}, {"anziehen", "lösen"}}
	reInstallDM = rx(`install|einbau|montage`)
	reRemoveDM  = rx(`remov|ausbau|demontage`)
)

func (g *generator) procedureMismatch(r *rand.Rand, d *s1000d.Doc) (sample, bool) {
	isInstall := strings.HasPrefix(d.InfoCode, "72") || reInstallDM.MatchString(d.InfoName)
	isRemove := strings.HasPrefix(d.InfoCode, "52") || reRemoveDM.MatchString(d.InfoName)
	if isInstall == isRemove {
		return sample{}, false
	}
	type cand struct {
		t        *xmldoc.Node
		from, to string
		pos      int
	}
	var cands []cand
	for _, s := range leafSteps(d) {
		for _, p := range childrenNamed(s, "para") {
			t := p.FirstText()
			if t == nil {
				continue
			}
			for _, v := range installVerbs {
				from, to := v[0], v[1]
				if isRemove {
					from, to = to, from
				}
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(from) + `\b`)
				if loc := re.FindStringIndex(t.Data); loc != nil {
					cands = append(cands, cand{t, from, to, loc[0]})
				}
			}
		}
	}
	if len(cands) == 0 {
		return sample{}, false
	}
	c := cands[r.Intn(len(cands))]
	old := c.t.Data
	c.t.Data = old[:c.pos] + c.to + old[c.pos+len(c.from):]
	defer func() { c.t.Data = old }()
	step := c.t.Parent.Ancestor("proceduralStep")
	undo := mark(c.t.Parent, 0)
	frag := g.stepWindow(step, 1, 1)
	undo()
	inj := analysis.LLMFinding{Code: "PROCEDURE_MISMATCH", Severity: analysis.SevError,
		Message: fmt.Sprintf("Schritt „%s“ passt nicht zum Datenmodul „%s“.",
			textutil.Truncate(xmldoc.Norm(c.t.Data), 100), d.Title()),
		Suggestion: fmt.Sprintf("„%s“ statt „%s“.", c.from, c.to)}
	return sample{fragment: frag, injected: []analysis.LLMFinding{inj}}, true
}
