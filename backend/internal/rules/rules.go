// Package rules implements project specific business rules. Rules can be
// written by hand (XPath, terminology, title style, sentence length,
// dictionary) or imported from a BREX data module.
package rules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"scheck/internal/analysis"
	"scheck/internal/s1000d"
	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

// Rule types.
const (
	TypeXPathForbidden = "xpath_forbidden"
	TypeXPathRequired  = "xpath_required"
	TypeXPathValues    = "xpath_values"
	TypeForbiddenTerms = "forbidden_terms"
	TypeTitleCase      = "title_case"
	TypeSentenceLength = "sentence_length"
	TypeDictionary     = "dictionary"
)

var Types = []string{TypeXPathForbidden, TypeXPathRequired, TypeXPathValues, TypeForbiddenTerms, TypeTitleCase, TypeSentenceLength, TypeDictionary}

type Rule struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Severity string          `json:"severity"`
	Enabled  bool            `json:"enabled"`
	Source   string          `json:"source"`
	Config   json.RawMessage `json:"config"`
}

// AppliesTo restricts a rule to some data modules.
type AppliesTo struct {
	InfoCodes  []string `json:"infoCodes,omitempty"`  // prefix match, e.g. "520", "7"
	Contents   []string `json:"contents,omitempty"`   // description, procedure, ipd ...
	DMCPattern string   `json:"dmcPattern,omitempty"` // regular expression
}

func (a AppliesTo) match(d *s1000d.Doc) bool {
	if len(a.InfoCodes) > 0 {
		ok := false
		for _, p := range a.InfoCodes {
			if strings.HasPrefix(d.InfoCode, p) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	if len(a.Contents) > 0 {
		ok := false
		for _, ct := range a.Contents {
			if ct == d.Content {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	if a.DMCPattern != "" {
		re, err := regexp.Compile(a.DMCPattern)
		if err != nil || !re.MatchString(d.DMC) {
			return false
		}
	}
	return true
}

type XPathConfig struct {
	XPath     string    `json:"xpath"`
	Context   string    `json:"context,omitempty"`
	Message   string    `json:"message"`
	Allowed   []string  `json:"allowed,omitempty"`
	Pattern   string    `json:"pattern,omitempty"`
	Ranges    []string  `json:"ranges,omitempty"`
	AppliesTo AppliesTo `json:"appliesTo"`
}

type Term struct {
	Term        string `json:"term"`
	Replacement string `json:"replacement,omitempty"`
	Note        string `json:"note,omitempty"`
}

type TermsConfig struct {
	Terms         []Term    `json:"terms"`
	Scope         string    `json:"scope"` // all | titles | text
	CaseSensitive bool      `json:"caseSensitive"`
	PartialWord   bool      `json:"partialWord"`
	AppliesTo     AppliesTo `json:"appliesTo"`
}

type TitleCaseConfig struct {
	Style      string    `json:"style"` // sentence | title | upper
	Elements   []string  `json:"elements"`
	Exceptions []string  `json:"exceptions"`
	AppliesTo  AppliesTo `json:"appliesTo"`
}

type SentenceConfig struct {
	Procedural  int       `json:"procedural"`
	Descriptive int       `json:"descriptive"`
	AppliesTo   AppliesTo `json:"appliesTo"`
}

type DictionaryConfig struct {
	Words []string `json:"words"`
}

// Validate checks the rule configuration.
func Validate(r Rule) error {
	switch r.Type {
	case TypeXPathForbidden, TypeXPathRequired, TypeXPathValues:
		var c XPathConfig
		if err := json.Unmarshal(r.Config, &c); err != nil {
			return err
		}
		if c.XPath == "" {
			return fmt.Errorf("xpath fehlt")
		}
		if _, err := xmldoc.Compile(c.XPath); err != nil {
			return err
		}
		if c.Context != "" {
			if _, err := xmldoc.Compile(c.Context); err != nil {
				return err
			}
		}
		if c.Pattern != "" {
			if _, err := regexp.Compile("^(?:" + c.Pattern + ")$"); err != nil {
				return fmt.Errorf("pattern: %w", err)
			}
		}
	case TypeForbiddenTerms:
		var c TermsConfig
		if err := json.Unmarshal(r.Config, &c); err != nil {
			return err
		}
		if len(c.Terms) == 0 {
			return fmt.Errorf("keine Begriffe angegeben")
		}
	case TypeTitleCase:
		var c TitleCaseConfig
		if err := json.Unmarshal(r.Config, &c); err != nil {
			return err
		}
		if c.Style != "sentence" && c.Style != "title" && c.Style != "upper" {
			return fmt.Errorf("style muss sentence, title oder upper sein")
		}
	case TypeSentenceLength:
		var c SentenceConfig
		if err := json.Unmarshal(r.Config, &c); err != nil {
			return err
		}
	case TypeDictionary:
		var c DictionaryConfig
		if err := json.Unmarshal(r.Config, &c); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unbekannter Regeltyp %q", r.Type)
	}
	switch r.Severity {
	case analysis.SevError, analysis.SevWarning, analysis.SevInfo:
	default:
		return fmt.Errorf("severity muss error, warning oder info sein")
	}
	return nil
}

// Dictionary returns accepted words from all enabled dictionary rules.
func Dictionary(rs []Rule) map[string]bool {
	out := map[string]bool{}
	for _, r := range rs {
		if !r.Enabled || r.Type != TypeDictionary {
			continue
		}
		var c DictionaryConfig
		if json.Unmarshal(r.Config, &c) == nil {
			for _, w := range c.Words {
				for _, part := range textutil.Words(w) {
					out[strings.ToLower(part)] = true
				}
			}
		}
	}
	return out
}

// Evaluate runs all enabled rules over the corpus.
func Evaluate(c *analysis.Corpus, rs []Rule) []analysis.Finding {
	var out []analysis.Finding
	for _, r := range rs {
		if !r.Enabled || r.Type == TypeDictionary {
			continue
		}
		if err := Validate(r); err != nil {
			out = append(out, analysis.Finding{Category: analysis.CatRules, Code: "RULE_CONFIG_ERROR", Severity: analysis.SevError,
				Message: fmt.Sprintf("Regel %q ist fehlerhaft konfiguriert: %v", r.Name, err)})
			continue
		}
		for _, d := range c.DMs() {
			if d.Kind == s1000d.KindBREX {
				continue
			}
			out = append(out, evalRule(d, r)...)
		}
	}
	return out
}

func finding(d *s1000d.Doc, n *xmldoc.Node, r Rule, msg string) analysis.Finding {
	f := analysis.Finding{FileID: d.FileID, File: d.Path, DMC: d.DMC, Category: analysis.CatRules,
		Code: "RULE:" + r.Name, Severity: r.Severity, Message: msg}
	if n != nil {
		f.Path = n.Path()
		f.Line = n.Line
		f.Context = textutil.Truncate(xmldoc.Norm(n.TextContent()), 240)
	}
	return f
}

func evalRule(d *s1000d.Doc, r Rule) []analysis.Finding {
	switch r.Type {
	case TypeXPathForbidden, TypeXPathRequired, TypeXPathValues:
		var c XPathConfig
		_ = json.Unmarshal(r.Config, &c)
		if !c.AppliesTo.match(d) {
			return nil
		}
		return evalXPath(d, r, c)
	case TypeForbiddenTerms:
		var c TermsConfig
		_ = json.Unmarshal(r.Config, &c)
		if !c.AppliesTo.match(d) {
			return nil
		}
		return evalTerms(d, r, c)
	case TypeTitleCase:
		var c TitleCaseConfig
		_ = json.Unmarshal(r.Config, &c)
		if !c.AppliesTo.match(d) {
			return nil
		}
		return evalTitleCase(d, r, c)
	case TypeSentenceLength:
		var c SentenceConfig
		_ = json.Unmarshal(r.Config, &c)
		if !c.AppliesTo.match(d) {
			return nil
		}
		return evalSentences(d, r, c)
	}
	return nil
}

func msgOr(m, def string) string {
	if strings.TrimSpace(m) != "" {
		return m
	}
	return def
}

func evalXPath(d *s1000d.Doc, r Rule, c XPathConfig) []analysis.Finding {
	var out []analysis.Finding
	contexts := []*xmldoc.Node{d.Root}
	if c.Context != "" {
		ms, _, err := xmldoc.Select(d.Root, c.Context)
		if err != nil {
			return []analysis.Finding{finding(d, nil, r, err.Error())}
		}
		contexts = nil
		for _, m := range ms {
			contexts = append(contexts, m.Node)
		}
	}
	for _, ctx := range contexts {
		ms, truthy, err := xmldoc.Select(ctx, c.XPath)
		if err != nil {
			return []analysis.Finding{finding(d, nil, r, err.Error())}
		}
		switch r.Type {
		case TypeXPathForbidden:
			if !truthy {
				continue
			}
			for _, m := range ms {
				msg := msgOr(c.Message, "Nicht zulässiges Element/Attribut gefunden ("+c.XPath+").")
				if m.AttrName != "" {
					msg += fmt.Sprintf(" [@%s=%q]", m.AttrName, m.Value)
				}
				out = append(out, finding(d, m.Node, r, msg))
				if len(out) > 200 {
					return out
				}
			}
		case TypeXPathRequired:
			if !truthy {
				n := ctx
				if n.Kind == xmldoc.Document {
					n = n.Root()
				}
				out = append(out, finding(d, n, r, msgOr(c.Message, "Erforderliches Element fehlt ("+c.XPath+").")))
			}
		case TypeXPathValues:
			for _, m := range ms {
				v := strings.TrimSpace(m.Value)
				if valueAllowed(v, c) {
					continue
				}
				f := finding(d, m.Node, r, msgOr(c.Message, "Unzulässiger Wert.")+fmt.Sprintf(" Wert: %q", v))
				if len(c.Allowed) > 0 && len(c.Allowed) <= 12 {
					f.Suggestion = "Erlaubt: " + strings.Join(c.Allowed, ", ")
				}
				out = append(out, f)
				if len(out) > 200 {
					return out
				}
			}
		}
	}
	return out
}

func valueAllowed(v string, c XPathConfig) bool {
	if len(c.Allowed) == 0 && c.Pattern == "" && len(c.Ranges) == 0 {
		return true
	}
	for _, a := range c.Allowed {
		if v == a {
			return true
		}
	}
	if c.Pattern != "" {
		if re, err := regexp.Compile("^(?:" + c.Pattern + ")$"); err == nil && re.MatchString(v) {
			return true
		}
	}
	for _, rg := range c.Ranges {
		parts := strings.SplitN(rg, "~", 2)
		if len(parts) != 2 {
			continue
		}
		lo, hi := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		fv, e1 := strconv.ParseFloat(v, 64)
		fl, e2 := strconv.ParseFloat(lo, 64)
		fh, e3 := strconv.ParseFloat(hi, 64)
		if e1 == nil && e2 == nil && e3 == nil {
			if fv >= fl && fv <= fh {
				return true
			}
		} else if v >= lo && v <= hi {
			return true
		}
	}
	return false
}

var titleNodes = map[string]bool{"title": true, "techName": true, "infoName": true}

func evalTerms(d *s1000d.Doc, r Rule, c TermsConfig) []analysis.Finding {
	var out []analysis.Finding
	root := d.Root.Root()
	var rec func(n *xmldoc.Node, inTitle bool)
	rec = func(n *xmldoc.Node, inTitle bool) {
		for ch := n.FirstChild; ch != nil; ch = ch.Next {
			if ch.Kind == xmldoc.Element {
				if ch.Name == "dmCode" || ch.Name == "identNumber" || ch.Name == "verbatimText" {
					continue
				}
				rec(ch, inTitle || titleNodes[ch.Name])
				continue
			}
			if ch.Kind != xmldoc.Text {
				continue
			}
			if (c.Scope == "titles" && !inTitle) || (c.Scope == "text" && inTitle) {
				continue
			}
			for _, t := range c.Terms {
				if t.Term == "" {
					continue
				}
				if containsTerm(ch.Data, t.Term, c.CaseSensitive, c.PartialWord) {
					msg := fmt.Sprintf("Unzulässiger Begriff „%s“.", t.Term)
					if t.Note != "" {
						msg += " " + t.Note
					}
					f := finding(d, n, r, msg)
					if t.Replacement != "" {
						f.Suggestion = t.Replacement
					}
					out = append(out, f)
				}
			}
		}
	}
	if is := root.Child("identAndStatusSection"); is != nil {
		if t := is.Find("dmTitle"); t != nil {
			rec(t, true)
		}
	}
	if content := root.Child("content"); content != nil {
		rec(content, false)
	} else if root.Name != "dmodule" {
		rec(root, false) // fragment (e.g. synthetic training data)
	}
	return out
}

func containsTerm(text, term string, caseSensitive, partial bool) bool {
	t, s := term, text
	if !caseSensitive {
		t, s = strings.ToLower(term), strings.ToLower(text)
	}
	start := 0
	for {
		i := strings.Index(s[start:], t)
		if i < 0 {
			return false
		}
		i += start
		if partial {
			return true
		}
		before, after := rune(' '), rune(' ')
		if i > 0 {
			before = lastRune(s[:i])
		}
		if i+len(t) < len(s) {
			after = firstRune(s[i+len(t):])
		}
		if !unicode.IsLetter(before) && !unicode.IsDigit(before) && !unicode.IsLetter(after) && !unicode.IsDigit(after) {
			return true
		}
		start = i + len(t)
	}
}

func lastRune(s string) rune {
	r := []rune(s)
	return r[len(r)-1]
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return ' '
}

var smallWords = map[string]bool{"a": true, "an": true, "the": true, "and": true, "or": true, "of": true, "to": true, "in": true,
	"on": true, "for": true, "with": true, "at": true, "by": true, "from": true, "und": true, "oder": true, "der": true,
	"die": true, "das": true, "mit": true, "von": true, "für": true, "zum": true, "zur": true, "im": true, "am": true}

func evalTitleCase(d *s1000d.Doc, r Rule, c TitleCaseConfig) []analysis.Finding {
	elems := c.Elements
	if len(elems) == 0 {
		elems = []string{"levelledPara", "figure", "table", "proceduralStep"}
	}
	exc := map[string]bool{}
	for _, e := range c.Exceptions {
		exc[strings.ToLower(e)] = true
	}
	var out []analysis.Finding
	check := func(n *xmldoc.Node, text string) {
		if text == "" {
			return
		}
		if expected := applyCase(text, c.Style, exc); expected != text {
			f := finding(d, n, r, fmt.Sprintf("Titel %q entspricht nicht der Schreibweise %q.", text, c.Style))
			f.Suggestion = expected
			out = append(out, f)
		}
	}
	root := d.Root.Root()
	for _, e := range elems {
		if e == "dmTitle" {
			if t := root.Find("techName"); t != nil {
				check(t, xmldoc.Norm(t.TextContent()))
			}
			continue
		}
		for _, n := range root.FindAll(e) {
			if t := n.Child("title"); t != nil {
				check(t, xmldoc.Norm(t.TextContent()))
			}
		}
	}
	return out
}

func applyCase(text, style string, exc map[string]bool) string {
	words, pos := textutil.WordsWithPos(text)
	b := []byte(text)
	out := string(b)
	replace := func(i int, w string) {
		out = out[:pos[i]] + w + out[pos[i]+len(words[i]):]
	}
	for i := len(words) - 1; i >= 0; i-- {
		w := words[i]
		lw := strings.ToLower(w)
		if exc[lw] || (textutil.IsUpperWord(w) && len([]rune(w)) > 1 && style != "upper") {
			continue
		}
		switch style {
		case "upper":
			replace(i, strings.ToUpper(w))
		case "sentence":
			if i == 0 {
				replace(i, capFirst(lw))
			} else {
				replace(i, lw)
			}
		case "title":
			if i == 0 || !smallWords[lw] {
				replace(i, capFirst(w))
			} else {
				replace(i, lw)
			}
		}
	}
	return out
}

func capFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func evalSentences(d *s1000d.Doc, r Rule, c SentenceConfig) []analysis.Finding {
	if c.Procedural <= 0 {
		c.Procedural = 20
	}
	if c.Descriptive <= 0 {
		c.Descriptive = 25
	}
	var out []analysis.Finding
	for _, p := range d.Root.Root().FindAll("para", "notePara", "warningAndCautionPara") {
		if p.Ancestor("identAndStatusSection") != nil {
			continue
		}
		limit, kind := c.Descriptive, "beschreibenden"
		if p.Ancestor("proceduralStep", "isolationStep") != nil {
			limit, kind = c.Procedural, "prozeduralen"
		}
		text := xmldoc.Norm(p.TextExcluding(func(x *xmldoc.Node) bool {
			return x.Name == "randomList" || x.Name == "sequentialList"
		}))
		for _, s := range textutil.Sentences(text) {
			if n := len(strings.Fields(s)); n > limit {
				f := finding(d, p, r, fmt.Sprintf("Satz mit %d Wörtern überschreitet das Limit von %d Wörtern für %s Text.", n, limit, kind))
				f.Context = textutil.Truncate(s, 240)
				out = append(out, f)
			}
		}
	}
	return out
}
