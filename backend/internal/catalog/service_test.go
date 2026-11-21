package catalog

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func newSeededCatalog(t *testing.T) *Service {
	t.Helper()
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err := SeedReferenceData(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return NewService(db)
}

func TestSeedReferenceDataIsIdempotentAndCoversMovementTypes(t *testing.T) {
	service := newSeededCatalog(t)
	if SeedCount() < 40 || SeedCount() > 80 {
		t.Fatalf("seed count = %d, want 40..80", SeedCount())
	}
	if err := SeedReferenceData(context.Background(), service.db); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ex_push_up", "act_walk_outdoor", "act_run_treadmill", "act_cycle_stationary", "ex_cat_cow"} {
		if _, err := service.GetExercise(context.Background(), id); err != nil {
			t.Errorf("seeded exercise %s: %v", id, err)
		}
	}
	page, err := service.ListExercises(context.Background(), Filters{Category: "cardio", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total < 6 {
		t.Fatalf("cardio entries = %d, want at least six activity variants", page.Total)
	}
}

func TestListExercisesAppliesAllEquipmentFilters(t *testing.T) {
	service := newSeededCatalog(t)
	page, err := service.ListExercises(context.Background(), Filters{Equipment: []string{"dumbbell", "flat_bench"}, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, item := range page.Items {
		ids[item.ID] = true
	}
	if !ids["ex_dumbbell_bench_press"] || !ids["ex_dumbbell_fly"] || ids["ex_dumbbell_row"] {
		t.Fatalf("equipment filters did not use AND semantics: %v", ids)
	}
}

func TestListExercisesRejectsUnknownVocabularyFilter(t *testing.T) {
	service := newSeededCatalog(t)
	_, err := service.ListExercises(context.Background(), Filters{Equipment: []string{"not_real"}})
	if err != ErrInvalidFilter {
		t.Fatalf("ListExercises() error = %v, want ErrInvalidFilter", err)
	}
}

func TestConcurrentListExercisesDoesNotExhaustSQLitePool(t *testing.T) {
	service := newSeededCatalog(t)
	service.SetCacheEnabled(false)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < cap(errCh); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.ListExercises(ctx, Filters{Limit: 20})
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent list query failed: %v", err)
		}
	}
}
