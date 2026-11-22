package admindata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ambatumove/core/internal/calories"
	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
)

type facilityExportRecord struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
}

func (s *Service) Export(ctx context.Context, kind Kind, writer io.Writer) error {
	if s.db == nil {
		return fmt.Errorf("reference-data database is unavailable")
	}
	if _, ok := ParseKind(string(kind)); !ok {
		return fmt.Errorf("unsupported reference-data type %q", kind)
	}
	var data any
	var err error
	switch kind {
	case Equipment:
		data, err = exportEquipment(ctx, s.db)
	case Facility:
		data, err = exportFacilities(ctx, s.db)
	case Exercise:
		data, err = exportExercises(ctx, s.db)
	case MET:
		data, err = exportMET(ctx, s.db)
	case Gym:
		data, err = exportGyms(ctx, s.db)
	}
	if err != nil {
		return fmt.Errorf("export %s data: %w", kind, err)
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(data)
}

func exportEquipment(ctx context.Context, db *sql.DB) ([]catalog.Equipment, error) {
	rows, err := db.QueryContext(ctx, "SELECT id,label,category,active FROM equipment ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]catalog.Equipment, 0)
	for rows.Next() {
		var item catalog.Equipment
		var active int
		if err := rows.Scan(&item.ID, &item.Label, &item.Category, &active); err != nil {
			return nil, err
		}
		item.Active = active != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func exportFacilities(ctx context.Context, db *sql.DB) ([]facilityExportRecord, error) {
	rows, err := db.QueryContext(ctx, "SELECT id,label,category FROM facilities ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]facilityExportRecord, 0)
	for rows.Next() {
		var item facilityExportRecord
		if err := rows.Scan(&item.ID, &item.Label, &item.Category); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func exportExercises(ctx context.Context, db *sql.DB) ([]catalog.Exercise, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,name,description,category,movement_pattern,difficulty,measurement_type,min_space,instructions_json,cautions_json,active FROM exercises ORDER BY id`)
	if err != nil {
		return nil, err
	}
	items := make([]catalog.Exercise, 0)
	for rows.Next() {
		var item catalog.Exercise
		var instructions, cautions string
		var active int
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Category, &item.MovementPattern, &item.Difficulty, &item.MeasurementType, &item.MinimumSpace, &instructions, &cautions, &active); err != nil {
			_ = rows.Close()
			return nil, err
		}
		item.Active = active != 0
		if err := json.Unmarshal([]byte(instructions), &item.Instructions); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(cautions), &item.Cautions); err != nil {
			_ = rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].PrimaryMuscles, err = stringColumn(ctx, db, "SELECT muscle_group_id FROM exercise_muscles WHERE exercise_id=? AND is_primary=1 ORDER BY muscle_group_id", items[i].ID); err != nil {
			return nil, err
		}
		if items[i].SecondaryMuscles, err = stringColumn(ctx, db, "SELECT muscle_group_id FROM exercise_muscles WHERE exercise_id=? AND is_primary=0 ORDER BY muscle_group_id", items[i].ID); err != nil {
			return nil, err
		}
		if items[i].RequiredEquipment, err = stringColumn(ctx, db, "SELECT equipment_id FROM exercise_equipment WHERE exercise_id=? AND required=1 ORDER BY equipment_id", items[i].ID); err != nil {
			return nil, err
		}
		if items[i].Environments, err = stringColumn(ctx, db, "SELECT environment FROM exercise_environments WHERE exercise_id=? ORDER BY environment", items[i].ID); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func exportMET(ctx context.Context, db *sql.DB) ([]calories.METProfile, error) {
	rows, err := db.QueryContext(ctx, `SELECT m.id,m.activity_id,e.name,m.intensity_code,m.modality,m.speed_min_kmh,m.speed_max_kmh,m.met_value,m.compendium_year,m.compendium_code,m.activity_description,m.source_url
		FROM activity_met_profiles m JOIN exercises e ON e.id=m.activity_id ORDER BY m.activity_id,m.intensity_code,m.modality,m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]calories.METProfile, 0)
	for rows.Next() {
		var item calories.METProfile
		var speedMin, speedMax sql.NullFloat64
		var year int
		var code, description, sourceURL string
		if err := rows.Scan(&item.ID, &item.ActivityID, &item.Activity, &item.Intensity, &item.Modality, &speedMin, &speedMax, &item.MET, &year, &code, &description, &sourceURL); err != nil {
			return nil, err
		}
		if speedMin.Valid {
			item.SpeedMinKmh = &speedMin.Float64
		}
		if speedMax.Valid {
			item.SpeedMaxKmh = &speedMax.Float64
		}
		item.Source = calories.Source{Title: "2024 Adult Compendium of Physical Activities", Year: year, ActivityCode: code, ActivityDescription: description, URL: sourceURL}
		items = append(items, item)
	}
	return items, rows.Err()
}

func exportGyms(ctx context.Context, db *sql.DB) ([]gymExportRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,name,address_json,latitude,longitude,timezone,contact_json,opening_hours_json,price_json,verification_status,source_type,source_reference,verified_at,last_updated_at,active
		FROM gyms WHERE source_type<>'synthetic' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	items := make([]gymExportRecord, 0)
	for rows.Next() {
		var record gymExportRecord
		var addressJSON, contactJSON string
		var openingJSON, priceJSON, sourceReference, verifiedAt sql.NullString
		var active int
		gym := &record.Gym
		if err := rows.Scan(&gym.ID, &gym.Name, &addressJSON, &gym.Location.Latitude, &gym.Location.Longitude, &gym.Timezone, &contactJSON, &openingJSON, &priceJSON, &gym.VerificationStatus, &gym.SourceType, &sourceReference, &verifiedAt, &gym.LastUpdatedAt, &active); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(addressJSON), &gym.Address); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(contactJSON), &gym.Contact); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if openingJSON.Valid {
			if err := json.Unmarshal([]byte(openingJSON.String), &gym.OpeningHours); err != nil {
				_ = rows.Close()
				return nil, err
			}
		}
		if priceJSON.Valid {
			if err := json.Unmarshal([]byte(priceJSON.String), &gym.PriceInfo); err != nil {
				_ = rows.Close()
				return nil, err
			}
		}
		if sourceReference.Valid {
			gym.SourceReference = &sourceReference.String
		}
		if verifiedAt.Valid {
			gym.VerifiedAt = &verifiedAt.String
		}
		gym.Summary = gyms.Summary{ID: gym.ID, Name: gym.Name, Address: gym.Address, Location: gym.Location, VerificationStatus: gym.VerificationStatus, SourceType: gym.SourceType}
		record.Active = active != 0
		gym.Facilities, err = stringColumn(ctx, db, "SELECT facility_id FROM gym_facilities WHERE gym_id=? ORDER BY facility_id", gym.ID)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		gym.Equipment, err = exportGymEquipment(ctx, db, gym.ID)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return items, rows.Close()
}

func exportGymEquipment(ctx context.Context, db *sql.DB, gymID string) ([]gyms.EquipmentItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT ge.equipment_id,e.label,ge.quantity,ge.status,ge.verified_at,ge.source_type FROM gym_equipment ge JOIN equipment e ON e.id=ge.equipment_id WHERE ge.gym_id=? ORDER BY ge.equipment_id`, gymID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]gyms.EquipmentItem, 0)
	for rows.Next() {
		var item gyms.EquipmentItem
		var quantity sql.NullInt64
		var verified sql.NullString
		if err := rows.Scan(&item.EquipmentID, &item.Label, &quantity, &item.Status, &verified, &item.SourceType); err != nil {
			return nil, err
		}
		if quantity.Valid {
			value := int(quantity.Int64)
			item.Quantity = &value
		}
		if verified.Valid {
			item.VerifiedAt = &verified.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func stringColumn(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
