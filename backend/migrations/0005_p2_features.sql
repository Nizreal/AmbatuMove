CREATE TABLE user_equipment_presets (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    updated_at TEXT NOT NULL
);

CREATE TABLE user_equipment_preset_items (
    user_id TEXT NOT NULL REFERENCES user_equipment_presets(user_id) ON DELETE CASCADE,
    equipment_id TEXT NOT NULL REFERENCES equipment(id) ON DELETE RESTRICT,
    PRIMARY KEY(user_id, equipment_id)
);

ALTER TABLE workout_logs ADD COLUMN version INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1);
ALTER TABLE workout_logs ADD COLUMN source TEXT NOT NULL DEFAULT 'planned' CHECK (source IN ('planned','manual'));
ALTER TABLE workout_logs ADD COLUMN idempotency_request_hash TEXT;

CREATE TABLE workout_log_corrections (
    id TEXT PRIMARY KEY,
    log_id TEXT NOT NULL REFERENCES workout_logs(id) ON DELETE RESTRICT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    actor_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    from_version INTEGER NOT NULL CHECK (from_version >= 1),
    to_version INTEGER NOT NULL CHECK (to_version = from_version + 1),
    previous_state_json TEXT NOT NULL,
    new_state_json TEXT NOT NULL,
    changed_fields_json TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(log_id, to_version)
);

CREATE INDEX idx_workout_log_corrections_owner ON workout_log_corrections(user_id, log_id, to_version DESC);
CREATE INDEX idx_workout_logs_owner_source_time ON workout_logs(user_id, source, started_at DESC);

CREATE TABLE workout_revisions (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES workout_sessions(id) ON DELETE RESTRICT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    version INTEGER NOT NULL CHECK (version >= 1),
    snapshot_json TEXT NOT NULL,
    reason TEXT NOT NULL,
    actor_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL,
    UNIQUE(session_id, version)
);

CREATE INDEX idx_workout_revisions_owner ON workout_revisions(user_id, session_id, version DESC);

CREATE TABLE gym_reviews (
    id TEXT PRIMARY KEY,
    gym_id TEXT NOT NULL REFERENCES gyms(id) ON DELETE RESTRICT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    rating INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','hidden')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(gym_id, user_id)
);

CREATE INDEX idx_gym_reviews_public ON gym_reviews(gym_id, status, created_at DESC, id);
CREATE INDEX idx_gym_reviews_moderation ON gym_reviews(status, created_at, id);

CREATE TABLE gym_review_moderation_audit (
    id TEXT PRIMARY KEY,
    review_id TEXT NOT NULL REFERENCES gym_reviews(id) ON DELETE RESTRICT,
    moderator_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    previous_status TEXT NOT NULL,
    new_status TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_gym_review_audit_review ON gym_review_moderation_audit(review_id, created_at DESC);

CREATE TABLE favorite_gyms (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    gym_id TEXT NOT NULL REFERENCES gyms(id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL,
    PRIMARY KEY(user_id, gym_id)
);

CREATE INDEX idx_favorite_gyms_owner_time ON favorite_gyms(user_id, created_at DESC, gym_id);

CREATE TABLE coach_rate_windows (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    window_minute INTEGER NOT NULL,
    request_count INTEGER NOT NULL CHECK (request_count >= 0)
);

-- Existing saved workouts become a coherent baseline at their current version.
INSERT INTO workout_revisions(id,session_id,user_id,version,snapshot_json,reason,actor_user_id,created_at)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-a' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
       id,user_id,version,snapshot_json,'baseline',user_id,updated_at
FROM workout_sessions;
