// Package analysis implements the semantic checks for S-Series exports.
package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"scheck/internal/s1000d"
	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

const (
	SevError   = "error"
	SevWarning = "warning"
	SevInfo    = "info"
)

const (
	CatLists       = "lists"
	CatTitles      = "titles"
	CatReferences  = "references"
	CatConsistency = "consistency"
	CatRules       = "rules"
	CatLLM         = "llm"
	CatImages      = "images"
)

// Finding is a single semantic issue.
type Finding struct {
	FileID     string `json:"fileId"`
	File       string `json:"file"`
	DMC        string `json:"dmc"`
	Category   string `json:"category"`
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Context    string `json:"context"`
	Suggestion string `json:"suggestion"`
}

// Fingerprint identifies a finding across analysis runs (used to keep
// dismissed findings dismissed in later runs).
func (f Finding) Fingerprint() string {
	loc := f.DMC
	if loc == "" {
		loc = f.File
	}
	h := sha256.Sum256([]byte(strings.Join([]string{f.Code, loc, f.Path, f.Message}, "\x1f")))
	return hex.EncodeToString(h[:16])
}

// Corpus is everything known about one snapshot.
type Corpus struct {
	Docs       []*s1000d.Doc
	ICNs       map[string]string // ICN key -> file path
	Dictionary map[string]bool   // lower-case accepted words
}

func (c *Corpus) DMs() []*s1000d.Doc {
	var out []*s1000d.Doc
	for _, d := range c.Docs {
		if d.Kind == s1000d.KindDM || d.Kind == s1000d.KindBREX {
			out = append(out, d)
		}
	}
	return out
}

// Options select which check groups run.
type Options struct {
	Lists       bool `json:"lists"`
	Titles      bool `json:"titles"`
	References  bool `json:"references"`
	Consistency bool `json:"consistency"`
	Rules       bool `json:"rules"`
	LLM         bool `json:"llm"`
	Images      bool `json:"images"`
	LLMMaxDocs  int  `json:"llmMaxDocs"`
}

func DefaultOptions() Options {
	return Options{Lists: true, Titles: true, References: true, Consistency: true, Rules: true, Images: true, LLMMaxDocs: 25}
}

func newFinding(d *s1000d.Doc, n *xmldoc.Node, cat, code, sev, msg string) Finding {
	f := Finding{FileID: d.FileID, File: d.Path, DMC: d.DMC, Category: cat, Code: code, Severity: sev, Message: msg}
	if n != nil {
		f.Path = n.Path()
		f.Line = n.Line
		f.Context = textutil.Truncate(xmldoc.Norm(n.TextContent()), 240)
	}
	return f
}

// RunDeterministic executes all non-LLM checks selected in opts (business
// rules are evaluated separately by the rules package).
func RunDeterministic(c *Corpus, opts Options) []Finding {
	var out []Finding
	if opts.Lists {
		out = append(out, CheckLists(c)...)
	}
	if opts.Titles {
		out = append(out, CheckTitles(c)...)
		out = append(out, CheckSpelling(c)...)
	}
	if opts.References {
		out = append(out, CheckReferences(c)...)
	}
	if opts.Consistency {
		out = append(out, CheckConsistency(c)...)
	}
	return out
}

func q(s string) string { return "„" + textutil.Truncate(s, 120) + "“" }

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
