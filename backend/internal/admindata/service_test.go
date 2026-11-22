package admindata

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ambatumove/core/internal/calories"
	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func newAdminDataService(t *testing.T) (*Service, *catalog.Service, *gyms.Service) {
	t.Helper()
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "admin-data.sqlite"))
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
	if err := gyms.SeedFacilities(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	catalogService := catalog.NewService(db)
	gymService := gyms.NewService(db)
	if err := calories.NewService(db, catalogService, gymService).Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	return New(db, catalogService, gymService), catalogService, gymService
}

func TestEquipmentImportDryRunIsReadOnlyAndExportRoundTrips(t *testing.T) {
	service, catalogService, _ := newAdminDataService(t)
	ctx := context.Background()
	if _, err := catalogService.ListEquipment(ctx, "free_weight"); err != nil {
		t.Fatal(err)
	}
	otherCatalog := catalog.NewService(service.db)
	otherGyms := gyms.NewService(service.db)
	service = New(service.db, otherCatalog, otherGyms)
	const document = `[{"id":"hex_dumbbell","label":"Hex Dumbbell","category":"free_weight","active":true}]`
	result, err := service.Import(ctx, Equipment, strings.NewReader(document), true)
	if err != nil || result.Count != 1 || !result.DryRun {
		t.Fatalf("dry-run result=%+v err=%v", result, err)
	}
	var found int
	if err := service.db.QueryRowContext(ctx, `SELECT count(*) FROM equipment WHERE id='hex_dumbbell'`).Scan(&found); err != nil || found != 0 {
		t.Fatalf("dry-run wrote equipment: count=%d err=%v", found, err)
	}
	result, err = service.Import(ctx, Equipment, strings.NewReader(document), false)
	if err != nil || result.Count != 1 || result.DryRun {
		t.Fatalf("import result=%+v err=%v", result, err)
	}
	items, err := catalogService.ListEquipment(ctx, "free_weight")
	if err != nil {
		t.Fatal(err)
	}
	found = 0
	for _, item := range items {
		if item.ID == "hex_dumbbell" && item.Label == "Hex Dumbbell" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("reference cache did not expose imported equipment: %+v", items)
	}
	var exported strings.Builder
	if err := service.Export(ctx, Equipment, &exported); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Import(ctx, Equipment, strings.NewReader(exported.String()), true); err != nil {
		t.Fatalf("exported dataset did not validate for reimport: %v", err)
	}
}

func TestExerciseImportValidatesEntireFileBeforeWriting(t *testing.T) {
	service, _, _ := newAdminDataService(t)
	const document = `[
{"id":"ex_import_valid","name":"Imported Exercise","category":"strength","movement_pattern":"horizontal_push","primary_muscles":["chest"],"secondary_muscles":["triceps"],"required_equipment":["bodyweight"],"difficulty":"beginner","measurement_type":"repetitions","environments":["home"],"minimum_space":"small","description":"Imported","instructions":["Use control."],"cautions":[],"active":true},
{"id":"ex_import_invalid","name":"Invalid Exercise","category":"strength","movement_pattern":"horizontal_push","primary_muscles":["chest"],"secondary_muscles":[],"required_equipment":["unknown_gear"],"difficulty":"beginner","measurement_type":"repetitions","environments":["home"],"minimum_space":"small","description":"Imported","instructions":[],"cautions":[],"active":true}
]`
	if _, err := service.Import(context.Background(), Exercise, strings.NewReader(document), false); err == nil {
		t.Fatal("import with unknown equipment reference succeeded")
	}
	var count int
	if err := service.db.QueryRow(`SELECT count(*) FROM exercises WHERE id IN ('ex_import_valid','ex_import_invalid')`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid dataset partially wrote %d exercises (err=%v)", count, err)
	}
}

func TestExerciseFacilityAndMETImportExportRoundTrips(t *testing.T) {
	service, _, _ := newAdminDataService(t)
	ctx := context.Background()
	inputs := []struct {
		kind Kind
		data string
	}{
		{Facility, `[{"id":"sauna","label":"Sauna","category":"amenity"}]`},
		{Exercise, `[{"id":"act_imported_walk","name":"Imported Walk","category":"cardio","movement_pattern":"walking","primary_muscles":[],"secondary_muscles":[],"required_equipment":[],"difficulty":"beginner","measurement_type":"duration_distance","environments":["outdoor"],"minimum_space":"unrestricted","description":"A documented outdoor walk.","instructions":["Walk at a comfortable pace."],"cautions":[],"active":true}]`},
		{MET, `[{"id":"met_imported_walk","activity_id":"act_imported_walk","activity":"Imported Walk","intensity":"moderate","modality":"outdoor","speed_min_kmh":3,"speed_max_kmh":5,"met":3.4,"source":{"title":"2024 Adult Compendium of Physical Activities","year":2024,"activity_code":"WALK-1","activity_description":"Walking, moderate pace","url":"https://pacompendium.com/walking/"}}]`},
	}
	for _, input := range inputs {
		if _, err := service.Import(ctx, input.kind, strings.NewReader(input.data), false); err != nil {
			t.Fatalf("import %s: %v", input.kind, err)
		}
		var output strings.Builder
		if err := service.Export(ctx, input.kind, &output); err != nil {
			t.Fatalf("export %s: %v", input.kind, err)
		}
		if _, err := service.Import(ctx, input.kind, strings.NewReader(output.String()), true); err != nil {
			t.Fatalf("exported %s file did not validate for reimport: %v", input.kind, err)
		}
	}
}

func TestGymImportUsesCuratedDatasetAndNotSurveyQueue(t *testing.T) {
	service, _, gymService := newAdminDataService(t)
	path := filepath.Join("..", "..", "data", "gyms.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gymService.Search(context.Background(), gyms.Search{Limit: 100}); err != nil {
		t.Fatal(err)
	}
	service = New(service.db, catalog.NewService(service.db), gyms.NewService(service.db))
	result, err := service.Import(context.Background(), Gym, strings.NewReader(string(data)), false)
	if err != nil || result.Count != 9 {
		t.Fatalf("curated gym import=%+v err=%v", result, err)
	}
	page, err := gymService.Search(context.Background(), gyms.Search{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 9 {
		t.Fatalf("public gym count=%d, want only 9 curated gyms", page.Total)
	}
	if _, err := gymService.Get(context.Background(), "gym_bandung_fithub_merdeka"); err != gyms.ErrNotFound {
		t.Fatalf("survey queue FIT HUB Merdeka was imported: err=%v", err)
	}
	var exported strings.Builder
	if err := service.Export(context.Background(), Gym, &exported); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Import(context.Background(), Gym, strings.NewReader(exported.String()), true); err != nil {
		t.Fatalf("exported gym file did not validate for reimport: %v", err)
	}
}

func TestDuplicateIdentifiersAndUnknownJSONFieldsAreRejected(t *testing.T) {
	service, _, _ := newAdminDataService(t)
	duplicate := `[{"id":"same","label":"One","category":"free_weight","active":true},{"id":"same","label":"Two","category":"free_weight","active":true}]`
	if _, err := service.Import(context.Background(), Equipment, strings.NewReader(duplicate), false); err == nil {
		t.Fatal("duplicate identifiers were accepted")
	}
	unknownField := `[{"id":"new","label":"New","category":"free_weight","active":true,"surprise":true}]`
	if _, err := service.Import(context.Background(), Equipment, strings.NewReader(unknownField), false); err == nil {
		t.Fatal("unsupported JSON field was accepted")
	}
	var count int
	if err := service.db.QueryRow(`SELECT count(*) FROM equipment WHERE id='same' OR id='new'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid datasets wrote %d equipment rows (err=%v)", count, err)
	}
}

func TestMETImportRejectsUnknownExerciseAndDuplicateMapping(t *testing.T) {
	service, _, _ := newAdminDataService(t)
	const unknownActivity = `[{"id":"met_bad","activity_id":"missing","activity":"Unknown","intensity":"moderate","modality":"outdoor","met":4,"source":{"title":"Compendium","year":2024,"activity_code":"123","activity_description":"Unknown","url":"https://example.test"}}]`
	if _, err := service.Import(context.Background(), MET, strings.NewReader(unknownActivity), false); err == nil {
		t.Fatal("unknown MET activity reference was accepted")
	}
	const duplicateMapping = `[
{"id":"met_custom_1","activity_id":"act_walk_outdoor","activity":"Walking","intensity":"moderate","modality":"outdoor","speed_min_kmh":4,"speed_max_kmh":5,"met":3.8,"source":{"title":"Compendium","year":2024,"activity_code":"A","activity_description":"Walking","url":"https://example.test"}},
{"id":"met_custom_2","activity_id":"act_walk_outdoor","activity":"Walking","intensity":"moderate","modality":"outdoor","speed_min_kmh":4,"speed_max_kmh":5,"met":3.9,"source":{"title":"Compendium","year":2024,"activity_code":"B","activity_description":"Walking","url":"https://example.test"}}
]`
	if _, err := service.Import(context.Background(), MET, strings.NewReader(duplicateMapping), false); err == nil {
		t.Fatal("duplicate activity/intensity/modality mapping was accepted")
	}
	var count int
	if err := service.db.QueryRow(`SELECT count(*) FROM activity_met_profiles WHERE id LIKE 'met_custom_%'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid MET dataset wrote %d rows (err=%v)", count, err)
	}
}
