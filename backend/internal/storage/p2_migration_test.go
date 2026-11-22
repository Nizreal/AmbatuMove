package storage

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/ambatumove/core/migrations"
)

func TestMigration0005PreservesPopulatedVersionFourDatabase(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "populated-v4.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	v4 := fstest.MapFS{}
	for _, name := range []string{"0001_reference_catalog.sql", "0002_gyms_calories.sql", "0003_identity_and_persistence.sql", "0004_reference_data_revision.sql"} {
		contents, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		v4[name] = &fstest.MapFile{Data: contents}
	}
	if err := ApplyMigrations(ctx, db, v4); err != nil {
		t.Fatalf("apply v4 migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES('migration-user','migration@example.test','hash','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO workout_sessions(id,user_id,goal,workout_type,status,version,snapshot_json,created_at,updated_at) VALUES('migration-workout','migration-user','general_fitness','strength','planned',3,'{"id":"migration-workout","version":3}','2026-01-02T00:00:00Z','2026-01-03T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO workout_logs(id,user_id,session_id,status,started_at,finished_at,notes,items_json,created_at) VALUES('migration-log','migration-user','migration-workout','completed','2026-01-02T10:00:00Z','2026-01-02T10:30:00Z','kept','[]','2026-01-02T10:30:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatalf("upgrade populated v4 database: %v", err)
	}
	var sessionID, logStatus string
	var version int
	var source string
	if err := db.QueryRowContext(ctx, "SELECT session_id,status,version,source FROM workout_logs WHERE id='migration-log'").Scan(&sessionID, &logStatus, &version, &source); err != nil {
		t.Fatal(err)
	}
	if sessionID != "migration-workout" || logStatus != "completed" || version != 1 || source != "planned" {
		t.Fatalf("existing workout log changed during upgrade: session=%s status=%s version=%d source=%s", sessionID, logStatus, version, source)
	}
	var revisionVersion int
	var snapshot, reason string
	if err := db.QueryRowContext(ctx, "SELECT version,snapshot_json,reason FROM workout_revisions WHERE session_id='migration-workout'").Scan(&revisionVersion, &snapshot, &reason); err != nil {
		t.Fatal(err)
	}
	if revisionVersion != 3 || reason != "baseline" || snapshot != `{"id":"migration-workout","version":3}` {
		t.Fatalf("baseline snapshot mismatch: version=%d reason=%s snapshot=%s", revisionVersion, reason, snapshot)
	}
	if err := ApplyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatalf("repeat startup migrations: %v", err)
	}
}
