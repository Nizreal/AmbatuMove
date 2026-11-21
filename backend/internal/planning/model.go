package planning

type GenerateRequest struct {
	Goal                 string   `json:"goal"`
	WorkoutType          string   `json:"workout_type"`
	DurationMinutes      int      `json:"duration_minutes"`
	ExperienceLevel      string   `json:"experience_level"`
	Environment          string   `json:"environment"`
	Space                string   `json:"space,omitempty"`
	AvailableEquipment   []string `json:"available_equipment,omitempty"`
	UnavailableEquipment []string `json:"unavailable_equipment,omitempty"`
	ExcludedExerciseIDs  []string `json:"excluded_exercise_ids,omitempty"`
	FocusMuscles         []string `json:"focus_muscles,omitempty"`
	GymID                string   `json:"gym_id,omitempty"`
	EquipmentSource      string   `json:"equipment_source,omitempty"`
	PreferredActivityIDs []string `json:"preferred_activity_ids,omitempty"`
	Intensity            string   `json:"intensity,omitempty"`
	Save                 bool     `json:"save,omitempty"`
	Seed                 *int64   `json:"seed,omitempty"`
}

type Constraints struct {
	DurationMinutes    int      `json:"duration_minutes"`
	Environment        string   `json:"environment"`
	AvailableEquipment []string `json:"available_equipment"`
	Space              string   `json:"space,omitempty"`
}

type Session struct {
	ID                       *string                `json:"id"`
	Saved                    bool                   `json:"saved"`
	Version                  int                    `json:"version"`
	Goal                     string                 `json:"goal"`
	WorkoutType              string                 `json:"workout_type"`
	Status                   string                 `json:"status"`
	Constraints              SessionConstraints     `json:"constraints"`
	PlannedDurationSeconds   int                    `json:"planned_duration_seconds"`
	EstimatedDurationSeconds int                    `json:"estimated_duration_seconds"`
	Sections                 []Section              `json:"sections"`
	Warnings                 []string               `json:"warnings"`
	Generation               Generation             `json:"generation"`
	GenerationPreferences    *GenerationPreferences `json:"generation_preferences,omitempty"`
}

// GenerationPreferences stores the original request inputs needed to adapt a
// session without losing its training intent. It is optional for compatibility
// with snapshots saved before these preferences were recorded.
type GenerationPreferences struct {
	FocusMuscles          []string `json:"focus_muscles,omitempty"`
	PreferredActivityIDs  []string `json:"preferred_activity_ids,omitempty"`
	ExcludedExerciseIDs   []string `json:"excluded_exercise_ids,omitempty"`
	AvailableEquipment    []string `json:"available_equipment,omitempty"`
	AvailableEquipmentSet bool     `json:"available_equipment_set"`
	EquipmentSource       string   `json:"equipment_source,omitempty"`
	GymID                 string   `json:"gym_id,omitempty"`
	Intensity             string   `json:"intensity,omitempty"`
}

type SessionConstraints struct {
	DurationMinutes      int      `json:"duration_minutes"`
	Environment          string   `json:"environment"`
	AvailableEquipment   []string `json:"available_equipment"`
	UnavailableEquipment []string `json:"unavailable_equipment,omitempty"`
	ExperienceLevel      string   `json:"experience_level"`
	Space                string   `json:"space"`
}

type ConstraintChanges struct {
	DurationMinutes      *int     `json:"duration_minutes,omitempty"`
	Goal                 *string  `json:"goal,omitempty"`
	WorkoutType          *string  `json:"workout_type,omitempty"`
	ExperienceLevel      *string  `json:"experience_level,omitempty"`
	Intensity            *string  `json:"intensity,omitempty"`
	Environment          string   `json:"environment,omitempty"`
	Space                string   `json:"space,omitempty"`
	AvailableEquipment   []string `json:"available_equipment,omitempty"`
	UnavailableEquipment []string `json:"unavailable_equipment,omitempty"`
	FocusMuscles         []string `json:"focus_muscles,omitempty"`
	PreferredActivityIDs []string `json:"preferred_activity_ids,omitempty"`
	ExcludedExerciseIDs  []string `json:"excluded_exercise_ids,omitempty"`
	GymID                *string  `json:"gym_id,omitempty"`
	EquipmentSource      *string  `json:"equipment_source,omitempty"`
}

type AdaptResult struct {
	OriginalSummary struct {
		ID                       *string `json:"id"`
		Status                   string  `json:"status"`
		WorkoutType              string  `json:"workout_type"`
		EstimatedDurationSeconds int     `json:"estimated_duration_seconds"`
	} `json:"original_summary"`
	AdaptedWorkout Session  `json:"adapted_workout"`
	ChangesApplied []string `json:"changes_applied"`
	ReplacedItems  []string `json:"replaced_items"`
	Warnings       []string `json:"warnings"`
}

type Section struct {
	Type  string `json:"type"`
	Items []Item `json:"items"`
}

type Item struct {
	ItemID                   string         `json:"item_id"`
	ExerciseID               string         `json:"exercise_id"`
	Order                    int            `json:"order"`
	Prescription             map[string]any `json:"prescription"`
	EstimatedDurationSeconds int            `json:"estimated_duration_seconds"`
}

type Generation struct {
	AlgorithmVersion string `json:"algorithm_version"`
	Seed             int64  `json:"seed"`
}

type Failure struct {
	Constraints []string `json:"failed_constraints"`
	Suggestions []string `json:"suggestions"`
}

type AlternativeRequest struct {
	AvailableEquipment   []string `json:"available_equipment"`
	UnavailableEquipment []string `json:"unavailable_equipment,omitempty"`
	ExperienceLevel      string   `json:"experience_level"`
	Environment          string   `json:"environment"`
	Space                string   `json:"space,omitempty"`
	Goal                 string   `json:"goal"`
	MaxResults           int      `json:"max_results"`
	ExcludedExerciseIDs  []string `json:"excluded_exercise_ids,omitempty"`
}

type Alternative struct {
	ExerciseID        string   `json:"exercise_id"`
	Score             float64  `json:"score"`
	RequiredEquipment []string `json:"required_equipment"`
	Reason            string   `json:"reason"`
	Tradeoffs         []string `json:"tradeoffs"`
}

type Alternatives struct {
	OriginalExerciseID string        `json:"original_exercise_id"`
	Alternatives       []Alternative `json:"alternatives"`
}
