package saved

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/programs"
)

var ErrNotFound = errors.New("saved resource not found")
var ErrVersion = errors.New("resource version conflict")

type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }
func (s *Service) SaveWorkout(ctx context.Context, userID string, v planning.Session) (planning.Session, error) {
	id, err := newID()
	if err != nil {
		return v, err
	}
	now := time.Now().UTC()
	v.ID = &id
	v.Saved = true
	v.Version = 1
	v.Status = "planned"
	raw, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO workout_sessions(id,user_id,goal,workout_type,status,version,snapshot_json,created_at,updated_at) VALUES(?,?,?,?,'planned',1,?,?,?)`, id, userID, v.Goal, v.WorkoutType, string(raw), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return v, err
}
func (s *Service) GetWorkout(ctx context.Context, userID, id string) (planning.Session, error) {
	var raw string
	var status string
	var version int
	err := s.db.QueryRowContext(ctx, "SELECT snapshot_json,status,version FROM workout_sessions WHERE user_id=? AND id=? AND deleted_at IS NULL", userID, id).Scan(&raw, &status, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return planning.Session{}, ErrNotFound
	}
	var v planning.Session
	if err == nil {
		err = json.Unmarshal([]byte(raw), &v)
	}
	if err == nil {
		v.Status = status
		v.Version = version
	}
	return v, err
}

type WorkoutFilters struct{ Status, ProgramID, From, To string }

func (s *Service) ListWorkouts(ctx context.Context, userID string, limit, offset int, filters WorkoutFilters) ([]planning.Session, int, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 || filters.Status != "" && filters.Status != "planned" && filters.Status != "in_progress" && filters.Status != "completed" && filters.Status != "cancelled" || len(filters.ProgramID) > 80 {
		return nil, 0, ErrVersion
	}
	where := []string{"user_id=?", "deleted_at IS NULL"}
	args := []any{userID}
	if filters.Status != "" {
		where = append(where, "status=?")
		args = append(args, filters.Status)
	}
	if filters.ProgramID != "" {
		where = append(where, "program_id=?")
		args = append(args, filters.ProgramID)
	}
	if filters.From != "" {
		if _, err := time.Parse("2006-01-02", filters.From); err != nil {
			return nil, 0, ErrVersion
		}
		where = append(where, "created_at>=?")
		args = append(args, filters.From+"T00:00:00Z")
	}
	if filters.To != "" {
		date, err := time.Parse("2006-01-02", filters.To)
		if err != nil {
			return nil, 0, ErrVersion
		}
		end := date.Add(24 * time.Hour).Format("2006-01-02")
		where = append(where, "created_at<?")
		args = append(args, end+"T00:00:00Z")
	}
	if filters.From != "" && filters.To != "" && filters.To < filters.From {
		return nil, 0, ErrVersion
	}
	condition := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM workout_sessions WHERE "+condition, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, "SELECT snapshot_json,status,version FROM workout_sessions WHERE "+condition+" ORDER BY created_at DESC LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]planning.Session, 0)
	for rows.Next() {
		var raw string
		var status string
		var version int
		var v planning.Session
		if err = rows.Scan(&raw, &status, &version); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, 0, err
		}
		v.Status = status
		v.Version = version
		items = append(items, v)
	}
	return items, total, rows.Err()
}
func (s *Service) UpdateWorkout(ctx context.Context, userID, id string, expected int, v planning.Session) (planning.Session, error) {
	v.Version = expected + 1
	v.Saved = true
	raw, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, "UPDATE workout_sessions SET version=?,status=?,snapshot_json=?,updated_at=? WHERE id=? AND user_id=? AND version=? AND deleted_at IS NULL AND status!='completed'", v.Version, v.Status, string(raw), now, id, userID, expected)
	if err != nil {
		return v, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		var exists int
		if e := s.db.QueryRowContext(ctx, "SELECT 1 FROM workout_sessions WHERE id=? AND user_id=? AND deleted_at IS NULL", id, userID).Scan(&exists); e != nil {
			return v, ErrNotFound
		}
		return v, ErrVersion
	}
	v.ID = &id
	return v, nil
}
func (s *Service) ArchiveWorkout(ctx context.Context, userID, id string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE workout_sessions SET deleted_at=?,status=CASE WHEN status='completed' THEN status ELSE 'cancelled' END,updated_at=? WHERE id=? AND user_id=? AND deleted_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), id, userID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Service) SaveProgram(ctx context.Context, userID string, v programs.Program, daysPerWeek int) (programs.Program, error) {
	id, err := newID()
	if err != nil {
		return v, err
	}
	now := time.Now().UTC()
	v.ID = &id
	v.Saved = true
	v.Version = 1
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	for i := range v.Schedule {
		if v.Schedule[i].Session == nil {
			continue
		}
		session := v.Schedule[i].Session
		sessionID, e := newID()
		if e != nil {
			return v, e
		}
		session.ID = &sessionID
		session.Saved = true
		session.Version = 1
		session.Status = "planned"
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workout_programs(id,user_id,goal,start_date,timezone,weeks,days_per_week,status,version,snapshot_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,'active',1,?,?,?)`, id, userID, v.Goal, v.StartDate, v.Timezone, v.Weeks, daysPerWeek, string(raw), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return v, err
	}
	for _, entry := range v.Schedule {
		if entry.Session == nil || entry.Session.ID == nil {
			continue
		}
		session := entry.Session
		sessionJSON, e := json.Marshal(session)
		if e != nil {
			return v, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO workout_sessions(id,user_id,program_id,goal,workout_type,status,version,snapshot_json,created_at,updated_at) VALUES(?,?,?,?,?,'planned',1,?,?,?)`, *session.ID, userID, id, session.Goal, session.WorkoutType, string(sessionJSON), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); e != nil {
			return v, e
		}
	}
	if err = tx.Commit(); err != nil {
		return v, err
	}
	return v, nil
}
func (s *Service) GetProgram(ctx context.Context, userID, id string) (programs.Program, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT snapshot_json FROM workout_programs WHERE user_id=? AND id=? AND status='active'", userID, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return programs.Program{}, ErrNotFound
	}
	var v programs.Program
	if err == nil {
		err = json.Unmarshal([]byte(raw), &v)
	}
	if err == nil {
		err = s.refreshProgramSessions(ctx, userID, &v)
	}
	return v, err
}
func (s *Service) ListPrograms(ctx context.Context, userID string, limit, offset int) ([]programs.Program, int, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, 0, ErrVersion
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM workout_programs WHERE user_id=? AND status='active'", userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT snapshot_json FROM workout_programs WHERE user_id=? AND status='active' ORDER BY start_date DESC LIMIT ? OFFSET ?", userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]programs.Program, 0)
	for rows.Next() {
		var raw string
		var v programs.Program
		if err = rows.Scan(&raw); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, 0, err
		}
		if err = s.refreshProgramSessions(ctx, userID, &v); err != nil {
			return nil, 0, err
		}
		items = append(items, v)
	}
	return items, total, rows.Err()
}
func (s *Service) refreshProgramSessions(ctx context.Context, userID string, v *programs.Program) error {
	for i := range v.Schedule {
		if v.Schedule[i].Session == nil || v.Schedule[i].Session.ID == nil {
			continue
		}
		current, err := s.GetWorkout(ctx, userID, *v.Schedule[i].Session.ID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		v.Schedule[i].Session = &current
	}
	return nil
}
func (s *Service) UpdateProgram(ctx context.Context, userID, id string, expected int, v programs.Program) (programs.Program, error) {
	v.Version = expected + 1
	v.Saved = true
	raw, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE workout_programs SET version=?,snapshot_json=?,updated_at=? WHERE id=? AND user_id=? AND version=? AND status='active'", v.Version, string(raw), time.Now().UTC().Format(time.RFC3339Nano), id, userID, expected)
	if err != nil {
		return v, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		var exists int
		if e := tx.QueryRowContext(ctx, "SELECT 1 FROM workout_programs WHERE id=? AND user_id=? AND status='active'", id, userID).Scan(&exists); e != nil {
			return v, ErrNotFound
		}
		return v, ErrVersion
	}
	for _, entry := range v.Schedule {
		if entry.Session == nil || entry.Session.ID == nil {
			continue
		}
		session := *entry.Session
		session.Saved = true
		sessionJSON, e := json.Marshal(session)
		if e != nil {
			return v, e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE workout_sessions SET version=version+1,status=?,snapshot_json=?,updated_at=? WHERE id=? AND user_id=? AND program_id=? AND status!='completed' AND deleted_at IS NULL", session.Status, string(sessionJSON), time.Now().UTC().Format(time.RFC3339Nano), *session.ID, userID, id); e != nil {
			return v, e
		}
	}
	if err = tx.Commit(); err != nil {
		return v, err
	}
	v.ID = &id
	return v, nil
}
func (s *Service) ArchiveProgram(ctx context.Context, userID, id string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE workout_programs SET status='archived',updated_at=? WHERE id=? AND user_id=? AND status='active'", time.Now().UTC().Format(time.RFC3339Nano), id, userID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func newID() (string, error) { var id string; err := randomUUID(&id); return id, err }
func randomUUID(out *string) error {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	*out = fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	return nil
}
