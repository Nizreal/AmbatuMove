package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"
)

var ErrHistoryLimit = errors.New("history exceeds bounded analytics window")

type TrendWeek struct {
	WeekStart             string  `json:"week_start"`
	CompletedSessions     int     `json:"completed_sessions"`
	ManualActivities      int     `json:"manual_activities"`
	TrainingMinutes       int     `json:"training_minutes"`
	StrengthVolumeKg      float64 `json:"strength_volume_kg"`
	CardioDistanceKm      float64 `json:"cardio_distance_km"`
	CardioDurationMinutes int     `json:"cardio_duration_minutes"`
}
type Trends struct {
	From     string      `json:"from"`
	To       string      `json:"to"`
	Timezone string      `json:"timezone"`
	Weeks    []TrendWeek `json:"weeks"`
}
type ProgressionItem struct {
	ExerciseID    string   `json:"exercise_id"`
	Metric        string   `json:"metric"`
	Status        string   `json:"status"`
	Observations  int      `json:"observations"`
	Baseline      float64  `json:"baseline"`
	Current       float64  `json:"current"`
	ChangePercent *float64 `json:"change_percent,omitempty"`
	Suggestion    string   `json:"suggestion"`
	ReasonCode    string   `json:"reason_code"`
	Explanation   string   `json:"explanation"`
}
type Progression struct {
	LookbackDays int               `json:"lookback_days"`
	Timezone     string            `json:"timezone"`
	Items        []ProgressionItem `json:"items"`
}

type analyticsLog struct{ status, source, start, finish, items string }
type exerciseMeasure struct{ category, measurement string }
type exerciseObservation struct {
	date   time.Time
	value  float64
	effort int
}

func (s *Service) Trends(ctx context.Context, userID string, days int, timezone string) (Trends, error) {
	if days < 7 || days > 365 {
		return Trends{}, ErrInvalid
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Trends{}, ErrInvalid
	}
	end := time.Now().In(loc)
	start := end.AddDate(0, 0, -days)
	logs, err := s.analyticsLogs(ctx, userID, start, end)
	if err != nil {
		return Trends{}, err
	}
	measures, err := s.exerciseMeasures(ctx)
	if err != nil {
		return Trends{}, err
	}
	weeks := map[string]*TrendWeek{}
	for _, entry := range logs {
		if entry.status != "completed" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, entry.start)
		if err != nil {
			continue
		}
		local := at.In(loc)
		weekStart := localDateWeek(local)
		w := weeks[weekStart]
		if w == nil {
			w = &TrendWeek{WeekStart: weekStart}
			weeks[weekStart] = w
		}
		if entry.source == "manual" {
			w.ManualActivities++
		} else {
			w.CompletedSessions++
		}
		if finish, err := time.Parse(time.RFC3339Nano, entry.finish); err == nil && finish.After(at) {
			w.TrainingMinutes += int(finish.Sub(at).Minutes())
		}
		var items []PerformedItem
		if json.Unmarshal([]byte(entry.items), &items) != nil {
			continue
		}
		for _, item := range items {
			m, ok := measures[item.ExerciseID]
			if !ok {
				continue
			}
			if m.category == "cardio" {
				w.CardioDistanceKm += item.ActualDistanceKm
				w.CardioDurationMinutes += item.ActualDurationSeconds / 60
			} else if m.category == "strength" {
				for _, set := range item.PerformedSets {
					w.StrengthVolumeKg += float64(set.Reps) * set.LoadKg
				}
			}
		}
	}
	keys := make([]string, 0, len(weeks))
	for key := range weeks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := Trends{From: start.Format("2006-01-02"), To: end.Format("2006-01-02"), Timezone: timezone, Weeks: make([]TrendWeek, 0, len(keys))}
	for _, key := range keys {
		out.Weeks = append(out.Weeks, *weeks[key])
	}
	return out, nil
}

func (s *Service) Progression(ctx context.Context, userID string, days int, exerciseID, timezone string, limit int) (Progression, error) {
	if days < 28 || days > 365 || limit < 1 || limit > 50 {
		return Progression{}, ErrInvalid
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Progression{}, ErrInvalid
	}
	end := time.Now().In(loc)
	start := end.AddDate(0, 0, -days)
	logs, err := s.analyticsLogs(ctx, userID, start, end)
	if err != nil {
		return Progression{}, err
	}
	measures, err := s.exerciseMeasures(ctx)
	if err != nil {
		return Progression{}, err
	}
	obs := map[string][]exerciseObservation{}
	for _, entry := range logs {
		if entry.status != "completed" {
			continue
		}
		at, e := time.Parse(time.RFC3339Nano, entry.start)
		if e != nil {
			continue
		}
		var items []PerformedItem
		if json.Unmarshal([]byte(entry.items), &items) != nil {
			continue
		}
		for _, item := range items {
			m, ok := measures[item.ExerciseID]
			if !ok || (exerciseID != "" && item.ExerciseID != exerciseID) {
				continue
			}
			value := 0.0
			if m.measurement == "repetitions" {
				for _, set := range item.PerformedSets {
					multiplier := set.LoadKg
					if multiplier == 0 {
						multiplier = 1
					}
					value += float64(set.Reps) * multiplier
				}
			} else if m.measurement == "duration_distance" {
				value = float64(item.ActualDurationSeconds) / 60
			} else {
				continue
			}
			if value > 0 {
				obs[item.ExerciseID] = append(obs[item.ExerciseID], exerciseObservation{date: at, value: value, effort: item.PerceivedEffort})
			}
		}
	}
	ids := make([]string, 0, len(obs))
	for id := range obs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := Progression{LookbackDays: days, Timezone: timezone, Items: make([]ProgressionItem, 0)}
	for _, id := range ids {
		values := obs[id]
		sort.SliceStable(values, func(i, j int) bool { return values[i].date.Before(values[j].date) })
		metric := ""
		if m := measures[id]; m.measurement == "repetitions" {
			metric = "repetition_volume"
		} else {
			metric = "duration_minutes"
		}
		item := ProgressionItem{ExerciseID: id, Metric: metric, Status: "insufficient_data", Observations: len(values), Suggestion: "maintain_current_level", ReasonCode: "insufficient_observations", Explanation: "At least six completed observations are required before a trend-based progression suggestion is considered."}
		if len(values) >= 6 {
			base := meanObservation(values[:3])
			current := meanObservation(values[len(values)-3:])
			pct := 0.0
			if base > 0 {
				pct = (current - base) / base * 100
			}
			pct = math.Round(pct*10) / 10
			item.Baseline = math.Round(base*10) / 10
			item.Current = math.Round(current*10) / 10
			item.ChangePercent = &pct
			item.Status = "suggestion"
			item.Suggestion = "maintain_current_level"
			item.ReasonCode = "maintain_or_review_effort"
			item.Explanation = "Recent performance is compared with the first three observations. Keep the current level unless effort and technique feel comfortable."
			easy := true
			for _, v := range values[len(values)-3:] {
				if v.effort < 1 || v.effort > 7 {
					easy = false
				}
			}
			if pct >= 5 && easy {
				item.Suggestion = "consider_small_progression"
				item.ReasonCode = "consistent_performance_with_managed_effort"
				item.Explanation = "Recent completed performance is at least 5% above the baseline and reported effort is at most 7/10. Consider a small change only if technique remains controlled; confirm before changing a plan."
			}
		}
		result.Items = append(result.Items, item)
		if len(result.Items) >= limit {
			break
		}
	}
	return result, nil
}

func (s *Service) analyticsLogs(ctx context.Context, userID string, start, end time.Time) ([]analyticsLog, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status,source,started_at,finished_at,items_json FROM workout_logs WHERE user_id=? AND started_at>=? AND started_at<? ORDER BY started_at DESC LIMIT 5001`, userID, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]analyticsLog, 0)
	for rows.Next() {
		var x analyticsLog
		if err = rows.Scan(&x.status, &x.source, &x.start, &x.finish, &x.items); err != nil {
			return nil, err
		}
		items = append(items, x)
		if len(items) > 5000 {
			return nil, ErrHistoryLimit
		}
	}
	return items, rows.Err()
}
func (s *Service) exerciseMeasures(ctx context.Context) (map[string]exerciseMeasure, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,category,measurement_type FROM exercises WHERE active=1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make(map[string]exerciseMeasure)
	for rows.Next() {
		var id string
		var item exerciseMeasure
		if err = rows.Scan(&id, &item.category, &item.measurement); err != nil {
			return nil, err
		}
		items[id] = item
	}
	return items, rows.Err()
}
func localDateWeek(at time.Time) string {
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
	delta := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -delta).Format("2006-01-02")
}
func meanObservation(items []exerciseObservation) float64 {
	sum := 0.0
	for _, item := range items {
		sum += item.value
	}
	return sum / float64(len(items))
}
