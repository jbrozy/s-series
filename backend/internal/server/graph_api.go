package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/lib/pq"

	"scheck/internal/graph"
	"scheck/internal/s1000d"
)

// storeGraph replaces the knowledge graph of a snapshot.
func storeGraph(ctx context.Context, tx *sql.Tx, sid string, g *graph.Graph) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM graph_nodes WHERE snapshot_id=$1`, sid); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn("graph_nodes", "snapshot_id", "key", "type", "label", "file_id", "props"))
	if err != nil {
		return err
	}
	for _, n := range g.Nodes() {
		props, _ := json.Marshal(n.Props)
		var fid any
		if n.FileID != "" {
			fid = n.FileID
		}
		if _, err := stmt.ExecContext(ctx, sid, n.Key, n.Type, n.Label, fid, string(props)); err != nil {
			return err
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		return err
	}
	stmt.Close()
	ids := map[string]int64{}
	rows, err := tx.QueryContext(ctx, `SELECT id, key FROM graph_nodes WHERE snapshot_id=$1`, sid)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var k string
		if err := rows.Scan(&id, &k); err != nil {
			rows.Close()
			return err
		}
		ids[k] = id
	}
	rows.Close()
	stmt, err = tx.PrepareContext(ctx, pq.CopyIn("graph_edges", "snapshot_id", "src", "dst", "type", "weight"))
	if err != nil {
		return err
	}
	for _, e := range g.Edges() {
		if _, err := stmt.ExecContext(ctx, sid, ids[e.Src], ids[e.Dst], e.Type, e.Weight); err != nil {
			return err
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		return err
	}
	return stmt.Close()
}

func icnFileMap(files []fileRow) map[string]string {
	m := map[string]string{}
	for _, f := range files {
		if s1000d.IsRaster(f.path) || s1000d.IsVector(f.path) {
			m[s1000d.ICNKey(f.path)] = f.id
		}
	}
	return m
}

// ensureGraph builds the graph for uploads indexed before the graph existed.
func (s *Server) ensureGraph(ctx context.Context, sid string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM graph_nodes WHERE snapshot_id=$1`, sid).Scan(&n); err != nil || n > 0 {
		return err
	}
	var status string
	_ = s.db.QueryRowContext(ctx, `SELECT status FROM snapshots WHERE id=$1`, sid).Scan(&status)
	if status != "ready" {
		return nil
	}
	c, files, _, err := s.loadCorpus(ctx, sid)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := storeGraph(ctx, tx, sid, graph.Build(c, icnFileMap(files))); err != nil {
		return err
	}
	return tx.Commit()
}

type gNode struct {
	ID       int64           `json:"id"`
	Key      string          `json:"key"`
	Type     string          `json:"type"`
	Label    string          `json:"label"`
	FileID   string          `json:"fileId,omitempty"`
	Props    json.RawMessage `json:"props"`
	Degree   int             `json:"degree"`
	Findings map[string]int  `json:"findings,omitempty"`
}

type gEdge struct {
	Source int64  `json:"source"`
	Target int64  `json:"target"`
	Type   string `json:"type"`
	Weight int    `json:"weight"`
}

var defaultGraphTypes = []string{graph.TModel, graph.TSNS, graph.TDM, graph.TPM, graph.TICN}

// typePriority decides what is dropped first when the graph is too large.
var typePriority = map[string]int{graph.TModel: 0, graph.TSNS: 1, graph.TPM: 2, graph.TDM: 3, graph.TICN: 4, graph.TPart: 5, graph.TAcronym: 6}

const maxGraphNodes = 4000

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request, u *User) {
	sid := r.PathValue("sid")
	if _, err := s.requireSnapshot(r.Context(), sid, u, "viewer"); err != nil {
		fail(w, err)
		return
	}
	if err := s.ensureGraph(r.Context(), sid); err != nil {
		fail(w, err)
		return
	}
	q := r.URL.Query()
	types := defaultGraphTypes
	if t := q.Get("types"); t != "" {
		types = strings.Split(t, ",")
	}
	want := map[string]bool{}
	for _, t := range types {
		want[t] = true
	}
	focus, _ := strconv.ParseInt(q.Get("focus"), 10, 64)
	depth, _ := strconv.Atoi(q.Get("depth"))
	if depth <= 0 || depth > 4 {
		depth = 2
	}

	// load the complete graph of the upload
	all := map[int64]*gNode{}
	counts := map[string]int{}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, key, type, label, COALESCE(file_id::text,''), props FROM graph_nodes WHERE snapshot_id=$1`, sid)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		n := &gNode{}
		var props string
		if err := rows.Scan(&n.ID, &n.Key, &n.Type, &n.Label, &n.FileID, &props); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		n.Props = json.RawMessage(props)
		all[n.ID] = n
		counts[n.Type]++
	}
	rows.Close()
	var edges []gEdge
	adj := map[int64][]int64{}
	rows, err = s.db.QueryContext(r.Context(), `SELECT src, dst, type, weight FROM graph_edges WHERE snapshot_id=$1`, sid)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var e gEdge
		if err := rows.Scan(&e.Source, &e.Target, &e.Type, &e.Weight); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		edges = append(edges, e)
		adj[e.Source] = append(adj[e.Source], e.Target)
		adj[e.Target] = append(adj[e.Target], e.Source)
		all[e.Source].Degree++
		all[e.Target].Degree++
	}
	rows.Close()

	// open findings of the latest analysis per file
	byFile := map[string]map[string]int{}
	rows, err = s.db.QueryContext(r.Context(), `SELECT file_id::text, severity, count(*) FROM findings
		WHERE status='open' AND file_id IS NOT NULL AND run_id=(SELECT id FROM analysis_runs WHERE snapshot_id=$1 AND status='done' ORDER BY created_at DESC LIMIT 1)
		GROUP BY 1, 2`, sid)
	if err == nil {
		for rows.Next() {
			var fid, sev string
			var n int
			_ = rows.Scan(&fid, &sev, &n)
			if byFile[fid] == nil {
				byFile[fid] = map[string]int{}
			}
			byFile[fid][sev] = n
		}
		rows.Close()
	}

	// selection: type filter, optionally neighbourhood of a focus node
	keep := map[int64]bool{}
	if focus > 0 && all[focus] != nil {
		dist := map[int64]int{focus: 0}
		queue := []int64{focus}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if dist[cur] >= depth {
				continue
			}
			for _, nb := range adj[cur] {
				if _, seen := dist[nb]; !seen && (want[all[nb].Type] || nb == focus) {
					dist[nb] = dist[cur] + 1
					queue = append(queue, nb)
				}
			}
		}
		for id := range dist {
			keep[id] = true
		}
	} else {
		for id, n := range all {
			if want[n.Type] {
				keep[id] = true
			}
		}
	}
	truncated := false
	if len(keep) > maxGraphNodes {
		truncated = true
		ids := make([]int64, 0, len(keep))
		for id := range keep {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool {
			a, b := all[ids[i]], all[ids[j]]
			if typePriority[a.Type] != typePriority[b.Type] {
				return typePriority[a.Type] < typePriority[b.Type]
			}
			return a.Degree > b.Degree
		})
		keep = map[int64]bool{}
		for _, id := range ids[:maxGraphNodes] {
			keep[id] = true
		}
	}

	outNodes := make([]*gNode, 0, len(keep))
	for id := range keep {
		n := all[id]
		if f := byFile[n.FileID]; f != nil {
			n.Findings = f
		}
		outNodes = append(outNodes, n)
	}
	sort.Slice(outNodes, func(i, j int) bool { return outNodes[i].ID < outNodes[j].ID })
	outEdges := []gEdge{}
	for _, e := range edges {
		if keep[e.Source] && keep[e.Target] {
			outEdges = append(outEdges, e)
		}
	}

	// project-level insights
	stats := graphStats(all, edges)
	writeJSON(w, 200, map[string]any{"nodes": outNodes, "edges": outEdges, "counts": counts, "truncated": truncated, "stats": stats})
}

func graphStats(all map[int64]*gNode, edges []gEdge) map[string]any {
	type flags struct{ Missing, Unused, BREX bool }
	parse := func(n *gNode) flags {
		var f flags
		_ = json.Unmarshal(n.Props, &f)
		return f
	}
	incoming := map[int64]map[string]bool{}
	for _, e := range edges {
		if incoming[e.Target] == nil {
			incoming[e.Target] = map[string]bool{}
		}
		incoming[e.Target][e.Type] = true
	}
	pms := 0
	for _, n := range all {
		if n.Type == graph.TPM {
			pms++
		}
	}
	var missingDM, missingICN, unusedICN, orphans []int64
	for id, n := range all {
		f := parse(n)
		switch n.Type {
		case graph.TDM:
			if f.Missing {
				missingDM = append(missingDM, id)
				continue
			}
			if f.BREX {
				continue
			}
			in := incoming[id]
			if (pms > 0 && !in[graph.EContains]) || (pms == 0 && !in[graph.EReferences]) {
				orphans = append(orphans, id)
			}
		case graph.TICN:
			if f.Missing {
				missingICN = append(missingICN, id)
			}
			if f.Unused {
				unusedICN = append(unusedICN, id)
			}
		}
	}
	orphanRule := "nicht referenziert"
	if pms > 0 {
		orphanRule = "in keinem Publikationsmodul"
	}
	return map[string]any{
		"missingDMs": missingDM, "missingICNs": missingICN, "unusedICNs": unusedICN,
		"orphanDMs": orphans, "orphanRule": orphanRule,
	}
}

// graphContext returns related modules from the knowledge graph:
// referenced/referencing modules and modules of the same assembly.
func (s *Server) graphContext(ctx context.Context, sid, fileID string, k int) []string {
	rows, err := s.db.QueryContext(ctx, `
		WITH me AS (SELECT id FROM graph_nodes WHERE snapshot_id=$1 AND file_id::text=$2 AND type='dm' LIMIT 1),
		nb AS (
			SELECT e.dst AS nid, 'verweist auf' AS rel, 1 AS prio FROM graph_edges e JOIN me ON e.src=me.id WHERE e.type='references'
			UNION ALL
			SELECT e.src, 'verweist hierher', 1 FROM graph_edges e JOIN me ON e.dst=me.id WHERE e.type='references'
			UNION ALL
			SELECT sib.src, 'gleiche Baugruppe', 2 FROM graph_edges e JOIN me ON e.src=me.id AND e.type='part_of'
				JOIN graph_edges sib ON sib.dst=e.dst AND sib.type='part_of' AND sib.src<>me.id
		)
		SELECT DISTINCT ON (n.id) n.label, n.props->>'dmc', nb.rel, nb.prio, n.file_id::text
		FROM nb JOIN graph_nodes n ON n.id=nb.nid
		WHERE n.type='dm' AND n.file_id IS NOT NULL
		ORDER BY n.id, nb.prio`, sid, fileID)
	if err != nil {
		return nil
	}
	type rel struct {
		label, dmc, rel, file string
		prio                  int
	}
	var rels []rel
	for rows.Next() {
		var x rel
		if rows.Scan(&x.label, &x.dmc, &x.rel, &x.prio, &x.file) == nil {
			rels = append(rels, x)
		}
	}
	rows.Close()
	sort.SliceStable(rels, func(i, j int) bool { return rels[i].prio < rels[j].prio })
	var out []string
	for _, x := range rels {
		if len(out) >= k {
			break
		}
		var texts []string
		cr, err := s.db.QueryContext(ctx, `SELECT text FROM chunks WHERE file_id::text=$1 AND element<>'dmTitle' ORDER BY id LIMIT 4`, x.file)
		if err == nil {
			for cr.Next() {
				var t string
				_ = cr.Scan(&t)
				texts = append(texts, t)
			}
			cr.Close()
		}
		out = append(out, fmt.Sprintf("%s (%s, %s): %s", x.dmc, x.label, x.rel, truncateRunes(strings.Join(texts, " "), 500)))
	}
	return out
}
