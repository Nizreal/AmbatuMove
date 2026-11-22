package tracking

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

var ErrVersion = errors.New("workout log version conflict")

type Correction struct {
	ExpectedVersion int              `json:"expected_version"`
	Reason          string           `json:"reason"`
	Notes           *string          `json:"notes,omitempty"`
	Status          *string          `json:"status,omitempty"`
	StartedAt       *string          `json:"started_at,omitempty"`
	FinishedAt      *string          `json:"finished_at,omitempty"`
	Items           *[]PerformedItem `json:"items,omitempty"`
}

type CorrectionRecord struct {
	ID            string   `json:"id"`
	LogID         string   `json:"log_id"`
	AuthorUserID  string   `json:"author_user_id"`
	FromVersion   int      `json:"from_version"`
	ToVersion     int      `json:"to_version"`
	Previous      Log      `json:"previous_state"`
	Current       Log      `json:"new_state"`
	ChangedFields []string `json:"changed_fields"`
	Reason        string   `json:"reason"`
	CreatedAt     string   `json:"created_at"`
}

func (s *Service) Correct(ctx context.Context, userID, logID string, request Correction) (Log, error) {
	if request.ExpectedVersion < 1 || len(request.Reason) < 3 || len(request.Reason) > 500 || strings.TrimSpace(request.Reason) != request.Reason || (request.Notes == nil && request.Status == nil && request.StartedAt == nil && request.FinishedAt == nil && request.Items == nil) {
		return Log{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Log{}, err
	}
	defer tx.Rollback()
	previous, err := scanLog(ctx, tx, userID, logID)
	if errors.Is(err, sql.ErrNoRows) {
		return Log{}, ErrNotFound
	}
	if err != nil {
		return Log{}, err
	}
	if previous.Version != request.ExpectedVersion {
		return Log{}, ErrVersion
	}
	current := previous
	changed := make([]string, 0, 5)
	if request.Notes != nil {
		if len(*request.Notes) > 2000 || strings.TrimSpace(*request.Notes) != *request.Notes {
			return Log{}, ErrInvalid
		}
		if current.Notes != *request.Notes {
			current.Notes = *request.Notes
			changed = append(changed, "notes")
		}
	}
	if request.Status != nil {
		if !validStatus(*request.Status) {
			return Log{}, ErrInvalid
		}
		if current.Status != *request.Status {
			current.Status = *request.Status
			changed = append(changed, "status")
		}
	}
	if request.Items != nil {
		if len(*request.Items) != len(previous.Items) {
			return Log{}, ErrInvalid
		}
		original := make(map[string]PerformedItem, len(previous.Items))
		for _, item := range previous.Items {
			original[item.ItemID] = item
		}
		seen := map[string]bool{}
		for _, item := range *request.Items {
			old, ok := original[item.ItemID]
			if !ok || seen[item.ItemID] || old.ExerciseID != item.ExerciseID {
				return Log{}, ErrInvalid
			}
			seen[item.ItemID] = true
			if err := s.validatePerformedItem(ctx, item); err != nil {
				return Log{}, err
			}
		}
		if !jsonEqual(current.Items, *request.Items) {
			current.Items = append([]PerformedItem(nil), (*request.Items)...)
			changed = append(changed, "items")
		}
	}
	if request.StartedAt != nil {
		current.StartedAt = *request.StartedAt
	}
	if request.FinishedAt != nil {
		current.FinishedAt = *request.FinishedAt
	}
	if request.StartedAt != nil || request.FinishedAt != nil {
		start, err := time.Parse(time.RFC3339, current.StartedAt)
		if err != nil {
			return Log{}, ErrInvalid
		}
		finish, err := time.Parse(time.RFC3339, current.FinishedAt)
		if err != nil || finish.Before(start) || finish.Sub(start) > 24*time.Hour {
			return Log{}, ErrInvalid
		}
		current.StartedAt = start.UTC().Format(time.RFC3339Nano)
		current.FinishedAt = finish.UTC().Format(time.RFC3339Nano)
		if current.StartedAt != previous.StartedAt {
			changed = append(changed, "started_at")
		}
		if current.FinishedAt != previous.FinishedAt {
			changed = append(changed, "finished_at")
		}
	}
	if len(changed) == 0 {
		return Log{}, ErrInvalid
	}
	current.Version = previous.Version + 1
	previousJSON, err := json.Marshal(previous)
	if err != nil {
		return Log{}, err
	}
	currentJSON, err := json.Marshal(current)
	if err != nil {
		return Log{}, err
	}
	changedJSON, _ := json.Marshal(changed)
	auditID, err := newID()
	if err != nil {
		return Log{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE workout_logs SET status=?,started_at=?,finished_at=?,notes=?,items_json=?,version=? WHERE id=? AND user_id=? AND version=?`, current.Status, nullString(current.StartedAt), nullString(current.FinishedAt), current.Notes, mustJSON(current.Items), current.Version, logID, userID, previous.Version)
	if err != nil {
		return Log{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return Log{}, ErrVersion
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO workout_log_corrections(id,log_id,user_id,actor_user_id,from_version,to_version,previous_state_json,new_state_json,changed_fields_json,reason,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, auditID, logID, userID, userID, previous.Version, current.Version, string(previousJSON), string(currentJSON), string(changedJSON), request.Reason, now); err != nil {
		return Log{}, err
	}
	if err = tx.Commit(); err != nil {
		return Log{}, err
	}
	current.CreatedAt = previous.CreatedAt
	return current, nil
}

func (s *Service) CorrectionHistory(ctx context.Context, userID, logID string, limit, offset int) ([]CorrectionRecord, int, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, 0, ErrInvalid
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM workout_logs WHERE user_id=? AND id=?", userID, logID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	} else if err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM workout_log_corrections WHERE user_id=? AND log_id=?", userID, logID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,actor_user_id,from_version,to_version,previous_state_json,new_state_json,changed_fields_json,reason,created_at FROM workout_log_corrections WHERE user_id=? AND log_id=? ORDER BY to_version DESC LIMIT ? OFFSET ?`, userID, logID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]CorrectionRecord, 0)
	for rows.Next() {
		var item CorrectionRecord
		var oldRaw, newRaw, fields string
		if err = rows.Scan(&item.ID, &item.AuthorUserID, &item.FromVersion, &item.ToVersion, &oldRaw, &newRaw, &fields, &item.Reason, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.LogID = logID
		if err = json.Unmarshal([]byte(oldRaw), &item.Previous); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(newRaw), &item.Current); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(fields), &item.ChangedFields); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Service) validatePerformedItem(ctx context.Context, item PerformedItem) error {
	var measurement string
	if err := s.db.QueryRowContext(ctx, "SELECT measurement_type FROM exercises WHERE id=? AND active=1", item.ExerciseID).Scan(&measurement); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalid
		}
		return err
	}
	if item.ItemID == "" || len(item.ItemID) > 100 || item.PerceivedEffort < 0 || item.PerceivedEffort > 10 || item.ActualDurationSeconds < 0 || item.ActualDurationSeconds > 86400 || math.IsNaN(item.ActualDistanceKm) || math.IsInf(item.ActualDistanceKm, 0) || item.ActualDistanceKm < 0 || item.ActualDistanceKm > 1000 {
		return ErrInvalid
	}
	switch measurement {
	case "repetitions":
		if len(item.PerformedSets) == 0 || item.ActualDurationSeconds != 0 || item.ActualDistanceKm != 0 {
			return ErrInvalid
		}
	case "duration":
		if len(item.PerformedSets) != 0 || item.ActualDurationSeconds < 1 || item.ActualDistanceKm != 0 {
			return ErrInvalid
		}
	case "duration_distance":
		if len(item.PerformedSets) != 0 || item.ActualDurationSeconds < 1 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	for _, set := range item.PerformedSets {
		if set.Reps < 1 || set.Reps > 1000 || math.IsNaN(set.LoadKg) || math.IsInf(set.LoadKg, 0) || set.LoadKg < 0 || set.LoadKg > 1000 {
			return ErrInvalid
		}
	}
	return nil
}

type logQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func scanLog(ctx context.Context, q logQueryer, userID, id string) (Log, error) {
	var l Log
	var sid, start, finish sql.NullString
	var items string
	err := q.QueryRowContext(ctx, "SELECT id,session_id,status,started_at,finished_at,notes,items_json,created_at,version,source FROM workout_logs WHERE user_id=? AND id=?", userID, id).Scan(&l.ID, &sid, &l.Status, &start, &finish, &l.Notes, &items, &l.CreatedAt, &l.Version, &l.Source)
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
func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
