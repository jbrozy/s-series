package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"log"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AuditEntry is one row of the audit trail.
type AuditEntry struct {
	ID          int64           `json:"id"`
	At          time.Time       `json:"at"`
	UserID      string          `json:"userId"`
	UserName    string          `json:"userName"`
	ViaToken    string          `json:"viaToken"`
	ProjectID   string          `json:"projectId"`
	ProjectName string          `json:"projectName"`
	Action      string          `json:"action"`
	TargetType  string          `json:"targetType"`
	TargetID    string          `json:"targetId"`
	Summary     string          `json:"summary"`
	Details     json.RawMessage `json:"details"`
	IP          string          `json:"ip"`
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// audit records an action. It never fails the request: a lost audit row is
// logged, the user action itself already happened.
func (s *Server) audit(r *http.Request, u *User, pid, action, targetType, targetID, summary string, details map[string]any) {
	s.auditCtx(r.Context(), clientIP(r), u, pid, action, targetType, targetID, summary, details)
}

func (s *Server) auditCtx(ctx context.Context, ip string, u *User, pid, action, targetType, targetID, summary string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	d, _ := json.Marshal(details)
	var uid, uname, token any = nil, "", ""
	if u != nil {
		uid, uname, token = u.ID, u.Name, u.Token
	}
	var p any
	if pid != "" {
		p = pid
	}
	// detach from the request: the row must be written even if the client goes away
	ctx = context.WithoutCancel(ctx)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO audit_log(user_id, user_name, via_token, project_id, project_name, action, target_type, target_id, summary, details, ip)
		VALUES ($1,$2,$3,$4,COALESCE((SELECT name FROM projects WHERE id=$4::uuid),''),$5,$6,$7,$8,$9,$10)`,
		uid, uname, token, p, action, targetType, targetID, summary, string(d), ip); err != nil {
		log.Printf("Audit-Eintrag %s nicht gespeichert: %v", action, err)
	}
}

func auditFilter(r *http.Request, where []string, args []any) ([]string, []any) {
	q := r.URL.Query()
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if v := q.Get("action"); v != "" {
		add("action LIKE ?", strings.TrimSuffix(v, "*")+"%")
	}
	if v := q.Get("user"); v != "" {
		add("user_id::text = ?", v)
	}
	if v := q.Get("from"); v != "" {
		add("at >= ?::date", v)
	}
	if v := q.Get("to"); v != "" {
		add("at < ?::date + 1", v)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		add("(summary ILIKE ? OR user_name ILIKE ? OR target_id ILIKE ? OR project_name ILIKE ?)", "%"+v+"%")
	}
	if v := q.Get("target"); v != "" {
		add("target_id = ?", v)
	}
	return where, args
}

const auditCols = `id, at, COALESCE(user_id::text,''), user_name, via_token, COALESCE(project_id::text,''), project_name, action, target_type, target_id, summary, details, ip`

func scanAudit(sc interface{ Scan(...any) error }) (AuditEntry, error) {
	var e AuditEntry
	var d []byte
	err := sc.Scan(&e.ID, &e.At, &e.UserID, &e.UserName, &e.ViaToken, &e.ProjectID, &e.ProjectName, &e.Action, &e.TargetType, &e.TargetID, &e.Summary, &d, &e.IP)
	e.Details = d
	return e, err
}

func (s *Server) queryAudit(w http.ResponseWriter, r *http.Request, where []string, args []any) {
	where, args = auditFilter(r, where, args)
	cond := "true"
	if len(where) > 0 {
		cond = strings.Join(where, " AND ")
	}
	if r.URL.Query().Get("format") == "csv" {
		s.writeAuditCSV(w, r, cond, args)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM audit_log WHERE `+cond, args...).Scan(&total); err != nil {
		fail(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT `+auditCols+` FROM audit_log WHERE `+cond+` ORDER BY at DESC, id DESC LIMIT `+
		strconv.Itoa(limit)+` OFFSET `+strconv.Itoa(max(0, offset)), args...)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	items := []AuditEntry{}
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			fail(w, err)
			return
		}
		items = append(items, e)
	}
	writeJSON(w, 200, map[string]any{"total": total, "items": items})
}

func (s *Server) writeAuditCSV(w http.ResponseWriter, r *http.Request, cond string, args []any) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT `+auditCols+` FROM audit_log WHERE `+cond+` ORDER BY at DESC, id DESC LIMIT 100000`, args...)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "protokoll-" + time.Now().Format("2006-01-02") + ".csv"}))
	_, _ = w.Write([]byte("\xef\xbb\xbf"))
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{"Zeitpunkt (UTC)", "Benutzer", "API-Token", "Projekt", "Aktion", "Objekt", "Objekt-ID", "Beschreibung", "Details", "IP"})
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			break
		}
		_ = cw.Write([]string{e.At.UTC().Format(time.RFC3339), e.UserName, e.ViaToken, e.ProjectName, e.Action, e.TargetType, e.TargetID, e.Summary, string(e.Details), e.IP})
	}
	cw.Flush()
}

func (s *Server) handleProjectAudit(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "editor"); err != nil {
		fail(w, err)
		return
	}
	s.queryAudit(w, r, []string{"project_id = $1"}, []any{pid})
}

func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request, u *User) {
	var where []string
	var args []any
	if p := r.URL.Query().Get("project"); p != "" {
		where, args = []string{"project_id::text = $1"}, []any{p}
	}
	s.queryAudit(w, r, where, args)
}

var roleDE = map[string]string{"owner": "Eigentümer", "editor": "Redakteur", "viewer": "Leser"}

func (s *Server) userName(ctx context.Context, uid string) string {
	var n string
	_ = s.db.QueryRowContext(ctx, `SELECT name FROM users WHERE id::text=$1`, uid).Scan(&n)
	return n
}
