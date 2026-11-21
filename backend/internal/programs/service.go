package programs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ambatumove/core/internal/planning"
)

var ErrInvalidRequest = errors.New("invalid program request")
var ErrUnfeasible = errors.New("program constraints cannot be satisfied")

const algorithmVersion = "program_planner_v1"

type Service struct{ planner *planning.Service }

func NewService(plannerService *planning.Service) *Service { return &Service{planner: plannerService} }

func (s *Service) Generate(ctx context.Context, request GenerateRequest) (Program, error) {
	if err := ctx.Err(); err != nil {
		return Program{}, err
	}
	if request.Weeks < 1 || request.Weeks > 12 || request.DaysPerWeek < 1 || request.DaysPerWeek > 7 || request.SessionDuration < 5 || request.SessionDuration > 180 {
		return Program{}, ErrInvalidRequest
	}
	if len(request.AvailableDays) != request.DaysPerWeek {
		return Program{}, fmt.Errorf("%w: available_days count must equal days_per_week", ErrInvalidRequest)
	}
	location, err := time.LoadLocation(request.Timezone)
	if err != nil || request.Timezone == "" {
		return Program{}, fmt.Errorf("%w: timezone must be a valid IANA time zone", ErrInvalidRequest)
	}
	start, err := time.ParseInLocation("2006-01-02", request.StartDate, location)
	if err != nil {
		return Program{}, fmt.Errorf("%w: start_date must use YYYY-MM-DD", ErrInvalidRequest)
	}
	days := map[string]bool{}
	for _, day := range request.AvailableDays {
		if !validWeekday(day) || days[day] {
			return Program{}, ErrInvalidRequest
		}
		days[day] = true
	}
	if request.Goal == "" || request.ExperienceLevel == "" || request.Environment == "" {
		return Program{}, ErrInvalidRequest
	}
	workoutType := request.WorkoutType
	if workoutType == "" {
		switch request.Goal {
		case "endurance":
			workoutType = "cardio"
		case "mobility", "active_recovery":
			workoutType = "mobility"
		default:
			workoutType = "strength"
		}
	}
	if request.Goal == "strength" || request.Goal == "hypertrophy" {
		if request.DaysPerWeek > 6 {
			return Program{}, fmt.Errorf("%w: strength programs require at least one weekly rest day", ErrInvalidRequest)
		}
	}
	seed, err := programSeed(request.Seed)
	if err != nil {
		return Program{}, err
	}
	entries := make([]ScheduleEntry, 0, request.Weeks*7)
	for dayOffset := 0; dayOffset < request.Weeks*7; dayOffset++ {
		if err := ctx.Err(); err != nil {
			return Program{}, err
		}
		date := start.AddDate(0, 0, dayOffset)
		weekday := weekdayName(date.Weekday())
		entry := ScheduleEntry{Date: date.Format("2006-01-02"), Week: dayOffset/7 + 1, Day: weekday, Kind: "rest", Session: nil}
		if days[weekday] {
			focus, sessionType, goal, muscles, intensity := sessionPlan(workoutType, request.Goal, request.DaysPerWeek, dayOffset/7, sessionDayIndex(request.AvailableDays, weekday))
			daySeed := seed + int64(dayOffset)
			gen := planning.GenerateRequest{Goal: goal, WorkoutType: sessionType, DurationMinutes: request.SessionDuration, ExperienceLevel: request.ExperienceLevel,
				Environment: request.Environment, Space: request.Space, AvailableEquipment: request.AvailableEquipment, UnavailableEquipment: request.UnavailableEquipment,
				PreferredActivityIDs: request.PreferredActivityIDs, ExcludedExerciseIDs: request.ExcludedExerciseIDs, FocusMuscles: muscles, Intensity: intensity, Seed: &daySeed, GymID: request.GymID}
			session, failure, err := s.planner.Generate(ctx, gen)
			if err != nil {
				if errors.Is(err, planning.ErrUnfeasible) {
					return Program{}, fmt.Errorf("%w on %s: %v", ErrUnfeasible, entry.Date, failure)
				}
				return Program{}, err
			}
			entry.Kind, entry.Focus, entry.Session = "workout", focus, &session
		}
		entries = append(entries, entry)
	}
	if err := ctx.Err(); err != nil {
		return Program{}, err
	}
	return Program{ID: nil, Saved: false, Version: 1, Goal: request.Goal, Weeks: request.Weeks, Timezone: request.Timezone, StartDate: start.Format("2006-01-02"), Schedule: entries, Generation: Generation{AlgorithmVersion: algorithmVersion, Seed: seed}, Warnings: []string{}}, nil
}

func sessionPlan(workoutType, goal string, days, week, day int) (focus, sessionType, sessionGoal string, muscles []string, intensity string) {
	sessionType, sessionGoal = workoutType, goal
	if workoutType == "cardio" {
		focus, intensity = "aerobic_base", "moderate"
		if (week+day)%4 == 3 {
			intensity = "vigorous"
		}
		return
	}
	if workoutType == "mobility" {
		return "mobility", "mobility", "mobility", nil, "low"
	}
	if workoutType == "mixed" {
		focus = []string{"strength", "cardio", "mobility"}[(week+day)%3]
		if focus == "cardio" {
			sessionType, intensity = "cardio", "moderate"
		} else if focus == "mobility" {
			sessionType, sessionGoal = "mobility", "active_recovery"
		}
		return
	}
	if days == 1 {
		focus = "full_body"
	} else {
		if day%2 == 0 {
			focus, muscles = "upper", []string{"chest", "back", "shoulders", "biceps", "triceps"}
		} else {
			focus, muscles = "lower", []string{"quadriceps", "hamstrings", "glutes", "calves"}
		}
	}
	if days >= 5 && day >= 4 {
		focus, sessionType, sessionGoal, muscles, intensity = "mobility_recovery", "mobility", "active_recovery", nil, "low"
	}
	return
}

func sessionDayIndex(available []string, day string) int {
	for index, candidate := range available {
		if candidate == day {
			return index
		}
	}
	return 0
}

func validWeekday(day string) bool {
	switch day {
	case "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday":
		return true
	default:
		return false
	}
}

func weekdayName(day time.Weekday) string {
	return []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}[day]
}

func programSeed(seed *int64) (int64, error) {
	if seed != nil {
		return *seed, nil
	}
	return time.Now().UTC().UnixNano(), nil
}
