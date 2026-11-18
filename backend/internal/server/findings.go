package server

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"scheck/internal/analysis"
)

// Findings belong to an analysis run. Workflow data (assignee, comments)
// is keyed by project and fingerprint, so it follows a finding from one
// upload to the next until the finding disappears.

type FindingRow struct {
	ID           int64  `json:"id"`
	FileID       string `json:"fileId"`
	File         string `json:"file"`
	DMC          string `json:"dmc"`
	Category     string `json:"category"`
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Path         string `json:"path"`
	Line         int    `json:"line"`
	Context      string `json:"context"`
	Suggestion   string `json:"suggestion"`
	Status       string `json:"status"`
	StatusNote   string `json:"statusNote"`
	Fingerprint  string `json:"fingerprint"`
	AssigneeID   string `json:"assigneeId"`
	AssigneeName string `json:"assigneeName"`
	Comments     int    `json:"comments"`
	Change       string `json:"change"` // new | persisting | "" (no previous upload to compare with)
}

// findingQuery holds the fixed parameters of every finding query:
// $1 run, $2 project, $3 run of the previous upload ("" when there is none).
type findingQuery struct {
	run, project, prev string
}

func (q findingQuery) args() []any { return []any{q.run, q.project, q.prev} }

const findingSelect = `SELECT f.id, COALESCE(f.file_id::text,''), f.file_path, f.dmc, f.category, f.code, f.severity, f.message, f.xpath, f.line,
	f.context, f.suggestion, f.status, f.status_note, f.fingerprint,
	COALESCE(t.assignee_id::text,''), COALESCE(au.name,''),
	(SELECT count(*) FROM finding_comments c WHERE c.project_id=$2::uuid AND c.fingerprint=f.fingerprint),
	CASE WHEN $3::text = '' THEN ''
	     WHEN EXISTS (SELECT 1 FROM findings p WHERE p.run_id::text=$3 AND p.fingerprint=f.fingerprint) THEN 'persisting'
	     ELSE 'new' END
	FROM findings f
	LEFT JOIN finding_tracks t ON t.project_id=$2::uuid AND t.fingerprint=f.fingerprint
	LEFT JOIN users au ON au.id=t.assignee_id`

const findingOrder = ` ORDER BY CASE f.severity WHEN 'error' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, f.dmc, f.file_path, f.line, f.id`

func scanFinding(sc interface{ Scan(...any) error }) (FindingRow, error) {
	var f FindingRow
	err := sc.Scan(&f.ID, &f.FileID, &f.File, &f.DMC, &f.Category, &f.Code, &f.Severity, &f.Message, &f.Path, &f.Line, &f.Context,
		&f.Suggestion, &f.Status, &f.StatusNote, &f.Fingerprint, &f.AssigneeID, &f.AssigneeName, &f.Comments, &f.Change)
	return f, err
}

// previousRun returns the latest completed run of the upload before the
// one analysed by rid, or "".
func (s *Server) previousRun(ctx context.Context, rid string) string {
	var prev string
	_ = s.db.QueryRowContext(ctx, `SELECT r2.id FROM analysis_runs r
		JOIN snapshots sn ON sn.id=r.snapshot_id
		JOIN snapshots s2 ON s2.project_id=sn.project_id AND s2.created_at < sn.created_at
		JOIN LATERAL (SELECT id FROM analysis_runs WHERE snapshot_id=s2.id AND status='done' ORDER BY created_at DESC LIMIT 1) r2 ON true
		WHERE r.id::text=$1 ORDER BY s2.created_at DESC LIMIT 1`, rid).Scan(&prev)
	return prev
}

func (s *Server) newFindingQuery(ctx context.Context, rid, pid string) findingQuery {
	return findingQuery{run: rid, project: pid, prev: s.previousRun(ctx, rid)}
}

// filter turns query parameters into conditions on top of the fixed ones.
func (q findingQuery) filter(r *http.Request, u *User) (string, []any) {
	p := r.URL.Query()
	where := []string{"f.run_id=$1::uuid"}
	args := q.args()
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	for _, c := range []struct{ param, col string }{{"category", "f.category"}, {"severity", "f.severity"}, {"status", "f.status"}, {"file", "f.file_id::text"}, {"code", "f.code"}} {
		if v := p.Get(c.param); v != "" {
			add(c.col+" = ANY(string_to_array(?, ','))", v)
		}
	}
	if v := strings.TrimSpace(p.Get("q")); v != "" {
		add("(f.message ILIKE ? OR f.dmc ILIKE ? OR f.file_path ILIKE ? OR f.context ILIKE ? OR f.code ILIKE ?)", "%"+v+"%")
	}
	switch v := p.Get("assignee"); v {
	case "":
	case "me":
		add("t.assignee_id::text = ?", u.ID)
	case "none":
		where = append(where, "t.assignee_id IS NULL")
	default:
		add("t.assignee_id::text = ?", v)
	}
	switch p.Get("change") {
	case "new":
		where = append(where, "$3::text <> '' AND NOT EXISTS (SELECT 1 FROM findings p WHERE p.run_id::text=$3 AND p.fingerprint=f.fingerprint)")
	case "persisting":
		where = append(where, "EXISTS (SELECT 1 FROM findings p WHERE p.run_id::text=$3 AND p.fingerprint=f.fingerprint)")
	}
	return strings.Join(where, " AND "), args
}

func (s *Server) handleListFindings(w http.ResponseWriter, r *http.Request, u *User) {
	rid := r.PathValue("rid")
	_, pid, err := s.requireRun(r.Context(), rid, u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	fq := s.newFindingQuery(r.Context(), rid, pid)
	where, args := fq.filter(r, u)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM findings f LEFT JOIN finding_tracks t ON t.project_id=$2::uuid AND t.fingerprint=f.fingerprint
		WHERE $3::text IS NOT NULL AND `+where, args...).Scan(&total); err != nil {
		fail(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), findingSelect+` WHERE `+where+findingOrder+
		` LIMIT `+strconv.Itoa(limit)+` OFFSET `+strconv.Itoa(max(0, offset)), args...)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	items := []FindingRow{}
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			fail(w, err)
			return
		}
		items = append(items, f)
	}
	writeJSON(w, 200, map[string]any{"total": total, "items": items, "facets": s.findingFacets(r.Context(), fq, u), "previousRun": fq.prev})
}

func (s *Server) findingFacets(ctx context.Context, fq findingQuery, u *User) map[string]map[string]int {
	facets := map[string]map[string]int{"category": {}, "severity": {}, "status": {}, "change": {}, "assignee": {}}
	for _, col := range []string{"category", "severity", "status"} {
		rows, err := s.db.QueryContext(ctx, `SELECT `+col+`, count(*) FROM findings WHERE run_id=$1 GROUP BY 1`, fq.run)
		if err != nil {
			continue
		}
		for rows.Next() {
			var k string
			var n int
			_ = rows.Scan(&k, &n)
			facets[col][k] = n
		}
		rows.Close()
	}
	if fq.prev != "" {
		var newN, persisting, resolved int
		_ = s.db.QueryRowContext(ctx, `SELECT
			count(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM findings p WHERE p.run_id=$2 AND p.fingerprint=f.fingerprint)),
			count(*) FILTER (WHERE EXISTS (SELECT 1 FROM findings p WHERE p.run_id=$2 AND p.fingerprint=f.fingerprint))
			FROM findings f WHERE f.run_id=$1`, fq.run, fq.prev).Scan(&newN, &persisting)
		_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM findings p WHERE p.run_id=$2 AND p.status<>'dismissed'
			AND NOT EXISTS (SELECT 1 FROM findings f WHERE f.run_id=$1 AND f.fingerprint=p.fingerprint)`, fq.run, fq.prev).Scan(&resolved)
		facets["change"] = map[string]int{"new": newN, "persisting": persisting, "resolved": resolved}
	}
	var mine, unassigned int
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE t.assignee_id::text=$3), count(*) FILTER (WHERE t.assignee_id IS NULL)
		FROM findings f LEFT JOIN finding_tracks t ON t.project_id=$2 AND t.fingerprint=f.fingerprint WHERE f.run_id=$1`,
		fq.run, fq.project, u.ID).Scan(&mine, &unassigned)
	facets["assignee"] = map[string]int{"me": mine, "none": unassigned}
	return facets
}

// handleResolvedFindings lists findings of the previous upload that are gone
// in this one: they were fixed (or the content changed).
func (s *Server) handleResolvedFindings(w http.ResponseWriter, r *http.Request, u *User) {
	rid := r.PathValue("rid")
	_, pid, err := s.requireRun(r.Context(), rid, u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	prev := s.previousRun(r.Context(), rid)
	items := []FindingRow{}
	if prev == "" {
		writeJSON(w, 200, map[string]any{"previousRun": "", "items": items})
		return
	}
	// show the previous findings; "change" is computed against the run before that
	fq := findingQuery{run: prev, project: pid, prev: ""}
	rows, err := s.db.QueryContext(r.Context(), findingSelect+` WHERE f.run_id=$1::uuid AND f.status<>'dismissed'
		AND NOT EXISTS (SELECT 1 FROM findings c WHERE c.run_id::text=$4 AND c.fingerprint=f.fingerprint)`+findingOrder+` LIMIT 1000`,
		append(fq.args(), rid)...)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			fail(w, err)
			return
		}
		f.Change = "resolved"
		items = append(items, f)
	}
	var label string
	_ = s.db.QueryRowContext(r.Context(), `SELECT sn.label FROM analysis_runs r JOIN snapshots sn ON sn.id=r.snapshot_id WHERE r.id=$1`, prev).Scan(&label)
	writeJSON(w, 200, map[string]any{"previousRun": prev, "previousLabel": label, "items": items})
}

var categoryDE = map[string]string{
	analysis.CatLists: "Listen", analysis.CatTitles: "Überschriften/Schreibung", analysis.CatReferences: "Verweise",
	analysis.CatConsistency: "Konsistenz", analysis.CatRules: "Business Rules", analysis.CatLLM: "KI-Review", analysis.CatImages: "Bilder",
}
var severityDE = map[string]string{"error": "Fehler", "warning": "Warnung", "info": "Hinweis"}
var statusDE = map[string]string{"open": "offen", "accepted": "bestätigt", "dismissed": "verworfen", "fixed": "behoben"}
var changeDE = map[string]string{"new": "neu", "persisting": "unverändert", "": ""}

func (s *Server) writeFindingsCSV(ctx context.Context, w io.Writer, rid, pid string) error {
	_, _ = w.Write([]byte("\xef\xbb\xbf")) // BOM for Excel
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{"Schwere", "Kategorie", "Code", "DMC", "Datei", "Zeile", "Pfad", "Meldung", "Kontext", "Vorschlag", "Status",
		"Begründung", "Zuständig", "Kommentare", "Gegenüber Vorgänger"})
	fq := s.newFindingQuery(ctx, rid, pid)
	rows, err := s.db.QueryContext(ctx, findingSelect+` WHERE f.run_id=$1::uuid`+findingOrder, fq.args()...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return err
		}
		_ = cw.Write([]string{severityDE[f.Severity], categoryDE[f.Category], f.Code, f.DMC, f.File, strconv.Itoa(f.Line), f.Path,
			f.Message, f.Context, f.Suggestion, statusDE[f.Status], f.StatusNote, f.AssigneeName, strconv.Itoa(f.Comments), changeDE[f.Change]})
	}
	cw.Flush()
	return cw.Error()
}

func (s *Server) handleFindingsCSV(w http.ResponseWriter, r *http.Request, u *User) {
	rid := r.PathValue("rid")
	_, pid, err := s.requireRun(r.Context(), rid, u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "befunde-" + rid[:8] + ".csv"}))
	_ = s.writeFindingsCSV(r.Context(), w, rid, pid)
}

// requireFinding loads the identifying data of a finding and checks access.
type findingRef struct {
	id, run, project, fingerprint, status, code, location string
}

func (s *Server) requireFinding(ctx context.Context, fid string, u *User, min string) (findingRef, error) {
	ref := findingRef{id: fid}
	if err := s.db.QueryRowContext(ctx, `SELECT run_id, fingerprint, status, code, COALESCE(NULLIF(dmc,''), file_path) FROM findings WHERE id::text=$1`, fid).
		Scan(&ref.run, &ref.fingerprint, &ref.status, &ref.code, &ref.location); err != nil {
		return ref, errNotFound
	}
	_, pid, err := s.requireRun(ctx, ref.run, u, min)
	ref.project = pid
	return ref, err
}

func (s *Server) findingResponse(w http.ResponseWriter, r *http.Request, ref findingRef) {
	fq := s.newFindingQuery(r.Context(), ref.run, ref.project)
	f, err := scanFinding(s.db.QueryRowContext(r.Context(), findingSelect+` WHERE f.run_id=$1::uuid AND f.id=$4`, append(fq.args(), ref.id)...))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, f)
}

func (s *Server) handleUpdateFinding(w http.ResponseWriter, r *http.Request, u *User) {
	ref, err := s.requireFinding(r.Context(), r.PathValue("fid"), u, "editor")
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	if _, ok := statusDE[in.Status]; !ok {
		writeErr(w, 400, "Ungültiger Status")
		return
	}
	if in.Status == "dismissed" && strings.TrimSpace(in.Note) == "" {
		writeErr(w, 400, "Bitte begründen, warum der Befund kein Fehler ist. Die Begründung steht im Protokoll und im Prüfbericht.")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE findings SET status=$1, status_note=$2 WHERE id=$3`, in.Status, in.Note, ref.id); err != nil {
		fail(w, err)
		return
	}
	if in.Status == "dismissed" {
		_, _ = s.db.ExecContext(r.Context(), `INSERT INTO finding_suppressions(project_id, fingerprint, note, created_by) VALUES ($1,$2,$3,$4)
			ON CONFLICT (project_id, fingerprint) DO UPDATE SET note=EXCLUDED.note`, ref.project, ref.fingerprint, in.Note, u.ID)
	} else {
		_, _ = s.db.ExecContext(r.Context(), `DELETE FROM finding_suppressions WHERE project_id=$1 AND fingerprint=$2`, ref.project, ref.fingerprint)
	}
	s.audit(r, u, ref.project, "finding.status", "finding", ref.fingerprint,
		fmt.Sprintf("Befund %s in %s: %s → %s", ref.code, ref.location, statusDE[ref.status], statusDE[in.Status]),
		map[string]any{"from": ref.status, "to": in.Status, "note": in.Note, "findingId": ref.id})
	s.findingResponse(w, r, ref)
}

func (s *Server) handleAssignFinding(w http.ResponseWriter, r *http.Request, u *User) {
	ref, err := s.requireFinding(r.Context(), r.PathValue("fid"), u, "editor")
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		UserID string `json:"userId"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	var prevName string
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(u.name,'') FROM finding_tracks t LEFT JOIN users u ON u.id=t.assignee_id
		WHERE t.project_id=$1 AND t.fingerprint=$2`, ref.project, ref.fingerprint).Scan(&prevName)
	summary := ""
	if in.UserID == "" {
		if _, err := s.db.ExecContext(r.Context(), `DELETE FROM finding_tracks WHERE project_id=$1 AND fingerprint=$2`, ref.project, ref.fingerprint); err != nil {
			fail(w, err)
			return
		}
		summary = fmt.Sprintf("Zuweisung von Befund %s in %s aufgehoben", ref.code, ref.location)
	} else {
		var name string
		if err := s.db.QueryRowContext(r.Context(), `SELECT u.name FROM users u JOIN project_members m ON m.user_id=u.id
			WHERE m.project_id=$1 AND u.id::text=$2 AND NOT u.disabled`, ref.project, in.UserID).Scan(&name); err != nil {
			writeErr(w, 400, "Zuweisen geht nur an aktive Mitglieder dieses Projekts.")
			return
		}
		if _, err := s.db.ExecContext(r.Context(), `INSERT INTO finding_tracks(project_id, fingerprint, assignee_id, assigned_by) VALUES ($1,$2,$3,$4)
			ON CONFLICT (project_id, fingerprint) DO UPDATE SET assignee_id=EXCLUDED.assignee_id, assigned_by=EXCLUDED.assigned_by, assigned_at=now()`,
			ref.project, ref.fingerprint, in.UserID, u.ID); err != nil {
			fail(w, err)
			return
		}
		summary = fmt.Sprintf("Befund %s in %s an %s zugewiesen", ref.code, ref.location, name)
	}
	s.audit(r, u, ref.project, "finding.assign", "finding", ref.fingerprint, summary, map[string]any{"from": prevName, "to": in.UserID, "findingId": ref.id})
	s.findingResponse(w, r, ref)
}

type Activity struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"` // comment | event
	User    string    `json:"user"`
	Text    string    `json:"text"`
	Action  string    `json:"action,omitempty"`
	Comment int64     `json:"commentId,omitempty"`
}

func (s *Server) handleFindingActivity(w http.ResponseWriter, r *http.Request, u *User) {
	ref, err := s.requireFinding(r.Context(), r.PathValue("fid"), u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	out := []Activity{}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, created_at, user_name, body FROM finding_comments WHERE project_id=$1 AND fingerprint=$2`, ref.project, ref.fingerprint)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var a Activity
		_ = rows.Scan(&a.Comment, &a.At, &a.User, &a.Text)
		a.Kind = "comment"
		out = append(out, a)
	}
	rows.Close()
	rows, err = s.db.QueryContext(r.Context(), `SELECT at, user_name, summary, action FROM audit_log
		WHERE project_id=$1 AND target_type='finding' AND target_id=$2 AND action<>'finding.comment'`, ref.project, ref.fingerprint)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var a Activity
		_ = rows.Scan(&a.At, &a.User, &a.Text, &a.Action)
		a.Kind = "event"
		out = append(out, a)
	}
	rows.Close()
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	writeJSON(w, 200, out)
}

func (s *Server) handleCommentFinding(w http.ResponseWriter, r *http.Request, u *User) {
	ref, err := s.requireFinding(r.Context(), r.PathValue("fid"), u, "editor")
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" || len(in.Body) > 4000 {
		writeErr(w, 400, "Kommentar darf nicht leer sein und höchstens 4000 Zeichen haben.")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO finding_comments(project_id, fingerprint, user_id, user_name, body) VALUES ($1,$2,$3,$4,$5)`,
		ref.project, ref.fingerprint, u.ID, u.Name, in.Body); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, u, ref.project, "finding.comment", "finding", ref.fingerprint, fmt.Sprintf("Kommentar zu Befund %s in %s", ref.code, ref.location),
		map[string]any{"findingId": ref.id})
	s.handleFindingActivity(w, r, u)
}
