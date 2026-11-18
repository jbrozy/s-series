package server

import (
	"bytes"
	"mime"
	"net/http"
	"strconv"
	"time"

	"scheck/internal/rules"
	"scheck/internal/synth"
)

// handleSynthetic generates a JSONL training dataset from the snapshot.
func (s *Server) handleSynthetic(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	pid, err := s.requireSnapshot(r.Context(), sid, u, "editor")
	if err != nil {
		fail(w, err)
		return
	}
	q := r.URL.Query()
	opts := synth.Options{Format: q.Get("format"), Input: q.Get("input"), Focus: q.Get("focus"), Seed: time.Now().UnixNano()}
	opts.Count, _ = strconv.Atoi(q.Get("count"))
	if v, err := strconv.ParseFloat(q.Get("cleanRatio"), 64); err == nil {
		opts.CleanRatio = v
	}
	if v, err := strconv.ParseInt(q.Get("seed"), 10, 64); err == nil && q.Get("seed") != "" {
		opts.Seed = v
	}
	c, _, _, err := s.loadCorpus(r.Context(), sid)
	if err != nil {
		fail(w, err)
		return
	}
	rs, err := s.loadRules(r.Context(), pid)
	if err != nil {
		fail(w, err)
		return
	}
	c.Dictionary = rules.Dictionary(rs)
	var buf bytes.Buffer
	stats, err := synth.Generate(c, rs, opts, &buf)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if q.Get("stats") == "1" {
		writeJSON(w, 200, stats)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "scheck-training-" + sid[:8] + ".jsonl"}))
	w.Header().Set("X-Samples", strconv.Itoa(stats.Written))
	_, _ = w.Write(buf.Bytes())
}
