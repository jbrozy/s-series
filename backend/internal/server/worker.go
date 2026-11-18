package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"

	"scheck/internal/analysis"
	"scheck/internal/cache"
	"scheck/internal/graph"
	"scheck/internal/imaging"
	"scheck/internal/rules"
	"scheck/internal/s1000d"
)

// RunWorkers processes background jobs until ctx is cancelled.
func (s *Server) RunWorkers(ctx context.Context, n int) {
	s.recoverStaleJobs(ctx)
	var wg sync.WaitGroup
	for i := 0; i < max(1, n); i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for ctx.Err() == nil {
				job, err := s.rdb.Dequeue(5 * time.Second)
				if err != nil {
					log.Printf("Worker %d: Queue-Fehler: %v", id, err)
					time.Sleep(3 * time.Second)
					continue
				}
				if job == nil {
					continue
				}
				s.runJob(ctx, *job)
			}
		}(i)
	}
	wg.Wait()
}

// recoverStaleJobs re-enqueues work that was interrupted by a restart.
func (s *Server) recoverStaleJobs(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, status FROM analysis_runs r WHERE status IN ('queued','running')
		AND NOT EXISTS (SELECT 1 FROM snapshots sn WHERE sn.id=r.snapshot_id AND sn.status IN ('uploaded','indexing'))`)
	if err == nil {
		for rows.Next() {
			var id, st string
			_ = rows.Scan(&id, &st)
			_ = s.rdb.Enqueue(cache.Job{Type: "analyze", ID: id})
		}
		rows.Close()
	}
	rows, err = s.db.QueryContext(ctx, `SELECT s.id, COALESCE((SELECT r.id::text FROM analysis_runs r WHERE r.snapshot_id=s.id AND r.status='queued' ORDER BY created_at DESC LIMIT 1),'')
		FROM snapshots s WHERE s.status IN ('uploaded','indexing')`)
	if err == nil {
		for rows.Next() {
			var sid, rid string
			_ = rows.Scan(&sid, &rid)
			_ = s.rdb.Enqueue(cache.Job{Type: "index", ID: sid, Arg: rid})
		}
		rows.Close()
	}
}

func (s *Server) runJob(ctx context.Context, j cache.Job) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("Job %s %s abgestürzt: %v", j.Type, j.ID, v)
		}
	}()
	start := time.Now()
	var err error
	switch j.Type {
	case "index":
		err = s.indexSnapshot(ctx, j.ID)
		if err == nil && j.Arg != "" {
			err = s.analyze(ctx, j.Arg)
		} else if err != nil && j.Arg != "" {
			s.failRun(j.Arg, err)
		}
	case "analyze":
		err = s.analyze(ctx, j.ID)
	case "detect":
		err = s.detectImage(ctx, j.ID)
	case "embed":
		err = s.embedSnapshot(ctx, j.ID)
	default:
		err = fmt.Errorf("unbekannter Job-Typ %q", j.Type)
	}
	if err != nil {
		log.Printf("Job %s %s fehlgeschlagen: %v", j.Type, j.ID, err)
		return
	}
	log.Printf("Job %s %s erledigt in %s", j.Type, j.ID, time.Since(start).Round(time.Millisecond))
}

func (s *Server) failRun(rid string, err error) {
	_, _ = s.db.Exec(`UPDATE analysis_runs SET status='failed', error=$1, finished_at=now(), message='Fehlgeschlagen' WHERE id=$2`, err.Error(), rid)
}

func (s *Server) progress(rid string, pct int, msg string) {
	_, _ = s.db.Exec(`UPDATE analysis_runs SET progress=$1, message=$2 WHERE id=$3`, pct, msg, rid)
}

// ---------------------------------------------------------------- indexing

type fileRow struct {
	id, path, kind string
}

func (s *Server) snapshotFiles(ctx context.Context, sid string) ([]fileRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, path, kind FROM files WHERE snapshot_id=$1 ORDER BY path`, sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fileRow
	for rows.Next() {
		var f fileRow
		if err := rows.Scan(&f.id, &f.path, &f.kind); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// loadCorpus parses all XML files of a snapshot.
func (s *Server) loadCorpus(ctx context.Context, sid string) (*analysis.Corpus, []fileRow, map[string]string, error) {
	files, err := s.snapshotFiles(ctx, sid)
	if err != nil {
		return nil, nil, nil, err
	}
	c := &analysis.Corpus{ICNs: map[string]string{}, Dictionary: map[string]bool{}}
	parseErrs := map[string]string{}
	for _, f := range files {
		if s1000d.IsRaster(f.path) || s1000d.IsVector(f.path) {
			c.ICNs[s1000d.ICNKey(f.path)] = f.path
			continue
		}
		if !s1000d.IsMarkup(f.path) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.snapshotDir(sid), filepath.FromSlash(f.path)))
		if err != nil {
			parseErrs[f.id] = err.Error()
			continue
		}
		d, err := s1000d.Parse(f.id, f.path, data)
		if err != nil {
			parseErrs[f.id] = err.Error()
			continue
		}
		c.Docs = append(c.Docs, d)
	}
	c.Docs = s1000d.MergeHTML(c.Docs)
	return c, files, parseErrs, nil
}

func (s *Server) indexSnapshot(ctx context.Context, sid string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE snapshots SET status='indexing', error='' WHERE id=$1`, sid); err != nil {
		return err
	}
	c, files, parseErrs, err := s.loadCorpus(ctx, sid)
	if err == nil {
		err = s.writeIndex(ctx, sid, c, files, parseErrs)
	}
	if err != nil {
		_, _ = s.db.Exec(`UPDATE snapshots SET status='failed', error=$1 WHERE id=$2`, err.Error(), sid)
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE snapshots SET status='ready' WHERE id=$1`, sid)
	if s.modelSettings(ctx).Embedding.Ready() {
		_ = s.rdb.Enqueue(cache.Job{Type: "embed", ID: sid})
	}
	return err
}

func (s *Server) writeIndex(ctx context.Context, sid string, c *analysis.Corpus, files []fileRow, parseErrs map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE snapshot_id=$1`, sid); err != nil {
		return err
	}
	for id, msg := range parseErrs {
		if _, err := tx.ExecContext(ctx, `UPDATE files SET parse_error=$1 WHERE id=$2`, msg, id); err != nil {
			return err
		}
	}
	upd, err := tx.PrepareContext(ctx, `UPDATE files SET kind=$1, dmc=$2, issue=$3, lang=$4, title=$5, info_code=$6, parse_error='' WHERE id=$7`)
	if err != nil {
		return err
	}
	ins, err := tx.PrepareContext(ctx, `INSERT INTO chunks(snapshot_id, file_id, breadcrumb, element, path, line, text) VALUES ($1,$2,$3,$4,$5,$6,$7)`)
	if err != nil {
		return err
	}
	for _, d := range c.Docs {
		if _, err := upd.ExecContext(ctx, d.Kind, d.DMC, d.Issue, d.Lang, d.Title(), d.InfoCode, d.FileID); err != nil {
			return err
		}
		for _, ch := range s1000d.Chunks(d) {
			if _, err := ins.ExecContext(ctx, sid, d.FileID, ch.Breadcrumb, ch.Element, ch.Path, ch.Line, ch.Text); err != nil {
				return err
			}
		}
	}
	upd.Close()
	ins.Close()
	if err := storeGraph(ctx, tx, sid, graph.Build(c, icnFileMap(files))); err != nil {
		return fmt.Errorf("Wissensgraph: %w", err)
	}
	// image rows
	for _, f := range files {
		if f.kind != s1000d.KindICN {
			continue
		}
		status := "pending"
		errMsg := ""
		w, h := 0, 0
		if !s1000d.IsRaster(f.path) {
			status = "unsupported"
			errMsg = "Vektorformat (" + strings.ToUpper(strings.TrimPrefix(filepath.Ext(f.path), ".")) + "). Bitte manuell oder als Rasterexport prüfen."
		} else if iw, ih, err := imaging.Size(filepath.Join(s.snapshotDir(sid), filepath.FromSlash(f.path))); err == nil {
			w, h = iw, ih
		} else {
			status, errMsg = "failed", "Bild nicht lesbar: "+err.Error()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO images(snapshot_id, file_id, status, error, width, height) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (file_id) DO NOTHING`, sid, f.id, status, errMsg, w, h); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------------------------------------------------------------- analysis

func (s *Server) analyze(ctx context.Context, rid string) error {
	run, err := s.loadRun(ctx, rid)
	if err != nil {
		return err
	}
	if run.Status == "done" {
		return nil
	}
	sid := run.SnapshotID
	pid, err := s.projectOfSnapshot(ctx, sid)
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE analysis_runs SET status='running', started_at=now(), progress=2, message='Lade Projektdateien', error='' WHERE id=$1`, rid)
	err = s.doAnalyze(ctx, rid, sid, pid, run.Options)
	if err != nil {
		s.failRun(rid, err)
	}
	return err
}

func (s *Server) doAnalyze(ctx context.Context, rid, sid, pid string, opts analysis.Options) error {
	c, _, parseErrs, err := s.loadCorpus(ctx, sid)
	if err != nil {
		return err
	}
	rs, err := s.loadRules(ctx, pid)
	if err != nil {
		return err
	}
	c.Dictionary = rules.Dictionary(rs)
	s.progress(rid, 10, fmt.Sprintf("%d Dokumente geladen, semantische Prüfung läuft", len(c.Docs)))

	findings := analysis.RunDeterministic(c, opts)
	for id, msg := range parseErrs {
		findings = append(findings, analysis.Finding{FileID: id, Category: analysis.CatConsistency, Code: "XML_UNREADABLE",
			Severity: analysis.SevError, Message: "Datei konnte nicht gelesen werden: " + msg})
	}
	if opts.Rules {
		s.progress(rid, 35, "Business Rules werden geprüft")
		findings = append(findings, rules.Evaluate(c, rs)...)
	}
	var notes []string
	if opts.LLM {
		ms := s.modelSettings(ctx)
		if !ms.Chat.Ready() {
			notes = append(notes, "KI-Review übersprungen: kein Chat-Modell konfiguriert")
		} else {
			if ms.Embedding.Ready() {
				s.progress(rid, 38, "Embeddings für Kontextsuche werden berechnet")
				if err := s.embedSnapshot(ctx, sid); err != nil {
					notes = append(notes, "Embeddings: "+err.Error())
				}
			}
			lf, n, err := s.llmReview(ctx, rid, sid, c, rs, ms, opts.LLMMaxDocs)
			findings = append(findings, lf...)
			notes = append(notes, fmt.Sprintf("KI-Review: %d Datenmodule geprüft", n))
			if err != nil {
				notes = append(notes, "KI-Review abgebrochen: "+err.Error())
			}
		}
	}
	if opts.Images {
		s.progress(rid, 80, "Bilder werden auf Körperteile geprüft")
		n, err := s.detectSnapshotImages(ctx, sid)
		if err != nil {
			notes = append(notes, "Bildprüfung: "+err.Error())
		} else if n > 0 {
			notes = append(notes, fmt.Sprintf("Bildprüfung: %d Bilder analysiert", n))
		} else if n == 0 {
			notes = append(notes, "Bildprüfung: keine neuen Bilder. Bereits geprüfte Bilder behalten ihren Status.")
		}
		findings = append(findings, s.imageFindings(ctx, sid)...)
	}
	s.progress(rid, 92, "Befunde werden gespeichert")
	return s.storeFindings(ctx, rid, pid, findings, notes, len(c.Docs))
}

func (s *Server) storeFindings(ctx context.Context, rid, pid string, findings []analysis.Finding, notes []string, docs int) error {
	suppressed := map[string]string{}
	rows, err := s.db.QueryContext(ctx, `SELECT fingerprint, note FROM finding_suppressions WHERE project_id=$1`, pid)
	if err == nil {
		for rows.Next() {
			var fp, note string
			_ = rows.Scan(&fp, &note)
			suppressed[fp] = note
		}
		rows.Close()
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].DMC != findings[j].DMC {
			return findings[i].DMC < findings[j].DMC
		}
		return findings[i].Line < findings[j].Line
	})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM findings WHERE run_id=$1`, rid); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn("findings", "run_id", "file_id", "file_path", "dmc", "category", "code", "severity",
		"message", "xpath", "line", "context", "suggestion", "fingerprint", "status", "status_note"))
	if err != nil {
		return err
	}
	bySev := map[string]int{}
	byCat := map[string]int{}
	dismissed := 0
	seen := map[string]bool{}
	for _, f := range findings {
		fp := f.Fingerprint()
		if seen[fp] {
			continue
		}
		seen[fp] = true
		status, note := "open", ""
		if n, ok := suppressed[fp]; ok {
			status, note = "dismissed", n
			dismissed++
		} else {
			bySev[f.Severity]++
		}
		byCat[f.Category]++
		var fileID any
		if f.FileID != "" {
			fileID = f.FileID
		}
		if _, err := stmt.ExecContext(ctx, rid, fileID, f.File, f.DMC, f.Category, f.Code, f.Severity, f.Message, f.Path, f.Line,
			f.Context, f.Suggestion, fp, status, note); err != nil {
			return err
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		return err
	}
	stmt.Close()
	sum, _ := json.Marshal(map[string]any{"bySeverity": bySev, "byCategory": byCat, "dismissed": dismissed, "total": len(seen),
		"documents": docs, "notes": notes})
	if _, err := tx.ExecContext(ctx, `UPDATE analysis_runs SET status='done', progress=100, message='Fertig', summary=$1, finished_at=now() WHERE id=$2`,
		string(sum), rid); err != nil {
		return err
	}
	return tx.Commit()
}

// llmReview runs the LLM review on the most relevant data modules.
func (s *Server) llmReview(ctx context.Context, rid, sid string, c *analysis.Corpus, rs []rules.Rule, ms modelSettingsT, maxDocs int) ([]analysis.Finding, int, error) {
	if maxDocs <= 0 {
		maxDocs = 25
	}
	dms := []*s1000d.Doc{}
	for _, d := range c.Docs {
		if d.Kind == s1000d.KindDM {
			dms = append(dms, d)
		}
	}
	if len(dms) > maxDocs {
		dms = dms[:maxDocs]
	}
	rulesText := rules.Describe(rs)
	llmCache := cache.LLMCache{R: s.rdb, TTL: 30 * 24 * time.Hour}
	var out []analysis.Finding
	for i, d := range dms {
		if ctx.Err() != nil {
			return out, i, ctx.Err()
		}
		s.progress(rid, 40+int(35*float64(i)/float64(len(dms))), fmt.Sprintf("KI-Review %d/%d: %s", i+1, len(dms), d.Label()))
		related := s.relatedContext(ctx, sid, d, ms, 5)
		cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		fs, err := analysis.ReviewWithLLM(cctx, ms.Chat, d, rulesText, related, llmCache)
		cancel()
		out = append(out, fs...)
		if err != nil {
			out = append(out, analysis.Finding{FileID: d.FileID, File: d.Path, DMC: d.DMC, Category: analysis.CatLLM, Code: "LLM_ERROR",
				Severity: analysis.SevInfo, Message: "KI-Review für dieses Datenmodul fehlgeschlagen: " + err.Error()})
		}
	}
	return out, len(dms), nil
}

// relatedContext returns short snippets of other modules similar to d.
func (s *Server) relatedContext(ctx context.Context, sid string, d *s1000d.Doc, ms modelSettingsT, k int) []string {
	// structurally related modules from the knowledge graph first
	out := s.graphContext(ctx, sid, d.FileID, 3)
	query := d.Title()
	if query == "" {
		return out
	}
	hits, err := s.search(ctx, sid, query, k+5, ms)
	if err != nil {
		return out
	}
	for _, h := range hits {
		if h.FileID == d.FileID {
			continue
		}
		out = append(out, h.Breadcrumb+": "+truncateRunes(h.Text, 300))
		if len(out) >= k {
			break
		}
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
