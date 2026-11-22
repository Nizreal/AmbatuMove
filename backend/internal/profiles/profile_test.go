package profiles

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ambatumove/core/internal/catalog"
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
	if err = catalog.SeedReferenceData(context.Background(), db); err != nil {
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

func TestEquipmentPresetDistinguishesUnsetAndConfiguredEmptyAndNormalizes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "presets.sqlite")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err = catalog.SeedReferenceData(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"u1", "u2"} {
		if _, err = db.Exec("INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES(?,?,?,?)", user, user+"@example.test", "hash", "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	s := NewService(db)
	unconfigured, err := s.GetEquipmentPreset(context.Background(), "u1")
	if err != nil || unconfigured.Configured || len(unconfigured.Equipment) != 0 {
		t.Fatalf("unset preset = %+v err=%v", unconfigured, err)
	}
	if err = s.PutEquipmentPreset(context.Background(), "u1", []string{"dumbbell", "bodyweight", "dumbbell"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEquipmentPreset(context.Background(), "u1")
	if err != nil || !got.Configured || len(got.Equipment) != 2 || got.Equipment[0] != "bodyweight" || got.Equipment[1] != "dumbbell" {
		t.Fatalf("normalized preset = %+v err=%v", got, err)
	}
	if err = s.PutEquipmentPreset(context.Background(), "u1", []string{}); err != nil {
		t.Fatal(err)
	}
	empty, err := s.GetEquipmentPreset(context.Background(), "u1")
	if err != nil || !empty.Configured || len(empty.Equipment) != 0 {
		t.Fatalf("empty preset = %+v err=%v", empty, err)
	}
	other, err := s.GetEquipmentPreset(context.Background(), "u2")
	if err != nil || other.Configured {
		t.Fatalf("other user's preset = %+v err=%v", other, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err = storage.ApplyMigrations(context.Background(), restarted, migrations.FS); err != nil {
		t.Fatal(err)
	}
	persisted, err := NewService(restarted).GetEquipmentPreset(context.Background(), "u1")
	if err != nil || !persisted.Configured || len(persisted.Equipment) != 0 {
		t.Fatalf("preset did not survive restart: %+v err=%v", persisted, err)
	}
}

func TestEquipmentPresetRejectsUnknownEquipmentAtomically(t *testing.T) {
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "preset-invalid.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES('u1','a@example.test','hash','2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	s := NewService(db)
	if err = s.PutEquipmentPreset(context.Background(), "u1", []string{"bodyweight", "not-a-real-tool"}); err != ErrInvalid {
		t.Fatalf("unknown equipment error = %v", err)
	}
	got, err := s.GetEquipmentPreset(context.Background(), "u1")
	if err != nil || got.Configured {
		t.Fatalf("invalid write changed preset: %+v err=%v", got, err)
	}
}
