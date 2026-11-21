package planning

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	mathrand "math/rand"
	"sort"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
)

var ErrInvalidRequest = errors.New("invalid workout request")
var ErrUnfeasible = errors.New("workout constraints cannot be satisfied")
var ErrNotFound = errors.New("exercise not found")

const algorithmVersion = "planner_v1"

type Service struct {
	catalog *catalog.Service
	gyms    gyms.InventoryProvider
}

func NewService(catalogService *catalog.Service, inventory ...gyms.InventoryProvider) *Service {
	service := &Service{catalog: catalogService}
	if len(inventory) > 0 {
		service.gyms = inventory[0]
	}
	return service
}

func (s *Service) Generate(ctx context.Context, request GenerateRequest) (Session, *Failure, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, nil, err
	}
	if err := validateRequest(request); err != nil {
		return Session{}, nil, err
	}
	seed, err := sessionSeed(request.Seed)
	if err != nil {
		return Session{}, nil, err
	}
	request.Seed = &seed
	warnings := make([]string, 0)
	if request.GymID != "" {
		if s.gyms == nil {
			return Session{}, nil, fmt.Errorf("%w: gym inventory service is unavailable", ErrInvalidRequest)
		}
		inventory, verification, err := s.gyms.AvailableEquipment(ctx, request.GymID)
		if err != nil {
			return Session{}, nil, fmt.Errorf("%w: gym_id does not identify a usable gym", ErrInvalidRequest)
		}
		inventorySet := makeSet(inventory)
		switch request.EquipmentSource {
		case "explicit":
			if request.AvailableEquipment == nil {
				return Session{}, nil, fmt.Errorf("%w: equipment_source explicit requires available_equipment", ErrInvalidRequest)
			}
			warnings = append(warnings, "Explicit equipment intentionally overrides the recorded gym inventory.")
		case "", "gym_inventory":
			if request.AvailableEquipment == nil {
				request.AvailableEquipment = inventory
			} else {
				intersection := make([]string, 0, len(request.AvailableEquipment))
				for _, id := range request.AvailableEquipment {
					if inventorySet[id] {
						intersection = append(intersection, id)
					} else {
						warnings = append(warnings, "Some requested equipment is not recorded in the selected gym inventory.")
					}
				}
				request.AvailableEquipment = intersection
			}
		}
		if verification != "verified" {
			warnings = append(warnings, "Gym inventory is not currently verified and does not describe live equipment availability.")
		}
	}
	available := makeSet(request.AvailableEquipment)
	available["bodyweight"] = true
	for _, item := range request.UnavailableEquipment {
		delete(available, item)
	}
	request.AvailableEquipment = make([]string, 0, len(available))
	for equipment := range available {
		request.AvailableEquipment = append(request.AvailableEquipment, equipment)
	}
	sort.Strings(request.AvailableEquipment)
	all, err := s.catalog.AllExercises(ctx)
	if err != nil {
		return Session{}, nil, err
	}
	blocked := makeSet(request.ExcludedExerciseIDs)
	preferred := makeSet(request.PreferredActivityIDs)
	filter := func(category string) []catalog.ExerciseSummary {
		items := make([]catalog.ExerciseSummary, 0)
		for _, exercise := range all {
			if ctx.Err() != nil {
				return nil
			}
			if exercise.Category != category || blocked[exercise.ID] || !levelAllowed(exercise.Difficulty, request.ExperienceLevel) || !spaceAllowed(exercise.MinimumSpace, request.Space) || !environmentAllowed(exercise.Environments, request.Environment) || !equipmentAllowed(exercise.RequiredEquipment, available) {
				continue
			}
			if len(request.FocusMuscles) > 0 && !hasAny(exercise.PrimaryMuscles, request.FocusMuscles) {
				continue
			}
			if category == "cardio" && len(preferred) > 0 && !preferred[exercise.ID] {
				continue
			}
			items = append(items, exercise)
		}
		return items
	}
	strength, mobility, cardio := filter("strength"), filter("mobility"), filter("cardio")
	if err := ctx.Err(); err != nil {
		return Session{}, nil, err
	}
	var candidates []catalog.ExerciseSummary
	switch request.WorkoutType {
	case "strength":
		candidates = strength
	case "mobility":
		candidates = mobility
	case "cardio":
		candidates = cardio
	case "mixed":
		if len(strength) == 0 || len(cardio) == 0 {
			missing := "strength_activity"
			if len(strength) > 0 {
				missing = "cardio_activity"
			}
			return Session{}, &Failure{Constraints: []string{missing}, Suggestions: []string{"Allow compatible strength equipment and a walking, running, or cycling activity", "Change the environment or workout duration"}}, ErrUnfeasible
		}
		candidates = append(append(append([]catalog.ExerciseSummary{}, strength...), cardio...), mobility...)
	}
	if len(candidates) == 0 {
		failed := []string{"compatible_activity"}
		suggestions := []string{"Allow more equipment or space", "Choose a compatible environment or activity"}
		if request.WorkoutType == "cardio" && len(preferred) > 0 {
			failed = append(failed, "preferred_activity")
		}
		return Session{}, &Failure{Constraints: failed, Suggestions: suggestions}, ErrUnfeasible
	}
	chooser := mathrand.New(mathrand.NewSource(seed))
	chooser.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	if err := ctx.Err(); err != nil {
		return Session{}, nil, err
	}
	return assemble(request, seed, candidates, strength, cardio, mobility, warnings), nil, nil
}

func validateRequest(request GenerateRequest) error {
	if !oneOf(request.Goal, "general_fitness", "strength", "hypertrophy", "endurance", "mobility", "active_recovery") ||
		!oneOf(request.WorkoutType, "strength", "cardio", "mobility", "mixed") ||
		request.DurationMinutes < 5 || request.DurationMinutes > 180 ||
		!oneOf(request.ExperienceLevel, "beginner", "intermediate", "advanced") ||
		!oneOf(request.Environment, "home", "gym", "indoor", "outdoor") {
		return ErrInvalidRequest
	}
	if request.Space == "" {
		request.Space = "medium"
	}
	if !oneOf(request.Space, "small", "medium", "large", "unrestricted") {
		return ErrInvalidRequest
	}
	if request.WorkoutType == "cardio" && len(request.FocusMuscles) > 0 {
		return fmt.Errorf("%w: focus_muscles cannot be used for cardio-only sessions", ErrInvalidRequest)
	}
	if request.Intensity != "" && !oneOf(request.Intensity, "low", "moderate", "vigorous") {
		return ErrInvalidRequest
	}
	if request.EquipmentSource != "" && request.EquipmentSource != "explicit" && request.EquipmentSource != "gym_inventory" {
		return ErrInvalidRequest
	}
	return nil
}

func assemble(request GenerateRequest, seed int64, candidates, strength, cardio, mobility []catalog.ExerciseSummary, warnings []string) Session {
	duration := request.DurationMinutes * 60
	space := request.Space
	if space == "" {
		space = "medium"
	}
	intensity := request.Intensity
	if intensity == "" {
		intensity = "moderate"
	}
	sections := make([]Section, 0, 3)
	order := 0
	addTimed := func(section, exerciseID, intensity string, seconds int) {
		order++
		sections = appendSectionItem(sections, section, Item{ItemID: fmt.Sprintf("wi_%03d", order), ExerciseID: exerciseID, Order: order, Prescription: map[string]any{"duration_seconds": seconds, "intensity": intensity}, EstimatedDurationSeconds: seconds})
	}
	if request.WorkoutType == "cardio" {
		activity := candidates[0]
		warm := duration / 10
		cool := duration / 10
		if warm < 60 {
			warm = 60
		}
		if cool < 60 {
			cool = 60
		}
		if warm+cool >= duration {
			warm, cool = duration/5, duration/5
		}
		main := duration - warm - cool
		addTimed("warmup", activity.ID, "low", warm)
		addTimed("main", activity.ID, intensity, main)
		addTimed("cooldown", activity.ID, "low", cool)
	} else {
		warm := duration / 10
		cool := duration / 10
		if warm < 60 {
			warm = 60
		}
		if cool < 60 {
			cool = 60
		}
		if warm+cool >= duration {
			warm, cool = duration/5, duration/5
		}
		warmList := mobility
		if len(warmList) == 0 {
			warmList = candidates
		}
		coolList := mobility
		if len(coolList) == 0 {
			coolList = candidates
		}
		addTimed("warmup", warmList[0].ID, "low", warm)
		mainDuration := duration - warm - cool
		if request.WorkoutType == "mobility" {
			list := choosePool(mobility, candidates)
			count := min(4, max(1, mainDuration/120))
			for i := 0; i < count; i++ {
				seconds := mainDuration / count
				addTimed("main", list[i%len(list)].ID, "moderate", seconds)
			}
		} else if request.WorkoutType == "mixed" {
			// Reserve meaningful time for one strength movement and one requested cardio mode.
			strengthBudget := mainDuration / 2
			if strengthBudget < 1 {
				strengthBudget = 1
			}
			addStrengthItem(&sections, &order, strength[0], request.ExperienceLevel, strengthBudget)
			cardioBudget := mainDuration - strengthBudget
			if cardioBudget < 1 {
				cardioBudget = 1
			}
			addTimed("main", cardio[0].ID, intensity, cardioBudget)
		} else {
			pool := choosePool(strength, candidates)
			sets, reps, rest := 3, 10, restSeconds(request.ExperienceLevel)
			perItem := sets*reps*3 + (sets-1)*rest + 15
			count := min(4, max(1, mainDuration/perItem))
			for i := 0; i < count; i++ {
				addStrengthItem(&sections, &order, pool[i%len(pool)], request.ExperienceLevel, mainDuration/count)
			}
		}
		addTimed("cooldown", coolList[0].ID, "low", cool)
	}
	sections = mergeSections(sections)
	total := 0
	for _, section := range sections {
		for _, item := range section.Items {
			total += item.EstimatedDurationSeconds
		}
	}
	if total > duration {
		sections, total = fitSections(sections, duration)
	}
	available := append([]string{}, request.AvailableEquipment...)
	return Session{
		ID: nil, Saved: false, Version: 1, Goal: request.Goal, WorkoutType: request.WorkoutType, Status: "planned",
		Constraints:            SessionConstraints{DurationMinutes: request.DurationMinutes, Environment: request.Environment, AvailableEquipment: available, UnavailableEquipment: request.UnavailableEquipment, ExperienceLevel: request.ExperienceLevel, Space: space},
		PlannedDurationSeconds: duration, EstimatedDurationSeconds: total, Sections: sections,
		Warnings: warnings, Generation: Generation{AlgorithmVersion: algorithmVersion, Seed: seed},
		GenerationPreferences: &GenerationPreferences{
			FocusMuscles: append([]string(nil), request.FocusMuscles...), PreferredActivityIDs: append([]string(nil), request.PreferredActivityIDs...),
			ExcludedExerciseIDs: append([]string(nil), request.ExcludedExerciseIDs...), AvailableEquipment: append([]string(nil), request.AvailableEquipment...),
			AvailableEquipmentSet: request.AvailableEquipment != nil, EquipmentSource: request.EquipmentSource, GymID: request.GymID, Intensity: request.Intensity,
		},
	}
}

func (s *Service) AdaptPreview(ctx context.Context, workout Session, changes ConstraintChanges) (AdaptResult, *Failure, error) {
	if workout.Status != "planned" && workout.Status != "in_progress" {
		return AdaptResult{}, nil, fmt.Errorf("%w: only planned or in-progress sessions can be adapted", ErrInvalidRequest)
	}
	request := GenerateRequest{
		Goal: workout.Goal, WorkoutType: workout.WorkoutType, DurationMinutes: workout.Constraints.DurationMinutes,
		ExperienceLevel: workout.Constraints.ExperienceLevel, Environment: workout.Constraints.Environment,
		Space: workout.Constraints.Space, AvailableEquipment: append([]string{}, workout.Constraints.AvailableEquipment...),
		UnavailableEquipment: append([]string{}, workout.Constraints.UnavailableEquipment...), Seed: &workout.Generation.Seed,
	}
	if preferences := workout.GenerationPreferences; preferences != nil {
		request.FocusMuscles = append([]string(nil), preferences.FocusMuscles...)
		request.PreferredActivityIDs = append([]string(nil), preferences.PreferredActivityIDs...)
		request.ExcludedExerciseIDs = append([]string(nil), preferences.ExcludedExerciseIDs...)
		request.GymID = preferences.GymID
		request.EquipmentSource = preferences.EquipmentSource
		request.Intensity = preferences.Intensity
		if preferences.AvailableEquipmentSet {
			request.AvailableEquipment = append(make([]string, 0, len(preferences.AvailableEquipment)), preferences.AvailableEquipment...)
		}
	}
	if request.ExperienceLevel == "" {
		request.ExperienceLevel = "beginner"
	}
	if changes.DurationMinutes != nil {
		request.DurationMinutes = *changes.DurationMinutes
	}
	if changes.Goal != nil {
		request.Goal = *changes.Goal
	}
	if changes.WorkoutType != nil {
		request.WorkoutType = *changes.WorkoutType
	}
	if changes.ExperienceLevel != nil {
		request.ExperienceLevel = *changes.ExperienceLevel
	}
	if changes.Intensity != nil {
		request.Intensity = *changes.Intensity
	}
	if changes.Environment != "" {
		request.Environment = changes.Environment
	}
	if changes.Space != "" {
		request.Space = changes.Space
	}
	if changes.AvailableEquipment != nil {
		request.AvailableEquipment = append([]string{}, changes.AvailableEquipment...)
	}
	if changes.UnavailableEquipment != nil {
		request.UnavailableEquipment = append([]string{}, changes.UnavailableEquipment...)
	}
	if changes.FocusMuscles != nil {
		request.FocusMuscles = append([]string{}, changes.FocusMuscles...)
	}
	if changes.PreferredActivityIDs != nil {
		request.PreferredActivityIDs = append([]string{}, changes.PreferredActivityIDs...)
	}
	if changes.ExcludedExerciseIDs != nil {
		request.ExcludedExerciseIDs = append([]string{}, changes.ExcludedExerciseIDs...)
	}
	if changes.GymID != nil {
		request.GymID = *changes.GymID
	}
	if changes.EquipmentSource != nil {
		request.EquipmentSource = *changes.EquipmentSource
	}
	updated, failure, err := s.Generate(ctx, request)
	if err != nil {
		return AdaptResult{}, failure, err
	}
	type occurrenceKey struct{ section, exercise string }
	oldItems := make(map[occurrenceKey][]string)
	allOldIDs := make(map[string]bool)
	for _, section := range workout.Sections {
		for _, item := range section.Items {
			key := occurrenceKey{section: section.Type, exercise: item.ExerciseID}
			oldItems[key] = append(oldItems[key], item.ItemID)
			if item.ItemID != "" {
				allOldIDs[item.ItemID] = true
			}
		}
	}
	usedIDs := make(map[string]bool)
	matchedOldIDs := make(map[string]bool)
	newOrder := 0
	for si := range updated.Sections {
		for ii := range updated.Sections[si].Items {
			item := &updated.Sections[si].Items[ii]
			key := occurrenceKey{section: updated.Sections[si].Type, exercise: item.ExerciseID}
			matched := false
			for len(oldItems[key]) > 0 {
				oldID := oldItems[key][0]
				oldItems[key] = oldItems[key][1:]
				if oldID == "" || usedIDs[oldID] {
					continue
				}
				item.ItemID = oldID
				usedIDs[oldID] = true
				matchedOldIDs[oldID] = true
				matched = true
				break
			}
			if matched {
				continue
			}
			if !usedIDs[item.ItemID] && !allOldIDs[item.ItemID] {
				usedIDs[item.ItemID] = true
				continue
			}
			baseID := item.ItemID
			if baseID == "" {
				baseID = fmt.Sprintf("wi_%03d", item.Order)
			}
			for {
				newOrder++
				candidate := fmt.Sprintf("%s_adapt_%d", baseID, newOrder)
				if !usedIDs[candidate] && !allOldIDs[candidate] {
					item.ItemID = candidate
					usedIDs[candidate] = true
					break
				}
			}
		}
	}
	preserved, replaced := make([]string, 0), make([]string, 0)
	for _, section := range workout.Sections {
		for _, item := range section.Items {
			if matchedOldIDs[item.ItemID] {
				preserved = append(preserved, item.ItemID)
			} else if item.ItemID != "" {
				replaced = append(replaced, item.ItemID)
			}
		}
	}
	sort.Strings(preserved)
	sort.Strings(replaced)
	changesApplied := make([]string, 0, 5)
	if changes.DurationMinutes != nil && *changes.DurationMinutes != workout.Constraints.DurationMinutes {
		changesApplied = append(changesApplied, "duration_minutes")
	}
	if changes.Goal != nil && *changes.Goal != workout.Goal {
		changesApplied = append(changesApplied, "goal")
	}
	if changes.WorkoutType != nil && *changes.WorkoutType != workout.WorkoutType {
		changesApplied = append(changesApplied, "workout_type")
	}
	if changes.ExperienceLevel != nil && *changes.ExperienceLevel != workout.Constraints.ExperienceLevel {
		changesApplied = append(changesApplied, "experience_level")
	}
	if changes.Intensity != nil {
		changesApplied = append(changesApplied, "intensity")
	}
	if changes.Environment != "" && changes.Environment != workout.Constraints.Environment {
		changesApplied = append(changesApplied, "environment")
	}
	if changes.Space != "" && changes.Space != workout.Constraints.Space {
		changesApplied = append(changesApplied, "space")
	}
	if changes.AvailableEquipment != nil {
		changesApplied = append(changesApplied, "available_equipment")
	}
	if changes.UnavailableEquipment != nil {
		changesApplied = append(changesApplied, "unavailable_equipment")
	}
	if changes.FocusMuscles != nil {
		changesApplied = append(changesApplied, "focus_muscles")
	}
	if changes.PreferredActivityIDs != nil {
		changesApplied = append(changesApplied, "preferred_activity_ids")
	}
	if changes.ExcludedExerciseIDs != nil {
		changesApplied = append(changesApplied, "excluded_exercise_ids")
	}
	if changes.GymID != nil {
		changesApplied = append(changesApplied, "gym_id")
	}
	if changes.EquipmentSource != nil {
		changesApplied = append(changesApplied, "equipment_source")
	}
	var result AdaptResult
	result.OriginalSummary.ID = workout.ID
	result.OriginalSummary.Status = workout.Status
	result.OriginalSummary.WorkoutType = workout.WorkoutType
	result.OriginalSummary.EstimatedDurationSeconds = workout.EstimatedDurationSeconds
	result.AdaptedWorkout = updated
	result.ChangesApplied = changesApplied
	result.ReplacedItems = replaced
	result.Warnings = updated.Warnings
	_ = preserved
	if err := ctx.Err(); err != nil {
		return AdaptResult{}, nil, err
	}
	return result, nil, nil
}

func addStrengthItem(sections *[]Section, order *int, exercise catalog.ExerciseSummary, level string, maxSeconds int) {
	sets, reps, rest := 3, 10, restSeconds(level)
	work := sets*reps*3 + (sets-1)*rest
	for work > maxSeconds && sets > 1 {
		sets--
		work = sets*reps*3 + (sets-1)*rest
	}
	if work > maxSeconds {
		reps = max(5, maxSeconds/3)
		work = sets*reps*3 + (sets-1)*rest
	}
	*order++
	item := Item{ItemID: fmt.Sprintf("wi_%03d", *order), ExerciseID: exercise.ID, Order: *order, Prescription: map[string]any{"sets": sets, "reps": reps, "rest_seconds": rest, "load_kg": nil}, EstimatedDurationSeconds: work}
	*sections = appendSectionItem(*sections, "main", item)
}

func appendSectionItem(sections []Section, sectionType string, item Item) []Section {
	for i := range sections {
		if sections[i].Type == sectionType {
			sections[i].Items = append(sections[i].Items, item)
			return sections
		}
	}
	return append(sections, Section{Type: sectionType, Items: []Item{item}})
}

func mergeSections(sections []Section) []Section {
	order := map[string]int{"warmup": 0, "main": 1, "cooldown": 2}
	sort.SliceStable(sections, func(i, j int) bool { return order[sections[i].Type] < order[sections[j].Type] })
	return sections
}

func fitSections(sections []Section, duration int) ([]Section, int) {
	var total int
	for si := range sections {
		for ii := range sections[si].Items {
			total += sections[si].Items[ii].EstimatedDurationSeconds
		}
	}
	if total <= duration {
		return sections, total
	}
	for si := range sections {
		for ii := range sections[si].Items {
			item := &sections[si].Items[ii]
			if item.EstimatedDurationSeconds < 1 {
				continue
			}
			reduced := item.EstimatedDurationSeconds - (total - duration)
			if reduced < 30 {
				reduced = 30
			}
			total += reduced - item.EstimatedDurationSeconds
			item.EstimatedDurationSeconds = reduced
			if seconds, ok := item.Prescription["duration_seconds"]; ok {
				_ = seconds
				item.Prescription["duration_seconds"] = reduced
			}
			if total <= duration {
				return sections, total
			}
		}
	}
	return sections, duration
}

func (s *Service) FindAlternatives(ctx context.Context, originalID string, request AlternativeRequest) (Alternatives, error) {
	original, err := s.catalog.GetExercise(ctx, originalID)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			return Alternatives{}, ErrNotFound
		}
		return Alternatives{}, err
	}
	if request.MaxResults < 1 || request.MaxResults > 10 || !oneOf(request.ExperienceLevel, "beginner", "intermediate", "advanced") || !oneOf(request.Environment, "home", "gym", "indoor", "outdoor") {
		return Alternatives{}, ErrInvalidRequest
	}
	items, err := s.catalog.AllExercises(ctx)
	if err != nil {
		return Alternatives{}, err
	}
	available := makeSet(request.AvailableEquipment)
	available["bodyweight"] = true
	for _, id := range request.UnavailableEquipment {
		delete(available, id)
	}
	excluded := makeSet(request.ExcludedExerciseIDs)
	excluded[originalID] = true
	candidates := make([]Alternative, 0)
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return Alternatives{}, err
		}
		if excluded[item.ID] || item.Category != original.Category || !levelAllowed(item.Difficulty, request.ExperienceLevel) || !spaceAllowed(item.MinimumSpace, request.Space) || !environmentAllowed(item.Environments, request.Environment) || !equipmentAllowed(item.RequiredEquipment, available) {
			continue
		}
		muscle := similarity(original.PrimaryMuscles, item.PrimaryMuscles)
		pattern := 0.0
		if original.MovementPattern == item.MovementPattern {
			pattern = 1
		} else if original.Category == "cardio" {
			pattern = 0.5
		}
		level := 1.0 - float64(abs(levelRank(original.Difficulty)-levelRank(item.Difficulty)))*0.5
		practical := 1.0
		if len(item.RequiredEquipment) > len(original.RequiredEquipment) {
			practical = 0.5
		}
		score := 0.35*muscle + 0.30*pattern + 0.20*level + 0.15*practical
		reason := "Compatible movement with similar target muscles and available equipment."
		tradeoffs := []string{}
		if muscle < 1 {
			tradeoffs = append(tradeoffs, "Targets a different muscle emphasis")
		}
		if pattern < 1 {
			tradeoffs = append(tradeoffs, "Uses a different movement pattern")
		}
		candidates = append(candidates, Alternative{ExerciseID: item.ID, Score: score, RequiredEquipment: item.RequiredEquipment, Reason: reason, Tradeoffs: tradeoffs})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if ctx.Err() != nil {
			return false
		}
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].ExerciseID < candidates[j].ExerciseID
		}
		return candidates[i].Score > candidates[j].Score
	})
	if err := ctx.Err(); err != nil {
		return Alternatives{}, err
	}
	if len(candidates) > request.MaxResults {
		candidates = candidates[:request.MaxResults]
	}
	return Alternatives{OriginalExerciseID: originalID, Alternatives: candidates}, nil
}

func sessionSeed(seed *int64) (int64, error) {
	if seed != nil {
		return *seed, nil
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(raw[:]) & 0x7fffffffffffffff), nil
}

func makeSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func equipmentAllowed(required []string, available map[string]bool) bool {
	for _, id := range required {
		if !available[id] {
			return false
		}
	}
	return true
}

func environmentAllowed(values []string, requested string) bool {
	for _, value := range values {
		if value == requested || requested == "indoor" && (value == "home" || value == "gym") {
			return true
		}
	}
	return false
}

func spaceAllowed(minimum, requested string) bool {
	if minimum == "unrestricted" {
		return true
	}
	if requested == "" {
		requested = "medium"
	}
	return spaceRank(minimum) <= spaceRank(requested)
}

func spaceRank(space string) int {
	switch space {
	case "small":
		return 1
	case "medium":
		return 2
	case "large":
		return 3
	default:
		return 4
	}
}

func levelAllowed(difficulty, experience string) bool {
	return levelRank(difficulty) <= levelRank(experience)
}

func levelRank(level string) int {
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

func restSeconds(level string) int {
	switch level {
	case "beginner":
		return 60
	case "advanced":
		return 90
	default:
		return 75
	}
}

func choosePool(preferred, fallback []catalog.ExerciseSummary) []catalog.ExerciseSummary {
	if len(preferred) > 0 {
		return preferred
	}
	return fallback
}

func hasAny(a, b []string) bool {
	set := makeSet(a)
	for _, value := range b {
		if set[value] {
			return true
		}
	}
	return false
}

func similarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	common := 0
	set := makeSet(a)
	for _, item := range b {
		if set[item] {
			common++
		}
	}
	return float64(common) / float64(max(len(a), len(b)))
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if item == value {
			return true
		}
	}
	return false
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = fmt.Sprintf
