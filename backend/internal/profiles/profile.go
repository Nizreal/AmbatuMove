package profiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid fitness profile")

type Profile struct {
	Goal                 string    `json:"goal"`
	ExperienceLevel      string    `json:"experience_level"`
	PreferredActivityIDs []string  `json:"preferred_activity_ids"`
	Equipment            []string  `json:"equipment"`
	Environment          string    `json:"environment"`
	Space                string    `json:"space"`
	WeightKg             *float64  `json:"weight_kg"`
	ExcludedIDs          []string  `json:"excluded_ids"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) Get(ctx context.Context, userID string) (Profile, error) {
	var p Profile
	var preferred, equipment, excluded, updated string
	var weight sql.NullFloat64
	err := s.db.QueryRowContext(ctx, "SELECT goal,experience_level,preferred_activity_ids_json,equipment_json,environment,space,weight_kg,excluded_ids_json,updated_at FROM fitness_profiles WHERE user_id=?", userID).Scan(&p.Goal, &p.ExperienceLevel, &preferred, &equipment, &p.Environment, &p.Space, &weight, &excluded, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{Goal: "general_fitness", ExperienceLevel: "beginner", PreferredActivityIDs: []string{}, Equipment: []string{}, Environment: "home", Space: "medium", ExcludedIDs: []string{}}, nil
	}
	if err != nil {
		return Profile{}, err
	}
	if err = json.Unmarshal([]byte(preferred), &p.PreferredActivityIDs); err != nil {
		return Profile{}, err
	}
	if err = json.Unmarshal([]byte(equipment), &p.Equipment); err != nil {
		return Profile{}, err
	}
	if err = json.Unmarshal([]byte(excluded), &p.ExcludedIDs); err != nil {
		return Profile{}, err
	}
	if weight.Valid {
		p.WeightKg = &weight.Float64
	}
	p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return p, err
}

func (s *Service) Put(ctx context.Context, userID string, p Profile) error {
	if p.Goal == "" {
		p.Goal = "general_fitness"
	}
	if p.ExperienceLevel == "" {
		p.ExperienceLevel = "beginner"
	}
	if p.Environment == "" {
		p.Environment = "home"
	}
	if p.Space == "" {
		p.Space = "medium"
	}
	if !oneOf(p.Goal, "general_fitness", "strength", "hypertrophy", "endurance", "mobility", "active_recovery") || !oneOf(p.ExperienceLevel, "beginner", "intermediate", "advanced") || !oneOf(p.Environment, "home", "gym", "indoor", "outdoor") || !oneOf(p.Space, "small", "medium", "large", "unrestricted") || (p.WeightKg != nil && (math.IsNaN(*p.WeightKg) || math.IsInf(*p.WeightKg, 0) || *p.WeightKg < 20 || *p.WeightKg > 300)) || !validIDs(p.PreferredActivityIDs) || !validIDs(p.Equipment) || !validIDs(p.ExcludedIDs) {
		return ErrInvalid
	}
	preferred, _ := json.Marshal(p.PreferredActivityIDs)
	equipment, _ := json.Marshal(p.Equipment)
	excluded, _ := json.Marshal(p.ExcludedIDs)
	var weight any
	if p.WeightKg != nil {
		weight = *p.WeightKg
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO fitness_profiles(user_id,goal,experience_level,preferred_activity_ids_json,equipment_json,environment,space,weight_kg,excluded_ids_json,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET goal=excluded.goal,experience_level=excluded.experience_level,preferred_activity_ids_json=excluded.preferred_activity_ids_json,equipment_json=excluded.equipment_json,environment=excluded.environment,space=excluded.space,weight_kg=excluded.weight_kg,excluded_ids_json=excluded.excluded_ids_json,updated_at=excluded.updated_at`, userID, p.Goal, p.ExperienceLevel, string(preferred), string(equipment), p.Environment, p.Space, weight, string(excluded), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func oneOf(v string, allowed ...string) bool {
	for _, x := range allowed {
		if v == x {
			return true
		}
	}
	return false
}
func validIDs(ids []string) bool {
	if len(ids) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if len(id) == 0 || len(id) > 80 || strings.TrimSpace(id) != id || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
