CREATE TABLE equipment (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    category TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1))
);

CREATE TABLE muscle_groups (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL
);

CREATE TABLE exercises (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL CHECK (category IN ('strength', 'cardio', 'mobility')),
    movement_pattern TEXT NOT NULL DEFAULT '',
    difficulty TEXT NOT NULL CHECK (difficulty IN ('beginner', 'intermediate', 'advanced')),
    measurement_type TEXT NOT NULL,
    min_space TEXT NOT NULL CHECK (min_space IN ('small', 'medium', 'large', 'unrestricted')),
    instructions_json TEXT NOT NULL DEFAULT '[]',
    cautions_json TEXT NOT NULL DEFAULT '[]',
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    updated_at TEXT NOT NULL
);

CREATE TABLE exercise_equipment (
    exercise_id TEXT NOT NULL REFERENCES exercises(id) ON DELETE RESTRICT,
    equipment_id TEXT NOT NULL REFERENCES equipment(id) ON DELETE RESTRICT,
    required INTEGER NOT NULL DEFAULT 1 CHECK (required IN (0, 1)),
    PRIMARY KEY (exercise_id, equipment_id)
);

CREATE TABLE exercise_muscles (
    exercise_id TEXT NOT NULL REFERENCES exercises(id) ON DELETE RESTRICT,
    muscle_group_id TEXT NOT NULL REFERENCES muscle_groups(id) ON DELETE RESTRICT,
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    PRIMARY KEY (exercise_id, muscle_group_id)
);

CREATE TABLE exercise_environments (
    exercise_id TEXT NOT NULL REFERENCES exercises(id) ON DELETE RESTRICT,
    environment TEXT NOT NULL,
    PRIMARY KEY (exercise_id, environment)
);

CREATE INDEX idx_exercises_category_active ON exercises(category, active, id);
CREATE INDEX idx_exercises_name ON exercises(name COLLATE NOCASE, id);
CREATE INDEX idx_exercise_equipment_equipment ON exercise_equipment(equipment_id, exercise_id);
CREATE INDEX idx_exercise_muscles_group ON exercise_muscles(muscle_group_id, exercise_id);
CREATE INDEX idx_exercise_environments_environment ON exercise_environments(environment, exercise_id);
