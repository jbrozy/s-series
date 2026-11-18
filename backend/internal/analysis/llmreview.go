package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"scheck/internal/llm"
	"scheck/internal/s1000d"
	"scheck/internal/textutil"
	"scheck/internal/xmldoc"
)

// ReviewSystemPrompt is shared by the live LLM review and the synthetic
// training data, so a fine-tuned model sees exactly the production format.
const ReviewSystemPrompt = `Du bist ein erfahrener technischer Redakteur und Prüfer für S1000D/S-Series-Dokumentation (z. B. Triebwerks- und Luftfahrtdokumentation).
Du prüfst Inhalte auf semantische und inhaltliche Fehler:
- doppelte oder fast doppelte Listeneinträge, Arbeitsschritte, Ersatzteile oder Hilfsmittel
- doppelte, falsch geschriebene oder uneinheitliche Überschriften
- Tippfehler und falsche Fachbegriffe
- widersprüchliche Angaben (Werte, Einheiten, Drehmomente, Mengen, Teilenummern)
- unlogische Reihenfolge von Arbeitsschritten
- fehlende oder falsch platzierte Sicherheitshinweise (WARNUNG/VORSICHT) vor gefährlichen Tätigkeiten
- uneinheitliche Terminologie und Verstöße gegen die angegebenen Projektregeln
Syntaxfehler (XML-Schema) werden bereits anderweitig geprüft, melde sie nicht.
Antworte ausschließlich mit JSON in dieser Form:
{"findings":[{"line":<Zeilennummer aus [Lnn]>,"code":"<KURZCODE>","severity":"error|warning|info","message":"<Beschreibung auf Deutsch>","suggestion":"<Korrekturvorschlag>"}]}
Wenn nichts zu beanstanden ist, antworte mit {"findings":[]}. Melde nur echte, konkret belegbare Probleme.`

// ReviewUserPrompt builds the user prompt.
func ReviewUserPrompt(rulesText string, related []string, subject string) string {
	var b strings.Builder
	if strings.TrimSpace(rulesText) != "" {
		b.WriteString("Projektregeln:\n" + rulesText + "\n\n")
	}
	if len(related) > 0 {
		b.WriteString("Kontext aus anderen Datenmodulen des Projekts (nur zum Abgleich, nicht prüfen):\n")
		for _, r := range related {
			b.WriteString("- " + r + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Zu prüfender Inhalt:\n")
	b.WriteString(subject)
	return b.String()
}

// LLMFinding is the JSON format produced by the model.
type LLMFinding struct {
	Line       int    `json:"line"`
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Cache stores model answers (implemented with Redis).
type Cache interface {
	Get(key string) (string, bool)
	Set(key, value string)
}

// ReviewWithLLM sends a rendered data module (in windows) to the chat model.
func ReviewWithLLM(ctx context.Context, ep llm.Endpoint, d *s1000d.Doc, rulesText string, related []string, cache Cache) ([]Finding, error) {
	full := s1000d.Render(d, 0)
	windows := splitWindows(full, 14000, 6)
	var out []Finding
	for _, w := range windows {
		user := ReviewUserPrompt(rulesText, related, w)
		key := cacheKey(ep.Model, user)
		answer, ok := "", false
		if cache != nil {
			answer, ok = cache.Get(key)
		}
		if !ok {
			var err error
			answer, err = ep.Chat(ctx, []llm.Message{{Role: "system", Content: ReviewSystemPrompt}, {Role: "user", Content: user}})
			if err != nil {
				return out, err
			}
			if cache != nil {
				cache.Set(key, answer)
			}
		}
		var parsed struct {
			Findings []LLMFinding `json:"findings"`
		}
		if err := json.Unmarshal([]byte(llm.ExtractJSON(answer)), &parsed); err != nil {
			return out, fmt.Errorf("Antwort des Modells ist kein gültiges JSON: %w", err)
		}
		lines := lineIndex(d)
		for _, lf := range parsed.Findings {
			if strings.TrimSpace(lf.Message) == "" {
				continue
			}
			sev := strings.ToLower(lf.Severity)
			if sev != SevError && sev != SevWarning && sev != SevInfo {
				sev = SevWarning
			}
			code := strings.ToUpper(strings.TrimSpace(lf.Code))
			if code == "" {
				code = "REVIEW"
			}
			f := Finding{FileID: d.FileID, File: d.Path, DMC: d.DMC, Category: CatLLM, Code: "LLM_" + strings.TrimPrefix(code, "LLM_"),
				Severity: sev, Message: lf.Message, Suggestion: lf.Suggestion, Line: lf.Line}
			if n := lines[lf.Line]; n != nil {
				f.Path = n.Path()
				f.Context = textutil.Truncate(xmldoc.Norm(n.TextContent()), 240)
			}
			out = append(out, f)
		}
	}
	return out, nil
}

func cacheKey(model, prompt string) string {
	h := sha256.Sum256([]byte(model + "\x00" + ReviewSystemPrompt + "\x00" + prompt))
	return hex.EncodeToString(h[:])
}

func lineIndex(d *s1000d.Doc) map[int]*xmldoc.Node {
	m := map[int]*xmldoc.Node{}
	d.Root.Root().Walk(func(n *xmldoc.Node) bool {
		if _, ok := m[n.Line]; !ok {
			m[n.Line] = n
		}
		return true
	})
	return m
}

func splitWindows(text string, size, maxWindows int) []string {
	if len(text) <= size {
		return []string{text}
	}
	lines := strings.Split(text, "\n")
	head := strings.Join(lines[:min(3, len(lines))], "\n") + "\n"
	var out []string
	var cur strings.Builder
	cur.WriteString(head)
	for _, l := range lines[min(3, len(lines)):] {
		if cur.Len()+len(l) > size {
			out = append(out, cur.String())
			if len(out) >= maxWindows {
				return out
			}
			cur.Reset()
			cur.WriteString(head)
		}
		cur.WriteString(l + "\n")
	}
	if cur.Len() > len(head) {
		out = append(out, cur.String())
	}
	return out
}
