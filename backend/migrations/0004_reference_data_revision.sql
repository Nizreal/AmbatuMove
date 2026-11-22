CREATE TABLE reference_data_revision (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0)
);

INSERT INTO reference_data_revision(id, revision) VALUES(1, 0);
