package gyms

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func newGymService(t *testing.T) *Service {
	t.Helper()
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "gyms.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SeedReferenceData(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	service := NewService(db)
	if err := service.SeedSyntheticFixtures(context.Background()); err != nil {
		t.Fatal(err)
	}
	return service
}

func TestSearchFiltersAllEquipmentAndFacilityAndReturnsDistance(t *testing.T) {
	service := newGymService(t)
	lat, lon, radius := -6.2, 106.8167, 5.0
	page, err := service.Search(context.Background(), Search{Latitude: &lat, Longitude: &lon, RadiusKm: &radius, Equipment: []string{"cable_machine", "flat_bench"}, Facilities: []string{"shower"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "gym_demo_jakarta_001" || page.Items[0].DistanceKm == nil || *page.Items[0].DistanceKm != 0 {
		t.Fatalf("unexpected gym page: %+v", page)
	}
	if page.Items[0].VerificationStatus != "unverified" || page.Items[0].SourceType != "synthetic" {
		t.Fatalf("synthetic provenance was not visible: %+v", page.Items[0])
	}
}

func TestSearchWithoutCoordinatesUsesValidBoundedPagination(t *testing.T) {
	service := newGymService(t)
	page, err := service.Search(context.Background(), Search{Limit: 1, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if page.Limit != 1 || page.Offset != 0 || page.Total < 3 || len(page.Items) != 1 {
		t.Fatalf("unexpected unlocated gym page: %+v", page)
	}
}

func TestGymDetailIncludesInventoryAndUnverifiedProvenance(t *testing.T) {
	service := newGymService(t)
	item, err := service.Get(context.Background(), "gym_demo_jakarta_001")
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Equipment) != 4 || len(item.Facilities) != 3 || item.SourceType != "synthetic" || item.VerifiedAt != nil {
		t.Fatalf("unexpected detail: %+v", item)
	}
	ids, status, err := service.AvailableEquipment(context.Background(), item.ID)
	if err != nil || status != "unverified" || len(ids) != 4 {
		t.Fatalf("available inventory ids=%v status=%s err=%v", ids, status, err)
	}
}

func TestSyntheticFixtureSeederDoesNotOverwriteStoredGymChanges(t *testing.T) {
	service := newGymService(t)
	ctx := context.Background()
	if _, err := service.db.ExecContext(ctx, `UPDATE gyms SET name='Admin-managed studio',active=0 WHERE id='gym_demo_jakarta_001'`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.ExecContext(ctx, `UPDATE gym_equipment SET quantity=9,status='out_of_service' WHERE gym_id='gym_demo_jakarta_001' AND equipment_id='dumbbell'`); err != nil {
		t.Fatal(err)
	}
	if err := service.SeedSyntheticFixtures(ctx); err != nil {
		t.Fatal(err)
	}
	var name string
	var active int
	if err := service.db.QueryRowContext(ctx, `SELECT name,active FROM gyms WHERE id='gym_demo_jakarta_001'`).Scan(&name, &active); err != nil {
		t.Fatal(err)
	}
	if name != "Admin-managed studio" || active != 0 {
		t.Fatalf("stored gym edit overwritten: name=%q active=%d", name, active)
	}
	var quantity int
	var status string
	if err := service.db.QueryRowContext(ctx, `SELECT quantity,status FROM gym_equipment WHERE gym_id='gym_demo_jakarta_001' AND equipment_id='dumbbell'`).Scan(&quantity, &status); err != nil {
		t.Fatal(err)
	}
	if quantity != 9 || status != "out_of_service" {
		t.Fatalf("stored inventory edit overwritten: quantity=%d status=%q", quantity, status)
	}
}

func TestSearchRejectsInvalidCoordinatesAndUnknownFilters(t *testing.T) {
	service := newGymService(t)
	lat, lon := 91.0, 0.0
	_, err := service.Search(context.Background(), Search{Latitude: &lat, Longitude: &lon})
	if !errors.Is(err, ErrInvalidSearch) {
		t.Fatalf("coordinate search error = %v", err)
	}
	_, err = service.Search(context.Background(), Search{Equipment: []string{"unknown"}})
	if !errors.Is(err, ErrInvalidSearch) {
		t.Fatalf("unknown filter error = %v", err)
	}
}

func TestHaversineDistanceForOneDegreeLatitude(t *testing.T) {
	distance := haversineKm(0, 0, 1, 0)
	if distance < 111 || distance > 112 {
		t.Fatalf("distance = %.2f km", distance)
	}
}
