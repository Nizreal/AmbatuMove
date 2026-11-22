package gyms

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func TestGymSubmissionReviewAppliesAtomicallyAndAudits(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "gym-submissions.sqlite"))
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
	service := NewService(db)
	if err = service.SeedSyntheticFixtures(ctx); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"u1", "admin1"} {
		if _, err = db.Exec("INSERT INTO users(id,email_normalized,password_hash,role,created_at) VALUES(?,?,?,?,?)", user, user+"@example.test", "hash", map[string]string{"u1": "user", "admin1": "admin"}[user], "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	quantity := 2
	if _, err = service.Equipment(ctx, "gym_demo_jakarta_001"); err != nil {
		t.Fatal(err)
	}
	submission, err := service.Submit(ctx, "u1", "equipment_correction", "gym_demo_jakarta_001", map[string]any{"equipment": []any{map[string]any{"equipment_id": "barbell", "quantity": quantity, "status": "present"}}}, []string{"https://example.test/survey-note"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.GetSubmission(ctx, submission.ID, "admin1", false); err != ErrSubmissionNotFound {
		t.Fatalf("foreign submission visible: %v", err)
	}
	if err = service.Review(ctx, submission.ID, "admin1", "approve", "verified synthetic test"); err != nil {
		t.Fatal(err)
	}
	inventory, err := service.Equipment(ctx, "gym_demo_jakarta_001")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range inventory {
		if item.EquipmentID == "barbell" && item.Quantity != nil && *item.Quantity == quantity {
			found = true
		}
	}
	if !found {
		t.Fatalf("approved inventory not applied: %+v", inventory)
	}
	var audits int
	if err = db.QueryRow("SELECT count(*) FROM audit_events WHERE entity_id=?", submission.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit rows=%d err=%v", audits, err)
	}
}

func TestNewGymSubmissionPublishesUnverifiedGymOnApproval(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "new-gym.sqlite"))
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
	service := NewService(db)
	if err = service.SeedSyntheticFixtures(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO users(id,email_normalized,password_hash,role,created_at) VALUES('admin','admin@example.test','hash','admin','2026-01-01T00:00:00Z')")
	if err != nil {
		t.Fatal(err)
	}
	submission, err := service.Submit(ctx, "admin", "new_gym", "", map[string]any{"name": "Synthetic Community Gym", "street": "1 Demo St", "city": "Jakarta", "region": "DKI Jakarta", "country": "Indonesia", "latitude": -6.2, "longitude": 106.8, "timezone": "Asia/Jakarta", "facilities": []string{"shower"}, "equipment": []any{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Review(ctx, submission.ID, "admin", "approve", "accepted"); err != nil {
		t.Fatal(err)
	}
	reviewed, err := service.GetSubmission(ctx, submission.ID, "admin", true)
	if err != nil || reviewed.GymID == nil {
		t.Fatalf("reviewed=%+v err=%v", reviewed, err)
	}
	gym, err := service.Get(ctx, *reviewed.GymID)
	if err != nil || gym.VerificationStatus != "unverified" || gym.SourceType != "user_submission" {
		t.Fatalf("gym=%+v err=%v", gym, err)
	}
}
