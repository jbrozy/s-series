package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/lib/pq"

	"scheck/internal/llm"
)

type SearchHit struct {
	ChunkID    int64   `json:"chunkId"`
	FileID     string  `json:"fileId"`
	DMC        string  `json:"dmc"`
	Breadcrumb string  `json:"breadcrumb"`
	Element    string  `json:"element"`
	Line       int     `json:"line"`
	Text       string  `json:"text"`
	Score      float64 `json:"score"`
	Mode       string  `json:"mode"`
}

// search combines full text, trigram similarity and (if configured)
// embeddings, so that knowledge from XML data modules is retrievable.
func (s *Server) search(ctx context.Context, sid, q string, k int, ms llm.Settings) ([]SearchHit, error) {
	hits := map[int64]*SearchHit{}
	// OR semantics for full text: every word may contribute
	words := strings.Fields(strings.NewReplacer("\"", " ", "-", " ", "'", " ").Replace(q))
	tsq := strings.Join(words, " or ")
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.file_id, f.dmc, c.breadcrumb, c.element, c.line, c.text,
		ts_rank(c.tsv, websearch_to_tsquery('simple', $4)) + word_similarity($2, c.text) + word_similarity($2, c.breadcrumb)
		FROM chunks c JOIN files f ON f.id=c.file_id
		WHERE c.snapshot_id=$1 AND (c.tsv @@ websearch_to_tsquery('simple', $4) OR $2 <% c.text OR c.breadcrumb ILIKE '%' || $2 || '%')
		ORDER BY 8 DESC LIMIT $3`, sid, q, k*3, tsq)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		h := &SearchHit{Mode: "text"}
		if err := rows.Scan(&h.ChunkID, &h.FileID, &h.DMC, &h.Breadcrumb, &h.Element, &h.Line, &h.Text, &h.Score); err != nil {
			rows.Close()
			return nil, err
		}
		hits[h.ChunkID] = h
	}
	rows.Close()

	if ms.Embedding.Ready() {
		vecs, err := ms.Embedding.Embed(ctx, []string{q})
		if err == nil {
			qv := vecs[0]
			rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.file_id, f.dmc, c.breadcrumb, c.element, c.line, c.text, c.embedding
				FROM chunks c JOIN files f ON f.id=c.file_id WHERE c.snapshot_id=$1 AND c.embedding IS NOT NULL`, sid)
			if err == nil {
				type scored struct {
					h SearchHit
					s float64
				}
				var all []scored
				for rows.Next() {
					var h SearchHit
					var emb pq.Float64Array
					if err := rows.Scan(&h.ChunkID, &h.FileID, &h.DMC, &h.Breadcrumb, &h.Element, &h.Line, &h.Text, &emb); err != nil {
						continue
					}
					all = append(all, scored{h, cosine(qv, emb)})
				}
				rows.Close()
				sort.Slice(all, func(i, j int) bool { return all[i].s > all[j].s })
				for i := 0; i < len(all) && i < k*2; i++ {
					sc := all[i]
					if ex, ok := hits[sc.h.ChunkID]; ok {
						ex.Score += sc.s
						ex.Mode = "hybrid"
					} else {
						h := sc.h
						h.Score, h.Mode = sc.s, "semantic"
						hits[h.ChunkID] = &h
					}
				}
			}
		}
	}
	out := make([]SearchHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > k {
		out = out[:k]
	}
	return out, nil
}

func cosine(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// embedSnapshot computes embeddings for all chunks without one.
func (s *Server) embedSnapshot(ctx context.Context, sid string) error {
	ms := s.modelSettings(ctx)
	if !ms.Embedding.Ready() {
		return nil
	}
	for {
		rows, err := s.db.QueryContext(ctx, `SELECT id, breadcrumb, text FROM chunks WHERE snapshot_id=$1 AND embedding IS NULL ORDER BY id LIMIT 32`, sid)
		if err != nil {
			return err
		}
		var ids []int64
		var texts []string
		for rows.Next() {
			var id int64
			var bc, t string
			_ = rows.Scan(&id, &bc, &t)
			ids = append(ids, id)
			texts = append(texts, truncateRunes(bc+"\n"+t, 2000))
		}
		rows.Close()
		if len(ids) == 0 {
			return nil
		}
		vecs, err := ms.Embedding.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("Embeddings: %w", err)
		}
		for i, id := range ids {
			if _, err := s.db.ExecContext(ctx, `UPDATE chunks SET embedding=$1 WHERE id=$2`, pq.Float64Array(vecs[i]), id); err != nil {
				return err
			}
		}
	}
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, 200, []SearchHit{})
		return
	}
	k, _ := strconv.Atoi(r.URL.Query().Get("k"))
	if k <= 0 || k > 100 {
		k = 20
	}
	hits, err := s.search(r.Context(), sid, q, k, s.modelSettings(r.Context()))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, hits)
}

// handleAsk answers a question about the project documentation (RAG).
func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Question string `json:"question"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	ms := s.modelSettings(r.Context())
	if !ms.Chat.Ready() {
		writeErr(w, 400, "Kein Chat-Modell konfiguriert (Administration → Modelle)")
		return
	}
	hits, err := s.search(r.Context(), sid, in.Question, 8, ms)
	if err != nil {
		fail(w, err)
		return
	}
	var ctxText strings.Builder
	for i, h := range hits {
		fmt.Fprintf(&ctxText, "[%d] %s (Zeile %d)\n%s\n\n", i+1, h.Breadcrumb, h.Line, truncateRunes(h.Text, 1200))
	}
	ep := ms.Chat
	ep.JSONMode = false
	answer, err := ep.Chat(r.Context(), []llm.Message{
		{Role: "system", Content: "Du beantwortest Fragen zur technischen Dokumentation eines S1000D-Projekts. Nutze ausschließlich die bereitgestellten Auszüge, " +
			"zitiere sie mit [Nummer] und sage klar, wenn die Auszüge die Frage nicht beantworten. Antworte auf Deutsch."},
		{Role: "user", Content: "Auszüge:\n" + ctxText.String() + "\nFrage: " + in.Question},
	})
	if err != nil {
		writeErr(w, 502, "Modellfehler: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"answer": answer, "sources": hits})
}
