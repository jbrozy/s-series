package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"scheck/internal/auth"
)

// ---------------------------------------------------------------- general settings

type GeneralSettings struct {
	AllowSignup bool `json:"allowSignup"`
}

const generalKey = "general"

func (s *Server) generalSettings(ctx context.Context) GeneralSettings {
	g := GeneralSettings{AllowSignup: s.cfg.AllowSignup}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, generalKey).Scan(&raw); err == nil {
		_ = json.Unmarshal([]byte(raw), &g)
	}
	return g
}

func (s *Server) handleGetGeneral(w http.ResponseWriter, r *http.Request, _ *User) {
	writeJSON(w, 200, s.generalSettings(r.Context()))
}

func (s *Server) handlePutGeneral(w http.ResponseWriter, r *http.Request, me *User) {
	var in GeneralSettings
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	b, _ := json.Marshal(in)
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO settings(key, value) VALUES ($1,$2)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, generalKey, string(b)); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, me, "", "settings.general", "settings", "general", map[bool]string{true: "Selbstregistrierung erlaubt", false: "Selbstregistrierung gesperrt"}[in.AllowSignup], map[string]any{"settings": in})
	writeJSON(w, 200, in)
}

// ---------------------------------------------------------------- admin: users

type AdminUser struct {
	User
	Projects  int    `json:"projects"`
	CreatedBy string `json:"createdBy"`
}

// createUser creates an account. Without a password a temporary one is
// generated. Accounts created by others must change the password at the
// first login.
func (s *Server) createUser(ctx context.Context, name, email, role, password, createdBy string) (string, string, error) {
	name = strings.TrimSpace(name)
	email = trimLower(email)
	if name == "" {
		return "", "", errf(400, "Bitte einen Namen angeben")
	}
	if !validEmail(email) {
		return "", "", errf(400, "Bitte eine gültige E-Mail-Adresse angeben")
	}
	if role == "" {
		role = "user"
	}
	if role != "user" && role != "admin" {
		return "", "", errf(400, "Ungültige Rolle")
	}
	temp := ""
	if password == "" {
		temp = auth.TempPassword()
		password = temp
	} else if err := auth.ValidatePassword(password); err != nil {
		return "", "", errf(400, "%s", err.Error())
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", "", err
	}
	var creator any
	if createdBy != "" {
		creator = createdBy
	}
	var id string
	err = s.db.QueryRowContext(ctx, `INSERT INTO users(email, name, password_hash, role, must_change_password, created_by)
		VALUES ($1,$2,$3,$4,true,$5) ON CONFLICT (email) DO NOTHING RETURNING id`, email, name, hash, role, creator).Scan(&id)
	if err == sql.ErrNoRows {
		return "", "", errf(409, "Es gibt bereits ein Konto mit der E-Mail-Adresse %s", email)
	}
	if err != nil {
		return "", "", err
	}
	if temp == "" {
		temp = password
	}
	return id, temp, nil
}

func (s *Server) loadAdminUser(ctx context.Context, id string) (*AdminUser, error) {
	row := s.db.QueryRowContext(ctx, `SELECT u.id, u.email, u.name, u.role, u.disabled, u.must_change_password, u.created_at, u.last_login_at,
		(SELECT count(*) FROM project_members m WHERE m.user_id=u.id), COALESCE(c.name,'')
		FROM users u LEFT JOIN users c ON c.id=u.created_by WHERE u.id::text=$1`, id)
	a := &AdminUser{}
	err := row.Scan(&a.ID, &a.Email, &a.Name, &a.Role, &a.Disabled, &a.MustChangePassword, &a.CreatedAt, &a.LastLogin, &a.Projects, &a.CreatedBy)
	return a, err
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, _ *User) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT u.id, u.email, u.name, u.role, u.disabled, u.must_change_password, u.created_at, u.last_login_at,
		(SELECT count(*) FROM project_members m WHERE m.user_id=u.id), COALESCE(c.name,'')
		FROM users u LEFT JOIN users c ON c.id=u.created_by ORDER BY lower(u.name)`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []AdminUser{}
	for rows.Next() {
		var a AdminUser
		if err := rows.Scan(&a.ID, &a.Email, &a.Name, &a.Role, &a.Disabled, &a.MustChangePassword, &a.CreatedAt, &a.LastLogin, &a.Projects, &a.CreatedBy); err != nil {
			fail(w, err)
			return
		}
		out = append(out, a)
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, me *User) {
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	id, temp, err := s.createUser(r.Context(), in.Name, in.Email, in.Role, in.Password, me.ID)
	if err != nil {
		fail(w, err)
		return
	}
	u, _ := s.loadAdminUser(r.Context(), id)
	resp := map[string]any{"user": u}
	if in.Password == "" {
		resp["tempPassword"] = temp
	}
	s.audit(r, me, "", "user.create", "user", id, "Konto für "+strings.TrimSpace(in.Name)+" ("+trimLower(in.Email)+") angelegt", map[string]any{"role": in.Role})
	writeJSON(w, 201, resp)
}

// otherActiveAdmins counts active admins other than uid.
func (s *Server) otherActiveAdmins(ctx context.Context, uid string) int {
	var n int
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role='admin' AND NOT disabled AND id::text<>$1`, uid).Scan(&n)
	return n
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request, me *User) {
	uid := r.PathValue("uid")
	target, err := s.loadUser(r.Context(), uid)
	if err != nil {
		fail(w, errNotFound)
		return
	}
	var in struct {
		Name     *string `json:"name"`
		Email    *string `json:"email"`
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	losesAdmin := target.IsAdmin() && ((in.Role != nil && *in.Role != "admin") || (in.Disabled != nil && *in.Disabled))
	if losesAdmin && uid == me.ID {
		writeErr(w, 400, "Eigene Administratorrechte können nicht entzogen werden")
		return
	}
	if losesAdmin && s.otherActiveAdmins(r.Context(), uid) == 0 {
		writeErr(w, 400, "Es muss mindestens ein aktiver Administrator bestehen bleiben")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			writeErr(w, 400, "Name darf nicht leer sein")
			return
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE users SET name=$1 WHERE id=$2`, n, uid); err != nil {
			fail(w, err)
			return
		}
	}
	if in.Email != nil {
		e := trimLower(*in.Email)
		if !validEmail(e) {
			writeErr(w, 400, "Bitte eine gültige E-Mail-Adresse angeben")
			return
		}
		var other string
		if err := tx.QueryRowContext(r.Context(), `SELECT id FROM users WHERE email=$1 AND id::text<>$2`, e, uid).Scan(&other); err == nil {
			writeErr(w, 409, "Die E-Mail-Adresse wird bereits von einem anderen Konto verwendet")
			return
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE users SET email=$1 WHERE id=$2`, e, uid); err != nil {
			fail(w, err)
			return
		}
	}
	if in.Role != nil {
		if *in.Role != "admin" && *in.Role != "user" {
			writeErr(w, 400, "Ungültige Rolle")
			return
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE users SET role=$1 WHERE id=$2`, *in.Role, uid); err != nil {
			fail(w, err)
			return
		}
	}
	if in.Disabled != nil {
		if _, err := tx.ExecContext(r.Context(), `UPDATE users SET disabled=$1 WHERE id=$2`, *in.Disabled, uid); err != nil {
			fail(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	if in.Disabled != nil && *in.Disabled {
		s.rdb.EndUserSessions(uid, "")
	}
	u, err := s.loadAdminUser(r.Context(), uid)
	if err != nil {
		fail(w, err)
		return
	}
	var changes []string
	if in.Name != nil && *in.Name != target.Name {
		changes = append(changes, "Name")
	}
	if in.Email != nil && trimLower(*in.Email) != target.Email {
		changes = append(changes, "E-Mail")
	}
	if in.Role != nil && *in.Role != target.Role {
		changes = append(changes, "Rolle → "+map[string]string{"admin": "Administrator", "user": "Benutzer"}[*in.Role])
	}
	if in.Disabled != nil && *in.Disabled != target.Disabled {
		changes = append(changes, map[bool]string{true: "gesperrt", false: "entsperrt"}[*in.Disabled])
	}
	if len(changes) > 0 {
		s.audit(r, me, "", "user.update", "user", uid, "Konto "+target.Name+": "+strings.Join(changes, ", "), map[string]any{"name": in.Name, "email": in.Email, "role": in.Role, "disabled": in.Disabled})
	}
	writeJSON(w, 200, u)
}

// handleResetPassword sets a new temporary password and signs the user out.
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request, me *User) {
	uid := r.PathValue("uid")
	if uid == me.ID {
		writeErr(w, 400, "Das eigene Passwort bitte unter „Konto“ ändern")
		return
	}
	temp := auth.TempPassword()
	hash, _ := auth.HashPassword(temp)
	res, err := s.db.ExecContext(r.Context(), `UPDATE users SET password_hash=$1, must_change_password=true WHERE id::text=$2`, hash, uid)
	if err != nil {
		fail(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, errNotFound)
		return
	}
	ended := s.rdb.EndUserSessions(uid, "")
	s.audit(r, me, "", "user.reset_password", "user", uid, "Passwort von "+s.userName(r.Context(), uid)+" zurückgesetzt", map[string]any{"endedSessions": ended})
	writeJSON(w, 200, map[string]any{"tempPassword": temp, "endedSessions": ended})
}

// handleDeleteUser removes an account. Projects owned by the user are
// handed over to the acting administrator.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request, me *User) {
	uid := r.PathValue("uid")
	if uid == me.ID {
		writeErr(w, 400, "Das eigene Konto kann nicht gelöscht werden")
		return
	}
	target, err := s.loadUser(r.Context(), uid)
	if err != nil {
		fail(w, errNotFound)
		return
	}
	if target.IsAdmin() && !target.Disabled && s.otherActiveAdmins(r.Context(), uid) == 0 {
		writeErr(w, 400, "Es muss mindestens ein aktiver Administrator bestehen bleiben")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	// projects that would lose their last owner go to the acting admin
	res, err := tx.ExecContext(r.Context(), `INSERT INTO project_members(project_id, user_id, role)
		SELECT p.id, $2, 'owner' FROM projects p
		WHERE EXISTS (SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id=$1 AND m.role='owner')
		  AND NOT EXISTS (SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.role='owner' AND m.user_id<>$1)
		ON CONFLICT (project_id, user_id) DO UPDATE SET role='owner'`, uid, me.ID)
	if err != nil {
		fail(w, err)
		return
	}
	handedOver, _ := res.RowsAffected()
	if _, err := tx.ExecContext(r.Context(), `UPDATE projects SET owner_id=$2 WHERE owner_id=$1`, uid, me.ID); err != nil {
		fail(w, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM users WHERE id=$1`, uid); err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	s.rdb.EndUserSessions(uid, "")
	s.audit(r, me, "", "user.delete", "user", uid, "Konto "+target.Name+" ("+target.Email+") gelöscht", map[string]any{"projectsHandedOver": handedOver})
	writeJSON(w, 200, map[string]any{"ok": true, "projectsHandedOver": handedOver})
}

type UserProject struct {
	ProjectID string    `json:"projectId"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	AddedAt   time.Time `json:"addedAt"`
}

func (s *Server) handleUserProjects(w http.ResponseWriter, r *http.Request, _ *User) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.id, p.name, m.role, m.added_at FROM project_members m
		JOIN projects p ON p.id=m.project_id WHERE m.user_id::text=$1 ORDER BY lower(p.name)`, r.PathValue("uid"))
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []UserProject{}
	for rows.Next() {
		var p UserProject
		if err := rows.Scan(&p.ProjectID, &p.Name, &p.Role, &p.AddedAt); err != nil {
			fail(w, err)
			return
		}
		out = append(out, p)
	}
	writeJSON(w, 200, out)
}

// ---------------------------------------------------------------- user directory

type UserRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// handleSearchUsers powers the person picker: active users matching q,
// optionally excluding members of a project.
func (s *Server) handleSearchUsers(w http.ResponseWriter, r *http.Request, u *User) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	pid := r.URL.Query().Get("project")
	if pid != "" {
		if _, err := s.requireProject(r.Context(), pid, u, "owner"); err != nil {
			fail(w, err)
			return
		}
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, email FROM users u
		WHERE NOT disabled
		  AND ($1 = '' OR name ILIKE '%' || $1 || '%' OR email ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM project_members m WHERE m.project_id::text=$2 AND m.user_id=u.id))
		ORDER BY lower(name) LIMIT 20`, q, pid)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []UserRef{}
	for rows.Next() {
		var x UserRef
		if err := rows.Scan(&x.ID, &x.Name, &x.Email); err != nil {
			fail(w, err)
			return
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
}
