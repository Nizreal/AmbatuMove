package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrNotFound = errors.New("catalog item not found")
var ErrInvalidFilter = errors.New("invalid catalog filter")

type Service struct {
	db    *sql.DB
	cache *referenceCache
}

func NewService(db *sql.DB) *Service { return &Service{db: db, cache: newReferenceCache()} }

func (s *Service) CacheStats() CacheStats       { return s.cache.stats() }
func (s *Service) SetCacheEnabled(enabled bool) { s.cache.setEnabled(enabled) }
func (s *Service) Warm(ctx context.Context) error {
	if _, err := s.ListExercises(ctx, Filters{Limit: 100}); err != nil {
		return err
	}
	if _, err := s.ListEquipment(ctx, ""); err != nil {
		return err
	}
	_, err := s.ListMuscles(ctx)
	return err
}

func (s *Service) ListEquipment(ctx context.Context, category string) ([]Equipment, error) {
	key := "equipment:" + category
	var cached []Equipment
	if s.cache.get(key, &cached) {
		return cached, nil
	}
	query := "SELECT id, label, category, active FROM equipment WHERE active=1"
	args := []any{}
	if category != "" {
		query += " AND category=?"
		args = append(args, category)
	}
	query += " ORDER BY category, label COLLATE NOCASE, id"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Equipment, 0)
	for rows.Next() {
		var item Equipment
		var active int
		if err := rows.Scan(&item.ID, &item.Label, &item.Category, &active); err != nil {
			return nil, err
		}
		item.Active = active == 1
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	s.cache.put(key, items)
	return items, nil
}

func (s *Service) ListMuscles(ctx context.Context) ([]map[string]string, error) {
	var cached []map[string]string
	if s.cache.get("muscles", &cached) {
		return cached, nil
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,label FROM muscle_groups ORDER BY label COLLATE NOCASE,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]string, 0)
	for rows.Next() {
		var id, label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, err
		}
		items = append(items, map[string]string{"id": id, "label": label})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	s.cache.put("muscles", items)
	return items, nil
}

func Categories() []map[string]string {
	return []map[string]string{
		{"id": "strength", "label": "Strength"},
		{"id": "cardio", "label": "Cardio"},
		{"id": "mobility", "label": "Mobility"},
	}
}

func (s *Service) ListExercises(ctx context.Context, filter Filters) (Page, error) {
	return s.listExercises(ctx, filter, true)
}

func (s *Service) listExercises(ctx context.Context, filter Filters, boundedPublicOffset bool) (Page, error) {
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	if filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 {
		return Page{}, ErrInvalidFilter
	}
	if (boundedPublicOffset && filter.Offset > 1000000) || len(filter.Query) > 120 || len(filter.Equipment) > 20 || len(filter.MuscleGroup) > 80 {
		return Page{}, ErrInvalidFilter
	}
	for _, id := range filter.Equipment {
		if len(id) > 80 {
			return Page{}, ErrInvalidFilter
		}
	}
	orderBy := map[string]string{"name": "e.name COLLATE NOCASE, e.id", "difficulty": "e.difficulty, e.id", "category": "e.category, e.name COLLATE NOCASE, e.id", "id": "e.id"}[filter.Sort]
	if orderBy == "" {
		if filter.Sort != "" {
			return Page{}, ErrInvalidFilter
		}
		orderBy = "e.name COLLATE NOCASE, e.id"
	}
	filter.Equipment = append([]string(nil), filter.Equipment...)
	sort.Strings(filter.Equipment)
	cacheKeyBytes, _ := json.Marshal(filter)
	cacheKey := "exercises:" + string(cacheKeyBytes)
	var cached Page
	if s.cache.get(cacheKey, &cached) {
		return cached, nil
	}
	if err := s.validateFilters(ctx, filter); err != nil {
		return Page{}, err
	}
	where, args := exerciseWhere(filter)
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM exercises e WHERE "+where, args...).Scan(&total); err != nil {
		return Page{}, err
	}
	query := "SELECT e.id,e.name,e.category,e.movement_pattern,e.difficulty,e.measurement_type,e.min_space FROM exercises e WHERE " + where + " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	listArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return Page{}, err
	}
	items := make([]ExerciseSummary, 0)
	for rows.Next() {
		var item ExerciseSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.Category, &item.MovementPattern, &item.Difficulty, &item.MeasurementType, &item.MinimumSpace); err != nil {
			_ = rows.Close()
			return Page{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Page{}, err
	}
	// Release the page cursor before enrichment queries. Keeping one cursor per
	// concurrent request can exhaust the bounded SQLite pool and deadlock the
	// per-item lookups below.
	if err := rows.Close(); err != nil {
		return Page{}, err
	}
	for i := range items {
		if items[i].PrimaryMuscles, err = s.muscles(ctx, items[i].ID, true); err != nil {
			return Page{}, err
		}
		if items[i].RequiredEquipment, err = s.equipmentIDs(ctx, items[i].ID); err != nil {
			return Page{}, err
		}
		if items[i].Environments, err = s.environments(ctx, items[i].ID); err != nil {
			return Page{}, err
		}
	}
	result := Page{Items: items, Limit: filter.Limit, Offset: filter.Offset, Total: total}
	s.cache.put(cacheKey, result)
	return result, nil
}

// AllExercises loads the complete active exercise catalog for internal planning.
// Public callers should use ListExercises and its bounded pagination instead.
func (s *Service) AllExercises(ctx context.Context) ([]ExerciseSummary, error) {
	const pageSize = 100
	items := make([]ExerciseSummary, 0)
	for offset := 0; ; offset += pageSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := s.listExercises(ctx, Filters{Limit: pageSize, Offset: offset}, false)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if len(items) >= page.Total || len(page.Items) == 0 {
			return items, nil
		}
	}
}

func (s *Service) GetExercise(ctx context.Context, id string) (Exercise, error) {
	var cached Exercise
	if s.cache.get("exercise:"+id, &cached) {
		return cached, nil
	}
	var result Exercise
	var instructions, cautions string
	var active int
	err := s.db.QueryRowContext(ctx, `SELECT id,name,description,category,movement_pattern,difficulty,measurement_type,min_space,instructions_json,cautions_json,active FROM exercises WHERE id=? AND active=1`, id).Scan(
		&result.ID, &result.Name, &result.Description, &result.Category, &result.MovementPattern, &result.Difficulty, &result.MeasurementType, &result.MinimumSpace, &instructions, &cautions, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return Exercise{}, ErrNotFound
	}
	if err != nil {
		return Exercise{}, err
	}
	result.Active = active == 1
	if err := json.Unmarshal([]byte(instructions), &result.Instructions); err != nil {
		return Exercise{}, fmt.Errorf("decode exercise instructions: %w", err)
	}
	if err := json.Unmarshal([]byte(cautions), &result.Cautions); err != nil {
		return Exercise{}, fmt.Errorf("decode exercise cautions: %w", err)
	}
	if result.PrimaryMuscles, err = s.muscles(ctx, id, true); err != nil {
		return Exercise{}, err
	}
	if result.SecondaryMuscles, err = s.muscles(ctx, id, false); err != nil {
		return Exercise{}, err
	}
	if result.RequiredEquipment, err = s.equipmentIDs(ctx, id); err != nil {
		return Exercise{}, err
	}
	if result.Environments, err = s.environments(ctx, id); err != nil {
		return Exercise{}, err
	}
	s.cache.put("exercise:"+id, result)
	return result, nil
}

func (s *Service) validateFilters(ctx context.Context, f Filters) error {
	if f.Category != "" && f.Category != "strength" && f.Category != "cardio" && f.Category != "mobility" {
		return ErrInvalidFilter
	}
	if f.Difficulty != "" && f.Difficulty != "beginner" && f.Difficulty != "intermediate" && f.Difficulty != "advanced" {
		return ErrInvalidFilter
	}
	if f.Environment != "" && !oneOf(f.Environment, "home", "gym", "indoor", "outdoor") {
		return ErrInvalidFilter
	}
	if f.MeasurementType != "" && !oneOf(f.MeasurementType, "repetitions", "duration", "duration_distance") {
		return ErrInvalidFilter
	}
	for _, id := range f.Equipment {
		var exists int
		if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM equipment WHERE id=? AND active=1", id).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidFilter
			}
			return err
		}
	}
	if f.MuscleGroup != "" {
		var exists int
		if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM muscle_groups WHERE id=?", f.MuscleGroup).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidFilter
			}
			return err
		}
	}
	return nil
}

func exerciseWhere(f Filters) (string, []any) {
	clauses := []string{"e.active=1"}
	args := make([]any, 0, 8+len(f.Equipment))
	if f.Query != "" {
		clauses = append(clauses, "lower(e.name) LIKE ?")
		args = append(args, "%"+strings.ToLower(f.Query)+"%")
	}
	for _, item := range []struct{ column, value string }{{"e.category", f.Category}, {"e.difficulty", f.Difficulty}, {"e.measurement_type", f.MeasurementType}} {
		if item.value != "" {
			clauses = append(clauses, item.column+"=?")
			args = append(args, item.value)
		}
	}
	if f.Environment != "" {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM exercise_environments ee WHERE ee.exercise_id=e.id AND ee.environment=?)")
		args = append(args, f.Environment)
	}
	if f.MuscleGroup != "" {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM exercise_muscles em WHERE em.exercise_id=e.id AND em.muscle_group_id=?)")
		args = append(args, f.MuscleGroup)
	}
	for _, equipment := range f.Equipment {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM exercise_equipment eq WHERE eq.exercise_id=e.id AND eq.equipment_id=?)")
		args = append(args, equipment)
	}
	return strings.Join(clauses, " AND "), args
}

func (s *Service) muscles(ctx context.Context, exerciseID string, primary bool) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT muscle_group_id FROM exercise_muscles WHERE exercise_id=? AND is_primary=? ORDER BY muscle_group_id", exerciseID, boolInt(primary))
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

func (s *Service) equipmentIDs(ctx context.Context, exerciseID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT equipment_id FROM exercise_equipment WHERE exercise_id=? AND required=1 ORDER BY equipment_id", exerciseID)
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

func (s *Service) environments(ctx context.Context, exerciseID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT environment FROM exercise_environments WHERE exercise_id=? ORDER BY environment", exerciseID)
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

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
