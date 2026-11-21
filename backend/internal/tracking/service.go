package tracking

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ambatumove/core/internal/planning"
	"math"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid workout log")
var ErrNotFound = errors.New("workout log not found")
var ErrConflict = errors.New("idempotency key conflict")

type Set struct {
	Reps   int     `json:"reps"`
	LoadKg float64 `json:"load_kg"`
}
type PerformedItem struct {
	ItemID                string  `json:"item_id"`
	ExerciseID            string  `json:"exercise_id"`
	PerformedSets         []Set   `json:"performed_sets,omitempty"`
	ActualDurationSeconds int     `json:"actual_duration_seconds,omitempty"`
	ActualDistanceKm      float64 `json:"actual_distance_km,omitempty"`
	PerceivedEffort       int     `json:"perceived_effort,omitempty"`
}
type Completion struct {
	Status     string          `json:"status"`
	StartedAt  string          `json:"started_at"`
	FinishedAt string          `json:"finished_at"`
	Items      []PerformedItem `json:"items"`
	Notes      string          `json:"notes,omitempty"`
}
type Log struct {
	ID         string          `json:"id"`
	SessionID  *string         `json:"session_id,omitempty"`
	Status     string          `json:"status"`
	StartedAt  string          `json:"started_at,omitempty"`
	FinishedAt string          `json:"finished_at,omitempty"`
	Notes      string          `json:"notes"`
	Items      []PerformedItem `json:"items"`
	CreatedAt  string          `json:"created_at"`
}
type Service struct{ db *sql.DB }
type Summary struct {
	From             string         `json:"from"`
	To               string         `json:"to"`
	Timezone         string         `json:"timezone"`
	Period           string         `json:"period,omitempty"`
	Sessions         int            `json:"sessions"`
	Completed        int            `json:"completed"`
	Minutes          int            `json:"minutes"`
	AdherenceRate    float64        `json:"adherence_rate"`
	StrengthVolumeKg int            `json:"strength_volume_kg"`
	ByActivityType   map[string]int `json:"by_activity_type"`
}

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) Complete(ctx context.Context, userID, sessionID, idempotencyKey string, request Completion) (Log, error) {
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 || strings.TrimSpace(request.Notes) != request.Notes || len(request.Notes) > 2000 || !validStatus(request.Status) || len(request.Items) == 0 && request.Status != "skipped" || len(request.Items) > 100 {
		return Log{}, ErrInvalid
	}
	started, err := time.Parse(time.RFC3339, request.StartedAt)
	if err != nil {
		return Log{}, ErrInvalid
	}
	finished, err := time.Parse(time.RFC3339, request.FinishedAt)
	if err != nil || finished.Before(started) || finished.Sub(started) > 24*time.Hour {
		return Log{}, ErrInvalid
	}
	// Persist one canonical representation so indexed time-range comparisons are
	// correct even when clients submit timestamps with different UTC offsets.
	request.StartedAt = started.UTC().Format(time.RFC3339Nano)
	request.FinishedAt = finished.UTC().Format(time.RFC3339Nano)
	var raw string
	if err = s.db.QueryRowContext(ctx, "SELECT snapshot_json FROM workout_sessions WHERE user_id=? AND id=? AND deleted_at IS NULL", userID, sessionID).Scan(&raw); err != nil {
		return Log{}, ErrNotFound
	}
	var session planning.Session
	if err = json.Unmarshal([]byte(raw), &session); err != nil {
		return Log{}, err
	}
	belongs := map[string]string{}
	for _, section := range session.Sections {
		for _, item := range section.Items {
			belongs[item.ItemID] = item.ExerciseID
		}
	}
	seen := map[string]bool{}
	for _, item := range request.Items {
		if item.ItemID == "" || seen[item.ItemID] || belongs[item.ItemID] != item.ExerciseID || item.PerceivedEffort < 0 || item.PerceivedEffort > 10 || item.ActualDurationSeconds < 0 || item.ActualDistanceKm < 0 {
			return Log{}, ErrInvalid
		}
		seen[item.ItemID] = true
		for _, set := range item.PerformedSets {
			if set.Reps < 1 || set.Reps > 1000 || set.LoadKg < 0 || set.LoadKg > 1000 {
				return Log{}, ErrInvalid
			}
		}
		if len(item.PerformedSets) == 0 && item.ActualDurationSeconds == 0 {
			return Log{}, ErrInvalid
		}
	}
	itemsJSON, _ := json.Marshal(request.Items)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var existingID, existingItems, existingStatus, existingStarted, existingFinished, existingNotes string
	err = s.db.QueryRowContext(ctx, "SELECT id,status,started_at,finished_at,notes,items_json FROM workout_logs WHERE user_id=? AND idempotency_key=?", userID, idempotencyKey).Scan(&existingID, &existingStatus, &existingStarted, &existingFinished, &existingNotes, &existingItems)
	if err == nil {
		if existingItems != string(itemsJSON) || existingStatus != request.Status || existingStarted != request.StartedAt || existingFinished != request.FinishedAt || existingNotes != request.Notes {
			return Log{}, ErrConflict
		}
		return Log{ID: existingID, SessionID: &sessionID, Status: request.Status, StartedAt: request.StartedAt, FinishedAt: request.FinishedAt, Notes: request.Notes, Items: request.Items}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Log{}, err
	}
	id, err := newID()
	if err != nil {
		return Log{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Log{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO workout_logs(id,user_id,session_id,status,started_at,finished_at,notes,idempotency_key,items_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, userID, sessionID, request.Status, request.StartedAt, request.FinishedAt, request.Notes, idempotencyKey, string(itemsJSON), now)
	if err != nil {
		return Log{}, err
	}
	_ = result
	if request.Status == "completed" {
		if _, err = tx.ExecContext(ctx, "UPDATE workout_sessions SET status='completed',updated_at=? WHERE id=? AND user_id=?", now, sessionID, userID); err != nil {
			return Log{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Log{}, err
	}
	return Log{ID: id, SessionID: &sessionID, Status: request.Status, StartedAt: request.StartedAt, FinishedAt: request.FinishedAt, Notes: request.Notes, Items: request.Items, CreatedAt: now}, nil
}

func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]Log, int, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, 0, ErrInvalid
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM workout_logs WHERE user_id=?", userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,session_id,status,started_at,finished_at,notes,items_json,created_at FROM workout_logs WHERE user_id=? ORDER BY COALESCE(started_at,created_at) DESC LIMIT ? OFFSET ?", userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]Log, 0)
	for rows.Next() {
		var l Log
		var sid, start, finish sql.NullString
		var items string
		if err = rows.Scan(&l.ID, &sid, &l.Status, &start, &finish, &l.Notes, &items, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		if sid.Valid {
			l.SessionID = &sid.String
		}
		if start.Valid {
			l.StartedAt = start.String
		}
		if finish.Valid {
			l.FinishedAt = finish.String
		}
		if err = json.Unmarshal([]byte(items), &l.Items); err != nil {
			return nil, 0, err
		}
		result = append(result, l)
	}
	return result, total, rows.Err()
}
func (s *Service) Get(ctx context.Context, userID, id string) (Log, error) {
	var l Log
	var sid, start, finish sql.NullString
	var items string
	err := s.db.QueryRowContext(ctx, "SELECT id,session_id,status,started_at,finished_at,notes,items_json,created_at FROM workout_logs WHERE user_id=? AND id=?", userID, id).Scan(&l.ID, &sid, &l.Status, &start, &finish, &l.Notes, &items, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	if sid.Valid {
		l.SessionID = &sid.String
	}
	if start.Valid {
		l.StartedAt = start.String
	}
	if finish.Valid {
		l.FinishedAt = finish.String
	}
	err = json.Unmarshal([]byte(items), &l.Items)
	return l, err
}
func (s *Service) Summary(ctx context.Context, userID, from, to, timezone, period string) (Summary, error) {
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return Summary{}, ErrInvalid
	}
	if period != "" && period != "week" && period != "month" {
		return Summary{}, ErrInvalid
	}
	start, err := time.ParseInLocation("2006-01-02", from, location)
	if err != nil {
		return Summary{}, ErrInvalid
	}
	end, err := time.ParseInLocation("2006-01-02", to, location)
	if err != nil || end.Before(start) || end.Sub(start) > 366*24*time.Hour {
		return Summary{}, ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, "SELECT status,started_at,finished_at,items_json FROM workout_logs WHERE user_id=? AND started_at>=? AND started_at<? ORDER BY started_at LIMIT 10000", userID, start.UTC().Format(time.RFC3339Nano), end.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Summary{}, err
	}
	defer rows.Close()
	summary := Summary{From: from, To: to, Timezone: timezone, Period: period, ByActivityType: map[string]int{}}
	for rows.Next() {
		var status string
		var started, finished sql.NullString
		var raw string
		if err = rows.Scan(&status, &started, &finished, &raw); err != nil {
			return Summary{}, err
		}
		summary.Sessions++
		if status == "completed" {
			summary.Completed++
		}
		if started.Valid && finished.Valid {
			a, e1 := time.Parse(time.RFC3339, started.String)
			b, e2 := time.Parse(time.RFC3339, finished.String)
			if e1 == nil && e2 == nil && b.After(a) {
				summary.Minutes += int(b.Sub(a).Minutes())
			}
		}
		var items []PerformedItem
		if json.Unmarshal([]byte(raw), &items) == nil {
			for _, item := range items {
				activity := "strength"
				if item.ActualDurationSeconds > 0 {
					activity = "cardio"
				}
				summary.ByActivityType[activity]++
				for _, set := range item.PerformedSets {
					summary.StrengthVolumeKg += int(set.LoadKg * float64(set.Reps))
				}
			}
		}
	}
	if summary.Sessions > 0 {
		summary.AdherenceRate = math.Round(float64(summary.Completed)/float64(summary.Sessions)*1000) / 10
	}
	return summary, rows.Err()
}
func validStatus(status string) bool {
	return status == "completed" || status == "partial" || status == "skipped"
}
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
