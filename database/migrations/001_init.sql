-- S-Check database schema
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    disabled      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    standard    TEXT NOT NULL DEFAULT 'S1000D',
    owner_id    UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS project_members (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    added_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE IF NOT EXISTS snapshots (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    label       TEXT NOT NULL,
    uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
    status      TEXT NOT NULL DEFAULT 'uploaded', -- uploaded | indexing | ready | failed
    error       TEXT NOT NULL DEFAULT '',
    file_count  INT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS snapshots_project_idx ON snapshots(project_id, created_at DESC);

CREATE TABLE IF NOT EXISTS files (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_id  UUID NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    path         TEXT NOT NULL,
    kind         TEXT NOT NULL,
    dmc          TEXT NOT NULL DEFAULT '',
    issue        TEXT NOT NULL DEFAULT '',
    lang         TEXT NOT NULL DEFAULT '',
    title        TEXT NOT NULL DEFAULT '',
    info_code    TEXT NOT NULL DEFAULT '',
    size         BIGINT NOT NULL DEFAULT 0,
    sha256       TEXT NOT NULL DEFAULT '',
    parse_error  TEXT NOT NULL DEFAULT '',
    UNIQUE (snapshot_id, path)
);
CREATE INDEX IF NOT EXISTS files_snapshot_idx ON files(snapshot_id, kind);
CREATE INDEX IF NOT EXISTS files_title_trgm ON files USING gin (title gin_trgm_ops);

CREATE TABLE IF NOT EXISTS chunks (
    id          BIGSERIAL PRIMARY KEY,
    snapshot_id UUID NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    file_id     UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    breadcrumb  TEXT NOT NULL,
    element     TEXT NOT NULL,
    path        TEXT NOT NULL,
    line        INT NOT NULL DEFAULT 0,
    text        TEXT NOT NULL,
    tsv         TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', breadcrumb || ' ' || text)) STORED,
    embedding   REAL[]
);
CREATE INDEX IF NOT EXISTS chunks_snapshot_idx ON chunks(snapshot_id);
CREATE INDEX IF NOT EXISTS chunks_tsv_idx ON chunks USING gin (tsv);
CREATE INDEX IF NOT EXISTS chunks_text_trgm ON chunks USING gin (text gin_trgm_ops);

CREATE TABLE IF NOT EXISTS analysis_runs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_id UUID NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'queued', -- queued | running | done | failed
    progress    INT NOT NULL DEFAULT 0,
    message     TEXT NOT NULL DEFAULT '',
    options     JSONB NOT NULL DEFAULT '{}',
    summary     JSONB NOT NULL DEFAULT '{}',
    error       TEXT NOT NULL DEFAULT '',
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS runs_snapshot_idx ON analysis_runs(snapshot_id, created_at DESC);

CREATE TABLE IF NOT EXISTS findings (
    id          BIGSERIAL PRIMARY KEY,
    run_id      UUID NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
    file_id     UUID REFERENCES files(id) ON DELETE CASCADE,
    file_path   TEXT NOT NULL DEFAULT '',
    dmc         TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL,
    code        TEXT NOT NULL,
    severity    TEXT NOT NULL,
    message     TEXT NOT NULL,
    xpath       TEXT NOT NULL DEFAULT '',
    line        INT NOT NULL DEFAULT 0,
    context     TEXT NOT NULL DEFAULT '',
    suggestion  TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open', -- open | accepted | dismissed | fixed
    status_note TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS findings_run_idx ON findings(run_id, category, severity);

-- dismissed findings stay dismissed in later runs of the same project
CREATE TABLE IF NOT EXISTS finding_suppressions (
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    fingerprint TEXT NOT NULL,
    note        TEXT NOT NULL DEFAULT '',
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, fingerprint)
);

CREATE TABLE IF NOT EXISTS business_rules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL,
    severity   TEXT NOT NULL DEFAULT 'warning',
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    source     TEXT NOT NULL DEFAULT 'manual',
    config     JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS rules_project_idx ON business_rules(project_id);

CREATE TABLE IF NOT EXISTS images (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_id     UUID NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    file_id         UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE UNIQUE,
    status          TEXT NOT NULL DEFAULT 'pending', -- pending | detecting | review | clean | anonymized | unsupported | failed
    width           INT NOT NULL DEFAULT 0,
    height          INT NOT NULL DEFAULT 0,
    detections      JSONB NOT NULL DEFAULT '[]',
    detector        TEXT NOT NULL DEFAULT '',
    anonymized_path TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    reviewed_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS images_snapshot_idx ON images(snapshot_id, status);

CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
