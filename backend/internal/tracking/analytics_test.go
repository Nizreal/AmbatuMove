package tracking

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func TestProgressionUsesCompletedHistoryAndTrendIsDeterministic(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "analytics.sqlite"))
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
	now := time.Now().UTC()
	for i, reps := range []int{10, 11, 12, 13, 14, 15} {
		start := now.AddDate(0, 0, -(60 - i*8))
		items, _ := json.Marshal([]PerformedItem{{ItemID: "item", ExerciseID: "ex_push_up", PerformedSets: []Set{{Reps: reps, LoadKg: 0}}, PerceivedEffort: 6}})
		if _, err = db.Exec(`INSERT INTO workout_logs(id,user_id,session_id,status,started_at,finished_at,notes,idempotency_key,items_json,created_at,source) VALUES(?,? ,NULL,'completed',?,?, '',?,?,?, 'manual')`, string(rune('a'+i)), "u1", start.Format(time.RFC3339Nano), start.Add(30*time.Minute).Format(time.RFC3339Nano), "analytics-key-"+string(rune('a'+i)), string(items), start.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	s := NewService(db)
	got, err := s.Progression(ctx, "u1", 90, "ex_push_up", "UTC", 20)
	if err != nil || len(got.Items) != 1 {
		t.Fatalf("progression=%+v err=%v", got, err)
	}
	item := got.Items[0]
	if item.Status != "suggestion" || item.Suggestion != "consider_small_progression" || item.Observations != 6 {
		t.Fatalf("progression item=%+v", item)
	}
	first, err := s.Trends(ctx, "u1", 90, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Trends(ctx, "u1", 90, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Weeks) == 0 || len(first.Weeks) != len(second.Weeks) || first.Weeks[0] != second.Weeks[0] {
		t.Fatalf("trends not stable: %+v vs %+v", first, second)
	}
}

func TestProgressionMarksSparseDataInsufficient(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "analytics-sparse.sqlite"))
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
	item, _ := json.Marshal([]PerformedItem{{ItemID: "item", ExerciseID: "ex_push_up", PerformedSets: []Set{{Reps: 10}}}})
	start := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if _, err = db.Exec(`INSERT INTO workout_logs(id,user_id,status,started_at,finished_at,items_json,created_at,source) VALUES('l1','u1','completed',?,?,?,?,'manual')`, start, time.Now().UTC().Format(time.RFC3339Nano), string(item), start); err != nil {
		t.Fatal(err)
	}
	got, err := s.Progression(ctx, "u1", 90, "ex_push_up", "UTC", 10)
	if err != nil || len(got.Items) != 1 || got.Items[0].Status != "insufficient_data" {
		t.Fatalf("sparse progression=%+v err=%v", got, err)
	}
}
