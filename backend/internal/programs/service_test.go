package programs

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func newProgramService(t *testing.T) *Service {
	t.Helper()
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "program.sqlite"))
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
	return NewService(planning.NewService(catalog.NewService(db)))
}

func TestGenerateHonorsCancelledContext(t *testing.T) {
	service := NewService(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.Generate(ctx, GenerateRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate error=%v, want context.Canceled", err)
	}
}

func TestGenerateFourWeekProgramHasFullCalendarWorkoutsAndRestDays(t *testing.T) {
	service := newProgramService(t)
	seed := int64(314)
	request := GenerateRequest{Goal: "hypertrophy", ExperienceLevel: "intermediate", Weeks: 4, DaysPerWeek: 4, StartDate: "2026-11-30", Timezone: "Asia/Jakarta", AvailableDays: []string{"monday", "tuesday", "thursday", "saturday"}, SessionDuration: 60, Environment: "gym", AvailableEquipment: []string{"barbell", "dumbbell", "flat_bench", "cable_machine"}, Seed: &seed}
	first, err := service.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("program with the same seed was not deterministic")
	}
	if len(first.Schedule) != 28 {
		t.Fatalf("schedule length = %d, want 28 calendar days", len(first.Schedule))
	}
	workouts, rests := 0, 0
	for _, day := range first.Schedule {
		if day.Kind == "rest" {
			rests++
			if day.Session != nil {
				t.Fatal("rest day unexpectedly has a session")
			}
			continue
		}
		workouts++
		if day.Session == nil || len(day.Session.Sections) == 0 || day.Session.EstimatedDurationSeconds > 60*60 {
			t.Fatalf("invalid full session on %s: %+v", day.Date, day.Session)
		}
	}
	if workouts != 16 || rests != 12 {
		t.Fatalf("workouts=%d rest=%d; want 16 and 12", workouts, rests)
	}
}

func TestGenerateProgramRejectsWeekdayCountMismatch(t *testing.T) {
	service := newProgramService(t)
	_, err := service.Generate(context.Background(), GenerateRequest{Goal: "general_fitness", ExperienceLevel: "beginner", Weeks: 1, DaysPerWeek: 2, StartDate: "2026-01-01", Timezone: "UTC", AvailableDays: []string{"monday"}, SessionDuration: 30, Environment: "home"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Generate() error = %v, want ErrInvalidRequest", err)
	}
}

func TestProgramSessionRetainsGenerationPreferencesForAdaptation(t *testing.T) {
	service := newProgramService(t)
	seed := int64(712)
	program, err := service.Generate(context.Background(), GenerateRequest{
		Goal: "endurance", WorkoutType: "cardio", ExperienceLevel: "beginner", Weeks: 1, DaysPerWeek: 1,
		StartDate: "2026-11-23", Timezone: "Asia/Jakarta", AvailableDays: []string{"monday"}, SessionDuration: 30,
		Environment: "outdoor", AvailableEquipment: []string{"bicycle"}, PreferredActivityIDs: []string{"act_cycle_outdoor"},
		ExcludedExerciseIDs: []string{"act_walk_outdoor"}, Seed: &seed,
	})
	if err != nil {
		t.Fatal(err)
	}
	var workout *planning.Session
	for _, entry := range program.Schedule {
		if entry.Session != nil {
			workout = entry.Session
			break
		}
	}
	if workout == nil || workout.GenerationPreferences == nil {
		t.Fatalf("generated program session omitted its original preferences: %+v", program.Schedule)
	}
	shorter := 15
	adapted, _, err := service.planner.AdaptPreview(context.Background(), *workout, planning.ConstraintChanges{DurationMinutes: &shorter})
	if err != nil {
		t.Fatal(err)
	}
	if adapted.AdaptedWorkout.WorkoutType != "cardio" || !reflect.DeepEqual(adapted.AdaptedWorkout.GenerationPreferences.PreferredActivityIDs, []string{"act_cycle_outdoor"}) || !reflect.DeepEqual(adapted.AdaptedWorkout.GenerationPreferences.ExcludedExerciseIDs, []string{"act_walk_outdoor"}) {
		t.Fatalf("program session adaptation lost intent: %+v", adapted.AdaptedWorkout)
	}
}
