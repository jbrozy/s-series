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
	ID                 string     `json:"id"`
	Email              string     `json:"email"`
	Name               string     `json:"name"`
	Role               string     `json:"role"`
	Disabled           bool       `json:"disabled"`
	MustChangePassword bool       `json:"mustChangePassword"`
	CreatedAt          time.Time  `json:"createdAt"`
	LastLogin          *time.Time `json:"lastLoginAt"`
	Token              string     `json:"-"` // name of the API token when authenticated by token
}

func (u *User) IsAdmin() bool { return u.Role == "admin" }

type authedHandler func(w http.ResponseWriter, r *http.Request, u *User)

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

func (s *Server) currentUser(r *http.Request) *User {
	tok := sessionToken(r)
	if tok == "" {
		return nil
	}
	uid, ok := s.rdb.Get("scheck:sess:" + tok)
	if !ok {
		return nil
	}
	u, err := s.loadUser(r.Context(), uid)
	if err != nil || u.Disabled {
		return nil
	}
	// sliding expiration
	s.rdb.Expire("scheck:sess:"+tok, time.Duration(s.cfg.SessionHours)*time.Hour)
	return u
}

// endpoints usable while a password change is pending
var passwordChangeAllowed = map[string]bool{"/api/auth/me": true, "/api/auth/password": true, "/api/auth/logout": true}

func (s *Server) authed(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, viaToken := s.tokenUser(r)
		if viaToken {
			if u == nil {
				writeErr(w, http.StatusUnauthorized, "API-Token ungültig, abgelaufen oder widerrufen")
				return
			}
			if tokenForbidden(r.URL.Path) {
				writeErr(w, http.StatusForbidden, "Dieser Bereich ist mit API-Token nicht erlaubt. Bitte im Browser anmelden.")
				return
			}
		} else {
			u = s.currentUser(r)
		}
		if u == nil {
			writeErr(w, http.StatusUnauthorized, "Nicht angemeldet")
			return
		}
		if u.MustChangePassword && !passwordChangeAllowed[r.URL.Path] {
			writeErr(w, http.StatusForbidden, "Bitte zuerst ein neues Passwort festlegen")
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

const userCols = `id, email, name, role, disabled, must_change_password, created_at, last_login_at`

func scanUser(sc interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := sc.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Disabled, &u.MustChangePassword, &u.CreatedAt, &u.LastLogin)
	return u, err
}

func (s *Server) loadUser(ctx context.Context, id string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id::text=$1`, id))
}

func (s *Server) startSession(w http.ResponseWriter, uid string) error {
	tok := auth.NewToken()
	ttl := time.Duration(s.cfg.SessionHours) * time.Hour
	if err := s.rdb.SetEX("scheck:sess:"+tok, ttl, uid); err != nil {
		return err
	}
	s.rdb.TrackSession(uid, tok, ttl)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds())})
	return nil
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	var n int
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM users`).Scan(&n)
	writeJSON(w, 200, map[string]any{"allowSignup": s.generalSettings(r.Context()).AllowSignup || n == 0, "firstUser": n == 0})
}

func validEmail(e string) bool {
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e && strings.Contains(e, ".")
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Name, Password string }
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.Email = trimLower(in.Email)
	in.Name = strings.TrimSpace(in.Name)
	if !validEmail(in.Email) || in.Name == "" {
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
	if count > 0 && !s.generalSettings(r.Context()).AllowSignup {
		writeErr(w, 403, "Die Selbstregistrierung ist deaktiviert. Bitte wenden Sie sich an einen Administrator.")
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
	s.audit(r, u, "", "user.register", "user", u.ID, u.Name+" hat sich registriert", nil)
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
		writeErr(w, 429, "Zu viele Anmeldeversuche. Bitte in 15 Minuten erneut versuchen.")
		return
	}
	var id, hash string
	var disabled bool
	err := s.db.QueryRowContext(r.Context(), `SELECT id, password_hash, disabled FROM users WHERE email=$1`, in.Email).Scan(&id, &hash, &disabled)
	if err != nil || !auth.CheckPassword(hash, in.Password) {
		s.audit(r, nil, "", "auth.login_failed", "user", in.Email, "Fehlgeschlagene Anmeldung für "+in.Email, nil)
		writeErr(w, 401, "E-Mail oder Passwort falsch")
		return
	}
	if disabled {
		writeErr(w, 403, "Dieses Konto ist deaktiviert. Bitte wenden Sie sich an einen Administrator.")
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `UPDATE users SET last_login_at=now() WHERE id=$1`, id)
	_ = s.rdb.Del("scheck:login:" + ip + ":" + in.Email)
	if err := s.startSession(w, id); err != nil {
		fail(w, err)
		return
	}
	u, _ := s.loadUser(r.Context(), id)
	s.audit(r, u, "", "auth.login", "user", id, u.Name+" hat sich angemeldet", nil)
	writeJSON(w, 200, u)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if tok := sessionToken(r); tok != "" {
		if uid, ok := s.rdb.Get("scheck:sess:" + tok); ok {
			s.rdb.UntrackSession(uid, tok)
		}
		_ = s.rdb.Del("scheck:sess:" + tok)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, u *User) { writeJSON(w, 200, u) }

// handleUpdateMe lets users change their display name.
func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request, u *User) {
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 100 {
		writeErr(w, 400, "Bitte einen Namen mit höchstens 100 Zeichen angeben")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET name=$1 WHERE id=$2`, in.Name, u.ID); err != nil {
		fail(w, err)
		return
	}
	nu, _ := s.loadUser(r.Context(), u.ID)
	writeJSON(w, 200, nu)
}

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
	if in.New == in.Current {
		writeErr(w, 400, "Das neue Passwort muss sich vom bisherigen unterscheiden")
		return
	}
	if err := auth.ValidatePassword(in.New); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	nh, _ := auth.HashPassword(in.New)
	if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET password_hash=$1, must_change_password=false WHERE id=$2`, nh, u.ID); err != nil {
		fail(w, err)
		return
	}
	// sign out all other devices
	ended := s.rdb.EndUserSessions(u.ID, sessionToken(r))
	nu, _ := s.loadUser(r.Context(), u.ID)
	s.audit(r, u, "", "auth.password", "user", u.ID, u.Name+" hat das Passwort geändert", map[string]any{"endedSessions": ended})
	writeJSON(w, 200, map[string]any{"ok": true, "user": nu, "endedSessions": ended})
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
