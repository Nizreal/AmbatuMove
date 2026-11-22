package saved

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ambatumove/core/internal/planning"
)

type revisionTx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Service) insertWorkoutRevision(ctx context.Context, tx revisionTx, userID, workoutID string, snapshot planning.Session, reason, createdAt string) error {
	if snapshot.Version < 1 {
		snapshot.Version = 1
	}
	snapshot.Saved = true
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	revisionID, err := newID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workout_revisions(id,session_id,user_id,version,snapshot_json,reason,actor_user_id,created_at) VALUES(?,?,?,?,?,?,?,?)`, revisionID, workoutID, userID, snapshot.Version, string(raw), reason, userID, createdAt)
	return err
}

func (s *Service) WorkoutRevisions(ctx context.Context, userID, workoutID string, limit, offset int) ([]WorkoutRevision, int, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, 0, ErrVersion
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM workout_sessions WHERE id=? AND user_id=?", workoutID, userID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	} else if err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM workout_revisions WHERE session_id=? AND user_id=?", workoutID, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,version,snapshot_json,reason,actor_user_id,created_at FROM workout_revisions WHERE session_id=? AND user_id=? ORDER BY version ASC LIMIT ? OFFSET ?`, workoutID, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]WorkoutRevision, 0)
	for rows.Next() {
		var item WorkoutRevision
		var raw string
		if err = rows.Scan(&item.ID, &item.Version, &raw, &item.Reason, &item.ActorID, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(raw), &item.Snapshot); err != nil {
			return nil, 0, err
		}
		item.WorkoutID = workoutID
		items = append(items, item)
	}
	return items, total, rows.Err()
}
