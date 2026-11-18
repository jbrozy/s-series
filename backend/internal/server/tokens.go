package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Personal API tokens let scripts and CI pipelines act as a user without a
// browser session. A token carries the rights of its owner in projects, but
// cannot manage accounts, tokens or the server (see tokenForbidden).

const tokenPrefix = "sck_"

type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func newAPIToken() string {
	b := make([]byte, 30)
	_, _ = rand.Read(b)
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
}

// tokenForbidden lists API areas a token may never use.
func tokenForbidden(path string) bool {
	return strings.HasPrefix(path, "/api/admin/") || strings.HasPrefix(path, "/api/auth/") || strings.HasPrefix(path, "/api/tokens")
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// tokenUser resolves a bearer token. ok=false means a token was sent but is
// not valid; callers must then reject the request instead of falling back to
// a cookie session.
func (s *Server) tokenUser(r *http.Request) (u *User, sent bool) {
	tok := bearerToken(r)
	if tok == "" {
		return nil, false
	}
	if !strings.HasPrefix(tok, tokenPrefix) {
		return nil, true
	}
	var uid, tid, name string
	err := s.db.QueryRowContext(r.Context(), `SELECT user_id, id, name FROM api_tokens
		WHERE token_hash=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())`, hashToken(tok)).Scan(&uid, &tid, &name)
	if err != nil {
		return nil, true
	}
	u, err = s.loadUser(r.Context(), uid)
	if err != nil || u.Disabled {
		return nil, true
	}
	u.Token = name
	_, _ = s.db.ExecContext(r.Context(), `UPDATE api_tokens SET last_used_at=now() WHERE id=$1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, tid)
	return u, true
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, prefix, created_at, expires_at, last_used_at, revoked_at
		FROM api_tokens WHERE user_id=$1 ORDER BY revoked_at IS NOT NULL, created_at DESC`, u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &t.ExpiresAt, &t.LastUsedAt, &t.RevokedAt); err != nil {
			fail(w, err)
			return
		}
		out = append(out, t)
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request, u *User) {
	var in struct {
		Name string `json:"name"`
		Days int    `json:"days"` // 0 = no expiry
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		writeErr(w, 400, "Bitte einen Namen angeben (höchstens 80 Zeichen), z. B. „CSDB-Export Pipeline“.")
		return
	}
	if in.Days < 0 || in.Days > 730 {
		writeErr(w, 400, "Gültigkeit zwischen 1 und 730 Tagen oder unbegrenzt.")
		return
	}
	var exp any
	if in.Days > 0 {
		exp = time.Now().AddDate(0, 0, in.Days)
	}
	tok := newAPIToken()
	var t APIToken
	err := s.db.QueryRowContext(r.Context(), `INSERT INTO api_tokens(user_id, name, prefix, token_hash, expires_at) VALUES ($1,$2,$3,$4,$5)
		RETURNING id, name, prefix, created_at, expires_at, last_used_at, revoked_at`, u.ID, in.Name, tok[:10], hashToken(tok), exp).
		Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &t.ExpiresAt, &t.LastUsedAt, &t.RevokedAt)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, u, "", "token.create", "token", t.ID, "API-Token „"+t.Name+"“ erstellt", map[string]any{"expiresAt": t.ExpiresAt})
	writeJSON(w, 201, map[string]any{"token": tok, "info": t})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request, u *User) {
	id := r.PathValue("id")
	var name string
	err := s.db.QueryRowContext(r.Context(), `UPDATE api_tokens SET revoked_at=now() WHERE id::text=$1 AND user_id=$2 AND revoked_at IS NULL RETURNING name`, id, u.ID).Scan(&name)
	if err != nil {
		fail(w, errNotFound)
		return
	}
	s.audit(r, u, "", "token.revoke", "token", id, "API-Token „"+name+"“ widerrufen", nil)
	w.WriteHeader(204)
}

// handleTools lists the scheck-ci binaries that ship with the server.
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request, u *User) {
	out := []map[string]any{}
	entries, _ := os.ReadDir(s.cfg.ToolsDir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			out = append(out, map[string]any{"name": e.Name(), "size": info.Size()})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleToolDownload(w http.ResponseWriter, r *http.Request, u *User) {
	name := filepath.Base(r.PathValue("name"))
	p := filepath.Join(s.cfg.ToolsDir, name)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		fail(w, errNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, p)
}
