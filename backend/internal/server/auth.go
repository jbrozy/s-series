package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"scheck/internal/auth"
)

const sessionCookie = "scheck_session"

type User struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	Disabled  bool       `json:"disabled"`
	CreatedAt time.Time  `json:"createdAt"`
	LastLogin *time.Time `json:"lastLoginAt"`
}

func (u *User) IsAdmin() bool { return u.Role == "admin" }

type authedHandler func(w http.ResponseWriter, r *http.Request, u *User)

func (s *Server) currentUser(r *http.Request) *User {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	uid, ok := s.rdb.Get("scheck:sess:" + c.Value)
	if !ok {
		return nil
	}
	u, err := s.loadUser(r.Context(), uid)
	if err != nil || u.Disabled {
		return nil
	}
	// sliding expiration
	s.rdb.Expire("scheck:sess:"+c.Value, time.Duration(s.cfg.SessionHours)*time.Hour)
	return u
}

func (s *Server) authed(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := s.currentUser(r)
		if u == nil {
			writeErr(w, http.StatusUnauthorized, "Nicht angemeldet")
			return
		}
		h(w, r, u)
	}
}

func (s *Server) admin(h authedHandler) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, u *User) {
		if !u.IsAdmin() {
			writeErr(w, http.StatusForbidden, "Nur für Administratoren")
			return
		}
		h(w, r, u)
	})
}

func (s *Server) loadUser(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := s.db.QueryRowContext(ctx, `SELECT id, email, name, role, disabled, created_at, last_login_at FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Disabled, &u.CreatedAt, &u.LastLogin)
	return u, err
}

func (s *Server) startSession(w http.ResponseWriter, uid string) error {
	tok := auth.NewToken()
	ttl := time.Duration(s.cfg.SessionHours) * time.Hour
	if err := s.rdb.SetEX("scheck:sess:"+tok, ttl, uid); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds())})
	return nil
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	var n int
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM users`).Scan(&n)
	writeJSON(w, 200, map[string]any{"allowSignup": s.cfg.AllowSignup || n == 0, "firstUser": n == 0})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Name, Password string }
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.Email = trimLower(in.Email)
	in.Name = strings.TrimSpace(in.Name)
	if _, err := mail.ParseAddress(in.Email); err != nil || in.Name == "" {
		writeErr(w, 400, "Bitte gültige E-Mail-Adresse und Namen angeben")
		return
	}
	if err := auth.ValidatePassword(in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var count int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM users`).Scan(&count); err != nil {
		fail(w, err)
		return
	}
	if count > 0 && !s.cfg.AllowSignup {
		writeErr(w, 403, "Registrierung ist deaktiviert – bitte Administrator kontaktieren")
		return
	}
	role := "user"
	if count == 0 {
		role = "admin" // first user administers the instance
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		fail(w, err)
		return
	}
	var id string
	err = s.db.QueryRowContext(r.Context(),
		`INSERT INTO users(email, name, password_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (email) DO NOTHING RETURNING id`,
		in.Email, in.Name, hash, role).Scan(&id)
	if err == sql.ErrNoRows {
		writeErr(w, 409, "E-Mail-Adresse ist bereits registriert")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.startSession(w, id); err != nil {
		fail(w, err)
		return
	}
	u, _ := s.loadUser(r.Context(), id)
	writeJSON(w, 201, u)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.Email = trimLower(in.Email)
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = strings.Split(r.RemoteAddr, ":")[0]
	}
	if n, _ := s.rdb.Incr("scheck:login:"+ip+":"+in.Email, 15*time.Minute); n > 10 {
		writeErr(w, 429, "Zu viele Anmeldeversuche – bitte in 15 Minuten erneut versuchen")
		return
	}
	var id, hash string
	var disabled bool
	err := s.db.QueryRowContext(r.Context(), `SELECT id, password_hash, disabled FROM users WHERE email=$1`, in.Email).Scan(&id, &hash, &disabled)
	if err != nil || !auth.CheckPassword(hash, in.Password) {
		writeErr(w, 401, "E-Mail oder Passwort falsch")
		return
	}
	if disabled {
		writeErr(w, 403, "Benutzer ist deaktiviert")
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `UPDATE users SET last_login_at=now() WHERE id=$1`, id)
	_ = s.rdb.Del("scheck:login:" + ip + ":" + in.Email)
	if err := s.startSession(w, id); err != nil {
		fail(w, err)
		return
	}
	u, _ := s.loadUser(r.Context(), id)
	writeJSON(w, 200, u)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.rdb.Del("scheck:sess:" + c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, u *User) { writeJSON(w, 200, u) }

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request, u *User) {
	var in struct{ Current, New string }
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	var hash string
	if err := s.db.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, u.ID).Scan(&hash); err != nil {
		fail(w, err)
		return
	}
	if !auth.CheckPassword(hash, in.Current) {
		writeErr(w, 400, "Aktuelles Passwort ist falsch")
		return
	}
	if err := auth.ValidatePassword(in.New); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	nh, _ := auth.HashPassword(in.New)
	if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET password_hash=$1 WHERE id=$2`, nh, u.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// EnsureAdmin creates the bootstrap admin from the environment if set.
func (s *Server) EnsureAdmin(ctx context.Context) error {
	if s.cfg.AdminEmail == "" || s.cfg.AdminPassword == "" {
		return nil
	}
	hash, err := auth.HashPassword(s.cfg.AdminPassword)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO users(email, name, password_hash, role) VALUES ($1, 'Administrator', $2, 'admin')
		ON CONFLICT (email) DO NOTHING`, strings.ToLower(s.cfg.AdminEmail), hash)
	return err
}

// ---------------------------------------------------------------- admin users

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, _ *User) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, email, name, role, disabled, created_at, last_login_at FROM users ORDER BY created_at`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Disabled, &u.CreatedAt, &u.LastLogin); err != nil {
			fail(w, err)
			return
		}
		out = append(out, u)
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request, me *User) {
	uid := r.PathValue("uid")
	var in struct {
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	if uid == me.ID && ((in.Role != nil && *in.Role != "admin") || (in.Disabled != nil && *in.Disabled)) {
		writeErr(w, 400, "Eigene Administratorrechte können nicht entzogen werden")
		return
	}
	if in.Role != nil {
		if *in.Role != "admin" && *in.Role != "user" {
			writeErr(w, 400, "Ungültige Rolle")
			return
		}
		if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET role=$1 WHERE id=$2`, *in.Role, uid); err != nil {
			fail(w, err)
			return
		}
	}
	if in.Disabled != nil {
		if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET disabled=$1 WHERE id=$2`, *in.Disabled, uid); err != nil {
			fail(w, err)
			return
		}
	}
	u, err := s.loadUser(r.Context(), uid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, u)
}
