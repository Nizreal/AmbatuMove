package calories

import (
	"context"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func newCalorieService(t *testing.T) *Service {
	t.Helper()
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "calories.sqlite"))
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
	catalogService := catalog.NewService(db)
	gymService := gyms.NewService(db)
	if err := gymService.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	service := NewService(db, catalogService, gymService)
	if err := service.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	return service
}

func TestRecommendReturnsConstraintAwareWalkingRunningAndCycling(t *testing.T) {
	service := newCalorieService(t)
	request := RecommendationRequest{TargetKcal: 200, WeightKg: 70, MaxDurationMinutes: 45, AvailableEquipment: []string{"bicycle"}, AvailableEquipmentSet: true, Environment: "outdoor", ExperienceLevel: "intermediate", Category: "cardio", MaxIntensity: "moderate", Limit: 10}
	result, err := service.Recommend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.TargetAchievable {
		t.Fatalf("target should be achievable: %+v", result)
	}
	found := map[string]bool{}
	for _, recommendation := range result.Recommendations {
		found[recommendation.ExerciseID] = true
		expectedDuration := int(math.Ceil((200 * request.TargetKcal) / (recommendation.MET * 3.5 * request.WeightKg)))
		if recommendation.RequiredDurationMinutes != expectedDuration {
			t.Fatalf("required duration = %d, want ceiling %d", recommendation.RequiredDurationMinutes, expectedDuration)
		}
		if recommendation.RequiredDurationMinutes < 1 || recommendation.EstimatedKcal+0.05 < request.TargetKcal || recommendation.RecommendedDurationMinutes > request.MaxDurationMinutes || !recommendation.MeetsTarget || !recommendation.FitsConstraints {
			t.Fatalf("recommendation has inconsistent duration/calories/constraints: %+v", recommendation)
		}
		forward, err := service.Estimate(context.Background(), EstimateRequest{ActivityID: recommendation.ExerciseID, Intensity: recommendation.Intensity, DurationMinutes: float64(recommendation.RecommendedDurationMinutes), WeightKg: request.WeightKg})
		if err != nil || forward.EstimatedKcal != recommendation.EstimatedKcal {
			t.Fatalf("inverse result does not match forward estimator: forward=%+v recommendation=%+v err=%v", forward, recommendation, err)
		}
	}
	for _, id := range []string{"act_walk_outdoor", "act_run_outdoor", "act_cycle_outdoor"} {
		if !found[id] {
			t.Fatalf("recommendation omitted suitable activity %q: %+v", id, result.Recommendations)
		}
	}
	if result.Method != "MET" || result.EstimateType != "gross" {
		t.Fatalf("unexpected estimate semantics: %+v", result)
	}
}

func TestRecommendUsesGymInventoryAndExcludedEquipment(t *testing.T) {
	service := newCalorieService(t)
	request := RecommendationRequest{TargetKcal: 150, WeightKg: 70, MaxDurationMinutes: 45, GymID: "gym_demo_jakarta_002", Environment: "gym", ExperienceLevel: "beginner", Category: "cardio", MaxIntensity: "moderate", Limit: 10}
	result, err := service.Recommend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	foundBike := false
	for _, recommendation := range result.Recommendations {
		if recommendation.ExerciseID == "act_cycle_stationary" {
			foundBike = true
		}
		if recommendation.ExerciseID == "act_walk_treadmill" || recommendation.ExerciseID == "act_run_treadmill" {
			t.Fatalf("recommended gym equipment recorded out of service: %+v", recommendation)
		}
	}
	if !foundBike {
		t.Fatalf("gym inventory did not supply its stationary bike: %+v", result)
	}
	request.ExcludedEquipment = []string{"stationary_bike"}
	request.ExcludedExerciseIDs = []string{"act_marching_in_place"}
	filtered, err := service.Recommend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Recommendations) != 0 || filtered.TargetAchievable || filtered.Reason == "" {
		t.Fatalf("excluded gym equipment remained usable: %+v", filtered)
	}
	request.GymID = "missing-gym"
	if _, err := service.Recommend(context.Background(), request); err != ErrGymNotFound {
		t.Fatalf("unknown gym error = %v, want ErrGymNotFound", err)
	}
}

func TestRecommendRankingIsDeterministicAndHonorsExperienceIntensityAndLimit(t *testing.T) {
	service := newCalorieService(t)
	request := RecommendationRequest{TargetKcal: 50, WeightKg: 70, MaxDurationMinutes: 30, Environment: "outdoor", ExperienceLevel: "beginner", Category: "cardio", MaxIntensity: "low", Limit: 1}
	first, err := service.Recommend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Recommend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first.Recommendations) != 1 {
		t.Fatalf("recommendations are unstable or ignored limit: first=%+v second=%+v", first, second)
	}
	for _, recommendation := range first.Recommendations {
		if recommendation.ExerciseID == "act_run_outdoor" || recommendation.Intensity == "moderate" || recommendation.Intensity == "brisk" || recommendation.Intensity == "vigorous" {
			t.Fatalf("recommendation exceeded experience/intensity limits: %+v", recommendation)
		}
	}
}

func TestRecommendReturnsBoundedPartialAlternativesAndCalculatesReturnedDuration(t *testing.T) {
	service := newCalorieService(t)
	request := RecommendationRequest{TargetKcal: 2000, WeightKg: 70, MaxDurationMinutes: 5, AvailableEquipment: []string{"bicycle"}, AvailableEquipmentSet: true, Environment: "outdoor", ExperienceLevel: "intermediate", Category: "cardio", MaxIntensity: "vigorous", Limit: 3}
	result, err := service.Recommend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetAchievable || len(result.Recommendations) != request.Limit || result.Reason == "" {
		t.Fatalf("expected limited, infeasible alternatives with explanation: %+v", result)
	}
	for _, recommendation := range result.Recommendations {
		want := roundOne(recommendation.MET * 3.5 * request.WeightKg * float64(request.MaxDurationMinutes) / 200)
		if recommendation.MeetsTarget || recommendation.RecommendedDurationMinutes != request.MaxDurationMinutes || recommendation.EstimatedKcal != want || recommendation.RequiredDurationMinutes <= request.MaxDurationMinutes {
			t.Fatalf("partial recommendation is inconsistent: %+v want_kcal=%.1f", recommendation, want)
		}
	}
}

func TestRecommendRejectsInvalidInputsAndUnavailableCyclingEquipment(t *testing.T) {
	service := newCalorieService(t)
	for _, request := range []RecommendationRequest{
		{TargetKcal: 0, WeightKg: 70},
		{TargetKcal: 200, WeightKg: 0},
		{TargetKcal: 200, WeightKg: 70, MaxDurationMinutes: 1441},
		{TargetKcal: 200, WeightKg: 70, MaxIntensity: "impossible"},
	} {
		if _, err := service.Recommend(context.Background(), request); err != ErrInvalidRequest {
			t.Fatalf("Recommend(%+v) error=%v, want invalid request", request, err)
		}
	}
	result, err := service.Recommend(context.Background(), RecommendationRequest{TargetKcal: 200, WeightKg: 70, MaxDurationMinutes: 45, Environment: "outdoor", ExperienceLevel: "intermediate", Category: "cardio", MaxIntensity: "vigorous"})
	if err != nil {
		t.Fatal(err)
	}
	for _, recommendation := range result.Recommendations {
		if recommendation.ExerciseID == "act_cycle_outdoor" {
			t.Fatalf("recommended outdoor cycling without a bicycle: %+v", result.Recommendations)
		}
	}
}

func roundOne(value float64) float64 { return math.Round(value*10) / 10 }

func TestEstimateUsesCompendiumAndRoundsAfterFormula(t *testing.T) {
	service := newCalorieService(t)
	result, err := service.Estimate(context.Background(), EstimateRequest{Activity: "walking", Intensity: "moderate", DurationMinutes: 15, WeightKg: 70})
	if err != nil {
		t.Fatal(err)
	}
	if result.MET != 3.8 || result.Source.ActivityCode != "17190" || result.EstimatedKcal != 69.8 || result.EstimateType != "gross" {
		t.Fatalf("unexpected estimate: %+v", result)
	}
	net, err := service.Estimate(context.Background(), EstimateRequest{Activity: "walking", Intensity: "moderate", DurationMinutes: 15, WeightKg: 70, EstimateType: "net"})
	if err != nil {
		t.Fatal(err)
	}
	if net.EstimatedKcal != 51.5 {
		t.Fatalf("net estimate = %.1f, want 51.5", net.EstimatedKcal)
	}
}

func TestEstimateRejectsUnsupportedSpeedIntensityCombination(t *testing.T) {
	service := newCalorieService(t)
	speed := 7.0
	_, err := service.Estimate(context.Background(), EstimateRequest{ActivityID: "act_walk_outdoor", Intensity: "moderate", DurationMinutes: 20, WeightKg: 70, SpeedKmh: &speed})
	if err != ErrUnsupported {
		t.Fatalf("Estimate() error = %v, want unsupported intensity", err)
	}
}

func TestEstimateValidatesWeightAndPace(t *testing.T) {
	service := newCalorieService(t)
	pace := 0.0
	_, err := service.Estimate(context.Background(), EstimateRequest{Activity: "running", Intensity: "easy", DurationMinutes: 10, WeightKg: 70, PaceSecondsPerKm: &pace})
	if err != ErrInvalidRequest {
		t.Fatalf("Estimate() error = %v, want invalid request", err)
	}
}
