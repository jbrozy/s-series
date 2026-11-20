package server

import (
	"fmt"
	"net/http"
	"strings"
)

// Global search (Strg+K): projects, data modules, open findings and rules
// across all projects the user can open. Data modules and findings come
// from the newest upload of each project.

type GlobalHit struct {
	Kind      string `json:"kind"` // project | file | finding | rule
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	ProjectID string `json:"projectId"`
	Project   string `json:"project"`
	Snapshot  string `json:"snapshotId,omitempty"`
	FileID    string `json:"fileId,omitempty"`
	Line      int    `json:"line,omitempty"`
	Severity  string `json:"severity,omitempty"`
	ID        string `json:"id"`
}

func (s *Server) handleGlobalSearch(w http.ResponseWriter, r *http.Request, u *User) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	out := map[string][]GlobalHit{"projects": {}, "files": {}, "findings": {}, "rules": {}}
	if len([]rune(q)) < 2 {
		writeJSON(w, 200, out)
		return
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	ctx := r.Context()
	// projects the user may open
	access := `SELECT p.id FROM projects p WHERE $1 OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id=$2)`
	latest := `SELECT DISTINCT ON (sn.project_id) sn.id, sn.project_id FROM snapshots sn
		WHERE sn.status='ready' AND sn.project_id IN (` + access + `) ORDER BY sn.project_id, sn.created_at DESC`
	args := []any{u.IsAdmin(), u.ID, like}

	rows, err := s.db.QueryContext(ctx, `SELECT p.id, p.name, p.description FROM projects p WHERE p.id IN (`+access+`)
		AND (p.name ILIKE $3 OR p.description ILIKE $3) ORDER BY p.name LIMIT 8`, args...)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var h GlobalHit
		_ = rows.Scan(&h.ProjectID, &h.Title, &h.Detail)
		h.Kind, h.ID, h.Project = "project", h.ProjectID, h.Title
		out["projects"] = append(out["projects"], h)
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT f.id, f.snapshot_id, l.project_id, p.name, COALESCE(NULLIF(f.dmc,''), f.path), f.title, f.kind
		FROM (`+latest+`) l JOIN files f ON f.snapshot_id=l.id JOIN projects p ON p.id=l.project_id
		WHERE f.kind <> 'other' AND (f.dmc ILIKE $3 OR f.title ILIKE $3 OR f.path ILIKE $3)
		ORDER BY (f.dmc ILIKE $3) DESC, f.dmc, f.path LIMIT 10`, args...)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var h GlobalHit
		var kind string
		_ = rows.Scan(&h.FileID, &h.Snapshot, &h.ProjectID, &h.Project, &h.Title, &h.Detail, &kind)
		h.Kind, h.ID = "file", h.FileID
		if kind == "icn" {
			h.Detail = "Grafik"
		}
		out["files"] = append(out["files"], h)
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT f.id, l.id, l.project_id, p.name, f.message, f.code, COALESCE(NULLIF(f.dmc,''), f.file_path), f.line, f.severity
		FROM (`+latest+`) l
		JOIN LATERAL (SELECT id FROM analysis_runs WHERE snapshot_id=l.id AND status='done' ORDER BY created_at DESC LIMIT 1) r ON true
		JOIN findings f ON f.run_id=r.id JOIN projects p ON p.id=l.project_id
		WHERE f.status IN ('open','accepted') AND (f.message ILIKE $3 OR f.code ILIKE $3 OR f.dmc ILIKE $3)
		ORDER BY CASE f.severity WHEN 'error' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, f.dmc LIMIT 10`, args...)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var h GlobalHit
		var id int64
		var code, loc string
		_ = rows.Scan(&id, &h.Snapshot, &h.ProjectID, &h.Project, &h.Title, &code, &loc, &h.Line, &h.Severity)
		h.Kind, h.ID = "finding", fmt.Sprint(id)
		h.Detail = code + " · " + loc
		out["findings"] = append(out["findings"], h)
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT br.id, br.project_id, p.name, br.name, br.type FROM business_rules br JOIN projects p ON p.id=br.project_id
		WHERE br.project_id IN (`+access+`) AND br.name ILIKE $3 ORDER BY br.name LIMIT 6`, args...)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var h GlobalHit
		_ = rows.Scan(&h.ID, &h.ProjectID, &h.Project, &h.Title, &h.Detail)
		h.Kind = "rule"
		out["rules"] = append(out["rules"], h)
	}
	rows.Close()
	writeJSON(w, 200, out)
}
