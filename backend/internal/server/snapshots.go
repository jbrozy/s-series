package server

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"scheck/internal/analysis"
	"scheck/internal/cache"
	"scheck/internal/s1000d"
)

type Snapshot struct {
	ID         string         `json:"id"`
	ProjectID  string         `json:"projectId"`
	Label      string         `json:"label"`
	UploadedBy string         `json:"uploadedBy"`
	Status     string         `json:"status"`
	Error      string         `json:"error"`
	FileCount  int            `json:"fileCount"`
	TotalBytes int64          `json:"totalBytes"`
	CreatedAt  time.Time      `json:"createdAt"`
	Kinds      map[string]int `json:"kinds,omitempty"`
	LatestRun  *Run           `json:"latestRun,omitempty"`
	Images     map[string]int `json:"images,omitempty"`
}

func (s *Server) snapshotRoot(sid string) string {
	return filepath.Join(s.cfg.DataDir, "snapshots", sid)
}
func (s *Server) snapshotDir(sid string) string { return filepath.Join(s.snapshotRoot(sid), "files") }
func (s *Server) anonDir(sid string) string     { return filepath.Join(s.snapshotRoot(sid), "anonymized") }

func (s *Server) removeSnapshotFiles(sid string) {
	if sid == "" || strings.ContainsAny(sid, `/\.`) {
		return
	}
	_ = os.RemoveAll(s.snapshotRoot(sid))
}

// sanitizePath turns an uploaded name into a safe relative path.
func sanitizePath(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")
	if name == "" || name == "." {
		return "", false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." || seg == "__MACOSX" || strings.HasPrefix(seg, "._") || seg == ".DS_Store" || seg == "Thumbs.db" {
			return "", false
		}
	}
	return name, true
}

type uploadState struct {
	sid      string
	dir      string
	maxBytes int64
	total    int64
	files    map[string]fileRec
}

type fileRec struct {
	size int64
	sum  string
}

func (u *uploadState) save(rel string, r io.Reader) error {
	rel, ok := sanitizePath(rel)
	if !ok {
		return nil
	}
	dst := filepath.Join(u.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	remaining := u.maxBytes - u.total
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, remaining+1))
	if err != nil {
		return err
	}
	if n > remaining {
		return errf(http.StatusRequestEntityTooLarge, "Upload überschreitet das Limit von %d MB", u.maxBytes>>20)
	}
	u.total += n
	u.files[rel] = fileRec{size: n, sum: hex.EncodeToString(h.Sum(nil))}
	return nil
}

func (u *uploadState) extractZip(tmp string, prefix string) error {
	zr, err := zip.OpenReader(tmp)
	if err != nil {
		return errf(400, "ZIP-Datei kann nicht gelesen werden: %v", err)
	}
	defer zr.Close()
	if len(zr.File) > 50000 {
		return errf(400, "ZIP enthält zu viele Dateien (max. 50000)")
	}
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		name := zf.Name
		if prefix != "" {
			name = prefix + "/" + name
		}
		if strings.HasSuffix(strings.ToLower(name), ".zip") {
			// nested exports (e.g. CSDB export with ICN archive)
			t, err := os.CreateTemp("", "scheck-*.zip")
			if err == nil {
				_, _ = io.Copy(t, io.LimitReader(rc, u.maxBytes-u.total))
				t.Close()
				err = u.extractZip(t.Name(), strings.TrimSuffix(name, path.Ext(name)))
				os.Remove(t.Name())
			}
			rc.Close()
			if err != nil {
				return err
			}
			continue
		}
		err = u.save(name, rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "editor"); err != nil {
		fail(w, err)
		return
	}
	maxBytes := s.cfg.MaxUploadMB << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(10<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, 400, "Multipart-Upload erwartet")
		return
	}
	var sid string
	if err := s.db.QueryRowContext(r.Context(), `INSERT INTO snapshots(project_id, label, uploaded_by, status) VALUES ($1,'', $2, 'uploading') RETURNING id`,
		pid, u.ID).Scan(&sid); err != nil {
		fail(w, err)
		return
	}
	st := &uploadState{sid: sid, dir: s.snapshotDir(sid), maxBytes: maxBytes * 4, files: map[string]fileRec{}}
	label := ""
	autoAnalyze := true
	opts := analysis.DefaultOptions()
	var firstName string
	cleanup := func(msg string, status int) {
		s.removeSnapshotFiles(sid)
		_, _ = s.db.Exec(`DELETE FROM snapshots WHERE id=$1`, sid)
		writeErr(w, status, msg)
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup("Upload abgebrochen: "+err.Error(), 400)
			return
		}
		if part.FileName() == "" {
			val, _ := io.ReadAll(io.LimitReader(part, 64<<10))
			switch part.FormName() {
			case "label":
				label = strings.TrimSpace(string(val))
			case "autoAnalyze":
				autoAnalyze = string(val) != "false"
			case "options":
				_ = json.Unmarshal(val, &opts)
			}
			continue
		}
		// browsers send the relative path for folder uploads in the filename
		name := part.FileName()
		if _, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition")); err == nil && params["filename"] != "" {
			name = params["filename"]
		}
		if firstName == "" {
			firstName = name
		}
		if strings.HasSuffix(strings.ToLower(name), ".zip") {
			tmp, err := os.CreateTemp("", "scheck-*.zip")
			if err != nil {
				cleanup(err.Error(), 500)
				return
			}
			_, err = io.Copy(tmp, part)
			tmp.Close()
			if err == nil {
				err = st.extractZip(tmp.Name(), "")
			}
			os.Remove(tmp.Name())
			if err != nil {
				var he *httpErr
				if ok := asHTTP(err, &he); ok {
					cleanup(he.msg, he.status)
				} else {
					cleanup("ZIP konnte nicht entpackt werden: "+err.Error(), 400)
				}
				return
			}
			continue
		}
		if err := st.save(name, part); err != nil {
			var he *httpErr
			if asHTTP(err, &he) {
				cleanup(he.msg, he.status)
			} else {
				cleanup(err.Error(), 500)
			}
			return
		}
	}
	if len(st.files) == 0 {
		cleanup("Keine Dateien im Upload", 400)
		return
	}
	if label == "" {
		label = strings.TrimSuffix(path.Base(strings.ReplaceAll(firstName, "\\", "/")), path.Ext(firstName))
		if len(st.files) > 1 && !strings.HasSuffix(strings.ToLower(firstName), ".zip") {
			label = fmt.Sprintf("Upload vom %s", time.Now().Format("02.01.2006 15:04"))
		}
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		cleanup(err.Error(), 500)
		return
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(r.Context(), `INSERT INTO files(snapshot_id, path, kind, size, sha256) VALUES ($1,$2,$3,$4,$5)`)
	if err != nil {
		cleanup(err.Error(), 500)
		return
	}
	for rel, rec := range st.files {
		if _, err := stmt.ExecContext(r.Context(), sid, rel, s1000d.KindByName(rel), rec.size, rec.sum); err != nil {
			cleanup(err.Error(), 500)
			return
		}
	}
	stmt.Close()
	if _, err := tx.ExecContext(r.Context(), `UPDATE snapshots SET label=$1, status='uploaded', file_count=$2, total_bytes=$3 WHERE id=$4`,
		label, len(st.files), st.total, sid); err != nil {
		cleanup(err.Error(), 500)
		return
	}
	var runID string
	if autoAnalyze {
		ob, _ := json.Marshal(opts)
		if err := tx.QueryRowContext(r.Context(), `INSERT INTO analysis_runs(snapshot_id, options, created_by, message) VALUES ($1,$2,$3,'Wartet auf Indexierung') RETURNING id`,
			sid, string(ob), u.ID).Scan(&runID); err != nil {
			cleanup(err.Error(), 500)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		cleanup(err.Error(), 500)
		return
	}
	if err := s.rdb.Enqueue(cache.Job{Type: "index", ID: sid, Arg: runID}); err != nil {
		log.Printf("Job konnte nicht eingereiht werden: %v", err)
	}
	snap, _ := s.loadSnapshot(r.Context(), sid)
	if snap != nil {
		s.audit(r, u, snap.ProjectID, "snapshot.upload", "snapshot", sid, fmt.Sprintf("Upload „%s“ mit %d Dateien hochgeladen", snap.Label, snap.FileCount),
			map[string]any{"files": snap.FileCount, "bytes": snap.TotalBytes, "autoAnalyze": autoAnalyze})
	}
	writeJSON(w, 201, snap)
}

func asHTTP(err error, target **httpErr) bool {
	he, ok := err.(*httpErr)
	if ok {
		*target = he
	}
	return ok
}

func (s *Server) loadSnapshot(ctx context.Context, sid string) (*Snapshot, error) {
	sn := &Snapshot{Kinds: map[string]int{}, Images: map[string]int{}}
	err := s.db.QueryRowContext(ctx, `SELECT s.id, s.project_id, s.label, COALESCE(u.name,''), s.status, s.error, s.file_count, s.total_bytes, s.created_at
		FROM snapshots s LEFT JOIN users u ON u.id=s.uploaded_by WHERE s.id::text=$1`, sid).
		Scan(&sn.ID, &sn.ProjectID, &sn.Label, &sn.UploadedBy, &sn.Status, &sn.Error, &sn.FileCount, &sn.TotalBytes, &sn.CreatedAt)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT kind, count(*) FROM files WHERE snapshot_id=$1 GROUP BY kind`, sid)
	if err == nil {
		for rows.Next() {
			var k string
			var n int
			_ = rows.Scan(&k, &n)
			sn.Kinds[k] = n
		}
		rows.Close()
	}
	rows, err = s.db.QueryContext(ctx, `SELECT status, count(*) FROM images WHERE snapshot_id=$1 GROUP BY status`, sid)
	if err == nil {
		for rows.Next() {
			var k string
			var n int
			_ = rows.Scan(&k, &n)
			sn.Images[k] = n
		}
		rows.Close()
	}
	var rid string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM analysis_runs WHERE snapshot_id=$1 ORDER BY created_at DESC LIMIT 1`, sid).Scan(&rid); err == nil {
		sn.LatestRun, _ = s.loadRun(ctx, rid)
	}
	return sn, nil
}

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request, u *User) {
	pid := r.PathValue("pid")
	if _, err := s.requireProject(r.Context(), pid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id FROM snapshots WHERE project_id=$1 AND status<>'uploading' ORDER BY created_at DESC`, pid)
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
	out := []*Snapshot{}
	for _, id := range ids {
		if sn, err := s.loadSnapshot(r.Context(), id); err == nil {
			out = append(out, sn)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	sn, err := s.loadSnapshot(r.Context(), sid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, sn)
}

func (s *Server) handleDeleteSnapshot(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	pid, err := s.requireSnapshot(r.Context(), sid, u, "editor")
	if err != nil {
		fail(w, err)
		return
	}
	var label string
	_ = s.db.QueryRowContext(r.Context(), `SELECT label FROM snapshots WHERE id=$1`, sid).Scan(&label)
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM snapshots WHERE id=$1`, sid); err != nil {
		fail(w, err)
		return
	}
	s.removeSnapshotFiles(sid)
	s.audit(r, u, pid, "snapshot.delete", "snapshot", sid, "Upload „"+label+"“ mit allen Prüfergebnissen gelöscht", nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type FileInfo struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	DMC        string `json:"dmc"`
	Issue      string `json:"issue"`
	Lang       string `json:"lang"`
	Title      string `json:"title"`
	InfoCode   string `json:"infoCode"`
	Size       int64  `json:"size"`
	ParseError string `json:"parseError"`
	Findings   int    `json:"findings"`
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT f.id, f.path, f.kind, f.dmc, f.issue, f.lang, f.title, f.info_code, f.size, f.parse_error,
		COALESCE((SELECT count(*) FROM findings x WHERE x.file_id=f.id AND x.status='open' AND x.run_id=(
			SELECT id FROM analysis_runs WHERE snapshot_id=$1 AND status='done' ORDER BY created_at DESC LIMIT 1)),0)
		FROM files f WHERE f.snapshot_id=$1 ORDER BY f.kind, f.dmc, f.path`, sid)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []FileInfo{}
	for rows.Next() {
		var f FileInfo
		if err := rows.Scan(&f.ID, &f.Path, &f.Kind, &f.DMC, &f.Issue, &f.Lang, &f.Title, &f.InfoCode, &f.Size, &f.ParseError, &f.Findings); err != nil {
			fail(w, err)
			return
		}
		out = append(out, f)
	}
	writeJSON(w, 200, out)
}

func contentType(p string) string {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".xml", ".sgm", ".sgml":
		return "text/xml; charset=utf-8"
	case ".htm", ".html", ".xhtml", ".js", ".mjs", ".css":
		// uploaded web pages are shown as source, never executed in the app origin
		return "text/plain; charset=utf-8"
	case ".cgm":
		return "image/cgm"
	case ".svg":
		return "image/svg+xml"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

func (s *Server) handleFileContent(w http.ResponseWriter, r *http.Request, u *User) {
	sid, fid := r.PathValue("sid"), r.PathValue("fid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	var p string
	if err := s.db.QueryRowContext(r.Context(), `SELECT path FROM files WHERE id::text=$1 AND snapshot_id::text=$2`, fid, sid).Scan(&p); err != nil {
		fail(w, errNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType(p))
	// uploaded content is untrusted: no scripts, no requests, own origin
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(p)}))
	}
	http.ServeFile(w, r, filepath.Join(s.snapshotDir(sid), filepath.FromSlash(p)))
}

// handleExport streams the snapshot as ZIP with anonymized images in place
// of the originals and the findings of the latest run as CSV.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	sn, err := s.loadSnapshot(r.Context(), sid)
	if err != nil {
		fail(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT f.path, COALESCE(i.anonymized_path,'') FROM files f
		LEFT JOIN images i ON i.file_id=f.id WHERE f.snapshot_id=$1 ORDER BY f.path`, sid)
	if err != nil {
		fail(w, err)
		return
	}
	type entry struct{ path, anon string }
	var entries []entry
	for rows.Next() {
		var e entry
		_ = rows.Scan(&e.path, &e.anon)
		entries = append(entries, e)
	}
	rows.Close()
	name := safeFileName(sn.Label) + "-geprueft.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	zw := zip.NewWriter(w)
	defer zw.Close()
	for _, e := range entries {
		src := filepath.Join(s.snapshotDir(sid), filepath.FromSlash(e.path))
		if e.anon != "" {
			src = filepath.Join(s.anonDir(sid), e.anon)
		}
		f, err := os.Open(src)
		if err != nil {
			continue
		}
		zf, err := zw.Create(e.path)
		if err == nil {
			_, _ = io.Copy(zf, f)
		}
		f.Close()
	}
	if sn.LatestRun != nil && sn.LatestRun.Status == "done" {
		if zf, err := zw.Create("S-Check-Befunde.csv"); err == nil {
			_ = s.writeFindingsCSV(r.Context(), zf, sn.LatestRun.ID, sn.ProjectID)
		}
	}
}

func safeFileName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, s)
	if s == "" {
		return "export"
	}
	return s
}
