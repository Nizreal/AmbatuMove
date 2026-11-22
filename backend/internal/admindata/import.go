package admindata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ambatumove/core/internal/calories"
	"github.com/ambatumove/core/internal/catalog"
)

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`)
var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidDataset, fmt.Sprintf(format, args...))
}

func checkID(id string) bool      { return identifierPattern.MatchString(id) }
func checkSlug(value string) bool { return slugPattern.MatchString(value) }
func nonBlank(value string) bool  { return strings.TrimSpace(value) != "" }

func uniqueIDs(kind Kind, ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for index, id := range ids {
		if !checkID(id) {
			return invalid("%s record %d has invalid identifier %q", kind, index+1, id)
		}
		if _, ok := seen[id]; ok {
			return invalid("%s dataset contains duplicate identifier %q", kind, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func exists(ctx context.Context, tx *sql.Tx, query, id string) (bool, error) {
	var found int
	err := tx.QueryRowContext(ctx, query, id).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func importEquipment(ctx context.Context, tx *sql.Tx, data []byte) (int, error) {
	records, err := decodeDataset[equipmentRecord](Equipment, data)
	if err != nil {
		return 0, err
	}
	ids := make([]string, len(records))
	items := make([]catalog.Equipment, len(records))
	for i, record := range records {
		item := record.Equipment
		ids[i] = item.ID
		if !nonBlank(item.Label) || len(item.Label) > 120 || !checkSlug(item.Category) {
			return 0, invalid("equipment record %d requires a label and category slug", i+1)
		}
		item.Label = strings.TrimSpace(item.Label)
		item.Active = record.Active == nil || *record.Active
		items[i] = item
	}
	if err := uniqueIDs(Equipment, ids); err != nil {
		return 0, err
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO equipment(id,label,category,active) VALUES(?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET label=excluded.label,category=excluded.category,active=excluded.active`, item.ID, item.Label, item.Category, boolInt(item.Active)); err != nil {
			return 0, fmt.Errorf("import equipment %s: %w", item.ID, err)
		}
	}
	return len(items), nil
}

func importFacilities(ctx context.Context, tx *sql.Tx, data []byte) (int, error) {
	records, err := decodeDataset[facilityRecord](Facility, data)
	if err != nil {
		return 0, err
	}
	ids := make([]string, len(records))
	for i, item := range records {
		ids[i] = item.ID
		if !nonBlank(item.Label) || len(item.Label) > 120 || !checkSlug(item.Category) {
			return 0, invalid("facility record %d requires a label and category slug", i+1)
		}
		records[i].Label = strings.TrimSpace(item.Label)
	}
	if err := uniqueIDs(Facility, ids); err != nil {
		return 0, err
	}
	for _, item := range records {
		if _, err := tx.ExecContext(ctx, `INSERT INTO facilities(id,label,category) VALUES(?,?,?)
			ON CONFLICT(id) DO UPDATE SET label=excluded.label,category=excluded.category`, item.ID, item.Label, item.Category); err != nil {
			return 0, fmt.Errorf("import facility %s: %w", item.ID, err)
		}
	}
	return len(records), nil
}

func importExercises(ctx context.Context, tx *sql.Tx, data []byte) (int, error) {
	records, err := decodeDataset[exerciseRecord](Exercise, data)
	if err != nil {
		return 0, err
	}
	ids := make([]string, len(records))
	items := make([]catalog.Exercise, len(records))
	for i, record := range records {
		item := record.Exercise
		ids[i] = item.ID
		if !nonBlank(item.Name) || len(item.Name) > 200 || !checkSlug(item.MovementPattern) || !nonBlank(item.Description) {
			return 0, invalid("exercise record %d requires name, description, and movement_pattern", i+1)
		}
		if item.Category != "strength" && item.Category != "cardio" && item.Category != "mobility" {
			return 0, invalid("exercise %q has unsupported category %q", item.ID, item.Category)
		}
		if item.Difficulty != "beginner" && item.Difficulty != "intermediate" && item.Difficulty != "advanced" {
			return 0, invalid("exercise %q has unsupported difficulty %q", item.ID, item.Difficulty)
		}
		if item.MeasurementType != "repetitions" && item.MeasurementType != "duration" && item.MeasurementType != "duration_distance" {
			return 0, invalid("exercise %q has unsupported measurement_type %q", item.ID, item.MeasurementType)
		}
		if item.MinimumSpace != "small" && item.MinimumSpace != "medium" && item.MinimumSpace != "large" && item.MinimumSpace != "unrestricted" {
			return 0, invalid("exercise %q has unsupported minimum_space %q", item.ID, item.MinimumSpace)
		}
		if item.PrimaryMuscles == nil || item.SecondaryMuscles == nil || item.RequiredEquipment == nil || item.Environments == nil || item.Instructions == nil || item.Cautions == nil {
			return 0, invalid("exercise %q must provide all list fields as arrays (use [] when empty)", item.ID)
		}
		if len(item.Instructions) > 50 || len(item.Cautions) > 50 || len(item.RequiredEquipment) > 50 || len(item.PrimaryMuscles)+len(item.SecondaryMuscles) > 50 || len(item.Environments) > 10 {
			return 0, invalid("exercise %q exceeds a list size limit", item.ID)
		}
		if record.Active == nil {
			item.Active = true
		} else {
			item.Active = *record.Active
		}
		items[i] = item
	}
	if err := uniqueIDs(Exercise, ids); err != nil {
		return 0, err
	}
	for _, item := range items {
		muscles := map[string]bool{}
		for _, id := range append(append([]string{}, item.PrimaryMuscles...), item.SecondaryMuscles...) {
			if !checkID(id) || muscles[id] {
				return 0, invalid("exercise %q has invalid or repeated muscle group %q", item.ID, id)
			}
			muscles[id] = true
			ok, err := exists(ctx, tx, "SELECT 1 FROM muscle_groups WHERE id=?", id)
			if err != nil {
				return 0, err
			}
			if !ok {
				return 0, invalid("exercise %q references unknown muscle group %q", item.ID, id)
			}
		}
		equipment := map[string]bool{}
		for _, id := range item.RequiredEquipment {
			if !checkID(id) || equipment[id] {
				return 0, invalid("exercise %q has invalid or repeated equipment %q", item.ID, id)
			}
			equipment[id] = true
			ok, err := exists(ctx, tx, "SELECT 1 FROM equipment WHERE id=? AND active=1", id)
			if err != nil {
				return 0, err
			}
			if !ok {
				return 0, invalid("exercise %q references unknown or inactive equipment %q", item.ID, id)
			}
		}
		environments := map[string]bool{}
		for _, environment := range item.Environments {
			if environment != "home" && environment != "gym" && environment != "indoor" && environment != "outdoor" || environments[environment] {
				return 0, invalid("exercise %q has unsupported or repeated environment %q", item.ID, environment)
			}
			environments[environment] = true
		}
		for _, text := range append(append([]string{}, item.Instructions...), item.Cautions...) {
			if !nonBlank(text) || len(text) > 1000 {
				return 0, invalid("exercise %q has an empty or oversized instruction/caution", item.ID)
			}
		}
	}
	for _, item := range items {
		instructions, _ := json.Marshal(item.Instructions)
		cautions, _ := json.Marshal(item.Cautions)
		if _, err := tx.ExecContext(ctx, `INSERT INTO exercises(id,name,description,category,movement_pattern,difficulty,measurement_type,min_space,instructions_json,cautions_json,active,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,category=excluded.category,movement_pattern=excluded.movement_pattern,difficulty=excluded.difficulty,measurement_type=excluded.measurement_type,min_space=excluded.min_space,instructions_json=excluded.instructions_json,cautions_json=excluded.cautions_json,active=excluded.active,updated_at=excluded.updated_at`, item.ID, item.Name, item.Description, item.Category, item.MovementPattern, item.Difficulty, item.MeasurementType, item.MinimumSpace, string(instructions), string(cautions), boolInt(item.Active), nowUTC()); err != nil {
			return 0, fmt.Errorf("import exercise %s: %w", item.ID, err)
		}
		for _, table := range []string{"exercise_equipment", "exercise_muscles", "exercise_environments"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE exercise_id=?", item.ID); err != nil {
				return 0, err
			}
		}
		for _, id := range item.RequiredEquipment {
			if _, err := tx.ExecContext(ctx, "INSERT INTO exercise_equipment(exercise_id,equipment_id,required) VALUES(?,?,1)", item.ID, id); err != nil {
				return 0, err
			}
		}
		for _, id := range item.PrimaryMuscles {
			if _, err := tx.ExecContext(ctx, "INSERT INTO exercise_muscles(exercise_id,muscle_group_id,is_primary) VALUES(?,?,1)", item.ID, id); err != nil {
				return 0, err
			}
		}
		for _, id := range item.SecondaryMuscles {
			if _, err := tx.ExecContext(ctx, "INSERT INTO exercise_muscles(exercise_id,muscle_group_id,is_primary) VALUES(?,?,0)", item.ID, id); err != nil {
				return 0, err
			}
		}
		for _, environment := range item.Environments {
			if _, err := tx.ExecContext(ctx, "INSERT INTO exercise_environments(exercise_id,environment) VALUES(?,?)", item.ID, environment); err != nil {
				return 0, err
			}
		}
	}
	return len(items), nil
}

func importMET(ctx context.Context, tx *sql.Tx, data []byte) (int, error) {
	records, err := decodeDataset[calories.METProfile](MET, data)
	if err != nil {
		return 0, err
	}
	ids := make([]string, len(records))
	mappings := make(map[string]string, len(records))
	for i, item := range records {
		ids[i] = item.ID
		if !checkID(item.ActivityID) || !nonBlank(item.Activity) || !oneOf(item.Intensity, "light", "easy", "moderate", "brisk", "vigorous") {
			return 0, invalid("MET record %d has invalid activity, intensity, or modality", i+1)
		}
		if !oneOf(item.Modality, "outdoor", "treadmill", "stationary", "strength", "bodyweight", "mobility") {
			return 0, invalid("MET record %d has unsupported modality %q", i+1, item.Modality)
		}
		if item.MET <= 0 || item.MET > 30 || math.IsNaN(item.MET) || math.IsInf(item.MET, 0) {
			return 0, invalid("MET profile %q value must be finite and in (0,30]", item.ID)
		}
		if item.Source.Year != 2024 || item.Source.Title != "2024 Adult Compendium of Physical Activities" || !nonBlank(item.Source.ActivityCode) || !nonBlank(item.Source.ActivityDescription) {
			return 0, invalid("MET profile %q has incomplete source attribution", item.ID)
		}
		parsedURL, err := url.ParseRequestURI(item.Source.URL)
		if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
			return 0, invalid("MET profile %q source URL must be an absolute HTTPS URL", item.ID)
		}
		for _, speed := range []*float64{item.SpeedMinKmh, item.SpeedMaxKmh} {
			if speed != nil && (*speed < 0 || *speed > 100 || math.IsNaN(*speed) || math.IsInf(*speed, 0)) {
				return 0, invalid("MET profile %q has an invalid speed boundary", item.ID)
			}
		}
		if item.SpeedMinKmh != nil && item.SpeedMaxKmh != nil && *item.SpeedMinKmh > *item.SpeedMaxKmh {
			return 0, invalid("MET profile %q has reversed speed boundaries", item.ID)
		}
		var activityName string
		err = tx.QueryRowContext(ctx, "SELECT name FROM exercises WHERE id=?", item.ActivityID).Scan(&activityName)
		if err == sql.ErrNoRows {
			return 0, invalid("MET profile %q references unknown exercise %q", item.ID, item.ActivityID)
		}
		if err != nil {
			return 0, err
		}
		if item.Activity != activityName {
			return 0, invalid("MET profile %q activity must match referenced exercise name %q", item.ID, activityName)
		}
		mapping := item.ActivityID + "\x00" + item.Intensity + "\x00" + item.Modality
		if previous, found := mappings[mapping]; found {
			return 0, invalid("MET profiles %q and %q repeat activity/intensity/modality mapping", previous, item.ID)
		}
		mappings[mapping] = item.ID
	}
	if err := uniqueIDs(MET, ids); err != nil {
		return 0, err
	}
	for _, item := range records {
		var existingID string
		err := tx.QueryRowContext(ctx, `SELECT id FROM activity_met_profiles WHERE activity_id=? AND intensity_code=? AND modality=? AND id<>?`, item.ActivityID, item.Intensity, item.Modality, item.ID).Scan(&existingID)
		if err == nil {
			return 0, invalid("MET profile %q conflicts with existing mapping %q", item.ID, existingID)
		}
		if err != sql.ErrNoRows {
			return 0, err
		}
	}
	for _, item := range records {
		if _, err := tx.ExecContext(ctx, `INSERT INTO activity_met_profiles(id,activity_id,intensity_code,modality,speed_min_kmh,speed_max_kmh,met_value,compendium_year,compendium_code,activity_description,source_url)
			VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET activity_id=excluded.activity_id,intensity_code=excluded.intensity_code,modality=excluded.modality,speed_min_kmh=excluded.speed_min_kmh,speed_max_kmh=excluded.speed_max_kmh,met_value=excluded.met_value,compendium_year=excluded.compendium_year,compendium_code=excluded.compendium_code,activity_description=excluded.activity_description,source_url=excluded.source_url`, item.ID, item.ActivityID, item.Intensity, item.Modality, item.SpeedMinKmh, item.SpeedMaxKmh, item.MET, item.Source.Year, item.Source.ActivityCode, item.Source.ActivityDescription, item.Source.URL); err != nil {
			return 0, fmt.Errorf("import MET profile %s: %w", item.ID, err)
		}
	}
	return len(records), nil
}

func importGyms(ctx context.Context, tx *sql.Tx, data []byte) (int, error) {
	records, err := decodeDataset[gymRecord](Gym, data)
	if err != nil {
		return 0, err
	}
	ids := make([]string, len(records))
	for index := range records {
		record := &records[index]
		gym := &record.Gym
		ids[index] = gym.ID
		if !nonBlank(gym.Name) || len(gym.Name) > 200 || !nonBlank(gym.Address.Street) || !nonBlank(gym.Address.City) || !nonBlank(gym.Address.Region) || !nonBlank(gym.Address.Country) {
			return 0, invalid("gym record %d requires name and complete address", index+1)
		}
		if math.IsNaN(gym.Location.Latitude) || math.IsInf(gym.Location.Latitude, 0) || gym.Location.Latitude < -90 || gym.Location.Latitude > 90 || math.IsNaN(gym.Location.Longitude) || math.IsInf(gym.Location.Longitude, 0) || gym.Location.Longitude < -180 || gym.Location.Longitude > 180 {
			return 0, invalid("gym %q has invalid coordinates", gym.ID)
		}
		if !nonBlank(gym.Timezone) {
			return 0, invalid("gym %q requires timezone", gym.ID)
		}
		if _, err := time.LoadLocation(gym.Timezone); err != nil {
			return 0, invalid("gym %q has unknown timezone %q", gym.ID, gym.Timezone)
		}
		if gym.VerificationStatus != "verified" && gym.VerificationStatus != "unverified" && gym.VerificationStatus != "stale" {
			return 0, invalid("gym %q has unsupported verification_status %q", gym.ID, gym.VerificationStatus)
		}
		if gym.SourceType != "survey" && gym.SourceType != "user_submission" && gym.SourceType != "official" && gym.SourceType != "admin" {
			return 0, invalid("gym %q has unsupported or non-public source_type %q", gym.ID, gym.SourceType)
		}
		if !nonBlank(pointerString(gym.SourceReference)) {
			return 0, invalid("gym %q requires source_reference", gym.ID)
		}
		if gym.VerificationStatus == "verified" && !validTimestamp(pointerString(gym.VerifiedAt)) {
			return 0, invalid("verified gym %q requires an RFC3339 verified_at timestamp", gym.ID)
		}
		if value := pointerString(gym.VerifiedAt); value != "" && !validTimestamp(value) {
			return 0, invalid("gym %q verified_at must be RFC3339", gym.ID)
		}
		if gym.LastUpdatedAt == "" {
			gym.LastUpdatedAt = nowUTC()
		} else if !validTimestamp(gym.LastUpdatedAt) {
			return 0, invalid("gym %q last_updated_at must be RFC3339", gym.ID)
		}
		if record.Active == nil {
			gymActive := true
			record.Active = &gymActive
		}
		if gym.Contact == nil {
			gym.Contact = map[string]any{}
		}
		if gym.OpeningHours == nil {
			gym.OpeningHours = map[string]any{}
		}
		if gym.PriceInfo == nil {
			gym.PriceInfo = map[string]any{}
		}
		if gym.Facilities == nil || gym.Equipment == nil {
			return 0, invalid("gym %q must provide facilities and equipment arrays (use [] when empty)", gym.ID)
		}
		facilitySeen := map[string]bool{}
		for _, id := range gym.Facilities {
			if !checkID(id) || facilitySeen[id] {
				return 0, invalid("gym %q has invalid or repeated facility %q", gym.ID, id)
			}
			facilitySeen[id] = true
			ok, err := exists(ctx, tx, "SELECT 1 FROM facilities WHERE id=?", id)
			if err != nil {
				return 0, err
			}
			if !ok {
				return 0, invalid("gym %q references unknown facility %q", gym.ID, id)
			}
		}
		equipmentSeen := map[string]bool{}
		for i := range gym.Equipment {
			item := &gym.Equipment[i]
			if !checkID(item.EquipmentID) || equipmentSeen[item.EquipmentID] || !nonBlank(item.Label) || len(item.Label) > 120 {
				return 0, invalid("gym %q has invalid or repeated equipment at position %d", gym.ID, i+1)
			}
			equipmentSeen[item.EquipmentID] = true
			if item.Quantity != nil && *item.Quantity < 0 {
				return 0, invalid("gym %q equipment %q has negative quantity", gym.ID, item.EquipmentID)
			}
			if item.Status != "present" && item.Status != "out_of_service" && item.Status != "unknown" {
				return 0, invalid("gym %q equipment %q has unsupported status", gym.ID, item.EquipmentID)
			}
			var equipmentLabel string
			err := tx.QueryRowContext(ctx, "SELECT label FROM equipment WHERE id=? AND active=1", item.EquipmentID).Scan(&equipmentLabel)
			if err == sql.ErrNoRows {
				return 0, invalid("gym %q references unknown or inactive equipment %q", gym.ID, item.EquipmentID)
			}
			if err != nil {
				return 0, err
			}
			if item.Label != equipmentLabel {
				return 0, invalid("gym %q equipment %q label must match canonical label %q", gym.ID, item.EquipmentID, equipmentLabel)
			}
			if item.SourceType == "" {
				item.SourceType = gym.SourceType
			}
			if !validSourceType(item.SourceType) {
				return 0, invalid("gym %q equipment %q has unsupported source_type %q", gym.ID, item.EquipmentID, item.SourceType)
			}
			if value := pointerString(item.VerifiedAt); value != "" && !validTimestamp(value) {
				return 0, invalid("gym %q equipment %q verified_at must be RFC3339", gym.ID, item.EquipmentID)
			}
		}
	}
	if err := uniqueIDs(Gym, ids); err != nil {
		return 0, err
	}
	for _, record := range records {
		gym := record.Gym
		address, _ := json.Marshal(gym.Address)
		contact, _ := json.Marshal(gym.Contact)
		opening, _ := json.Marshal(gym.OpeningHours)
		price, _ := json.Marshal(gym.PriceInfo)
		if _, err := tx.ExecContext(ctx, `INSERT INTO gyms(id,name,address_json,latitude,longitude,timezone,contact_json,opening_hours_json,price_json,verification_status,source_type,source_reference,verified_at,last_updated_at,active)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,address_json=excluded.address_json,latitude=excluded.latitude,longitude=excluded.longitude,timezone=excluded.timezone,contact_json=excluded.contact_json,opening_hours_json=excluded.opening_hours_json,price_json=excluded.price_json,verification_status=excluded.verification_status,source_type=excluded.source_type,source_reference=excluded.source_reference,verified_at=excluded.verified_at,last_updated_at=excluded.last_updated_at,active=excluded.active`, gym.ID, gym.Name, string(address), gym.Location.Latitude, gym.Location.Longitude, gym.Timezone, string(contact), string(opening), string(price), gym.VerificationStatus, gym.SourceType, gym.SourceReference, gym.VerifiedAt, gym.LastUpdatedAt, boolInt(*record.Active)); err != nil {
			return 0, fmt.Errorf("import gym %s: %w", gym.ID, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM gym_equipment WHERE gym_id=?", gym.ID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM gym_facilities WHERE gym_id=?", gym.ID); err != nil {
			return 0, err
		}
		for _, item := range gym.Equipment {
			if _, err := tx.ExecContext(ctx, "INSERT INTO gym_equipment(gym_id,equipment_id,quantity,status,verified_at,source_type) VALUES(?,?,?,?,?,?)", gym.ID, item.EquipmentID, item.Quantity, item.Status, item.VerifiedAt, item.SourceType); err != nil {
				return 0, err
			}
		}
		for _, id := range gym.Facilities {
			if _, err := tx.ExecContext(ctx, "INSERT INTO gym_facilities(gym_id,facility_id,verified_at,source_type) VALUES(?,?,?,?)", gym.ID, id, gym.VerifiedAt, gym.SourceType); err != nil {
				return 0, err
			}
		}
	}
	return len(records), nil
}

func validSourceType(value string) bool {
	return value == "survey" || value == "user_submission" || value == "official" || value == "admin"
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func validTimestamp(value string) bool {
	if value == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
