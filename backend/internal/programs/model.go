package programs

import "github.com/ambatumove/core/internal/planning"

type GenerateRequest struct {
	Goal                 string   `json:"goal"`
	WorkoutType          string   `json:"workout_type,omitempty"`
	ExperienceLevel      string   `json:"experience_level"`
	Weeks                int      `json:"weeks"`
	DaysPerWeek          int      `json:"days_per_week"`
	StartDate            string   `json:"start_date"`
	Timezone             string   `json:"timezone"`
	AvailableDays        []string `json:"available_days"`
	SessionDuration      int      `json:"session_duration_minutes"`
	Environment          string   `json:"environment"`
	Space                string   `json:"space,omitempty"`
	AvailableEquipment   []string `json:"available_equipment,omitempty"`
	UnavailableEquipment []string `json:"unavailable_equipment,omitempty"`
	PreferredActivityIDs []string `json:"preferred_activity_ids,omitempty"`
	GymID                string   `json:"gym_id,omitempty"`
	ExcludedExerciseIDs  []string `json:"excluded_exercise_ids,omitempty"`
	Save                 bool     `json:"save,omitempty"`
	Seed                 *int64   `json:"seed,omitempty"`
}

type Program struct {
	ID         *string         `json:"id"`
	Saved      bool            `json:"saved"`
	Version    int             `json:"version"`
	Goal       string          `json:"goal"`
	Weeks      int             `json:"weeks"`
	Timezone   string          `json:"timezone"`
	StartDate  string          `json:"start_date"`
	Schedule   []ScheduleEntry `json:"schedule"`
	Generation Generation      `json:"generation"`
	Warnings   []string        `json:"warnings"`
}

type ScheduleEntry struct {
	Date    string            `json:"date"`
	Week    int               `json:"week"`
	Day     string            `json:"day"`
	Kind    string            `json:"kind"`
	Focus   string            `json:"focus,omitempty"`
	Session *planning.Session `json:"session"`
}

type Generation struct {
	AlgorithmVersion string `json:"algorithm_version"`
	Seed             int64  `json:"seed"`
}
