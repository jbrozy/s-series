package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"scheck/internal/rules"
	"scheck/internal/s1000d"
)

func ruleTemplates() []rules.Rule {
	mk := func(name, typ, sev string, enabled bool, cfg any) rules.Rule {
		b, _ := json.Marshal(cfg)
		return rules.Rule{Name: name, Type: typ, Severity: sev, Enabled: enabled, Source: "template", Config: b}
	}
	return []rules.Rule{
		mk("Unklare Formulierungen", rules.TypeForbiddenTerms, "warning", true, rules.TermsConfig{Scope: "all", Terms: []rules.Term{
			{Term: "etc.", Replacement: "alle Elemente vollständig aufzählen", Note: "Aufzählungen müssen vollständig sein."},
			{Term: "usw.", Replacement: "alle Elemente vollständig aufzählen"},
			{Term: "and/or", Replacement: "„and“ oder „or“ eindeutig verwenden"},
			{Term: "und/oder", Replacement: "„und“ oder „oder“ eindeutig verwenden"},
			{Term: "bzw.", Replacement: "Alternativen ausformulieren"},
			{Term: "ggf.", Replacement: "Bedingung konkret benennen"},
		}}),
		mk("Satzlänge (ASD-STE100)", rules.TypeSentenceLength, "info", true, rules.SentenceConfig{Procedural: 20, Descriptive: 25}),
		mk("Projektwörterbuch", rules.TypeDictionary, "info", true, rules.DictionaryConfig{Words: []string{}}),
		mk("Titel in Satzschreibung (englische Projekte)", rules.TypeTitleCase, "warning", false,
			rules.TitleCaseConfig{Style: "sentence", Elements: []string{"levelledPara", "figure", "table"}}),
		mk("Keine Hervorhebungen in Titeln", rules.TypeXPathForbidden, "warning", false,
			rules.XPathConfig{XPath: "//title//emphasis", Message: "Titel dürfen keine Hervorhebungen (emphasis) enthalten."}),
		mk("Sicherheitsklassifikation 01", rules.TypeXPathValues, "error", false,
			rules.XPathConfig{XPath: "//dmStatus/security/@securityClassification", Allowed: []string{"01"},
				Message: "Datenmodule müssen als 01 (nicht klassifiziert) gekennzeichnet sein."}),
		mk("Prozeduren mit Sicherheitshinweisen", rules.TypeXPathRequired, "info", false,
			rules.XPathConfig{XPath: "//warning | //caution | //@warningRefs | //@cautionRefs",
				Message:   "Prozedur enthält keinen Warn- oder Vorsichtshinweis. Prüfen, ob Gefährdungen bestehen.",
				AppliesTo: rules.AppliesTo{Contents: []string{"procedure"}}}),
	}
}

func seedDefaultRules(ctx context.Context, tx *sql.Tx, pid string) error {
	for _, r := range ruleTemplates() {
		if _, err := tx.ExecContext(ctx, `INSERT INTO business_rules(project_id, name, type, severity, enabled, source, config)
			VALUES ($1,$2,$3,$4,$5,'manual',$6)`, pid, r.Name, r.Type, r.Severity, r.Enabled, string(r.Config)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) loadRules(ctx context.Context, pid string) ([]rules.Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type, severity, enabled, source, config FROM business_rules
		WHERE project_id=$1 ORDER BY source, created_at`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rules.Rule{}
	for rows.Next() {
		var r rules.Rule
		var cfg string
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Severity, &r.Enabled, &r.Source, &cfg); err != nil {
			return nil, err
		}
		r.Config = json.RawMessage(cfg)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Server) handleRuleTemplates(w http.ResponseWriter, r *http.Request, _ *User) {
	writeJSON(w, 200, map[string]any{"types": rules.Types, "templates": ruleTemplates()})
}

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	rs, err := s.loadRules(r.Context(), pid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, rs)
}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "editor"); err != nil {
		fail(w, err)
		return
	}
	var in rules.Rule
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeErr(w, 400, "Name fehlt")
		return
	}
	if err := rules.Validate(in); err != nil {
		writeErr(w, 400, "Regel ungültig: "+err.Error())
		return
	}
	var id string
	if err := s.db.QueryRowContext(r.Context(), `INSERT INTO business_rules(project_id, name, type, severity, enabled, source, config)
		VALUES ($1,$2,$3,$4,$5,'manual',$6) RETURNING id`, pid, in.Name, in.Type, in.Severity, in.Enabled, string(in.Config)).Scan(&id); err != nil {
		fail(w, err)
		return
	}
	in.ID, in.Source = id, "manual"
	s.audit(r, u, pid, "rule.create", "rule", in.ID, "Regel „"+in.Name+"“ angelegt", map[string]any{"type": in.Type, "severity": in.Severity, "config": in.Config})
	writeJSON(w, 201, in)
}

func (s *Server) ruleProject(ctx context.Context, id string) (string, error) {
	var pid string
	if err := s.db.QueryRowContext(ctx, `SELECT project_id FROM business_rules WHERE id::text=$1`, id).Scan(&pid); err != nil {
		return "", errNotFound
	}
	return pid, nil
}

func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request, u *User) {
	id := r.PathValue("id")
	pid, err := s.ruleProject(r.Context(), id)
	if err == nil {
		_, err = s.requireProject(r.Context(), pid, u, "editor")
	}
	if err != nil {
		fail(w, err)
		return
	}
	var in rules.Rule
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	if err := rules.Validate(in); err != nil {
		writeErr(w, 400, "Regel ungültig: "+err.Error())
		return
	}
	var old struct {
		Name, Severity string
		Enabled        bool
		Config         []byte
	}
	_ = s.db.QueryRowContext(r.Context(), `SELECT name, severity, enabled, config FROM business_rules WHERE id=$1`, id).Scan(&old.Name, &old.Severity, &old.Enabled, &old.Config)
	if _, err := s.db.ExecContext(r.Context(), `UPDATE business_rules SET name=$1, type=$2, severity=$3, enabled=$4, config=$5, updated_at=now()
		WHERE id=$6`, in.Name, in.Type, in.Severity, in.Enabled, string(in.Config), id); err != nil {
		fail(w, err)
		return
	}
	in.ID = id
	summary := "Regel „" + in.Name + "“ geändert"
	if old.Enabled != in.Enabled && old.Name == in.Name && old.Severity == in.Severity && string(old.Config) == string(in.Config) {
		summary = map[bool]string{true: "Regel „" + in.Name + "“ aktiviert", false: "Regel „" + in.Name + "“ deaktiviert"}[in.Enabled]
	}
	s.audit(r, u, pid, "rule.update", "rule", id, summary, map[string]any{
		"before": map[string]any{"name": old.Name, "severity": old.Severity, "enabled": old.Enabled, "config": json.RawMessage(nonEmptyJSON(old.Config))},
		"after":  map[string]any{"name": in.Name, "severity": in.Severity, "enabled": in.Enabled, "config": in.Config}})
	writeJSON(w, 200, in)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request, u *User) {
	id := r.PathValue("id")
	pid, err := s.ruleProject(r.Context(), id)
	if err == nil {
		_, err = s.requireProject(r.Context(), pid, u, "editor")
	}
	if err != nil {
		fail(w, err)
		return
	}
	var name string
	_ = s.db.QueryRowContext(r.Context(), `SELECT name FROM business_rules WHERE id=$1`, id).Scan(&name)
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM business_rules WHERE id=$1`, id); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, u, pid, "rule.delete", "rule", id, "Regel „"+name+"“ gelöscht", nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleImportBREX converts BREX data modules of a snapshot into rules.
// Previously imported BREX rules of the project are replaced.
func (s *Server) handleImportBREX(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "editor"); err != nil {
		fail(w, err)
		return
	}
	var in struct {
		SnapshotID string `json:"snapshotId"`
		FileID     string `json:"fileId"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	q := `SELECT f.id, f.path FROM files f JOIN snapshots s ON s.id=f.snapshot_id WHERE s.project_id=$1 AND f.snapshot_id::text=$2 AND f.kind='brex'`
	args := []any{pid, in.SnapshotID}
	if in.FileID != "" {
		q += ` AND f.id::text=$3`
		args = append(args, in.FileID)
	}
	rows, err := s.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		fail(w, err)
		return
	}
	type ref struct{ id, path string }
	var files []ref
	for rows.Next() {
		var f ref
		_ = rows.Scan(&f.id, &f.path)
		files = append(files, f)
	}
	rows.Close()
	if len(files) == 0 {
		writeErr(w, 404, "Kein BREX-Datenmodul im Snapshot gefunden")
		return
	}
	var imported []rules.Rule
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(s.snapshotDir(in.SnapshotID), filepath.FromSlash(f.path)))
		if err != nil {
			continue
		}
		d, err := s1000d.Parse(f.id, f.path, data)
		if err != nil {
			continue
		}
		imported = append(imported, rules.FromBREX(d)...)
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM business_rules WHERE project_id=$1 AND source='brex'`, pid); err != nil {
		fail(w, err)
		return
	}
	for _, rl := range imported {
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO business_rules(project_id, name, type, severity, enabled, source, config)
			VALUES ($1,$2,$3,$4,$5,'brex',$6)`, pid, rl.Name, rl.Type, rl.Severity, rl.Enabled, string(rl.Config)); err != nil {
			fail(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, u, pid, "rule.import_brex", "snapshot", in.SnapshotID, fmt.Sprintf("%d Regeln aus BREX importiert (bisherige BREX-Regeln ersetzt)", len(imported)), nil)
	writeJSON(w, 200, map[string]int{"imported": len(imported)})
}

func nonEmptyJSON(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}
