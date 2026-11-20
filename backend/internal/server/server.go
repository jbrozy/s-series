// Package server contains the HTTP API, the background worker and the
// glue between storage, analysis and image processing.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"scheck/internal/cache"
	"scheck/internal/config"
	"scheck/internal/llm"
)

type Server struct {
	cfg config.Config
	db  *sql.DB
	rdb *cache.Redis
	mux *http.ServeMux

	settingsMu sync.RWMutex
	settings   *llm.Settings
}

func New(cfg config.Config, db *sql.DB, rdb *cache.Redis) *Server {
	s := &Server{cfg: cfg, db: db, rdb: rdb, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/health", s.handleHealth)

	m.HandleFunc("POST /api/auth/register", s.handleRegister)
	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)
	m.HandleFunc("GET /api/auth/me", s.authed(s.handleMe))
	m.HandleFunc("PATCH /api/auth/me", s.authed(s.handleUpdateMe))
	m.HandleFunc("PUT /api/auth/me/preferences", s.authed(s.handlePutPreferences))
	m.HandleFunc("POST /api/auth/password", s.authed(s.handleChangePassword))
	m.HandleFunc("GET /api/auth/config", s.handleAuthConfig)

	m.HandleFunc("GET /api/projects", s.authed(s.handleListProjects))
	m.HandleFunc("POST /api/projects", s.authed(s.handleCreateProject))
	m.HandleFunc("GET /api/projects/{pid}", s.authed(s.handleGetProject))
	m.HandleFunc("PATCH /api/projects/{pid}", s.authed(s.handleUpdateProject))
	m.HandleFunc("DELETE /api/projects/{pid}", s.authed(s.handleDeleteProject))
	m.HandleFunc("GET /api/projects/{pid}/members", s.authed(s.handleListMembers))
	m.HandleFunc("POST /api/projects/{pid}/members", s.authed(s.handleAddMember))
	m.HandleFunc("PATCH /api/projects/{pid}/members/{uid}", s.authed(s.handleUpdateMember))
	m.HandleFunc("DELETE /api/projects/{pid}/members/{uid}", s.authed(s.handleRemoveMember))
	m.HandleFunc("GET /api/users", s.authed(s.handleSearchUsers))

	m.HandleFunc("GET /api/projects/{pid}/snapshots", s.authed(s.handleListSnapshots))
	m.HandleFunc("POST /api/projects/{pid}/snapshots", s.authed(s.handleUpload))
	m.HandleFunc("GET /api/snapshots/{sid}", s.authed(s.handleGetSnapshot))
	m.HandleFunc("DELETE /api/snapshots/{sid}", s.authed(s.handleDeleteSnapshot))
	m.HandleFunc("GET /api/snapshots/{sid}/files", s.authed(s.handleListFiles))
	m.HandleFunc("GET /api/snapshots/{sid}/files/{fid}/content", s.authed(s.handleFileContent))
	m.HandleFunc("GET /api/snapshots/{sid}/export", s.authed(s.handleExport))
	m.HandleFunc("GET /api/snapshots/{sid}/search", s.authed(s.handleSearch))
	m.HandleFunc("GET /api/snapshots/{sid}/graph", s.authed(s.handleGraph))
	m.HandleFunc("POST /api/snapshots/{sid}/ask", s.authed(s.handleAsk))
	m.HandleFunc("GET /api/snapshots/{sid}/synthetic", s.authed(s.handleSynthetic))

	m.HandleFunc("GET /api/snapshots/{sid}/runs", s.authed(s.handleListRuns))
	m.HandleFunc("POST /api/snapshots/{sid}/runs", s.authed(s.handleStartRun))
	m.HandleFunc("GET /api/runs/{rid}", s.authed(s.handleGetRun))
	m.HandleFunc("GET /api/runs/{rid}/findings", s.authed(s.handleListFindings))
	m.HandleFunc("GET /api/runs/{rid}/findings.csv", s.authed(s.handleFindingsCSV))
	m.HandleFunc("PATCH /api/findings/{fid}", s.authed(s.handleUpdateFinding))
	m.HandleFunc("PUT /api/findings/{fid}/assignee", s.authed(s.handleAssignFinding))
	m.HandleFunc("GET /api/findings/{fid}/activity", s.authed(s.handleFindingActivity))
	m.HandleFunc("POST /api/findings/{fid}/comments", s.authed(s.handleCommentFinding))
	m.HandleFunc("GET /api/runs/{rid}/resolved", s.authed(s.handleResolvedFindings))
	m.HandleFunc("POST /api/runs/{rid}/findings/bulk", s.authed(s.handleBulkFindings))
	m.HandleFunc("GET /api/search", s.authed(s.handleGlobalSearch))

	m.HandleFunc("GET /api/snapshots/{sid}/images", s.authed(s.handleListImages))
	m.HandleFunc("GET /api/images/{iid}", s.authed(s.handleGetImage))
	m.HandleFunc("GET /api/images/{iid}/original", s.authed(s.handleImageOriginal))
	m.HandleFunc("GET /api/images/{iid}/anonymized", s.authed(s.handleImageAnonymized))
	m.HandleFunc("PUT /api/images/{iid}/detections", s.authed(s.handleSaveDetections))
	m.HandleFunc("POST /api/images/{iid}/detect", s.authed(s.handleRedetect))
	m.HandleFunc("POST /api/images/{iid}/preview", s.authed(s.handlePreview))
	m.HandleFunc("POST /api/images/{iid}/anonymize", s.authed(s.handleAnonymize))
	m.HandleFunc("POST /api/images/{iid}/clean", s.authed(s.handleMarkClean))

	m.HandleFunc("GET /api/projects/{pid}/rules", s.authed(s.handleListRules))
	m.HandleFunc("POST /api/projects/{pid}/rules", s.authed(s.handleCreateRule))
	m.HandleFunc("PUT /api/rules/{id}", s.authed(s.handleUpdateRule))
	m.HandleFunc("DELETE /api/rules/{id}", s.authed(s.handleDeleteRule))
	m.HandleFunc("POST /api/projects/{pid}/rules/import-brex", s.authed(s.handleImportBREX))
	m.HandleFunc("GET /api/rules/templates", s.authed(s.handleRuleTemplates))

	m.HandleFunc("GET /api/projects/{pid}/audit", s.authed(s.handleProjectAudit))
	m.HandleFunc("GET /api/projects/{pid}/gate", s.authed(s.handleGetGate))
	m.HandleFunc("PUT /api/projects/{pid}/gate", s.authed(s.handlePutGate))
	m.HandleFunc("GET /api/snapshots/{sid}/release", s.authed(s.handleGetRelease))
	m.HandleFunc("POST /api/snapshots/{sid}/release", s.authed(s.handleDecideRelease))
	m.HandleFunc("GET /api/snapshots/{sid}/report", s.authed(s.handleReport))
	m.HandleFunc("GET /api/projects/{pid}/rules/export", s.authed(s.handleExportRules))
	m.HandleFunc("POST /api/projects/{pid}/rules/import", s.authed(s.handleImportRules))
	m.HandleFunc("GET /api/tokens", s.authed(s.handleListTokens))
	m.HandleFunc("GET /api/tools", s.authed(s.handleTools))
	m.HandleFunc("GET /api/tools/{name}", s.authed(s.handleToolDownload))
	m.HandleFunc("POST /api/tokens", s.authed(s.handleCreateToken))
	m.HandleFunc("DELETE /api/tokens/{id}", s.authed(s.handleRevokeToken))
	m.HandleFunc("GET /api/admin/audit", s.admin(s.handleAdminAudit))

	m.HandleFunc("GET /api/admin/settings/models", s.admin(s.handleGetModels))
	m.HandleFunc("PUT /api/admin/settings/models", s.admin(s.handlePutModels))
	m.HandleFunc("POST /api/admin/settings/models/test", s.admin(s.handleTestModel))
	m.HandleFunc("GET /api/admin/users", s.admin(s.handleListUsers))
	m.HandleFunc("POST /api/admin/users", s.admin(s.handleCreateUser))
	m.HandleFunc("PATCH /api/admin/users/{uid}", s.admin(s.handleUpdateUser))
	m.HandleFunc("DELETE /api/admin/users/{uid}", s.admin(s.handleDeleteUser))
	m.HandleFunc("POST /api/admin/users/{uid}/reset-password", s.admin(s.handleResetPassword))
	m.HandleFunc("GET /api/admin/users/{uid}/projects", s.admin(s.handleUserProjects))
	m.HandleFunc("GET /api/admin/settings/general", s.admin(s.handleGetGeneral))
	m.HandleFunc("PUT /api/admin/settings/general", s.admin(s.handlePutGeneral))
	m.HandleFunc("GET /api/admin/status", s.admin(s.handleStatus))
}

// Handler returns the HTTP handler with middleware.
func (s *Server) Handler() http.Handler {
	return s.recoverer(s.logger(s.security(s.mux)))
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "same-origin")
		// CSRF protection: state changing requests must carry a custom header
		// which browsers do not send cross-site without CORS preflight.
		// Bearer-token requests carry no cookies, so they cannot be forged cross-site.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && bearerToken(r) == "" {
			if r.Header.Get("X-Requested-With") != "scheck" {
				writeErr(w, http.StatusForbidden, "Fehlender X-Requested-With-Header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(c int) { w.status = c; w.ResponseWriter.WriteHeader(c) }
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		if r.URL.Path != "/api/health" {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic: %v\n%s", v, debug.Stack())
				writeErr(w, http.StatusInternalServerError, "Interner Fehler")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------- helpers

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

// errStatus maps errors to responses.
type httpErr struct {
	status int
	msg    string
}

func (e *httpErr) Error() string { return e.msg }

func errf(status int, format string, a ...any) error {
	return &httpErr{status, fmt.Sprintf(format, a...)}
}

var errNotFound = errf(http.StatusNotFound, "Nicht gefunden")
var errForbidden = errf(http.StatusForbidden, "Keine Berechtigung")

func fail(w http.ResponseWriter, err error) {
	var he *httpErr
	if errors.As(err, &he) {
		writeErr(w, he.status, he.msg)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "Nicht gefunden")
		return
	}
	log.Printf("Fehler: %v", err)
	writeErr(w, http.StatusInternalServerError, "Interner Fehler: "+err.Error())
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	if err := dec.Decode(v); err != nil {
		return errf(http.StatusBadRequest, "Ungültige Anfrage: %v", err)
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	dbOK := s.db.PingContext(ctx) == nil
	redisOK := s.rdb.Ping(ctx) == nil
	status := http.StatusOK
	if !dbOK || !redisOK {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]bool{"database": dbOK, "redis": redisOK})
}

func trimLower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
