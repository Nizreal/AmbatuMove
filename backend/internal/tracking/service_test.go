package tracking

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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
