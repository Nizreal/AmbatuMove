CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email_normalized TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
    created_at TEXT NOT NULL,
    disabled_at TEXT
);

CREATE TABLE auth_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE partner_api_keys (
    id TEXT PRIMARY KEY,
    key_prefix TEXT NOT NULL UNIQUE,
    group_name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    scopes_json TEXT NOT NULL,
    rate_tier TEXT NOT NULL DEFAULT 'standard',
    created_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE TABLE fitness_profiles (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    goal TEXT NOT NULL DEFAULT 'general_fitness',
    experience_level TEXT NOT NULL DEFAULT 'beginner',
    preferred_activity_ids_json TEXT NOT NULL DEFAULT '[]',
    equipment_json TEXT NOT NULL DEFAULT '[]',
    environment TEXT NOT NULL DEFAULT 'home',
    space TEXT NOT NULL DEFAULT 'medium',
    weight_kg REAL CHECK (weight_kg IS NULL OR (weight_kg >= 20 AND weight_kg <= 300)),
    excluded_ids_json TEXT NOT NULL DEFAULT '[]',
    updated_at TEXT NOT NULL
);

CREATE TABLE workout_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    program_id TEXT REFERENCES workout_programs(id) ON DELETE SET NULL,
    goal TEXT NOT NULL,
    workout_type TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('planned','in_progress','completed','cancelled')),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    snapshot_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE TABLE workout_programs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    goal TEXT NOT NULL,
    start_date TEXT NOT NULL,
    timezone TEXT NOT NULL,
    weeks INTEGER NOT NULL CHECK (weeks BETWEEN 1 AND 12),
    days_per_week INTEGER NOT NULL CHECK (days_per_week BETWEEN 1 AND 7),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    snapshot_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE workout_logs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id TEXT REFERENCES workout_sessions(id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK (status IN ('completed','partial','skipped')),
    started_at TEXT,
    finished_at TEXT,
    notes TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT,
    items_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(user_id, idempotency_key)
);

CREATE TABLE gym_submissions (
    id TEXT PRIMARY KEY,
    gym_id TEXT REFERENCES gyms(id) ON DELETE SET NULL,
    submitted_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    submission_type TEXT NOT NULL CHECK (submission_type IN ('new_gym','correction','equipment_correction')),
    changes_json TEXT NOT NULL,
    evidence_json TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    reviewed_by TEXT REFERENCES users(id) ON DELETE RESTRICT,
    review_reason TEXT,
    created_at TEXT NOT NULL,
    reviewed_at TEXT
);

CREATE TABLE idempotency_records (
    principal_key TEXT NOT NULL,
    route_key TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_json TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (principal_key, route_key, idempotency_key)
);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_auth_sessions_user_expiry ON auth_sessions(user_id, expires_at);
CREATE INDEX idx_partner_keys_group ON partner_api_keys(group_name, created_at);
CREATE INDEX idx_workout_sessions_owner ON workout_sessions(user_id, status, created_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_workout_programs_owner ON workout_programs(user_id, status, start_date);
CREATE INDEX idx_workout_logs_owner_time ON workout_logs(user_id, started_at DESC);
CREATE INDEX idx_gym_submissions_status ON gym_submissions(status, created_at);
CREATE INDEX idx_gym_submissions_owner ON gym_submissions(submitted_by, created_at DESC);
CREATE INDEX idx_audit_events_entity ON audit_events(entity_type, entity_id, timestamp);
