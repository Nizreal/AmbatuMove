package saved

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/programs"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func TestSavedWorkoutOwnershipVersionAndArchive(t *testing.T) {
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "saved.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"u1", "u2"} {
		if _, err = db.Exec("INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES(?,?,?,?)", id, id+"@example.test", "hash", "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	s := NewService(db)
	v, err := s.SaveWorkout(context.Background(), "u1", planning.Session{Goal: "strength", WorkoutType: "strength", Status: "planned"})
	if err != nil {
		t.Fatal(err)
	}
	if v.ID == nil || !v.Saved {
		t.Fatalf("saved session=%+v", v)
	}
	if _, err = s.GetWorkout(context.Background(), "u2", *v.ID); err != ErrNotFound {
		t.Fatalf("foreign owner read error=%v", err)
	}
	updated, err := s.UpdateWorkout(context.Background(), "u1", *v.ID, 1, v)
	if err != nil || updated.Version != 2 {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	if _, err = s.UpdateWorkout(context.Background(), "u1", *v.ID, 1, v); err != ErrVersion {
		t.Fatalf("stale write error=%v", err)
	}
	revisions, total, err := s.WorkoutRevisions(context.Background(), "u1", *v.ID, 10, 0)
	if err != nil || total != 2 || len(revisions) != 2 || revisions[0].Version != 1 || revisions[1].Version != 2 || revisions[0].Reason != "baseline" {
		t.Fatalf("revisions=%+v total=%d err=%v", revisions, total, err)
	}
	if _, _, err = s.WorkoutRevisions(context.Background(), "u2", *v.ID, 10, 0); err != ErrNotFound {
		t.Fatalf("foreign revision error=%v", err)
	}
	if err = s.ArchiveWorkout(context.Background(), "u1", *v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetWorkout(context.Background(), "u1", *v.ID); err != ErrNotFound {
		t.Fatalf("archived workout read error=%v", err)
	}
}

func TestSaveProgramPersistsNestedSessionsAtomically(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "saved-program.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES('u1','u1@example.test','hash','2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	store := NewService(db)
	preview := programs.Program{Goal: "endurance", Weeks: 1, StartDate: "2026-11-21", Timezone: "UTC", Schedule: []programs.ScheduleEntry{{Date: "2026-11-21", Week: 1, Day: "saturday", Kind: "workout", Session: &planning.Session{Goal: "endurance", WorkoutType: "cardio", Sections: []planning.Section{}}}}}
	savedProgram, err := store.SaveProgram(ctx, "u1", preview, 1)
	if err != nil {
		t.Fatal(err)
	}
	if savedProgram.ID == nil || savedProgram.Schedule[0].Session.ID == nil {
		t.Fatalf("program session IDs missing: %+v", savedProgram)
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM workout_sessions WHERE user_id='u1' AND program_id=?", *savedProgram.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("session rows=%d err=%v", count, err)
	}
	loaded, err := store.GetProgram(ctx, "u1", *savedProgram.ID)
	if err != nil || loaded.Schedule[0].Session.ID == nil {
		t.Fatalf("loaded program=%+v err=%v", loaded, err)
	}
}
