package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"scheck/internal/analysis"
	"scheck/internal/cache"
)

type Run struct {
	ID         string           `json:"id"`
	SnapshotID string           `json:"snapshotId"`
	Status     string           `json:"status"`
	Progress   int              `json:"progress"`
	Message    string           `json:"message"`
	Options    analysis.Options `json:"options"`
	Summary    json.RawMessage  `json:"summary"`
	Error      string           `json:"error"`
	CreatedAt  time.Time        `json:"createdAt"`
	StartedAt  *time.Time       `json:"startedAt"`
	FinishedAt *time.Time       `json:"finishedAt"`
}

func (s *Server) loadRun(ctx context.Context, rid string) (*Run, error) {
	r := &Run{}
	var opts, sum string
	err := s.db.QueryRowContext(ctx, `SELECT id, snapshot_id, status, progress, message, options, summary, error, created_at, started_at, finished_at
		FROM analysis_runs WHERE id::text=$1`, rid).Scan(&r.ID, &r.SnapshotID, &r.Status, &r.Progress, &r.Message, &opts, &sum, &r.Error,
		&r.CreatedAt, &r.StartedAt, &r.FinishedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(opts), &r.Options)
	r.Summary = json.RawMessage(sum)
	return r, nil
}

func (s *Server) requireRun(ctx context.Context, rid string, u *User, min string) (*Run, string, error) {
	run, err := s.loadRun(ctx, rid)
	if err != nil {
		return nil, "", errNotFound
	}
	pid, err := s.requireSnapshot(ctx, run.SnapshotID, u, min)
	return run, pid, err
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id FROM analysis_runs WHERE snapshot_id=$1 ORDER BY created_at DESC LIMIT 50`, sid)
	if err != nil {
		fail(w, err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	out := []*Run{}
	for _, id := range ids {
		if run, err := s.loadRun(r.Context(), id); err == nil {
			out = append(out, run)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "editor"); err != nil {
		fail(w, err)
		return
	}
	opts := analysis.DefaultOptions()
	if err := readJSON(r, &opts); err != nil {
		fail(w, err)
		return
	}
	var status string
	_ = s.db.QueryRowContext(r.Context(), `SELECT status FROM snapshots WHERE id=$1`, sid).Scan(&status)
	ob, _ := json.Marshal(opts)
	var rid string
	if err := s.db.QueryRowContext(r.Context(), `INSERT INTO analysis_runs(snapshot_id, options, created_by, message) VALUES ($1,$2,$3,'In Warteschlange') RETURNING id`,
		sid, string(ob), u.ID).Scan(&rid); err != nil {
		fail(w, err)
		return
	}
	job := cache.Job{Type: "analyze", ID: rid}
	if status != "ready" {
		job = cache.Job{Type: "index", ID: sid, Arg: rid}
	}
	if err := s.rdb.Enqueue(job); err != nil {
		fail(w, err)
		return
	}
	run, _ := s.loadRun(r.Context(), rid)
	writeJSON(w, 202, run)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request, u *User) {
	run, _, err := s.requireRun(r.Context(), r.PathValue("rid"), u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, run)
}

type FindingRow struct {
	ID         int64  `json:"id"`
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
	Status     string `json:"status"`
	StatusNote string `json:"statusNote"`
}

func findingFilter(r *http.Request, rid string) (string, []any) {
	q := r.URL.Query()
	where := []string{"run_id=$1"}
	args := []any{rid}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	for _, f := range []struct{ param, col string }{{"category", "category"}, {"severity", "severity"}, {"status", "status"}, {"file", "file_id::text"}, {"code", "code"}} {
		if v := q.Get(f.param); v != "" {
			vals := strings.Split(v, ",")
			add(f.col+" = ANY(string_to_array(?, ','))", strings.Join(vals, ","))
		}
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		// the same placeholder is used for all four columns
		add("(message ILIKE ? OR dmc ILIKE ? OR file_path ILIKE ? OR context ILIKE ? OR code ILIKE ?)", "%"+v+"%")
	}
	return strings.Join(where, " AND "), args
}

const findingCols = `id, COALESCE(file_id::text,''), file_path, dmc, category, code, severity, message, xpath, line, context, suggestion, status, status_note`

func scanFinding(sc interface{ Scan(...any) error }) (FindingRow, error) {
	var f FindingRow
	err := sc.Scan(&f.ID, &f.FileID, &f.File, &f.DMC, &f.Category, &f.Code, &f.Severity, &f.Message, &f.Path, &f.Line, &f.Context, &f.Suggestion, &f.Status, &f.StatusNote)
	return f, err
}

func (s *Server) handleListFindings(w http.ResponseWriter, r *http.Request, u *User) {
	rid := r.PathValue("rid")
	if _, _, err := s.requireRun(r.Context(), rid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	where, args := findingFilter(r, rid)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM findings WHERE `+where, args...).Scan(&total); err != nil {
		fail(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT `+findingCols+` FROM findings WHERE `+where+`
		ORDER BY CASE severity WHEN 'error' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, dmc, file_path, line, id
		LIMIT `+strconv.Itoa(limit)+` OFFSET `+strconv.Itoa(max(0, offset)), args...)
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
	// facets for filters
	facets := map[string]map[string]int{"category": {}, "severity": {}, "status": {}}
	for col := range facets {
		fr, err := s.db.QueryContext(r.Context(), `SELECT `+col+`, count(*) FROM findings WHERE run_id=$1 GROUP BY 1`, rid)
		if err != nil {
			continue
		}
		for fr.Next() {
			var k string
			var n int
			_ = fr.Scan(&k, &n)
			facets[col][k] = n
		}
		fr.Close()
	}
	writeJSON(w, 200, map[string]any{"total": total, "items": items, "facets": facets})
}

var categoryDE = map[string]string{
	analysis.CatLists: "Listen", analysis.CatTitles: "Überschriften/Schreibung", analysis.CatReferences: "Verweise",
	analysis.CatConsistency: "Konsistenz", analysis.CatRules: "Business Rules", analysis.CatLLM: "KI-Review", analysis.CatImages: "Bilder",
}
var severityDE = map[string]string{"error": "Fehler", "warning": "Warnung", "info": "Hinweis"}

func (s *Server) writeFindingsCSV(ctx context.Context, w io.Writer, rid string) error {
	_, _ = w.Write([]byte("\xef\xbb\xbf")) // BOM for Excel
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{"Schwere", "Kategorie", "Code", "DMC", "Datei", "Zeile", "Pfad", "Meldung", "Kontext", "Vorschlag", "Status"})
	rows, err := s.db.QueryContext(ctx, `SELECT `+findingCols+` FROM findings WHERE run_id=$1
		ORDER BY CASE severity WHEN 'error' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, dmc, file_path, line`, rid)
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
			f.Message, f.Context, f.Suggestion, f.Status})
	}
	cw.Flush()
	return cw.Error()
}

func (s *Server) handleFindingsCSV(w http.ResponseWriter, r *http.Request, u *User) {
	rid := r.PathValue("rid")
	if _, _, err := s.requireRun(r.Context(), rid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "befunde-" + rid[:8] + ".csv"}))
	_ = s.writeFindingsCSV(r.Context(), w, rid)
}

func (s *Server) handleUpdateFinding(w http.ResponseWriter, r *http.Request, u *User) {
	fid := r.PathValue("fid")
	var rid, fp string
	if err := s.db.QueryRowContext(r.Context(), `SELECT run_id, fingerprint FROM findings WHERE id::text=$1`, fid).Scan(&rid, &fp); err != nil {
		fail(w, errNotFound)
		return
	}
	_, pid, err := s.requireRun(r.Context(), rid, u, "editor")
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
	switch in.Status {
	case "open", "accepted", "dismissed", "fixed":
	default:
		writeErr(w, 400, "Ungültiger Status")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE findings SET status=$1, status_note=$2 WHERE id=$3`, in.Status, in.Note, fid); err != nil {
		fail(w, err)
		return
	}
	if in.Status == "dismissed" {
		_, _ = s.db.ExecContext(r.Context(), `INSERT INTO finding_suppressions(project_id, fingerprint, note, created_by) VALUES ($1,$2,$3,$4)
			ON CONFLICT (project_id, fingerprint) DO UPDATE SET note=EXCLUDED.note`, pid, fp, in.Note, u.ID)
	} else {
		_, _ = s.db.ExecContext(r.Context(), `DELETE FROM finding_suppressions WHERE project_id=$1 AND fingerprint=$2`, pid, fp)
	}
	f, err := scanFinding(s.db.QueryRowContext(r.Context(), `SELECT `+findingCols+` FROM findings WHERE id=$1`, fid))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, f)
}
