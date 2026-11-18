package server

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"
)

var roleRank = map[string]int{"viewer": 1, "editor": 2, "owner": 3}

type Project struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Standard      string     `json:"standard"`
	OwnerID       string     `json:"ownerId"`
	OwnerName     string     `json:"ownerName"`
	CreatedAt     time.Time  `json:"createdAt"`
	MyRole        string     `json:"myRole"`
	SnapshotCount int        `json:"snapshotCount"`
	LastUpload    *time.Time `json:"lastUpload"`
	// LatestLabel, LatestRunStatus and OpenFindings refer to the newest upload
	LatestLabel     string `json:"latestLabel"`
	LatestRunStatus string `json:"latestRunStatus"`
	OpenFindings    int    `json:"openFindings"`
	LatestRelease   string `json:"latestRelease"` // approved | rejected | ""
}

// projectRole returns the effective role of the user in the project.
func (s *Server) projectRole(ctx context.Context, pid string, u *User) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`, pid, u.ID).Scan(&role)
	if err == sql.ErrNoRows {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1)`, pid).Scan(&exists); err != nil || !exists {
			return "", errNotFound
		}
		if u.IsAdmin() {
			return "owner", nil
		}
		return "", errNotFound
	}
	if err != nil {
		return "", errNotFound // invalid uuid etc.
	}
	if u.IsAdmin() {
		return "owner", nil
	}
	return role, nil
}

func (s *Server) requireProject(ctx context.Context, pid string, u *User, min string) (string, error) {
	role, err := s.projectRole(ctx, pid, u)
	if err != nil {
		return "", err
	}
	if roleRank[role] < roleRank[min] {
		return role, errForbidden
	}
	return role, nil
}

func (s *Server) projectOfSnapshot(ctx context.Context, sid string) (string, error) {
	var pid string
	if err := s.db.QueryRowContext(ctx, `SELECT project_id FROM snapshots WHERE id::text=$1`, sid).Scan(&pid); err != nil {
		return "", errNotFound
	}
	return pid, nil
}

func (s *Server) requireSnapshot(ctx context.Context, sid string, u *User, min string) (string, error) {
	pid, err := s.projectOfSnapshot(ctx, sid)
	if err != nil {
		return "", err
	}
	_, err = s.requireProject(ctx, pid, u, min)
	return pid, err
}

const projectSelect = `SELECT p.id, p.name, p.description, p.standard, p.owner_id, ow.name, p.created_at,
	(SELECT count(*) FROM snapshots sn WHERE sn.project_id=p.id AND sn.status<>'uploading'),
	(SELECT max(created_at) FROM snapshots sn WHERE sn.project_id=p.id AND sn.status<>'uploading'),
	COALESCE(ls.label, ''), COALESCE(ls.run_status, ''), COALESCE(ls.open, 0), COALESCE(ls.release, '')
	FROM projects p JOIN users ow ON ow.id=p.owner_id
	LEFT JOIN LATERAL (
		SELECT sn.label, r.status AS run_status,
			(SELECT count(*) FROM findings f WHERE f.run_id=r.id AND f.status='open') AS open,
			(SELECT decision FROM snapshot_reviews rv WHERE rv.snapshot_id=sn.id ORDER BY decided_at DESC LIMIT 1) AS release
		FROM snapshots sn
		LEFT JOIN LATERAL (SELECT id, status FROM analysis_runs WHERE snapshot_id=sn.id ORDER BY created_at DESC LIMIT 1) r ON true
		WHERE sn.project_id=p.id AND sn.status<>'uploading'
		ORDER BY sn.created_at DESC LIMIT 1
	) ls ON true`

func scanProject(sc interface{ Scan(...any) error }) (Project, error) {
	var p Project
	err := sc.Scan(&p.ID, &p.Name, &p.Description, &p.Standard, &p.OwnerID, &p.OwnerName, &p.CreatedAt, &p.SnapshotCount, &p.LastUpload,
		&p.LatestLabel, &p.LatestRunStatus, &p.OpenFindings, &p.LatestRelease)
	return p, err
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request, u *User) {
	q := projectSelect + ` LEFT JOIN project_members m ON m.project_id=p.id AND m.user_id=$1
		WHERE m.user_id IS NOT NULL OR $2 ORDER BY p.created_at DESC`
	rows, err := s.db.QueryContext(r.Context(), q, u.ID, u.IsAdmin())
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			fail(w, err)
			return
		}
		out = append(out, p)
	}
	for i := range out {
		out[i].MyRole, _ = s.projectRole(r.Context(), out[i].ID, u)
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request, u *User) {
	var in struct{ Name, Description, Standard string }
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, 400, "Projektname fehlt")
		return
	}
	if in.Standard == "" {
		in.Standard = "S1000D"
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	var id string
	if err := tx.QueryRowContext(r.Context(), `INSERT INTO projects(name, description, standard, owner_id) VALUES ($1,$2,$3,$4) RETURNING id`,
		in.Name, in.Description, in.Standard, u.ID).Scan(&id); err != nil {
		fail(w, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO project_members(project_id, user_id, role) VALUES ($1,$2,'owner')`, id, u.ID); err != nil {
		fail(w, err)
		return
	}
	if err := seedDefaultRules(r.Context(), tx, id); err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	p, err := scanProject(s.db.QueryRowContext(r.Context(), projectSelect+` WHERE p.id=$1`, id))
	if err != nil {
		fail(w, err)
		return
	}
	p.MyRole = "owner"
	s.audit(r, u, p.ID, "project.create", "project", p.ID, "Projekt „"+p.Name+"“ angelegt", map[string]any{"standard": p.Standard})
	writeJSON(w, 201, p)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	role, err := s.requireProject(r.Context(), pid, u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	p, err := scanProject(s.db.QueryRowContext(r.Context(), projectSelect+` WHERE p.id=$1`, pid))
	if err != nil {
		fail(w, err)
		return
	}
	p.MyRole = role
	writeJSON(w, 200, p)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "owner"); err != nil {
		fail(w, err)
		return
	}
	var in struct{ Name, Description, Standard *string }
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE projects SET name=$1 WHERE id=$2`, strings.TrimSpace(*in.Name), pid)
	}
	if in.Description != nil {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE projects SET description=$1 WHERE id=$2`, *in.Description, pid)
	}
	if in.Standard != nil {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE projects SET standard=$1 WHERE id=$2`, *in.Standard, pid)
	}
	s.audit(r, u, pid, "project.update", "project", pid, "Projektangaben geändert", map[string]any{"name": in.Name, "description": in.Description, "standard": in.Standard})
	s.handleGetProject(w, r, u)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "owner"); err != nil {
		fail(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id FROM snapshots WHERE project_id=$1`, pid)
	if err == nil {
		var ids []string
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			s.removeSnapshotFiles(id)
		}
	}
	var pname string
	_ = s.db.QueryRowContext(r.Context(), `SELECT name FROM projects WHERE id=$1`, pid).Scan(&pname)
	s.audit(r, u, pid, "project.delete", "project", pid, "Projekt „"+pname+"“ gelöscht", nil)
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM projects WHERE id=$1`, pid); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type Member struct {
	UserID    string     `json:"userId"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	Added     time.Time  `json:"addedAt"`
	Disabled  bool       `json:"disabled"`
	LastLogin *time.Time `json:"lastLoginAt"`
}

func (s *Server) members(ctx context.Context, pid string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.email, u.name, m.role, m.added_at, u.disabled, u.last_login_at FROM project_members m
		JOIN users u ON u.id=m.user_id WHERE m.project_id=$1
		ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'editor' THEN 1 ELSE 2 END, lower(u.name)`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.Added, &m.Disabled, &m.LastLogin); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	ms, err := s.members(r.Context(), pid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, ms)
}

// handleAddMember adds existing users (by id or e-mail). Administrators can
// create a new account on the fly; the temporary password is returned once.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "owner"); err != nil {
		fail(w, err)
		return
	}
	var in struct {
		UserIDs []string `json:"userIds"`
		Email   string   `json:"email"`
		Role    string   `json:"role"`
		Create  *struct {
			Name string `json:"name"`
		} `json:"create"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	if _, ok := roleRank[in.Role]; !ok {
		writeErr(w, 400, "Rolle muss Eigentümer, Redakteur oder Leser sein")
		return
	}
	resp := map[string]any{}
	ids := in.UserIDs
	if in.Email != "" {
		var uid string
		err := s.db.QueryRowContext(r.Context(), `SELECT id FROM users WHERE email=$1`, trimLower(in.Email)).Scan(&uid)
		switch {
		case err == nil:
			ids = append(ids, uid)
		case in.Create != nil && u.IsAdmin():
			id, temp, err := s.createUser(r.Context(), in.Create.Name, in.Email, "user", "", u.ID)
			if err != nil {
				fail(w, err)
				return
			}
			ids = append(ids, id)
			resp["created"] = map[string]string{"name": strings.TrimSpace(in.Create.Name), "email": trimLower(in.Email), "tempPassword": temp}
		case u.IsAdmin():
			writeErr(w, 404, "Zu dieser E-Mail-Adresse gibt es noch kein Konto. Sie können es direkt anlegen.")
			return
		default:
			writeErr(w, 404, "Zu dieser E-Mail-Adresse gibt es noch kein Konto. Die Person kann sich registrieren oder ein Administrator legt das Konto an.")
			return
		}
	}
	if len(ids) == 0 {
		writeErr(w, 400, "Bitte mindestens eine Person auswählen")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	added := 0
	var addedIDs []string
	for _, id := range ids {
		res, err := tx.ExecContext(r.Context(), `INSERT INTO project_members(project_id, user_id, role)
			SELECT $1, id, $3 FROM users WHERE id::text=$2 AND NOT disabled
			ON CONFLICT (project_id, user_id) DO NOTHING`, pid, id, in.Role)
		if err != nil {
			fail(w, err)
			return
		}
		n, _ := res.RowsAffected()
		added += int(n)
		if n > 0 {
			addedIDs = append(addedIDs, id)
		}
	}
	if err := tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	for _, id := range addedIDs {
		s.audit(r, u, pid, "member.add", "user", id, s.userName(r.Context(), id)+" als "+roleDE[in.Role]+" hinzugefügt", map[string]any{"role": in.Role})
	}
	ms, err := s.members(r.Context(), pid)
	if err != nil {
		fail(w, err)
		return
	}
	resp["members"] = ms
	resp["added"] = added
	writeJSON(w, 200, resp)
}

func (s *Server) otherOwners(ctx context.Context, pid, uid string) int {
	var n int
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM project_members WHERE project_id=$1 AND role='owner' AND user_id::text<>$2`, pid, uid).Scan(&n)
	return n
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request, u *User) {
	pid, uid := r.PathValue("pid"), r.PathValue("uid")
	if _, err := s.requireProject(r.Context(), pid, u, "owner"); err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	if _, ok := roleRank[in.Role]; !ok {
		writeErr(w, 400, "Ungültige Rolle")
		return
	}
	if in.Role != "owner" && s.otherOwners(r.Context(), pid, uid) == 0 {
		writeErr(w, 400, "Das Projekt braucht mindestens einen Eigentümer")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE project_members SET role=$3 WHERE project_id=$1 AND user_id::text=$2`, pid, uid, in.Role)
	if err != nil {
		fail(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, errNotFound)
		return
	}
	s.audit(r, u, pid, "member.role", "user", uid, "Rolle von "+s.userName(r.Context(), uid)+" auf "+roleDE[in.Role]+" gesetzt", map[string]any{"role": in.Role})
	s.handleListMembers(w, r, u)
}

// handleRemoveMember removes a member; owners can remove anyone, every
// member can leave a project.
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request, u *User) {
	pid, uid := r.PathValue("pid"), r.PathValue("uid")
	min := "owner"
	if uid == u.ID {
		min = "viewer"
	}
	if _, err := s.requireProject(r.Context(), pid, u, min); err != nil {
		fail(w, err)
		return
	}
	var role string
	if err := s.db.QueryRowContext(r.Context(), `SELECT role FROM project_members WHERE project_id=$1 AND user_id::text=$2`, pid, uid).Scan(&role); err != nil {
		fail(w, errNotFound)
		return
	}
	if role == "owner" && s.otherOwners(r.Context(), pid, uid) == 0 {
		writeErr(w, 400, "Das Projekt braucht mindestens einen Eigentümer. Bitte zuerst einen weiteren Eigentümer bestimmen.")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM project_members WHERE project_id=$1 AND user_id::text=$2`, pid, uid); err != nil {
		fail(w, err)
		return
	}
	if uid == u.ID {
		s.audit(r, u, pid, "member.leave", "user", uid, u.Name+" hat das Projekt verlassen", nil)
	} else {
		s.audit(r, u, pid, "member.remove", "user", uid, s.userName(r.Context(), uid)+" aus dem Projekt entfernt", map[string]any{"role": role})
	}
	if uid == u.ID && !u.IsAdmin() {
		writeJSON(w, 200, []Member{})
		return
	}
	s.handleListMembers(w, r, u)
}
