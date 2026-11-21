package calories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
)

var ErrInvalidRequest = errors.New("invalid calorie estimate request")
var ErrUnsupported = errors.New("unsupported activity or intensity")
var ErrGymNotFound = errors.New("gym not found")

const sourceTitle = "2024 Adult Compendium of Physical Activities"

type Service struct {
	db      *sql.DB
	catalog *catalog.Service
	gyms    gyms.InventoryProvider
}

func NewService(db *sql.DB, referenceCatalog *catalog.Service, gymInventory gyms.InventoryProvider) *Service {
	return &Service{db: db, catalog: referenceCatalog, gyms: gymInventory}
}

func (s *Service) ListProfiles(ctx context.Context, activity string) ([]METProfile, error) {
	query := `SELECT m.id,m.activity_id,e.movement_pattern,m.intensity_code,m.modality,m.speed_min_kmh,m.speed_max_kmh,m.met_value,m.compendium_year,m.compendium_code,m.activity_description,m.source_url FROM activity_met_profiles m JOIN exercises e ON e.id=m.activity_id WHERE e.active=1`
	args := []any{}
	if activity != "" {
		query += " AND e.movement_pattern=?"
		args = append(args, strings.ToLower(activity))
	}
	query += " ORDER BY e.movement_pattern,m.modality,m.intensity_code,m.id"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := make([]METProfile, 0)
	for rows.Next() {
		profile, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (s *Service) Estimate(ctx context.Context, request EstimateRequest) (Estimate, error) {
	if (request.ActivityID == "" && (request.Activity == "" || request.Intensity == "")) || (request.ActivityID != "" && request.Activity != "") ||
		request.DurationMinutes <= 0 || request.DurationMinutes > 1440 || math.IsNaN(request.DurationMinutes) || math.IsInf(request.DurationMinutes, 0) || request.WeightKg < 20 || request.WeightKg > 300 || math.IsNaN(request.WeightKg) || math.IsInf(request.WeightKg, 0) {
		return Estimate{}, ErrInvalidRequest
	}
	if request.EstimateType == "" {
		request.EstimateType = "gross"
	}
	if request.EstimateType != "gross" && request.EstimateType != "net" {
		return Estimate{}, ErrInvalidRequest
	}
	if request.SpeedKmh != nil && (*request.SpeedKmh <= 0 || *request.SpeedKmh > 100 || math.IsNaN(*request.SpeedKmh) || math.IsInf(*request.SpeedKmh, 0)) || request.PaceSecondsPerKm != nil && (*request.PaceSecondsPerKm <= 0 || *request.PaceSecondsPerKm > 7200 || math.IsNaN(*request.PaceSecondsPerKm) || math.IsInf(*request.PaceSecondsPerKm, 0)) {
		return Estimate{}, ErrInvalidRequest
	}
	if request.SpeedKmh != nil && request.PaceSecondsPerKm != nil {
		derived := 3600 / *request.PaceSecondsPerKm
		if math.Abs(derived-*request.SpeedKmh) > maxFloat(0.5, derived*0.05) {
			return Estimate{}, ErrInvalidRequest
		}
	}
	speed := request.SpeedKmh
	if request.PaceSecondsPerKm != nil {
		derived := 3600 / *request.PaceSecondsPerKm
		speed = &derived
	}
	query := `SELECT m.id,m.activity_id,e.movement_pattern,m.intensity_code,m.modality,m.speed_min_kmh,m.speed_max_kmh,m.met_value,m.compendium_year,m.compendium_code,m.activity_description,m.source_url FROM activity_met_profiles m JOIN exercises e ON e.id=m.activity_id WHERE e.active=1`
	args := []any{}
	if request.ActivityID != "" {
		query += " AND m.activity_id=?"
		args = append(args, request.ActivityID)
		if request.Intensity != "" {
			query += " AND m.intensity_code=?"
			args = append(args, strings.ToLower(request.Intensity))
		}
	} else {
		query += " AND e.movement_pattern=? AND m.intensity_code=?"
		args = append(args, strings.ToLower(request.Activity), strings.ToLower(request.Intensity))
		if oneOf(strings.ToLower(request.Activity), "walking", "running", "cycling") {
			query += " AND m.modality='outdoor'"
		}
	}
	query += " ORDER BY CASE m.intensity_code WHEN 'moderate' THEN 0 WHEN 'light' THEN 1 ELSE 2 END,m.modality,m.id"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return Estimate{}, err
	}
	defer rows.Close()
	var selected *METProfile
	for rows.Next() {
		profile, err := scanProfile(rows)
		if err != nil {
			return Estimate{}, err
		}
		if request.ActivityID == "" && profile.Intensity != strings.ToLower(request.Intensity) {
			continue
		}
		if speed != nil {
			if profile.SpeedMinKmh == nil && profile.SpeedMaxKmh == nil || profile.SpeedMinKmh != nil && *speed < *profile.SpeedMinKmh || profile.SpeedMaxKmh != nil && *speed > *profile.SpeedMaxKmh {
				continue
			}
		}
		if selected == nil {
			copy := profile
			selected = &copy
		}
	}
	if err := rows.Err(); err != nil {
		return Estimate{}, err
	}
	if selected == nil {
		return Estimate{}, ErrUnsupported
	}
	if speed != nil && request.Intensity != "" && selected.Intensity != request.Intensity {
		return Estimate{}, ErrUnsupported
	}
	met := selected.MET
	kcal := kcalForMET(met, request.WeightKg, request.DurationMinutes, request.EstimateType)
	rounded := roundKcal(kcal)
	return Estimate{Activity: selected.Activity, ActivityID: selected.ActivityID, Intensity: selected.Intensity, DurationMinutes: request.DurationMinutes,
		WeightKg: request.WeightKg, MET: met, EstimatedKcal: rounded, EstimateType: request.EstimateType,
		Method: "standard_met_formula", Source: selected.Source, Limitations: []string{"Population-based estimate; individual expenditure may differ.", "Terrain, incline, wind, pace changes, and personal fitness can affect actual expenditure."}}, nil
}

func (s *Service) Recommend(ctx context.Context, request RecommendationRequest) (RecommendationResult, error) {
	if request.TargetKcal <= 0 || request.TargetKcal > 10000 || math.IsNaN(request.TargetKcal) || math.IsInf(request.TargetKcal, 0) ||
		request.WeightKg < 20 || request.WeightKg > 300 || math.IsNaN(request.WeightKg) || math.IsInf(request.WeightKg, 0) {
		return RecommendationResult{}, ErrInvalidRequest
	}
	if request.MaxDurationMinutes == 0 {
		request.MaxDurationMinutes = 180
	}
	if request.MaxDurationMinutes < 1 || request.MaxDurationMinutes > 1440 {
		return RecommendationResult{}, ErrInvalidRequest
	}
	if request.Limit == 0 {
		request.Limit = 8
	}
	if request.Limit < 1 || request.Limit > 20 || len(request.AvailableEquipment) > 20 || len(request.ExcludedEquipment) > 20 || len(request.ExcludedExerciseIDs) > 100 {
		return RecommendationResult{}, ErrInvalidRequest
	}
	if request.Environment != "" && !oneOf(request.Environment, "home", "gym", "indoor", "outdoor") ||
		request.ExperienceLevel != "" && !oneOf(request.ExperienceLevel, "beginner", "intermediate", "advanced") ||
		request.Category != "" && !oneOf(request.Category, "cardio", "strength", "mobility") ||
		request.MaxIntensity != "" && !oneOf(request.MaxIntensity, "low", "moderate", "vigorous") || len(request.GymID) > 80 {
		return RecommendationResult{}, ErrInvalidRequest
	}
	if request.ExperienceLevel == "" {
		request.ExperienceLevel = "beginner"
	}
	if request.MaxIntensity == "" {
		request.MaxIntensity = "moderate"
	}
	if request.GymID != "" && request.Environment == "" {
		request.Environment = "gym"
	}
	if s.catalog == nil {
		return RecommendationResult{}, errors.New("exercise catalog is unavailable")
	}
	knownEquipment, err := s.catalog.ListEquipment(ctx, "")
	if err != nil {
		return RecommendationResult{}, err
	}
	equipmentIDs := make(map[string]bool, len(knownEquipment))
	for _, item := range knownEquipment {
		equipmentIDs[item.ID] = true
	}
	for _, id := range append(append([]string{}, request.AvailableEquipment...), request.ExcludedEquipment...) {
		if len(id) == 0 || len(id) > 80 || !equipmentIDs[id] {
			return RecommendationResult{}, ErrInvalidRequest
		}
	}
	for _, id := range request.ExcludedExerciseIDs {
		if len(id) == 0 || len(id) > 80 {
			return RecommendationResult{}, ErrInvalidRequest
		}
	}

	available := make(map[string]bool)
	if request.GymID != "" {
		if s.gyms == nil {
			return RecommendationResult{}, errors.New("gym inventory service is unavailable")
		}
		inventory, _, inventoryErr := s.gyms.AvailableEquipment(ctx, request.GymID)
		if errors.Is(inventoryErr, gyms.ErrNotFound) {
			return RecommendationResult{}, ErrGymNotFound
		}
		if inventoryErr != nil {
			return RecommendationResult{}, inventoryErr
		}
		gymSet := make(map[string]bool, len(inventory))
		for _, id := range inventory {
			gymSet[id] = true
		}
		if request.AvailableEquipmentSet {
			for _, id := range request.AvailableEquipment {
				if gymSet[id] {
					available[id] = true
				}
			}
		} else {
			available = gymSet
		}
	} else if request.AvailableEquipmentSet {
		for _, id := range request.AvailableEquipment {
			available[id] = true
		}
	}
	available["bodyweight"] = true
	for _, id := range request.ExcludedEquipment {
		delete(available, id)
	}
	excludedExercises := make(map[string]bool, len(request.ExcludedExerciseIDs))
	for _, id := range request.ExcludedExerciseIDs {
		excludedExercises[id] = true
	}

	exercises, err := s.catalog.AllExercises(ctx)
	if err != nil {
		return RecommendationResult{}, err
	}
	exerciseByID := make(map[string]catalog.ExerciseSummary, len(exercises))
	for _, exercise := range exercises {
		exerciseByID[exercise.ID] = exercise
	}
	profiles, err := s.ListProfiles(ctx, "")
	if err != nil {
		return RecommendationResult{}, err
	}
	maxIntensityRank := recommendationIntensityLimit(request.MaxIntensity)
	candidates := make([]recommendationCandidate, 0, len(profiles))
	for _, profile := range profiles {
		if err := ctx.Err(); err != nil {
			return RecommendationResult{}, err
		}
		exercise, ok := exerciseByID[profile.ActivityID]
		if !ok || profile.MET <= 0 || math.IsNaN(profile.MET) || math.IsInf(profile.MET, 0) || excludedExercises[exercise.ID] ||
			request.Category != "" && exercise.Category != request.Category ||
			recommendationLevelRank(exercise.Difficulty) > recommendationLevelRank(request.ExperienceLevel) ||
			!recommendationEnvironmentAllowed(exercise.Environments, request.Environment) ||
			!recommendationEquipmentAllowed(exercise.RequiredEquipment, available) || recommendationActivityIntensity(profile.Intensity) > maxIntensityRank {
			continue
		}
		exactMinutes := (200 * request.TargetKcal) / (profile.MET * 3.5 * request.WeightKg)
		if math.IsNaN(exactMinutes) || math.IsInf(exactMinutes, 0) || exactMinutes > float64(math.MaxInt-1) {
			return RecommendationResult{}, ErrInvalidRequest
		}
		requiredMinutes := int(math.Ceil(exactMinutes))
		if requiredMinutes < 1 {
			requiredMinutes = 1
		}
		recommendedMinutes := minInt(requiredMinutes, request.MaxDurationMinutes)
		unroundedKcal := kcalForMET(profile.MET, request.WeightKg, float64(recommendedMinutes), "gross")
		meetsTarget := unroundedKcal >= request.TargetKcal
		reason := "Compatible with the supplied activity constraints and reaches the target within the available duration."
		if !meetsTarget {
			reason = "Compatible partial alternative; reaching the original target requires more than the available duration."
		}
		candidates = append(candidates, recommendationCandidate{recommendation: Recommendation{
			ExerciseID: exercise.ID, Name: exercise.Name, Category: exercise.Category, Intensity: profile.Intensity, MET: profile.MET,
			RequiredDurationMinutes: requiredMinutes, RecommendedDurationMinutes: recommendedMinutes, EstimatedKcal: roundKcal(unroundedKcal),
			MeetsTarget: meetsTarget, FitsConstraints: true, RequiredEquipment: append([]string{}, exercise.RequiredEquipment...), Reason: reason,
		}, experienceRank: recommendationLevelRank(exercise.Difficulty), intensityRank: recommendationActivityIntensity(profile.Intensity), profileID: profile.ID})
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.recommendation.MeetsTarget != b.recommendation.MeetsTarget {
			return a.recommendation.MeetsTarget
		}
		if a.experienceRank != b.experienceRank {
			return a.experienceRank < b.experienceRank
		}
		if a.intensityRank != b.intensityRank {
			return a.intensityRank < b.intensityRank
		}
		if a.recommendation.RequiredDurationMinutes != b.recommendation.RequiredDurationMinutes {
			return a.recommendation.RequiredDurationMinutes < b.recommendation.RequiredDurationMinutes
		}
		if a.recommendation.ExerciseID != b.recommendation.ExerciseID {
			return a.recommendation.ExerciseID < b.recommendation.ExerciseID
		}
		return a.profileID < b.profileID
	})

	// Keep the best-supported intensity/modality for each activity. This retains
	// a compact, useful choice set rather than filling the result with duplicates.
	recommendations := make([]Recommendation, 0, minInt(len(candidates), request.Limit))
	seenExercises := make(map[string]bool)
	for _, candidate := range candidates {
		if seenExercises[candidate.recommendation.ExerciseID] {
			continue
		}
		seenExercises[candidate.recommendation.ExerciseID] = true
		recommendations = append(recommendations, candidate.recommendation)
		if len(recommendations) == request.Limit {
			break
		}
	}
	targetAchievable := false
	for _, recommendation := range recommendations {
		targetAchievable = targetAchievable || recommendation.MeetsTarget
	}
	result := RecommendationResult{TargetKcal: request.TargetKcal, WeightKg: request.WeightKg, MaxDurationMinutes: request.MaxDurationMinutes,
		TargetAchievable: targetAchievable, Recommendations: recommendations, Method: "MET", EstimateType: "gross",
		Disclaimer: "Calorie expenditure values are estimates; actual expenditure varies between individuals and conditions."}
	if !targetAchievable {
		if len(recommendations) == 0 {
			result.Reason = "No supported activity satisfies the supplied equipment, environment, experience, category, intensity, and exclusion constraints."
		} else {
			result.Reason = "No compatible activity reaches the calorie target within the requested maximum duration; alternatives show estimated calories for the available time."
		}
	}
	return result, nil
}

type recommendationCandidate struct {
	recommendation Recommendation
	experienceRank int
	intensityRank  int
	profileID      string
}

func recommendationIntensityLimit(value string) int {
	switch value {
	case "low":
		return 1
	case "moderate":
		return 2
	default:
		return 3
	}
}

func recommendationActivityIntensity(value string) int {
	switch value {
	case "light", "easy":
		return 1
	case "moderate":
		return 2
	case "brisk":
		return 3
	case "vigorous":
		return 3
	default:
		return 100
	}
}

func recommendationEnvironmentAllowed(values []string, requested string) bool {
	if requested == "" {
		return true
	}
	for _, value := range values {
		if value == requested || requested == "indoor" && (value == "home" || value == "gym") {
			return true
		}
	}
	return false
}

func recommendationLevelRank(level string) int {
	switch level {
	case "beginner":
		return 1
	case "intermediate":
		return 2
	case "advanced":
		return 3
	default:
		return 0
	}
}

func recommendationEquipmentAllowed(required []string, available map[string]bool) bool {
	for _, id := range required {
		if !available[id] {
			return false
		}
	}
	return true
}

func roundKcal(value float64) float64 {
	return math.Round(math.Nextafter(value*10, math.Inf(1))) / 10
}

func kcalForMET(met, weightKg, durationMinutes float64, estimateType string) float64 {
	if estimateType == "net" {
		return math.Max(0, (met-1)*3.5*weightKg*durationMinutes/200)
	}
	return met * 3.5 * weightKg * durationMinutes / 200
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func scanProfile(rows *sql.Rows) (METProfile, error) {
	var profile METProfile
	var speedMin, speedMax sql.NullFloat64
	var year int
	var code, description, sourceURL string
	if err := rows.Scan(&profile.ID, &profile.ActivityID, &profile.Activity, &profile.Intensity, &profile.Modality, &speedMin, &speedMax, &profile.MET, &year, &code, &description, &sourceURL); err != nil {
		return METProfile{}, err
	}
	if speedMin.Valid {
		profile.SpeedMinKmh = &speedMin.Float64
	}
	if speedMax.Valid {
		profile.SpeedMaxKmh = &speedMax.Float64
	}
	profile.Source = Source{Title: sourceTitle, Year: year, ActivityCode: code, ActivityDescription: description, URL: sourceURL}
	return profile, nil
}

func (s *Service) Seed(ctx context.Context) error {
	query := `INSERT INTO activity_met_profiles
		(id,activity_id,intensity_code,modality,speed_min_kmh,speed_max_kmh,met_value,compendium_year,compendium_code,activity_description,source_url)
		VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET activity_id=excluded.activity_id,intensity_code=excluded.intensity_code,modality=excluded.modality,speed_min_kmh=excluded.speed_min_kmh,speed_max_kmh=excluded.speed_max_kmh,met_value=excluded.met_value,compendium_year=excluded.compendium_year,compendium_code=excluded.compendium_code,activity_description=excluded.activity_description,source_url=excluded.source_url`
	values := metSeedValues()
	for start := 0; start < len(values); start += 11 {
		if _, err := s.db.ExecContext(ctx, query, values[start:start+11]...); err != nil {
			return fmt.Errorf("seed MET activity profile %v: %w", values[start], err)
		}
	}
	return nil
}

func metSeedValues() []any {
	const base = "https://pacompendium.com/"
	return []any{
		"met_walk_out_light", "act_walk_outdoor", "light", "outdoor", 3.2, 3.9, 2.8, 2024, "17152", "Walking, 2.0 to 2.4 mph, level, slow pace, firm surface", base + "walking/",
		"met_walk_out_moderate", "act_walk_outdoor", "moderate", "outdoor", 4.5, 5.5, 3.8, 2024, "17190", "Walking, 2.8 to 3.4 mph, level, moderate pace, firm surface", base + "walking/",
		"met_walk_out_brisk", "act_walk_outdoor", "brisk", "outdoor", 5.6, 6.3, 4.8, 2024, "17200", "Walking, 3.5 to 3.9 mph, level, brisk, firm surface, walking for exercise", base + "walking/",
		"met_walk_tread_light", "act_walk_treadmill", "light", "treadmill", 3.2, 3.9, 3.0, 2024, "17349", "Walking, treadmill, 2.0 to 2.4 mph, 0% grade", base + "walking/",
		"met_walk_tread_moderate", "act_walk_treadmill", "moderate", "treadmill", 4.0, 4.7, 3.5, 2024, "17352", "Walking, treadmill, 2.5 to 2.9 mph, 0% grade", base + "walking/",
		"met_walk_tread_brisk", "act_walk_treadmill", "brisk", "treadmill", 4.8, 5.5, 3.8, 2024, "17355", "Walking, treadmill, 3.0 to 3.4 mph, 0% grade", base + "walking/",
		"met_run_easy", "act_run_outdoor", "easy", "outdoor", 6.4, 6.8, 6.5, 2024, "12028", "Running, 4 to 4.2 mph", base + "running/",
		"met_run_moderate", "act_run_outdoor", "moderate", "outdoor", 9.7, 10.1, 9.3, 2024, "12050", "Running, 6 to 6.3 mph", base + "running/",
		"met_run_vigorous", "act_run_outdoor", "vigorous", "outdoor", 10.4, 10.9, 10.5, 2024, "12060", "Running, 6.7 mph", base + "running/",
		"met_run_tread_easy", "act_run_treadmill", "easy", "treadmill", 6.4, 6.8, 6.5, 2024, "12028", "Running, 4 to 4.2 mph", base + "running/",
		"met_run_tread_moderate", "act_run_treadmill", "moderate", "treadmill", 9.7, 10.1, 9.3, 2024, "12050", "Running, 6 to 6.3 mph", base + "running/",
		"met_run_tread_vigorous", "act_run_treadmill", "vigorous", "treadmill", 10.4, 10.9, 10.5, 2024, "12060", "Running, 6.7 mph", base + "running/",
		"met_cycle_easy", "act_cycle_outdoor", "easy", "outdoor", 0, 16.0, 4.0, 2024, "01010", "Bicycling, less than 10 mph, leisure, to work or for pleasure", base + "bicycling/",
		"met_cycle_moderate", "act_cycle_outdoor", "moderate", "outdoor", 19.3, 22.4, 8.0, 2024, "01030", "Bicycling, 12 to 13.9 mph, leisure, moderate effort", base + "bicycling/",
		"met_cycle_vigorous", "act_cycle_outdoor", "vigorous", "outdoor", 22.5, 25.5, 10.0, 2024, "01040", "Bicycling, 14 to 15.9 mph, racing or leisure, fast, vigorous effort", base + "bicycling/",
		"met_stationary_light", "act_cycle_stationary", "light", "stationary", nil, nil, 4.0, 2024, "01214", "Bicycling, stationary, 50 watts, light effort", base + "bicycling/",
		"met_stationary_moderate", "act_cycle_stationary", "moderate", "stationary", nil, nil, 6.0, 2024, "01220", "Bicycling, stationary, 90 to 100 watts, moderate to vigorous effort", base + "bicycling/",
		"met_stationary_vigorous", "act_cycle_stationary", "vigorous", "stationary", nil, nil, 10.8, 2024, "01236", "Bicycling, stationary, 200 to 229 watts, vigorous", base + "bicycling/",
		"met_strength_moderate", "act_strength_training", "moderate", "strength", nil, nil, 3.5, 2024, "02054", "Resistance training, multiple exercises, 8 to 15 reps at varied resistance", base + "conditioning-exercise/",
		"met_strength_vigorous", "act_strength_training", "vigorous", "strength", nil, nil, 6.0, 2024, "02050", "Resistance training, power lifting or bodybuilding, vigorous effort", base + "conditioning-exercise/",
		"met_bodyweight_light", "act_bodyweight_training", "light", "bodyweight", nil, nil, 3.0, 2024, "02056", "Body weight resistance exercises, general", base + "conditioning-exercise/",
		"met_bodyweight_moderate", "act_bodyweight_training", "moderate", "bodyweight", nil, nil, 3.8, 2024, "02022", "Calisthenics, moderate effort", base + "conditioning-exercise/",
		"met_bodyweight_vigorous", "act_bodyweight_training", "vigorous", "bodyweight", nil, nil, 6.5, 2024, "02057", "Body weight resistance exercises, high intensity", base + "conditioning-exercise/",
		"met_marching_light", "act_marching_in_place", "light", "bodyweight", nil, nil, 2.8, 2024, "02024", "Calisthenics, light effort", base + "conditioning-exercise/",
		"met_marching_moderate", "act_marching_in_place", "moderate", "bodyweight", nil, nil, 3.8, 2024, "02022", "Calisthenics, moderate effort", base + "conditioning-exercise/",
		"met_marching_vigorous", "act_marching_in_place", "vigorous", "bodyweight", nil, nil, 7.5, 2024, "02020", "Calisthenics, vigorous effort", base + "conditioning-exercise/",
		"met_mobility_light", "act_mobility_flow", "light", "mobility", nil, nil, 2.3, 2024, "02150", "Yoga, Hatha", base + "conditioning-exercise/",
		"met_mobility_moderate", "act_mobility_flow", "moderate", "mobility", nil, nil, 4.0, 2024, "02160", "Yoga, Power", base + "conditioning-exercise/",
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if item == value {
			return true
		}
	}
	return false
}
