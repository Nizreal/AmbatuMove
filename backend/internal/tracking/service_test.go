package tracking

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/saved"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func TestCompletionIsOwnedValidatedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "tracking.sqlite"))
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
	store := saved.NewService(db)
	session, err := store.SaveWorkout(ctx, "u1", planning.Session{Goal: "strength", WorkoutType: "strength", Sections: []planning.Section{{Type: "main", Items: []planning.Item{{ItemID: "wi_001", ExerciseID: "ex_push_up"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339)
	finished := time.Now().UTC().Format(time.RFC3339)
	req := Completion{Status: "completed", StartedAt: started, FinishedAt: finished, Items: []PerformedItem{{ItemID: "wi_001", ExerciseID: "ex_push_up", PerformedSets: []Set{{Reps: 10, LoadKg: 0}}, PerceivedEffort: 6}}}
	service := NewService(db)
	first, err := service.Complete(ctx, "u1", *session.ID, "retry-key-0001", req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Complete(ctx, "u1", *session.ID, "retry-key-0001", req)
	if err != nil || first.ID != again.ID {
		t.Fatalf("idempotent retry first=%+v again=%+v err=%v", first, again, err)
	}
	req.Notes = "different body"
	if _, err = service.Complete(ctx, "u1", *session.ID, "retry-key-0001", req); err != ErrConflict {
		t.Fatalf("reused key error=%v", err)
	}
	if _, err = service.Complete(ctx, "other", *session.ID, "another-key-0001", req); err != ErrNotFound {
		t.Fatalf("foreign session error=%v", err)
	}
}

func TestManualLogSupportsMixedActivitiesAndIdempotentReplay(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "manual.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err = catalog.SeedReferenceData(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES('u1','u1@example.test','hash','2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	request := ManualRequest{Status: "completed", StartedAt: "2026-10-01T10:00:00+07:00", FinishedAt: "2026-10-01T10:40:00+07:00", Items: []ManualItem{
		{ExerciseID: "ex_push_up", PerformedSets: []Set{{Reps: 12, LoadKg: 0}}, PerceivedEffort: 6},
		{ExerciseID: "act_walk_outdoor", ActualDurationSeconds: 2400, ActualDistanceKm: 2.1, PerceivedEffort: 4},
	}}
	s := NewService(db)
	first, err := s.CreateManual(ctx, "u1", "manual-key-0001", request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Source != "manual" || first.SessionID != nil || len(first.Items) != 2 || first.Items[0].ItemID == "" || first.Items[0].ItemID == first.Items[1].ItemID {
		t.Fatalf("manual result=%+v", first)
	}
	again, err := s.CreateManual(ctx, "u1", "manual-key-0001", request)
	if err != nil || again.ID != first.ID {
		t.Fatalf("replay=%+v err=%v", again, err)
	}
	request.Notes = "changed"
	if _, err = s.CreateManual(ctx, "u1", "manual-key-0001", request); err != ErrConflict {
		t.Fatalf("different replay error=%v", err)
	}
	got, err := s.Get(ctx, "u1", first.ID)
	if err != nil || got.Source != "manual" || got.SessionID != nil {
		t.Fatalf("loaded manual log=%+v err=%v", got, err)
	}
	skipped, err := s.CreateManual(ctx, "u1", "manual-skipped-01", ManualRequest{Status: "skipped", StartedAt: "2026-10-02T10:00:00Z", FinishedAt: "2026-10-02T10:00:00Z"})
	if err != nil || skipped.Status != "skipped" || skipped.SessionID != nil || len(skipped.Items) != 0 {
		t.Fatalf("manual skipped log=%+v err=%v", skipped, err)
	}
}

func TestManualLogRejectsMismatchedMeasurementsAndDuplicateItemIDs(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "manual-invalid.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err = catalog.SeedReferenceData(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES('u1','u1@example.test','hash','2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	s := NewService(db)
	base := ManualRequest{Status: "completed", StartedAt: "2026-10-01T10:00:00Z", FinishedAt: "2026-10-01T10:10:00Z"}
	base.Items = []ManualItem{{ItemID: "dup", ExerciseID: "ex_push_up", PerformedSets: []Set{{Reps: 1}}}, {ItemID: "dup", ExerciseID: "ex_push_up", PerformedSets: []Set{{Reps: 1}}}}
	if _, err = s.CreateManual(ctx, "u1", "manual-key-0002", base); err != ErrInvalid {
		t.Fatalf("duplicate item ID error=%v", err)
	}
	base.Items = []ManualItem{{ExerciseID: "ex_push_up", ActualDurationSeconds: 100}}
	if _, err = s.CreateManual(ctx, "u1", "manual-key-0003", base); err != ErrInvalid {
		t.Fatalf("mismatched measurement error=%v", err)
	}
	base.Items = []ManualItem{{ExerciseID: "unknown", PerformedSets: []Set{{Reps: 1}}}}
	if _, err = s.CreateManual(ctx, "u1", "manual-key-0004", base); err != ErrInvalid {
		t.Fatalf("unknown exercise error=%v", err)
	}
}

func TestCorrectionIsVersionedAuditedOwnerScopedAndPersistent(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "corrections.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err = catalog.SeedReferenceData(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"u1", "u2"} {
		if _, err = db.Exec("INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES(?,?,?,?)", u, u+"@example.test", "hash", "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	s := NewService(db)
	request := ManualRequest{Status: "completed", StartedAt: "2026-10-01T10:00:00Z", FinishedAt: "2026-10-01T10:30:00Z", Notes: "before", Items: []ManualItem{{ExerciseID: "ex_push_up", PerformedSets: []Set{{Reps: 10, LoadKg: 0}}}}}
	log, err := s.CreateManual(ctx, "u1", "manual-key-0005", request)
	if err != nil {
		t.Fatal(err)
	}
	newNotes := "corrected"
	updated, err := s.Correct(ctx, "u1", log.ID, Correction{ExpectedVersion: 1, Reason: "fixed note", Notes: &newNotes})
	if err != nil || updated.Version != 2 || updated.Notes != newNotes {
		t.Fatalf("corrected=%+v err=%v", updated, err)
	}
	if _, err = s.Correct(ctx, "u1", log.ID, Correction{ExpectedVersion: 1, Reason: "stale", Notes: &newNotes}); err != ErrVersion {
		t.Fatalf("stale correction err=%v", err)
	}
	if _, err = s.Correct(ctx, "u2", log.ID, Correction{ExpectedVersion: 2, Reason: "foreign", Notes: &newNotes}); err != ErrNotFound {
		t.Fatalf("foreign correction err=%v", err)
	}
	history, total, err := s.CorrectionHistory(ctx, "u1", log.ID, 10, 0)
	if err != nil || total != 1 || len(history) != 1 || history[0].Previous.Notes != "before" || history[0].Current.Notes != "corrected" || history[0].ToVersion != 2 {
		t.Fatalf("history=%+v total=%d err=%v", history, total, err)
	}
}
