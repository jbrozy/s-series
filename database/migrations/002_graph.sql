-- Knowledge graph per upload (snapshot)
CREATE TABLE IF NOT EXISTS graph_nodes (
    id          BIGSERIAL PRIMARY KEY,
    snapshot_id UUID NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    key         TEXT NOT NULL,
    type        TEXT NOT NULL,
    label       TEXT NOT NULL,
    file_id     UUID REFERENCES files(id) ON DELETE CASCADE,
    props       JSONB NOT NULL DEFAULT '{}',
    UNIQUE (snapshot_id, key)
);
CREATE INDEX IF NOT EXISTS graph_nodes_type_idx ON graph_nodes(snapshot_id, type);
CREATE INDEX IF NOT EXISTS graph_nodes_file_idx ON graph_nodes(file_id);
CREATE INDEX IF NOT EXISTS graph_nodes_label_trgm ON graph_nodes USING gin (label gin_trgm_ops);

CREATE TABLE IF NOT EXISTS graph_edges (
    snapshot_id UUID NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    src         BIGINT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    dst         BIGINT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    weight      INT NOT NULL DEFAULT 1,
    PRIMARY KEY (src, dst, type)
);
CREATE INDEX IF NOT EXISTS graph_edges_dst_idx ON graph_edges(dst, type);
CREATE INDEX IF NOT EXISTS graph_edges_snapshot_idx ON graph_edges(snapshot_id);
