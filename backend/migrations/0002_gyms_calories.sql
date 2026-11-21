CREATE TABLE facilities (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'amenity'
);

CREATE TABLE gyms (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    address_json TEXT NOT NULL,
    latitude REAL NOT NULL CHECK (latitude >= -90 AND latitude <= 90),
    longitude REAL NOT NULL CHECK (longitude >= -180 AND longitude <= 180),
    timezone TEXT NOT NULL,
    contact_json TEXT NOT NULL DEFAULT '{}',
    opening_hours_json TEXT,
    price_json TEXT,
    verification_status TEXT NOT NULL CHECK (verification_status IN ('verified','unverified','stale')),
    source_type TEXT NOT NULL CHECK (source_type IN ('survey','user_submission','official','admin','synthetic')),
    source_reference TEXT,
    verified_at TEXT,
    last_updated_at TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0,1))
);

CREATE TABLE gym_facilities (
    gym_id TEXT NOT NULL REFERENCES gyms(id) ON DELETE RESTRICT,
    facility_id TEXT NOT NULL REFERENCES facilities(id) ON DELETE RESTRICT,
    verified_at TEXT,
    source_type TEXT NOT NULL,
    PRIMARY KEY (gym_id, facility_id)
);

CREATE TABLE gym_equipment (
    gym_id TEXT NOT NULL REFERENCES gyms(id) ON DELETE RESTRICT,
    equipment_id TEXT NOT NULL REFERENCES equipment(id) ON DELETE RESTRICT,
    quantity INTEGER CHECK (quantity IS NULL OR quantity >= 0),
    status TEXT NOT NULL CHECK (status IN ('present','out_of_service','unknown')),
    verified_at TEXT,
    source_type TEXT NOT NULL,
    PRIMARY KEY (gym_id, equipment_id)
);

CREATE TABLE activity_met_profiles (
    id TEXT PRIMARY KEY,
    activity_id TEXT NOT NULL REFERENCES exercises(id) ON DELETE RESTRICT,
    intensity_code TEXT NOT NULL,
    modality TEXT NOT NULL,
    speed_min_kmh REAL,
    speed_max_kmh REAL,
    met_value REAL NOT NULL CHECK (met_value > 0 AND met_value <= 30),
    compendium_year INTEGER NOT NULL,
    compendium_code TEXT NOT NULL,
    activity_description TEXT NOT NULL,
    source_url TEXT NOT NULL,
    UNIQUE(activity_id, intensity_code, modality)
);

CREATE INDEX idx_gyms_name ON gyms(name COLLATE NOCASE, id) WHERE active=1;
CREATE INDEX idx_gyms_coordinates ON gyms(latitude, longitude) WHERE active=1;
CREATE INDEX idx_gyms_verification ON gyms(verification_status, id) WHERE active=1;
CREATE INDEX idx_gym_facilities_facility ON gym_facilities(facility_id, gym_id);
CREATE INDEX idx_gym_equipment_equipment ON gym_equipment(equipment_id, gym_id, status);
CREATE INDEX idx_activity_met_activity ON activity_met_profiles(activity_id, intensity_code, modality);
