package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Quality gate: release criteria per project. An upload is released by a
// project owner; the decision records the gate result at that moment.

type GateConfig struct {
	MaxErrors          *int      `json:"maxErrors"`
	MaxWarnings        *int      `json:"maxWarnings"`
	MaxInfo            *int      `json:"maxInfo"`
	BlockingCategories []string  `json:"blockingCategories"`
	RequireLLM         bool      `json:"requireLlm"`
	Configured         bool      `json:"configured"`
	UpdatedAt          time.Time `json:"updatedAt"`
	UpdatedBy          string    `json:"updatedBy"`
}

type GateCheck struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Limit  *int   `json:"limit"`
	Actual int    `json:"actual"`
	OK     bool   `json:"ok"`
}

type GateResult struct {
	State     string      `json:"state"` // passed | failed | pending (no finished analysis)
	Reason    string      `json:"reason"`
	RunID     string      `json:"runId"`
	RunAt     *time.Time  `json:"runAt"`
	Checks    []GateCheck `json:"checks"`
	Config    GateConfig  `json:"config"`
	Evaluated time.Time   `json:"evaluatedAt"`
}

type Review struct {
	ID        int64           `json:"id"`
	RunID     string          `json:"runId"`
	Decision  string          `json:"decision"`
	Comment   string          `json:"comment"`
	Gate      json.RawMessage `json:"gate"`
	UserName  string          `json:"userName"`
	DecidedAt time.Time       `json:"decidedAt"`
}

func (s *Server) loadGate(ctx context.Context, pid string) GateConfig {
	var g GateConfig
	var me, mw, mi sql.NullInt64
	var by sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT g.max_errors, g.max_warnings, g.max_info, g.blocking_categories, g.require_llm, g.updated_at, u.name
		FROM quality_gates g LEFT JOIN users u ON u.id=g.updated_by WHERE g.project_id=$1`, pid).
		Scan(&me, &mw, &mi, pq.Array(&g.BlockingCategories), &g.RequireLLM, &g.UpdatedAt, &by)
	if err != nil {
		// default for projects without explicit gate: no errors allowed
		zero := 0
		return GateConfig{MaxErrors: &zero, BlockingCategories: []string{}}
	}
	toPtr := func(n sql.NullInt64) *int {
		if !n.Valid {
			return nil
		}
		v := int(n.Int64)
		return &v
	}
	g.MaxErrors, g.MaxWarnings, g.MaxInfo = toPtr(me), toPtr(mw), toPtr(mi)
	g.UpdatedBy = by.String
	g.Configured = true
	if g.BlockingCategories == nil {
		g.BlockingCategories = []string{}
	}
	return g
}

func (s *Server) handleGetGate(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, s.loadGate(r.Context(), pid))
}

func (s *Server) handlePutGate(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "owner"); err != nil {
		fail(w, err)
		return
	}
	var in GateConfig
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	for _, p := range []*int{in.MaxErrors, in.MaxWarnings, in.MaxInfo} {
		if p != nil && (*p < 0 || *p > 100000) {
			writeErr(w, 400, "Grenzwerte müssen zwischen 0 und 100000 liegen.")
			return
		}
	}
	cats := []string{}
	for _, c := range in.BlockingCategories {
		if _, ok := categoryDE[c]; !ok {
			writeErr(w, 400, "Unbekannte Kategorie: "+c)
			return
		}
		cats = append(cats, c)
	}
	before := s.loadGate(r.Context(), pid)
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO quality_gates(project_id, max_errors, max_warnings, max_info, blocking_categories, require_llm, updated_by, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,now())
		ON CONFLICT (project_id) DO UPDATE SET max_errors=EXCLUDED.max_errors, max_warnings=EXCLUDED.max_warnings, max_info=EXCLUDED.max_info,
		blocking_categories=EXCLUDED.blocking_categories, require_llm=EXCLUDED.require_llm, updated_by=EXCLUDED.updated_by, updated_at=now()`,
		pid, in.MaxErrors, in.MaxWarnings, in.MaxInfo, pq.Array(cats), in.RequireLLM, u.ID); err != nil {
		fail(w, err)
		return
	}
	after := s.loadGate(r.Context(), pid)
	s.audit(r, u, pid, "gate.update", "project", pid, "Freigabekriterien geändert: "+gateText(after), map[string]any{"before": before, "after": after})
	writeJSON(w, 200, after)
}

func limitText(p *int) string {
	if p == nil {
		return "beliebig"
	}
	return fmt.Sprint(*p)
}

func gateText(g GateConfig) string {
	parts := []string{"Fehler ≤ " + limitText(g.MaxErrors), "Warnungen ≤ " + limitText(g.MaxWarnings), "Hinweise ≤ " + limitText(g.MaxInfo)}
	if len(g.BlockingCategories) > 0 {
		names := make([]string, len(g.BlockingCategories))
		for i, c := range g.BlockingCategories {
			names[i] = categoryDE[c]
		}
		parts = append(parts, "ohne offene Befunde in: "+strings.Join(names, ", "))
	}
	if g.RequireLLM {
		parts = append(parts, "KI-Review erforderlich")
	}
	return strings.Join(parts, "; ")
}

// latestRun returns the newest run of an upload (any state).
func (s *Server) latestRun(ctx context.Context, sid string) (*Run, error) {
	var rid string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM analysis_runs WHERE snapshot_id=$1 ORDER BY created_at DESC LIMIT 1`, sid).Scan(&rid); err != nil {
		return nil, err
	}
	return s.loadRun(ctx, rid)
}

// evaluateGate checks the open findings of the newest finished analysis.
// Open means not dismissed and not marked fixed.
func (s *Server) evaluateGate(ctx context.Context, sid, pid string) GateResult {
	g := s.loadGate(ctx, pid)
	res := GateResult{Config: g, Checks: []GateCheck{}, Evaluated: time.Now()}
	run, err := s.latestRun(ctx, sid)
	switch {
	case err != nil:
		res.State, res.Reason = "pending", "Für diesen Upload wurde noch keine Analyse gestartet."
		return res
	case run.Status == "queued" || run.Status == "running":
		res.State, res.Reason, res.RunID = "pending", "Die Analyse läuft noch.", run.ID
		return res
	case run.Status == "failed":
		res.State, res.Reason, res.RunID = "failed", "Die letzte Analyse ist fehlgeschlagen: "+run.Error, run.ID
		return res
	}
	res.RunID, res.RunAt = run.ID, run.FinishedAt
	open := map[string]int{}
	byCat := map[string]int{}
	rows, err := s.db.QueryContext(ctx, `SELECT severity, category, count(*) FROM findings WHERE run_id=$1 AND status IN ('open','accepted') GROUP BY 1,2`, run.ID)
	if err == nil {
		for rows.Next() {
			var sev, cat string
			var n int
			_ = rows.Scan(&sev, &cat, &n)
			open[sev] += n
			byCat[cat] += n
		}
		rows.Close()
	}
	add := func(key, label string, limit *int, actual int) {
		ok := limit == nil || actual <= *limit
		res.Checks = append(res.Checks, GateCheck{Key: key, Label: label, Limit: limit, Actual: actual, OK: ok})
	}
	add("errors", "Offene Fehler", g.MaxErrors, open["error"])
	add("warnings", "Offene Warnungen", g.MaxWarnings, open["warning"])
	add("info", "Offene Hinweise", g.MaxInfo, open["info"])
	cats := append([]string{}, g.BlockingCategories...)
	sort.Strings(cats)
	zero := 0
	for _, c := range cats {
		add("category:"+c, "Offene Befunde in „"+categoryDE[c]+"“", &zero, byCat[c])
	}
	if g.RequireLLM {
		llm := 0
		if run.Options.LLM {
			llm = 1
		}
		one := 1
		res.Checks = append(res.Checks, GateCheck{Key: "llm", Label: "Analyse mit KI-Review", Limit: &one, Actual: llm, OK: llm == 1})
	}
	res.State = "passed"
	for _, c := range res.Checks {
		if !c.OK {
			res.State = "failed"
			res.Reason = "Mindestens ein Freigabekriterium ist nicht erfüllt."
		}
	}
	return res
}

func (s *Server) reviews(ctx context.Context, sid string) []Review {
	out := []Review{}
	rows, err := s.db.QueryContext(ctx, `SELECT id, COALESCE(run_id::text,''), decision, comment, gate, user_name, decided_at
		FROM snapshot_reviews WHERE snapshot_id=$1 ORDER BY decided_at DESC`, sid)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var rv Review
		var g []byte
		if rows.Scan(&rv.ID, &rv.RunID, &rv.Decision, &rv.Comment, &g, &rv.UserName, &rv.DecidedAt) == nil {
			rv.Gate = g
			out = append(out, rv)
		}
	}
	return out
}

type ReleaseState struct {
	Gate      GateResult `json:"gate"`
	Current   *Review    `json:"current"`
	Stale     bool       `json:"stale"` // decision refers to an older analysis
	History   []Review   `json:"history"`
	CanDecide bool       `json:"canDecide"`
}

func (s *Server) releaseState(ctx context.Context, sid, pid string, role string) ReleaseState {
	st := ReleaseState{Gate: s.evaluateGate(ctx, sid, pid), History: s.reviews(ctx, sid), CanDecide: roleRank[role] >= roleRank["owner"]}
	if len(st.History) > 0 {
		cur := st.History[0]
		st.Current = &cur
		st.Stale = cur.RunID != st.Gate.RunID
	}
	return st
}

func (s *Server) handleGetRelease(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	pid, err := s.requireSnapshot(r.Context(), sid, u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	role, _ := s.projectRole(r.Context(), pid, u)
	writeJSON(w, 200, s.releaseState(r.Context(), sid, pid, role))
}

func (s *Server) handleDecideRelease(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	pid, err := s.requireSnapshot(r.Context(), sid, u, "owner")
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.Comment = strings.TrimSpace(in.Comment)
	gate := s.evaluateGate(r.Context(), sid, pid)
	switch in.Decision {
	case "approved":
		if gate.State == "pending" {
			writeErr(w, 409, "Freigabe erst nach abgeschlossener Analyse möglich. "+gate.Reason)
			return
		}
		if gate.State == "failed" && in.Comment == "" {
			writeErr(w, 400, "Die Freigabekriterien sind nicht erfüllt. Eine Freigabe trotzdem braucht eine Begründung.")
			return
		}
	case "rejected":
		if in.Comment == "" {
			writeErr(w, 400, "Bitte begründen, warum der Upload abgelehnt wird.")
			return
		}
	default:
		writeErr(w, 400, "Entscheidung muss „approved“ oder „rejected“ sein.")
		return
	}
	g, _ := json.Marshal(gate)
	var runID any
	if gate.RunID != "" {
		runID = gate.RunID
	}
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO snapshot_reviews(snapshot_id, run_id, decision, comment, gate, user_id, user_name) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		sid, runID, in.Decision, in.Comment, string(g), u.ID, u.Name); err != nil {
		fail(w, err)
		return
	}
	var label string
	_ = s.db.QueryRowContext(r.Context(), `SELECT label FROM snapshots WHERE id=$1`, sid).Scan(&label)
	summary := "Upload „" + label + "“ freigegeben"
	if in.Decision == "rejected" {
		summary = "Upload „" + label + "“ abgelehnt"
	} else if gate.State == "failed" {
		summary += " (trotz nicht erfüllter Kriterien)"
	}
	s.audit(r, u, pid, "release."+in.Decision, "snapshot", sid, summary, map[string]any{"comment": in.Comment, "gate": gate.State, "run": gate.RunID})
	role, _ := s.projectRole(r.Context(), pid, u)
	writeJSON(w, 200, s.releaseState(r.Context(), sid, pid, role))
}
