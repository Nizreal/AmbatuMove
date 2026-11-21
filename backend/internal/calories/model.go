package calories

type Source struct {
	Title               string `json:"title"`
	Year                int    `json:"year"`
	ActivityCode        string `json:"activity_code"`
	ActivityDescription string `json:"activity_description"`
	URL                 string `json:"url"`
}

type METProfile struct {
	ID          string   `json:"id"`
	ActivityID  string   `json:"activity_id"`
	Activity    string   `json:"activity"`
	Intensity   string   `json:"intensity"`
	Modality    string   `json:"modality"`
	SpeedMinKmh *float64 `json:"speed_min_kmh,omitempty"`
	SpeedMaxKmh *float64 `json:"speed_max_kmh,omitempty"`
	MET         float64  `json:"met"`
	Source      Source   `json:"source"`
}

type Estimate struct {
	Activity        string   `json:"activity"`
	ActivityID      string   `json:"activity_id"`
	Intensity       string   `json:"intensity"`
	DurationMinutes float64  `json:"duration_minutes"`
	WeightKg        float64  `json:"weight_kg"`
	MET             float64  `json:"met"`
	EstimatedKcal   float64  `json:"estimated_kcal"`
	EstimateType    string   `json:"estimate_type"`
	Method          string   `json:"method"`
	Source          Source   `json:"source"`
	Limitations     []string `json:"limitations"`
}

type EstimateRequest struct {
	ActivityID       string
	Activity         string
	Intensity        string
	DurationMinutes  float64
	WeightKg         float64
	SpeedKmh         *float64
	PaceSecondsPerKm *float64
	EstimateType     string
}

type RecommendationRequest struct {
	TargetKcal            float64
	WeightKg              float64
	MaxDurationMinutes    int
	AvailableEquipment    []string
	AvailableEquipmentSet bool
	ExcludedEquipment     []string
	GymID                 string
	Environment           string
	ExperienceLevel       string
	Category              string
	MaxIntensity          string
	ExcludedExerciseIDs   []string
	Limit                 int
}

type Recommendation struct {
	ExerciseID                 string   `json:"exercise_id"`
	Name                       string   `json:"name"`
	Category                   string   `json:"category"`
	Intensity                  string   `json:"intensity"`
	MET                        float64  `json:"met"`
	RequiredDurationMinutes    int      `json:"required_duration_minutes"`
	RecommendedDurationMinutes int      `json:"recommended_duration_minutes"`
	EstimatedKcal              float64  `json:"estimated_kcal"`
	MeetsTarget                bool     `json:"meets_target"`
	FitsConstraints            bool     `json:"fits_constraints"`
	RequiredEquipment          []string `json:"required_equipment"`
	Reason                     string   `json:"reason"`
}

type RecommendationResult struct {
	TargetKcal         float64          `json:"target_kcal"`
	WeightKg           float64          `json:"weight_kg"`
	MaxDurationMinutes int              `json:"max_duration_minutes"`
	TargetAchievable   bool             `json:"target_achievable"`
	Reason             string           `json:"reason,omitempty"`
	Recommendations    []Recommendation `json:"recommendations"`
	Method             string           `json:"method"`
	EstimateType       string           `json:"estimate_type"`
	Disclaimer         string           `json:"disclaimer"`
}
