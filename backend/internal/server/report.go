package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"scheck/internal/rules"
)

// handleReport returns everything the printable inspection report needs for
// one upload: identity, analysis, gate, release decision, comparison with the
// previous upload, open findings and justified dismissals.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	pid, err := s.requireSnapshot(r.Context(), sid, u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	ctx := r.Context()
	sn, err := s.loadSnapshot(ctx, sid)
	if err != nil {
		fail(w, err)
		return
	}
	proj, err := scanProject(s.db.QueryRowContext(ctx, projectSelect+` WHERE p.id=$1`, pid))
	if err != nil {
		fail(w, err)
		return
	}
	role, _ := s.projectRole(ctx, pid, u)
	rep := map[string]any{
		"generatedAt": time.Now(),
		"generatedBy": u.Name,
		"project":     map[string]any{"id": proj.ID, "name": proj.Name, "standard": proj.Standard, "description": proj.Description},
		"snapshot":    sn,
		"release":     s.releaseState(ctx, sid, pid, role),
	}
	run, err := s.latestRun(ctx, sid)
	if err != nil || run.Status != "done" {
		rep["run"] = run
		writeJSON(w, 200, rep)
		return
	}
	rep["run"] = run

	// matrix category x severity (open = not dismissed, not fixed)
	matrix := map[string]map[string]int{}
	statusCount := map[string]int{}
	rows, err := s.db.QueryContext(ctx, `SELECT category, severity, status, count(*) FROM findings WHERE run_id=$1 GROUP BY 1,2,3`, run.ID)
	if err == nil {
		for rows.Next() {
			var cat, sev, st string
			var n int
			_ = rows.Scan(&cat, &sev, &st, &n)
			statusCount[st] += n
			if st == "open" || st == "accepted" {
				if matrix[cat] == nil {
					matrix[cat] = map[string]int{}
				}
				matrix[cat][sev] += n
			}
		}
		rows.Close()
	}
	rep["matrix"] = matrix
	rep["statusCount"] = statusCount

	fq := s.newFindingQuery(ctx, run.ID, pid)
	if fq.prev != "" {
		var label string
		_ = s.db.QueryRowContext(ctx, `SELECT sn.label FROM analysis_runs r JOIN snapshots sn ON sn.id=r.snapshot_id WHERE r.id=$1`, fq.prev).Scan(&label)
		c := s.findingFacets(ctx, fq, u)["change"]
		rep["comparison"] = map[string]any{"previousLabel": label, "new": c["new"], "persisting": c["persisting"], "resolved": c["resolved"]}
	}

	list := func(cond string) []FindingRow {
		out := []FindingRow{}
		rows, err := s.db.QueryContext(ctx, findingSelect+` WHERE f.run_id=$1::uuid AND `+cond+findingOrder+` LIMIT 2000`, fq.args()...)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			if f, err := scanFinding(rows); err == nil {
				out = append(out, f)
			}
		}
		return out
	}
	rep["open"] = list("f.status IN ('open','accepted')")
	rep["dismissed"] = list("f.status = 'dismissed'")

	imgs := map[string]int{}
	rows, err = s.db.QueryContext(ctx, `SELECT status, count(*) FROM images WHERE snapshot_id=$1 GROUP BY 1`, sid)
	if err == nil {
		for rows.Next() {
			var st string
			var n int
			_ = rows.Scan(&st, &n)
			imgs[st] = n
		}
		rows.Close()
	}
	rep["images"] = imgs

	var nRules, nActive int
	_ = s.db.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE enabled) FROM business_rules WHERE project_id=$1`, pid).Scan(&nRules, &nActive)
	rep["rules"] = map[string]int{"total": nRules, "active": nActive}
	var recent bool
	_ = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM audit_log WHERE action='report.view' AND target_id=$1 AND user_id=$2 AND at > now() - interval '10 minutes')`, sid, u.ID).Scan(&recent)
	if !recent {
		s.audit(r, u, pid, "report.view", "snapshot", sid, "Prüfbericht für „"+sn.Label+"“ erstellt", nil)
	}
	writeJSON(w, 200, rep)
}

// ---------------------------------------------------------------- rule sets

type ruleSetFile struct {
	Format     string       `json:"format"`
	Version    int          `json:"version"`
	Project    string       `json:"project"`
	ExportedAt time.Time    `json:"exportedAt"`
	Rules      []rules.Rule `json:"rules"`
}

const ruleSetFormat = "s-check-rules"

func (s *Server) projectRules(r *http.Request, pid string) ([]rules.Rule, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, type, severity, enabled, source, config FROM business_rules WHERE project_id=$1 ORDER BY created_at`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rules.Rule{}
	for rows.Next() {
		var rl rules.Rule
		var cfg []byte
		if err := rows.Scan(&rl.ID, &rl.Name, &rl.Type, &rl.Severity, &rl.Enabled, &rl.Source, &cfg); err != nil {
			return nil, err
		}
		rl.Config = cfg
		out = append(out, rl)
	}
	return out, nil
}

func (s *Server) handleExportRules(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	rs, err := s.projectRules(r, pid)
	if err != nil {
		fail(w, err)
		return
	}
	var name string
	_ = s.db.QueryRowContext(r.Context(), `SELECT name FROM projects WHERE id=$1`, pid).Scan(&name)
	for i := range rs {
		rs[i].ID = ""
	}
	file := ruleSetFile{Format: ruleSetFormat, Version: 1, Project: name, ExportedAt: time.Now(), Rules: rs}
	slug := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, name)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="regeln-%s.json"`, slug))
	s.audit(r, u, pid, "rule.export", "project", pid, fmt.Sprintf("%d Regeln exportiert", len(rs)), nil)
	writeJSON(w, 200, file)
}

// handleImportRules adds rules from a rule set file or another project.
// Rules with the same name and type as an existing rule are skipped.
func (s *Server) handleImportRules(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "editor"); err != nil {
		fail(w, err)
		return
	}
	var in struct {
		FromProject string          `json:"fromProject"`
		File        json.RawMessage `json:"file"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	var incoming []rules.Rule
	source := ""
	switch {
	case in.FromProject != "":
		if in.FromProject == pid {
			writeErr(w, 400, "Quelle und Ziel sind dasselbe Projekt.")
			return
		}
		if _, err := s.requireProject(r.Context(), in.FromProject, u, "viewer"); err != nil {
			fail(w, err)
			return
		}
		var err error
		if incoming, err = s.projectRules(r, in.FromProject); err != nil {
			fail(w, err)
			return
		}
		_ = s.db.QueryRowContext(r.Context(), `SELECT name FROM projects WHERE id=$1`, in.FromProject).Scan(&source)
		source = "Projekt „" + source + "“"
	case len(in.File) > 0:
		var f ruleSetFile
		if err := json.Unmarshal(in.File, &f); err != nil || f.Format != ruleSetFormat {
			writeErr(w, 400, "Die Datei ist kein S-Check-Regelset (erwartet: Export aus „Regeln exportieren“).")
			return
		}
		incoming = f.Rules
		source = "Datei (exportiert aus „" + f.Project + "“)"
	default:
		writeErr(w, 400, "Bitte eine Regeldatei oder ein Quellprojekt angeben.")
		return
	}
	existing, err := s.projectRules(r, pid)
	if err != nil {
		fail(w, err)
		return
	}
	have := map[string]bool{}
	for _, rl := range existing {
		have[rl.Type+"\x1f"+strings.ToLower(rl.Name)] = true
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	imported, skipped := 0, 0
	var invalid []string
	for _, rl := range incoming {
		key := rl.Type + "\x1f" + strings.ToLower(rl.Name)
		if have[key] {
			skipped++
			continue
		}
		if err := rules.Validate(rl); err != nil {
			invalid = append(invalid, rl.Name+": "+err.Error())
			continue
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO business_rules(project_id, name, type, severity, enabled, source, config)
			VALUES ($1,$2,$3,$4,$5,'manual',$6)`, pid, rl.Name, rl.Type, rl.Severity, rl.Enabled, string(nonEmptyJSON(rl.Config))); err != nil {
			fail(w, err)
			return
		}
		have[key] = true
		imported++
	}
	if err := tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, u, pid, "rule.import", "project", pid, fmt.Sprintf("%d Regeln aus %s übernommen, %d bereits vorhanden", imported, source, skipped),
		map[string]any{"invalid": invalid})
	writeJSON(w, 200, map[string]any{"imported": imported, "skipped": skipped, "invalid": invalid})
}
