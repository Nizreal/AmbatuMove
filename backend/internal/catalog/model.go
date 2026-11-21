package catalog

type Equipment struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
	Active   bool   `json:"active"`
}

type ExerciseSummary struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	MovementPattern   string   `json:"movement_pattern"`
	PrimaryMuscles    []string `json:"primary_muscles"`
	RequiredEquipment []string `json:"required_equipment"`
	Difficulty        string   `json:"difficulty"`
	MeasurementType   string   `json:"measurement_type"`
	Environments      []string `json:"environments"`
	MinimumSpace      string   `json:"minimum_space"`
}

type Exercise struct {
	ExerciseSummary
	Description      string   `json:"description"`
	SecondaryMuscles []string `json:"secondary_muscles"`
	Instructions     []string `json:"instructions"`
	Cautions         []string `json:"cautions"`
	Active           bool     `json:"active"`
}

type Filters struct {
	Query           string
	Category        string
	MuscleGroup     string
	Equipment       []string
	Difficulty      string
	Environment     string
	MeasurementType string
	Sort            string
	Limit           int
	Offset          int
}

type Page struct {
	Items  []ExerciseSummary `json:"items"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
	Total  int               `json:"total"`
}
