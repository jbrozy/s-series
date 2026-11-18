package server

import (
	"context"
	"encoding/json"
	"net/http"
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
	pid, _ := s.projectOfSnapshot(r.Context(), sid)
	s.audit(r, u, pid, "run.start", "snapshot", sid, "Analyse gestartet", map[string]any{"run": rid, "options": opts})
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
