package planning

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func newPlanner(t *testing.T) *Service {
	service, _, _ := newPlannerFixture(t)
	return service
}

func newPlannerFixture(t *testing.T) (*Service, *gyms.Service, *sql.DB) {
	t.Helper()
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "planner.sqlite"))
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
	gymService := gyms.NewService(db)
	if err := gymService.SeedSyntheticFixtures(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewService(catalog.NewService(db), gymService), gymService, db
}

func TestGenerateEquipmentFreeStrengthSessionIsDeterministicAndWithinBudget(t *testing.T) {
	service := newPlanner(t)
	seed := int64(42)
	request := GenerateRequest{Goal: "general_fitness", WorkoutType: "strength", DurationMinutes: 30, ExperienceLevel: "beginner", Environment: "home", Space: "small", Seed: &seed}
	first, failure, err := service.Generate(context.Background(), request)
	if err != nil || failure != nil {
		t.Fatalf("Generate() failure=%+v err=%v", failure, err)
	}
	second, _, err := service.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same seed and constraints produced different sessions")
	}
	if first.EstimatedDurationSeconds > first.PlannedDurationSeconds {
		t.Fatalf("estimated %d exceeds planned %d", first.EstimatedDurationSeconds, first.PlannedDurationSeconds)
	}
	if len(first.Sections) != 3 || first.Sections[0].Type != "warmup" || first.Sections[1].Type != "main" || first.Sections[2].Type != "cooldown" {
		t.Fatalf("unexpected session sections: %+v", first.Sections)
	}
	for _, item := range first.Sections[1].Items {
		if _, ok := item.Prescription["sets"]; !ok {
			t.Fatalf("strength prescription missing sets: %+v", item)
		}
	}
}

func TestGenerateOutdoorWalkingUsesCardioDurationsWithoutEquipment(t *testing.T) {
	service := newPlanner(t)
	seed := int64(7)
	request := GenerateRequest{Goal: "endurance", WorkoutType: "cardio", DurationMinutes: 25, ExperienceLevel: "beginner", Environment: "outdoor", Space: "unrestricted", PreferredActivityIDs: []string{"act_walk_outdoor"}, Seed: &seed}
	session, failure, err := service.Generate(context.Background(), request)
	if err != nil || failure != nil {
		t.Fatalf("Generate() failure=%+v err=%v", failure, err)
	}
	if session.EstimatedDurationSeconds != 1500 || len(session.Sections) != 3 {
		t.Fatalf("duration or sections incorrect: %+v", session)
	}
	for _, section := range session.Sections {
		for _, item := range section.Items {
			if item.ExerciseID != "act_walk_outdoor" || item.Prescription["duration_seconds"] == nil {
				t.Fatalf("cardio item is not a duration prescription: %+v", item)
			}
		}
	}
}

func TestGenerateRejectsInvalidAndInfeasibleRequests(t *testing.T) {
	service := newPlanner(t)
	_, _, err := service.Generate(context.Background(), GenerateRequest{Goal: "bad", WorkoutType: "strength", DurationMinutes: 30, ExperienceLevel: "beginner", Environment: "home"})
	if err != ErrInvalidRequest {
		t.Fatalf("invalid request error = %v", err)
	}
	_, failure, err := service.Generate(context.Background(), GenerateRequest{Goal: "strength", WorkoutType: "strength", DurationMinutes: 30, ExperienceLevel: "beginner", Environment: "outdoor", Space: "small", AvailableEquipment: []string{"squat_rack", "barbell"}})
	if err != ErrUnfeasible || failure == nil {
		t.Fatalf("infeasible error=%v failure=%+v", err, failure)
	}
}

func TestFindAlternativesRanksCompatibleMovesAndExcludesOriginal(t *testing.T) {
	service := newPlanner(t)
	result, err := service.FindAlternatives(context.Background(), "ex_cable_chest_fly", AlternativeRequest{AvailableEquipment: []string{"dumbbell", "flat_bench"}, ExperienceLevel: "intermediate", Environment: "gym", Space: "medium", Goal: "hypertrophy", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Alternatives) == 0 || result.Alternatives[0].ExerciseID != "ex_dumbbell_fly" {
		t.Fatalf("unexpected alternatives: %+v", result)
	}
}

func TestAdaptPreviewShortensSessionAndReportsPreservedAndReplacedItems(t *testing.T) {
	service := newPlanner(t)
	seed := int64(42)
	original, _, err := service.Generate(context.Background(), GenerateRequest{Goal: "strength", WorkoutType: "strength", DurationMinutes: 45, ExperienceLevel: "intermediate", Environment: "gym", AvailableEquipment: []string{"dumbbell", "barbell", "cable_machine", "flat_bench"}, Seed: &seed})
	if err != nil {
		t.Fatal(err)
	}
	shorter := 20
	adapted, failure, err := service.AdaptPreview(context.Background(), original, ConstraintChanges{DurationMinutes: &shorter})
	if err != nil || failure != nil {
		t.Fatalf("AdaptPreview() failure=%+v err=%v", failure, err)
	}
	if adapted.AdaptedWorkout.PlannedDurationSeconds != 1200 || adapted.AdaptedWorkout.EstimatedDurationSeconds > 1200 || adapted.OriginalSummary.EstimatedDurationSeconds != original.EstimatedDurationSeconds {
		t.Fatalf("adaptation durations are inconsistent: %+v", adapted)
	}
	if len(adapted.ChangesApplied) != 1 || adapted.ChangesApplied[0] != "duration_minutes" {
		t.Fatalf("changes applied = %v", adapted.ChangesApplied)
	}
}

func TestAdaptPreservesCyclingPreferencesWhenShorteningDuration(t *testing.T) {
	service := newPlanner(t)
	seed := int64(13)
	original, _, err := service.Generate(context.Background(), GenerateRequest{
		Goal: "endurance", WorkoutType: "cardio", DurationMinutes: 40, ExperienceLevel: "beginner",
		Environment: "outdoor", AvailableEquipment: []string{"bicycle"}, PreferredActivityIDs: []string{"act_cycle_outdoor"},
		ExcludedExerciseIDs: []string{"act_walk_outdoor"}, Intensity: "moderate", Seed: &seed,
	})
	if err != nil {
		t.Fatal(err)
	}
	shorter := 20
	result, _, err := service.AdaptPreview(context.Background(), original, ConstraintChanges{DurationMinutes: &shorter})
	if err != nil {
		t.Fatal(err)
	}
	adapted := result.AdaptedWorkout
	if adapted.WorkoutType != "cardio" || adapted.Goal != "endurance" || adapted.Constraints.DurationMinutes != shorter {
		t.Fatalf("adaptation lost original workout intent or duration: %+v", adapted)
	}
	if adapted.GenerationPreferences == nil || !reflect.DeepEqual(adapted.GenerationPreferences.PreferredActivityIDs, []string{"act_cycle_outdoor"}) || !reflect.DeepEqual(adapted.GenerationPreferences.ExcludedExerciseIDs, []string{"act_walk_outdoor"}) || adapted.GenerationPreferences.Intensity != "moderate" {
		t.Fatalf("generation preferences were not preserved: %+v", adapted.GenerationPreferences)
	}
	for _, section := range adapted.Sections {
		for _, item := range section.Items {
			if item.ExerciseID != "act_cycle_outdoor" {
				t.Fatalf("duration-only adaptation changed preferred activity: %+v", item)
			}
		}
	}
}

func TestAdaptPreservesDistinctItemIDsForRepeatedCardioOccurrences(t *testing.T) {
	service := newPlanner(t)
	for _, activityID := range []string{"act_walk_outdoor", "act_run_outdoor", "act_cycle_outdoor"} {
		level := "beginner"
		gear := []string(nil)
		if activityID == "act_run_outdoor" {
			level = "intermediate"
		}
		if activityID == "act_cycle_outdoor" {
			gear = []string{"bicycle"}
		}
		seed := int64(90)
		original, _, err := service.Generate(context.Background(), GenerateRequest{Goal: "endurance", WorkoutType: "cardio", DurationMinutes: 30, ExperienceLevel: level, Environment: "outdoor", Space: "unrestricted", AvailableEquipment: gear, PreferredActivityIDs: []string{activityID}, Seed: &seed})
		if err != nil {
			t.Fatalf("%s generate: %v", activityID, err)
		}
		oldBySection := make(map[string]string)
		for _, section := range original.Sections {
			if len(section.Items) != 1 || section.Items[0].ExerciseID != activityID {
				t.Fatalf("%s original did not repeat across workout sections: %+v", activityID, original.Sections)
			}
			oldBySection[section.Type] = section.Items[0].ItemID
		}
		shorter := 15
		result, _, err := service.AdaptPreview(context.Background(), original, ConstraintChanges{DurationMinutes: &shorter})
		if err != nil {
			t.Fatalf("%s adapt: %v", activityID, err)
		}
		seen := make(map[string]bool)
		for _, section := range result.AdaptedWorkout.Sections {
			if len(section.Items) != 1 || section.Items[0].ExerciseID != activityID {
				t.Fatalf("%s activity occurrence changed: %+v", activityID, result.AdaptedWorkout.Sections)
			}
			itemID := section.Items[0].ItemID
			if seen[itemID] {
				t.Fatalf("%s adaptation reused item ID %q across sections", activityID, itemID)
			}
			seen[itemID] = true
			if itemID != oldBySection[section.Type] {
				t.Fatalf("%s %s item ID = %q, want preserved %q", activityID, section.Type, itemID, oldBySection[section.Type])
			}
		}
	}
}

func TestAdaptPreservesMuscleEquipmentGymAndExclusionPreferences(t *testing.T) {
	service := newPlanner(t)
	seed := int64(71)
	original, _, err := service.Generate(context.Background(), GenerateRequest{
		Goal: "hypertrophy", WorkoutType: "strength", DurationMinutes: 40, ExperienceLevel: "intermediate", Environment: "gym",
		GymID: "gym_demo_jakarta_001", EquipmentSource: "gym_inventory", AvailableEquipment: []string{"dumbbell", "flat_bench", "cable_machine"},
		UnavailableEquipment: []string{"cable_machine"}, FocusMuscles: []string{"chest"}, ExcludedExerciseIDs: []string{"ex_cable_chest_fly"}, Intensity: "vigorous", Seed: &seed,
	})
	if err != nil {
		t.Fatal(err)
	}
	shorter := 25
	adapted, _, err := service.AdaptPreview(context.Background(), original, ConstraintChanges{DurationMinutes: &shorter})
	if err != nil {
		t.Fatal(err)
	}
	prefs := adapted.AdaptedWorkout.GenerationPreferences
	if prefs == nil || prefs.GymID != "gym_demo_jakarta_001" || prefs.EquipmentSource != "gym_inventory" || prefs.Intensity != "vigorous" || !reflect.DeepEqual(prefs.FocusMuscles, []string{"chest"}) || !reflect.DeepEqual(prefs.ExcludedExerciseIDs, []string{"ex_cable_chest_fly"}) || !reflect.DeepEqual(adapted.AdaptedWorkout.Constraints.UnavailableEquipment, []string{"cable_machine"}) {
		t.Fatalf("duration-only adaptation lost the original constraints: prefs=%+v constraints=%+v", prefs, adapted.AdaptedWorkout.Constraints)
	}
	for _, section := range adapted.AdaptedWorkout.Sections {
		for _, item := range section.Items {
			if item.ExerciseID == "ex_cable_chest_fly" {
				t.Fatal("adaptation selected an explicitly excluded exercise")
			}
		}
	}
	bodyweightOnly := []string{"bodyweight"}
	changed, _, err := service.AdaptPreview(context.Background(), original, ConstraintChanges{AvailableEquipment: bodyweightOnly})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range changed.AdaptedWorkout.Constraints.AvailableEquipment {
		if id != "bodyweight" {
			t.Fatalf("changed equipment availability was ignored: %v", changed.AdaptedWorkout.Constraints.AvailableEquipment)
		}
	}
}

func TestAdaptAppliesExplicitPreferenceOverrides(t *testing.T) {
	service := newPlanner(t)
	seed := int64(73)
	original, _, err := service.Generate(context.Background(), GenerateRequest{Goal: "strength", WorkoutType: "strength", DurationMinutes: 30, ExperienceLevel: "beginner", Environment: "home", Seed: &seed})
	if err != nil {
		t.Fatal(err)
	}
	goal, workoutType, experience, intensity := "endurance", "cardio", "intermediate", "moderate"
	adapted, _, err := service.AdaptPreview(context.Background(), original, ConstraintChanges{
		Goal: &goal, WorkoutType: &workoutType, ExperienceLevel: &experience, Intensity: &intensity,
		Environment: "outdoor", Space: "unrestricted", AvailableEquipment: []string{"bicycle"},
		PreferredActivityIDs: []string{"act_cycle_outdoor"}, ExcludedExerciseIDs: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if adapted.AdaptedWorkout.Goal != goal || adapted.AdaptedWorkout.WorkoutType != workoutType || adapted.AdaptedWorkout.Constraints.ExperienceLevel != experience || adapted.AdaptedWorkout.Constraints.Environment != "outdoor" || !reflect.DeepEqual(adapted.AdaptedWorkout.GenerationPreferences.PreferredActivityIDs, []string{"act_cycle_outdoor"}) || adapted.AdaptedWorkout.GenerationPreferences.Intensity != intensity {
		t.Fatalf("explicit preference overrides were not applied: %+v", adapted.AdaptedWorkout)
	}
	for _, section := range adapted.AdaptedWorkout.Sections {
		for _, item := range section.Items {
			if item.ExerciseID != "act_cycle_outdoor" {
				t.Fatalf("explicit cardio preference was not honored: %+v", item)
			}
		}
	}
}

func TestAdaptAcceptsLegacySessionSnapshotWithoutPreferences(t *testing.T) {
	service := newPlanner(t)
	var legacy Session
	if err := json.Unmarshal([]byte(`{"goal":"endurance","workout_type":"cardio","status":"planned","constraints":{"duration_minutes":30,"environment":"outdoor","available_equipment":[],"experience_level":"beginner","space":"unrestricted"},"generation":{"seed":9},"sections":[]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	shorter := 20
	if _, _, err := service.AdaptPreview(context.Background(), legacy, ConstraintChanges{DurationMinutes: &shorter}); err != nil {
		t.Fatalf("legacy snapshot without generation preferences failed to adapt: %v", err)
	}
}

func TestGenerateUsesGymInventoryAndAppliesUnavailableEquipmentWithoutMutation(t *testing.T) {
	service, gymService, _ := newPlannerFixture(t)
	ctx := context.Background()
	gymID := "gym_demo_jakarta_001"
	before, _, err := gymService.AvailableEquipment(ctx, gymID)
	if err != nil {
		t.Fatal(err)
	}
	seed := int64(33)
	session, failure, err := service.Generate(ctx, GenerateRequest{Goal: "strength", WorkoutType: "strength", DurationMinutes: 30, ExperienceLevel: "beginner", Environment: "gym", GymID: gymID, UnavailableEquipment: []string{"cable_machine", "dumbbell"}, Seed: &seed})
	if err != nil || failure != nil {
		t.Fatalf("Generate() failure=%+v err=%v", failure, err)
	}
	available := makeSet(before)
	for _, section := range session.Sections {
		for _, item := range section.Items {
			exercise, err := service.catalog.GetExercise(ctx, item.ExerciseID)
			if err != nil {
				t.Fatal(err)
			}
			for _, equipment := range exercise.RequiredEquipment {
				if equipment == "cable_machine" || equipment == "dumbbell" {
					t.Fatalf("excluded equipment %q was used by %s", equipment, item.ExerciseID)
				}
				if equipment != "bodyweight" && !available[equipment] {
					t.Fatalf("exercise %s requires equipment %q absent from gym inventory", item.ExerciseID, equipment)
				}
			}
		}
	}
	after, _, err := gymService.AvailableEquipment(ctx, gymID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("request changed stored gym inventory: before=%v after=%v", before, after)
	}
}

func TestGenerateRejectsUnknownGymID(t *testing.T) {
	service := newPlanner(t)
	_, _, err := service.Generate(context.Background(), GenerateRequest{Goal: "strength", WorkoutType: "strength", DurationMinutes: 30, ExperienceLevel: "beginner", Environment: "gym", GymID: "missing-gym"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown gym error=%v, want ErrInvalidRequest", err)
	}
}

func TestMixedWorkoutIncludesStrengthAndRequestedCardioWithinBudget(t *testing.T) {
	service := newPlanner(t)
	for _, cardioID := range []string{"act_walk_treadmill", "act_run_treadmill", "act_cycle_stationary"} {
		seed := int64(44)
		level := "beginner"
		if cardioID == "act_run_treadmill" {
			level = "intermediate"
		}
		session, failure, err := service.Generate(context.Background(), GenerateRequest{Goal: "general_fitness", WorkoutType: "mixed", DurationMinutes: 30, ExperienceLevel: level, Environment: "indoor", AvailableEquipment: []string{"bodyweight", "treadmill", "stationary_bike"}, PreferredActivityIDs: []string{cardioID}, Seed: &seed})
		if err != nil || failure != nil {
			t.Fatalf("cardio %s: Generate() failure=%+v err=%v", cardioID, failure, err)
		}
		seenStrength, seenCardio := false, false
		for _, section := range session.Sections {
			for _, item := range section.Items {
				if item.ExerciseID == cardioID {
					seenCardio = true
				}
				exercise, err := service.catalog.GetExercise(context.Background(), item.ExerciseID)
				if err != nil {
					t.Fatal(err)
				}
				if exercise.Category == "strength" {
					seenStrength = true
				}
			}
		}
		if !seenStrength || !seenCardio {
			t.Fatalf("mixed session omitted requested modality %s: strength=%v cardio=%v sections=%+v", cardioID, seenStrength, seenCardio, session.Sections)
		}
		if session.EstimatedDurationSeconds > session.PlannedDurationSeconds {
			t.Fatalf("mixed session exceeds time budget: %+v", session)
		}
	}
}

func TestMixedWorkoutReturnsUsefulFailureWhenCardioIsUnavailable(t *testing.T) {
	service := newPlanner(t)
	_, failure, err := service.Generate(context.Background(), GenerateRequest{Goal: "general_fitness", WorkoutType: "mixed", DurationMinutes: 30, ExperienceLevel: "beginner", Environment: "home", AvailableEquipment: []string{"bodyweight"}, PreferredActivityIDs: []string{"act_cycle_outdoor"}, Seed: ptrSeed(1)})
	if !errors.Is(err, ErrUnfeasible) || failure == nil {
		t.Fatalf("infeasible mixed request err=%v failure=%+v", err, failure)
	}
}

func TestPlannerCanSelectCatalogEntriesBeyondFirstHundred(t *testing.T) {
	service, _, db := newPlannerFixture(t)
	ctx := context.Background()
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("test_cardio_%03d", i)
		_, err := db.ExecContext(ctx, `INSERT INTO exercises(id,name,description,category,movement_pattern,difficulty,measurement_type,min_space,instructions_json,cautions_json,active,updated_at) VALUES(?,?,?,'cardio','walking','beginner','duration_distance','unrestricted','[]','[]',1,'2026-01-01T00:00:00Z')`, id, id, "test fixture")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO exercise_muscles(exercise_id,muscle_group_id,is_primary) VALUES(?, 'full_body', 1)`, id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO exercise_environments(exercise_id,environment) VALUES(?, 'outdoor')`, id); err != nil {
			t.Fatal(err)
		}
	}
	seed := int64(99)
	session, failure, err := service.Generate(ctx, GenerateRequest{Goal: "endurance", WorkoutType: "cardio", DurationMinutes: 25, ExperienceLevel: "beginner", Environment: "outdoor", Space: "unrestricted", PreferredActivityIDs: []string{"test_cardio_059"}, Seed: &seed})
	if err != nil || failure != nil {
		t.Fatalf("Generate() failure=%+v err=%v", failure, err)
	}
	for _, section := range session.Sections {
		for _, item := range section.Items {
			if item.ExerciseID == "test_cardio_059" {
				return
			}
		}
	}
	t.Fatalf("planner did not select eligible exercise after the first 100 catalog rows: %+v", session.Sections)
}

func TestGenerateHonorsAlreadyCancelledContext(t *testing.T) {
	service := newPlanner(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := service.Generate(ctx, GenerateRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate error=%v, want context.Canceled", err)
	}
}

func ptrSeed(seed int64) *int64 { return &seed }
