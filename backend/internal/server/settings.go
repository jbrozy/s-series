package server

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"net/http"
	"time"

	"scheck/internal/cache"
	"scheck/internal/imaging"
	"scheck/internal/llm"
)

type modelSettingsT = llm.Settings

const modelsKey = "models"

// modelSettings returns the stored model settings (defaults from env).
func (s *Server) modelSettings(ctx context.Context) llm.Settings {
	s.settingsMu.RLock()
	if s.settings != nil {
		v := *s.settings
		s.settingsMu.RUnlock()
		return v
	}
	s.settingsMu.RUnlock()
	ms := s.cfg.DefaultModels
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, modelsKey).Scan(&raw); err == nil {
		_ = json.Unmarshal([]byte(raw), &ms)
	}
	s.settingsMu.Lock()
	s.settings = &ms
	s.settingsMu.Unlock()
	return ms
}

func (s *Server) handleGetModels(w http.ResponseWriter, r *http.Request, _ *User) {
	writeJSON(w, 200, s.modelSettings(r.Context()).MaskSecrets())
}

func (s *Server) handlePutModels(w http.ResponseWriter, r *http.Request, me *User) {
	var in llm.Settings
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	in = in.MergeSecrets(s.modelSettings(r.Context()))
	switch in.Vision.Provider {
	case "none", "openai", "http", "heuristic":
	default:
		writeErr(w, 400, "Unbekannter Bildanalyse-Anbieter")
		return
	}
	b, _ := json.Marshal(in)
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO settings(key, value) VALUES ($1,$2)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, modelsKey, string(b)); err != nil {
		fail(w, err)
		return
	}
	s.settingsMu.Lock()
	s.settings = &in
	s.settingsMu.Unlock()
	if in.Embedding.Ready() {
		// backfill embeddings for existing snapshots
		rows, err := s.db.QueryContext(r.Context(), `SELECT DISTINCT snapshot_id FROM chunks WHERE embedding IS NULL`)
		if err == nil {
			for rows.Next() {
				var sid string
				_ = rows.Scan(&sid)
				_ = s.rdb.Enqueue(cache.Job{Type: "embed", ID: sid})
			}
			rows.Close()
		}
	}
	s.audit(r, me, "", "settings.models", "settings", "models", "Modell-Einstellungen geändert", map[string]any{"settings": in.MaskSecrets()})
	writeJSON(w, 200, in.MaskSecrets())
}

// handleTestModel sends a tiny request to the selected model.
func (s *Server) handleTestModel(w http.ResponseWriter, r *http.Request, _ *User) {
	var in struct {
		Kind     string        `json:"kind"`
		Settings *llm.Settings `json:"settings"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	ms := s.modelSettings(r.Context())
	if in.Settings != nil {
		ms = in.Settings.MergeSecrets(ms)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	start := time.Now()
	result := map[string]any{"ok": false}
	switch in.Kind {
	case "chat":
		ep := ms.Chat
		ep.Enabled = true
		ans, err := ep.Chat(ctx, []llm.Message{{Role: "user", Content: `Antworte nur mit dem JSON {"ok":true}.`}})
		if err != nil {
			result["error"] = err.Error()
		} else {
			result["ok"], result["answer"] = true, truncateRunes(ans, 300)
		}
	case "embedding":
		ep := ms.Embedding
		ep.Enabled = true
		v, err := ep.Embed(ctx, []string{"Hydraulikpumpe ausbauen"})
		if err != nil {
			result["error"] = err.Error()
		} else {
			result["ok"], result["dimensions"] = true, len(v[0])
		}
	case "vision":
		det := imaging.NewDetector(ms.Vision)
		if det == nil {
			result["error"] = "Bildanalyse ist deaktiviert"
			break
		}
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for i := range img.Pix {
			img.Pix[i] = 255
		}
		img.Set(10, 10, color.Black)
		dets, err := det.Detect(ctx, img)
		if err != nil {
			result["error"] = err.Error()
		} else {
			result["ok"], result["detections"] = true, len(dets)
		}
	default:
		writeErr(w, 400, "kind muss chat, embedding oder vision sein")
		return
	}
	result["ms"] = time.Since(start).Milliseconds()
	writeJSON(w, 200, result)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, _ *User) {
	var users, projects, snapshots, running int
	_ = s.db.QueryRowContext(r.Context(), `SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM projects), (SELECT count(*) FROM snapshots),
		(SELECT count(*) FROM analysis_runs WHERE status IN ('queued','running'))`).Scan(&users, &projects, &snapshots, &running)
	writeJSON(w, 200, map[string]any{"users": users, "projects": projects, "snapshots": snapshots, "activeRuns": running,
		"queueLength": s.rdb.QueueLength()})
}
