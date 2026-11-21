package profiles

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func TestProfileUpsertAndValidation(t *testing.T) {
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "profiles.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(context.Background(), "INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES('u1','a@b.co','hash','2026-01-01T00:00:00Z')")
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db)
	mass := 70.0
	p := Profile{Goal: "endurance", ExperienceLevel: "intermediate", PreferredActivityIDs: []string{"act_walk_outdoor"}, Equipment: []string{"dumbbells"}, Environment: "outdoor", Space: "unrestricted", WeightKg: &mass, ExcludedIDs: []string{"ex_burpee"}}
	if err = s.Put(context.Background(), "u1", p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(context.Background(), "u1")
	if err != nil || got.Goal != p.Goal || got.WeightKg == nil || *got.WeightKg != 70 {
		t.Fatalf("profile=%+v err=%v", got, err)
	}
	p.WeightKg = nil
	p.Environment = "space"
	if err = s.Put(context.Background(), "u1", p); err != ErrInvalid {
		t.Fatalf("invalid profile error=%v", err)
	}
}
