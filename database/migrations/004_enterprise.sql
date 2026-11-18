-- Enterprise features: audit trail, finding workflow, quality gate and release,
-- API tokens for CI pipelines.

-- Who changed what and when. Rows keep names and project names so the trail
-- stays readable after users or projects are deleted.
CREATE TABLE IF NOT EXISTS audit_log (
    id           BIGSERIAL PRIMARY KEY,
    at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    user_name    TEXT NOT NULL DEFAULT '',
    via_token    TEXT NOT NULL DEFAULT '',          -- token name when the action came from an API token
    project_id   UUID REFERENCES projects(id) ON DELETE SET NULL,
    project_name TEXT NOT NULL DEFAULT '',
    action       TEXT NOT NULL,                     -- e.g. finding.status, rule.update
    target_type  TEXT NOT NULL DEFAULT '',
    target_id    TEXT NOT NULL DEFAULT '',
    summary      TEXT NOT NULL DEFAULT '',          -- human readable, German
    details      JSONB NOT NULL DEFAULT '{}',
    ip           TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS audit_project_idx ON audit_log(project_id, at DESC);
CREATE INDEX IF NOT EXISTS audit_at_idx ON audit_log(at DESC);
CREATE INDEX IF NOT EXISTS audit_target_idx ON audit_log(target_type, target_id);

-- Assignment follows a finding across uploads (keyed by fingerprint).
CREATE TABLE IF NOT EXISTS finding_tracks (
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    fingerprint TEXT NOT NULL,
    assignee_id UUID REFERENCES users(id) ON DELETE SET NULL,
    assigned_by UUID REFERENCES users(id) ON DELETE SET NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, fingerprint)
);
CREATE INDEX IF NOT EXISTS finding_tracks_assignee_idx ON finding_tracks(assignee_id);

CREATE TABLE IF NOT EXISTS finding_comments (
    id          BIGSERIAL PRIMARY KEY,
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    fingerprint TEXT NOT NULL,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    user_name   TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finding_comments_idx ON finding_comments(project_id, fingerprint, created_at);

CREATE INDEX IF NOT EXISTS findings_fp_idx ON findings(run_id, fingerprint);

-- Release criteria per project. NULL limit = not checked.
CREATE TABLE IF NOT EXISTS quality_gates (
    project_id          UUID PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    max_errors          INT,
    max_warnings        INT,
    max_info            INT,
    blocking_categories TEXT[] NOT NULL DEFAULT '{}',   -- categories that must have no open finding
    require_llm         BOOLEAN NOT NULL DEFAULT false, -- release only after a run with KI-Review
    updated_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Release decisions. The latest row per snapshot is the current state.
CREATE TABLE IF NOT EXISTS snapshot_reviews (
    id          BIGSERIAL PRIMARY KEY,
    snapshot_id UUID NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    run_id      UUID REFERENCES analysis_runs(id) ON DELETE SET NULL,
    decision    TEXT NOT NULL,                      -- approved | rejected
    comment     TEXT NOT NULL DEFAULT '',
    gate        JSONB NOT NULL DEFAULT '{}',        -- gate evaluation at decision time
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    user_name   TEXT NOT NULL DEFAULT '',
    decided_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS snapshot_reviews_idx ON snapshot_reviews(snapshot_id, decided_at DESC);

-- Personal access tokens for scripts and CI. Only a SHA-256 hash is stored.
CREATE TABLE IF NOT EXISTS api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL,                     -- first characters, shown in lists
    token_hash   TEXT NOT NULL UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS api_tokens_user_idx ON api_tokens(user_id);
