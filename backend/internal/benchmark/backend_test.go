package benchmark

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ambatumove/core/internal/calories"
	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/programs"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

type fixture struct {
	db       *sql.DB
	catalog  *catalog.Service
	planner  *planning.Service
	programs *programs.Service
	gyms     *gyms.Service
	calories *calories.Service
}

func newFixture(tb testing.TB) *fixture {
	tb.Helper()
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(tb.TempDir(), "benchmark.sqlite"))
	if err != nil {
		tb.Fatal(err)
	}
	if err = storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		tb.Fatal(err)
	}
	if err = catalog.SeedReferenceData(context.Background(), db); err != nil {
		tb.Fatal(err)
	}
	catalogService := catalog.NewService(db)
	gymService := gyms.NewService(db)
	if err = gymService.Seed(context.Background()); err != nil {
		tb.Fatal(err)
	}
	calorieService := calories.NewService(db, catalogService, gymService)
	if err = calorieService.Seed(context.Background()); err != nil {
		tb.Fatal(err)
	}
	if err = seedPerformanceCorpus(context.Background(), db); err != nil {
		tb.Fatal(err)
	}
	planner := planning.NewService(catalogService, gymService)
	return &fixture{db: db, catalog: catalogService, planner: planner, programs: programs.NewService(planner), gyms: gymService, calories: calorieService}
}

func seedPerformanceCorpus(ctx context.Context, db *sql.DB) error {
	for i := 1; i <= 37; i++ {
		id := fmt.Sprintf("bench_ex_%03d", i)
		base := "ex_bodyweight_squat"
		if i%3 == 0 {
			base = "act_walk_outdoor"
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO exercises(id,name,description,category,movement_pattern,difficulty,measurement_type,min_space,instructions_json,cautions_json,active,updated_at) SELECT ?,?,description,category,movement_pattern,difficulty,measurement_type,min_space,instructions_json,cautions_json,1,updated_at FROM exercises WHERE id=?`, id, fmt.Sprintf("Synthetic benchmark activity %03d", i), base); err != nil {
			return err
		}
		for table, columns := range map[string]string{"exercise_equipment": "equipment_id,required", "exercise_muscles": "muscle_group_id,is_primary", "exercise_environments": "environment"} {
			if _, err := db.ExecContext(ctx, "INSERT INTO "+table+"(exercise_id,"+columns+") SELECT ?,"+columns+" FROM "+table+" WHERE exercise_id=?", id, base); err != nil {
				return err
			}
		}
	}
	for i := 1; i <= 17; i++ {
		id := fmt.Sprintf("bench_gym_%03d", i)
		if _, err := db.ExecContext(ctx, `INSERT INTO gyms(id,name,address_json,latitude,longitude,timezone,contact_json,verification_status,source_type,source_reference,last_updated_at,active) SELECT ?,?,address_json,latitude+?,longitude,timezone,contact_json,'unverified','synthetic','Synthetic performance fixture',last_updated_at,1 FROM gyms WHERE id='gym_demo_jakarta_001'`, id, fmt.Sprintf("Synthetic benchmark gym %03d", i), float64(i)*0.001); err != nil {
			return err
		}
		for _, table := range []string{"gym_equipment", "gym_facilities"} {
			columns := "equipment_id,quantity,status,verified_at,source_type"
			if table == "gym_facilities" {
				columns = "facility_id,verified_at,source_type"
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO "+table+"(gym_id,"+columns+") SELECT ?,"+columns+" FROM "+table+" WHERE gym_id='gym_demo_jakarta_001'", id); err != nil {
				return err
			}
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES('benchmark_user','benchmark@example.test','synthetic-only','2026-01-01T00:00:00Z')"); err != nil {
		return err
	}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("benchmark_session_%03d", i)
		now := fmt.Sprintf("2026-01-%02dT00:00:00Z", i%28+1)
		if _, err := db.ExecContext(ctx, "INSERT INTO workout_sessions(id,user_id,goal,workout_type,status,version,snapshot_json,created_at,updated_at) VALUES(?,'benchmark_user','general_fitness','strength','planned',1,'{}',?,?)", id, now, now); err != nil {
			return err
		}
	}
	return nil
}

func BenchmarkCatalogFiltering(b *testing.B) {
	f := newFixture(b)
	defer f.db.Close()
	filter := catalog.Filters{Category: "strength", Equipment: []string{"bodyweight"}, Limit: 50, Sort: "name"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.catalog.ListExercises(context.Background(), filter); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkExerciseAlternativeRanking(b *testing.B) {
	f := newFixture(b)
	defer f.db.Close()
	request := planning.AlternativeRequest{AvailableEquipment: []string{"bodyweight"}, ExperienceLevel: "beginner", Environment: "home", Space: "medium", Goal: "general_fitness", MaxResults: 10}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.planner.FindAlternatives(context.Background(), "ex_push_up", request); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkSessionGeneration(b *testing.B) {
	f := newFixture(b)
	defer f.db.Close()
	request := planning.GenerateRequest{Goal: "general_fitness", WorkoutType: "mixed", DurationMinutes: 30, ExperienceLevel: "beginner", Environment: "home", Seed: ptrInt64(42)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := f.planner.Generate(context.Background(), request); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkFourWeekProgramGeneration(b *testing.B) {
	f := newFixture(b)
	defer f.db.Close()
	request := programs.GenerateRequest{Goal: "endurance", WorkoutType: "cardio", ExperienceLevel: "beginner", Weeks: 4, DaysPerWeek: 4, StartDate: "2026-11-23", Timezone: "UTC", AvailableDays: []string{"monday", "tuesday", "thursday", "saturday"}, SessionDuration: 30, Environment: "outdoor", PreferredActivityIDs: []string{"act_walk_outdoor"}, Seed: ptrInt64(42)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.programs.Generate(context.Background(), request); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkMETEstimate(b *testing.B) {
	f := newFixture(b)
	defer f.db.Close()
	request := calories.EstimateRequest{ActivityID: "act_walk_outdoor", Intensity: "moderate", DurationMinutes: 30, WeightKg: 70, EstimateType: "gross"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.calories.Estimate(context.Background(), request); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCalorieRecommendations(b *testing.B) {
	f := newFixture(b)
	defer f.db.Close()
	request := calories.RecommendationRequest{TargetKcal: 250, WeightKg: 70, MaxDurationMinutes: 45, ExperienceLevel: "beginner", MaxIntensity: "moderate", Limit: 8}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.calories.Recommend(context.Background(), request); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkGymRadiusEquipmentFilter(b *testing.B) {
	f := newFixture(b)
	defer f.db.Close()
	lat, lon, radius := -6.2, 106.8167, 10.0
	request := gyms.Search{Latitude: &lat, Longitude: &lon, RadiusKm: &radius, Equipment: []string{"cable_machine"}, Facilities: []string{"shower"}, Limit: 20}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.gyms.Search(context.Background(), request); err != nil {
			b.Fatal(err)
		}
	}
}
func ptrInt64(value int64) *int64 { return &value }
