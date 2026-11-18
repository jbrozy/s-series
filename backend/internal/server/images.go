package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"scheck/internal/analysis"
	"scheck/internal/auth"
	"scheck/internal/cache"
	"scheck/internal/imaging"
)

type ImageInfo struct {
	ID         string              `json:"id"`
	SnapshotID string              `json:"snapshotId"`
	FileID     string              `json:"fileId"`
	Path       string              `json:"path"`
	ICN        string              `json:"icn"`
	Status     string              `json:"status"`
	Width      int                 `json:"width"`
	Height     int                 `json:"height"`
	Detections []imaging.Detection `json:"detections"`
	Detector   string              `json:"detector"`
	Anonymized bool                `json:"anonymized"`
	Error      string              `json:"error"`
	UpdatedAt  time.Time           `json:"updatedAt"`
	UsedIn     []string            `json:"usedIn,omitempty"`
}

const imageCols = `i.id, i.snapshot_id, i.file_id, f.path, i.status, i.width, i.height, i.detections, i.detector, i.anonymized_path<>'', i.error, i.updated_at`

func scanImage(sc interface{ Scan(...any) error }) (*ImageInfo, error) {
	im := &ImageInfo{}
	var dets string
	if err := sc.Scan(&im.ID, &im.SnapshotID, &im.FileID, &im.Path, &im.Status, &im.Width, &im.Height, &dets, &im.Detector,
		&im.Anonymized, &im.Error, &im.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(dets), &im.Detections)
	if im.Detections == nil {
		im.Detections = []imaging.Detection{}
	}
	im.ICN = strings.TrimSuffix(path.Base(im.Path), path.Ext(im.Path))
	return im, nil
}

func (s *Server) loadImage(ctx context.Context, iid string) (*ImageInfo, error) {
	im, err := scanImage(s.db.QueryRowContext(ctx, `SELECT `+imageCols+` FROM images i JOIN files f ON f.id=i.file_id WHERE i.id::text=$1`, iid))
	if err == nil && im.Width == 0 && im.Status != "unsupported" {
		// images indexed before sizes were stored
		if w, h, e := imaging.Size(filepath.Join(s.snapshotDir(im.SnapshotID), filepath.FromSlash(im.Path))); e == nil {
			im.Width, im.Height = w, h
			_, _ = s.db.ExecContext(ctx, `UPDATE images SET width=$1, height=$2 WHERE id=$3`, w, h, im.ID)
		}
	}
	return im, err
}

func (s *Server) requireImage(ctx context.Context, iid string, u *User, min string) (*ImageInfo, error) {
	im, err := s.loadImage(ctx, iid)
	if err != nil {
		return nil, errNotFound
	}
	if _, err := s.requireSnapshot(ctx, im.SnapshotID, u, min); err != nil {
		return nil, err
	}
	return im, nil
}

// detectSnapshotImages runs detection for all pending raster images.
// Returns -1 if detection is disabled.
func (s *Server) detectSnapshotImages(ctx context.Context, sid string) (int, error) {
	det := imaging.NewDetector(s.modelSettings(ctx).Vision)
	if det == nil {
		return -1, fmt.Errorf("Bildanalyse ist deaktiviert – Bilder können manuell geprüft werden")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM images WHERE snapshot_id=$1 AND status IN ('pending','failed')`, sid)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	var lastErr error
	for _, id := range ids {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if err := s.detectWith(ctx, id, det); err != nil {
			lastErr = err
		}
	}
	if lastErr != nil && len(ids) > 0 {
		return len(ids), fmt.Errorf("einige Bilder konnten nicht analysiert werden: %v", lastErr)
	}
	return len(ids), nil
}

func (s *Server) detectImage(ctx context.Context, iid string) error {
	det := imaging.NewDetector(s.modelSettings(ctx).Vision)
	if det == nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE images SET status='review', error='Bildanalyse ist deaktiviert – bitte manuell prüfen', updated_at=now() WHERE id=$1`, iid)
		return nil
	}
	return s.detectWith(ctx, iid, det)
}

func (s *Server) detectWith(ctx context.Context, iid string, det imaging.Detector) error {
	im, err := s.loadImage(ctx, iid)
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE images SET status='detecting', updated_at=now() WHERE id=$1`, iid)
	img, _, err := imaging.Load(filepath.Join(s.snapshotDir(im.SnapshotID), filepath.FromSlash(im.Path)))
	if err != nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE images SET status='failed', error=$1, updated_at=now() WHERE id=$2`, "Bild nicht lesbar: "+err.Error(), iid)
		return err
	}
	rgba := imaging.ToRGBA(img)
	dctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	dets, err := det.Detect(dctx, rgba)
	cancel()
	w, h := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	if err != nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE images SET status='failed', width=$1, height=$2, error=$3, detector=$4, updated_at=now() WHERE id=$5`,
			w, h, err.Error(), det.Name(), iid)
		return err
	}
	// keep manual boxes from earlier reviews (they replace overlapping auto boxes)
	var manual []imaging.Detection
	for _, d := range im.Detections {
		if d.Source == "manual" {
			manual = append(manual, d)
		}
	}
	var merged []imaging.Detection
	for _, d := range dets {
		dup := false
		for _, m := range manual {
			if iou(d.Box, m.Box) > 0.5 {
				dup = true
			}
		}
		if !dup {
			merged = append(merged, d)
		}
	}
	dets = append(merged, manual...)
	for i := range dets {
		if dets[i].ID == "" {
			dets[i].ID = auth.NewToken()[:10]
		}
		if dets[i].Mode == "" {
			dets[i].Mode = imaging.DefaultMode(dets[i].Label)
		}
	}
	status := "clean"
	if len(dets) > 0 {
		status = "review"
	}
	b, _ := json.Marshal(dets)
	_, err = s.db.ExecContext(ctx, `UPDATE images SET status=$1, width=$2, height=$3, detections=$4, detector=$5, error='', anonymized_path='', updated_at=now() WHERE id=$6`,
		status, w, h, string(b), det.Name(), iid)
	return err
}

// imageFindings converts image results into findings for the report.
func (s *Server) imageFindings(ctx context.Context, sid string) []analysis.Finding {
	rows, err := s.db.QueryContext(ctx, `SELECT `+imageCols+` FROM images i JOIN files f ON f.id=i.file_id WHERE i.snapshot_id=$1`, sid)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []analysis.Finding
	for rows.Next() {
		im, err := scanImage(rows)
		if err != nil {
			continue
		}
		f := analysis.Finding{FileID: im.FileID, File: im.Path, Category: analysis.CatImages}
		switch im.Status {
		case "review":
			var parts []string
			for _, d := range im.Detections {
				parts = append(parts, imaging.LabelDE(d.Label))
			}
			f.Code, f.Severity = "IMAGE_BODY_PARTS", analysis.SevError
			f.Message = fmt.Sprintf("Bild %s zeigt menschliche Körperteile (%s) – laut Standard unzulässig, muss unkenntlich gemacht werden.", im.ICN, strings.Join(dedupStr(parts), ", "))
			f.Suggestion = "Im Bild-Editor prüfen und anonymisieren (Hände bleiben als Silhouette erkennbar)."
		case "failed":
			f.Code, f.Severity, f.Message = "IMAGE_CHECK_FAILED", analysis.SevWarning, fmt.Sprintf("Bild %s konnte nicht geprüft werden: %s", im.ICN, im.Error)
		case "unsupported":
			f.Code, f.Severity, f.Message = "IMAGE_MANUAL_CHECK", analysis.SevInfo, fmt.Sprintf("Bild %s: %s", im.ICN, im.Error)
		default:
			continue
		}
		out = append(out, f)
	}
	return out
}

func iou(a, b [4]int) float64 {
	x1, y1 := max(a[0], b[0]), max(a[1], b[1])
	x2, y2 := min(a[2], b[2]), min(a[3], b[3])
	if x2 <= x1 || y2 <= y1 {
		return 0
	}
	inter := float64((x2 - x1) * (y2 - y1))
	area := func(r [4]int) float64 { return float64((r[2] - r[0]) * (r[3] - r[1])) }
	return inter / (area(a) + area(b) - inter)
}

func dedupStr(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// ---------------------------------------------------------------- handlers

func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	q := `SELECT ` + imageCols + ` FROM images i JOIN files f ON f.id=i.file_id WHERE i.snapshot_id=$1`
	args := []any{sid}
	if st := r.URL.Query().Get("status"); st != "" {
		q += ` AND i.status = ANY(string_to_array($2, ','))`
		args = append(args, st)
	}
	q += ` ORDER BY CASE i.status WHEN 'review' THEN 0 WHEN 'failed' THEN 1 WHEN 'detecting' THEN 2 WHEN 'pending' THEN 3 ELSE 4 END, f.path`
	rows, err := s.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []*ImageInfo{}
	for rows.Next() {
		im, err := scanImage(rows)
		if err != nil {
			fail(w, err)
			return
		}
		out = append(out, im)
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleGetImage(w http.ResponseWriter, r *http.Request, u *User) {
	im, err := s.requireImage(r.Context(), r.PathValue("iid"), u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	// where is the image used?
	rows, err := s.db.QueryContext(r.Context(), `SELECT DISTINCT f.dmc FROM chunks c JOIN files f ON f.id=c.file_id
		WHERE c.snapshot_id=$1 AND c.element='figure' AND c.text ILIKE '%' || $2 || '%' AND f.dmc<>''`, im.SnapshotID, im.ICN)
	if err == nil {
		for rows.Next() {
			var d string
			_ = rows.Scan(&d)
			im.UsedIn = append(im.UsedIn, d)
		}
		rows.Close()
	}
	writeJSON(w, 200, im)
}

func (s *Server) handleImageOriginal(w http.ResponseWriter, r *http.Request, u *User) {
	im, err := s.requireImage(r.Context(), r.PathValue("iid"), u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	p := filepath.Join(s.snapshotDir(im.SnapshotID), filepath.FromSlash(im.Path))
	ext := strings.ToLower(filepath.Ext(im.Path))
	if ext == ".tif" || ext == ".tiff" || ext == ".bmp" {
		// browsers cannot display TIFF/BMP: convert to PNG
		img, _, err := imaging.Load(p)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_ = imaging.Encode(w, img, "png")
		return
	}
	w.Header().Set("Content-Type", contentType(im.Path))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, p)
}

func (s *Server) handleImageAnonymized(w http.ResponseWriter, r *http.Request, u *User) {
	im, err := s.requireImage(r.Context(), r.PathValue("iid"), u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	var ap string
	_ = s.db.QueryRowContext(r.Context(), `SELECT anonymized_path FROM images WHERE id=$1`, im.ID).Scan(&ap)
	if ap == "" {
		fail(w, errNotFound)
		return
	}
	p := filepath.Join(s.anonDir(im.SnapshotID), ap)
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+path.Base(im.Path)+`"`)
		w.Header().Set("Content-Type", contentType(im.Path))
		http.ServeFile(w, r, p)
		return
	}
	ext := strings.ToLower(filepath.Ext(ap))
	if ext == ".tif" || ext == ".tiff" || ext == ".bmp" {
		img, _, err := imaging.Load(p)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_ = imaging.Encode(w, img, "png")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", contentType(ap))
	http.ServeFile(w, r, p)
}

func sanitizeDetections(in []imaging.Detection, wdt, hgt int) []imaging.Detection {
	modes := map[string]bool{}
	for _, m := range imaging.Modes {
		modes[m] = true
	}
	out := []imaging.Detection{}
	for _, d := range in {
		if wdt > 0 && hgt > 0 {
			d.Box = imaging.ClampBox(d.Box, wdt, hgt)
		}
		if d.Box[2]-d.Box[0] < 2 || d.Box[3]-d.Box[1] < 2 {
			continue
		}
		if d.ID == "" {
			d.ID = auth.NewToken()[:10]
		}
		if !modes[d.Mode] {
			d.Mode = imaging.DefaultMode(d.Label)
		}
		if d.Source != "auto" {
			d.Source = "manual"
		}
		d.Label = imaging.NormalizeLabel(d.Label)
		if d.Label == "" {
			d.Label = "hand"
		}
		out = append(out, d)
	}
	return out
}

func (s *Server) handleSaveDetections(w http.ResponseWriter, r *http.Request, u *User) {
	im, err := s.requireImage(r.Context(), r.PathValue("iid"), u, "editor")
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Detections []imaging.Detection `json:"detections"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	dets := sanitizeDetections(in.Detections, im.Width, im.Height)
	status := im.Status
	if status != "anonymized" {
		status = "review"
		if len(dets) == 0 {
			status = "clean"
		}
	}
	b, _ := json.Marshal(dets)
	if _, err := s.db.ExecContext(r.Context(), `UPDATE images SET detections=$1, status=$2, reviewed_by=$3, updated_at=now() WHERE id=$4`,
		string(b), status, u.ID, im.ID); err != nil {
		fail(w, err)
		return
	}
	im, _ = s.loadImage(r.Context(), im.ID)
	writeJSON(w, 200, im)
}

func (s *Server) handleRedetect(w http.ResponseWriter, r *http.Request, u *User) {
	im, err := s.requireImage(r.Context(), r.PathValue("iid"), u, "editor")
	if err != nil {
		fail(w, err)
		return
	}
	if r.URL.Query().Get("async") == "1" {
		_ = s.rdb.Enqueue(cache.Job{Type: "detect", ID: im.ID})
		writeJSON(w, 202, im)
		return
	}
	if err := s.detectImage(r.Context(), im.ID); err != nil {
		writeErr(w, 502, "Erkennung fehlgeschlagen: "+err.Error())
		return
	}
	im, _ = s.loadImage(r.Context(), im.ID)
	writeJSON(w, 200, im)
}

func (s *Server) renderAnonymized(im *ImageInfo, dets []imaging.Detection) ([]byte, string, error) {
	img, format, err := imaging.Load(filepath.Join(s.snapshotDir(im.SnapshotID), filepath.FromSlash(im.Path)))
	if err != nil {
		return nil, "", err
	}
	out := imaging.Anonymize(img, sanitizeDetections(dets, img.Bounds().Dx(), img.Bounds().Dy()))
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, out, format); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), format, nil
}

// handlePreview renders an anonymized PNG preview without saving.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request, u *User) {
	im, err := s.requireImage(r.Context(), r.PathValue("iid"), u, "viewer")
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Detections []imaging.Detection `json:"detections"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	img, _, err := imaging.Load(filepath.Join(s.snapshotDir(im.SnapshotID), filepath.FromSlash(im.Path)))
	if err != nil {
		fail(w, err)
		return
	}
	out := imaging.Anonymize(img, sanitizeDetections(in.Detections, img.Bounds().Dx(), img.Bounds().Dy()))
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_ = imaging.Encode(w, out, "png")
}

// handleAnonymize saves the anonymized version (same format and name as
// the original, used by the export).
func (s *Server) handleAnonymize(w http.ResponseWriter, r *http.Request, u *User) {
	im, err := s.requireImage(r.Context(), r.PathValue("iid"), u, "editor")
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Detections []imaging.Detection `json:"detections"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	if len(in.Detections) == 0 {
		writeErr(w, 400, "Keine Bereiche zum Anonymisieren markiert")
		return
	}
	data, _, err := s.renderAnonymized(im, in.Detections)
	if err != nil {
		fail(w, err)
		return
	}
	if err := os.MkdirAll(s.anonDir(im.SnapshotID), 0o755); err != nil {
		fail(w, err)
		return
	}
	name := im.ID + strings.ToLower(filepath.Ext(im.Path))
	if err := os.WriteFile(filepath.Join(s.anonDir(im.SnapshotID), name), data, 0o644); err != nil {
		fail(w, err)
		return
	}
	dets := sanitizeDetections(in.Detections, im.Width, im.Height)
	b, _ := json.Marshal(dets)
	if _, err := s.db.ExecContext(r.Context(), `UPDATE images SET status='anonymized', anonymized_path=$1, detections=$2, reviewed_by=$3, updated_at=now() WHERE id=$4`,
		name, string(b), u.ID, im.ID); err != nil {
		fail(w, err)
		return
	}
	im, _ = s.loadImage(r.Context(), im.ID)
	writeJSON(w, 200, im)
}

// handleMarkClean confirms that an image contains no body parts.
func (s *Server) handleMarkClean(w http.ResponseWriter, r *http.Request, u *User) {
	im, err := s.requireImage(r.Context(), r.PathValue("iid"), u, "editor")
	if err != nil {
		fail(w, err)
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE images SET status='clean', detections='[]', anonymized_path='', reviewed_by=$1, updated_at=now() WHERE id=$2`,
		u.ID, im.ID); err != nil {
		fail(w, err)
		return
	}
	im, _ = s.loadImage(r.Context(), im.ID)
	writeJSON(w, 200, im)
}
