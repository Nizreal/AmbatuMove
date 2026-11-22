package gyms

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrReviewInvalid = errors.New("invalid gym review")
var ErrReviewNotFound = errors.New("gym review not found")
var ErrReviewConflict = errors.New("review already submitted or state conflict")
var ErrReviewForbidden = errors.New("review moderation forbidden")

type Review struct {
	ID        string `json:"id"`
	GymID     string `json:"gym_id"`
	Rating    int    `json:"rating"`
	Comment   string `json:"comment"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"created_at"`
}
type ReviewModerationItem struct {
	Review
	UserID string `json:"user_id"`
}
type Favorite struct {
	Gym       Summary `json:"gym"`
	CreatedAt string  `json:"created_at"`
}

func (s *Service) SubmitReview(ctx context.Context, userID, gymID string, rating int, comment string) (Review, error) {
	if rating < 1 || rating > 5 || len(comment) > 2000 || strings.TrimSpace(comment) != comment {
		return Review{}, ErrReviewInvalid
	}
	if _, err := s.GetID(ctx, gymID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Review{}, ErrNotFound
		}
		return Review{}, err
	}
	id, err := newReviewID()
	if err != nil {
		return Review{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT INTO gym_reviews(id,gym_id,user_id,rating,comment,status,created_at,updated_at) VALUES(?,?,?,?,?,'pending',?,?)`, id, gymID, userID, rating, comment, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			return Review{}, ErrReviewConflict
		}
		return Review{}, err
	}
	return Review{ID: id, GymID: gymID, Rating: rating, Comment: comment, Status: "pending", CreatedAt: now}, nil
}

func (s *Service) ListApprovedReviews(ctx context.Context, gymID string, limit, offset int) ([]Review, int, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, 0, ErrInvalidSearch
	}
	if _, err := s.GetID(ctx, gymID); err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gym_reviews WHERE gym_id=? AND status='approved'", gymID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,gym_id,rating,comment,created_at FROM gym_reviews WHERE gym_id=? AND status='approved' ORDER BY created_at DESC,id LIMIT ? OFFSET ?`, gymID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]Review, 0)
	for rows.Next() {
		var item Review
		if err = rows.Scan(&item.ID, &item.GymID, &item.Rating, &item.Comment, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Service) ListReviewModeration(ctx context.Context, adminID, status string, limit, offset int) ([]ReviewModerationItem, int, error) {
	if !reviewState(status, "pending", "approved", "rejected", "hidden") || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, 0, ErrReviewInvalid
	}
	var role string
	if err := s.db.QueryRowContext(ctx, "SELECT role FROM users WHERE id=?", adminID).Scan(&role); err != nil || role != "admin" {
		return nil, 0, ErrReviewForbidden
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gym_reviews WHERE status=?", status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,gym_id,user_id,rating,comment,status,created_at FROM gym_reviews WHERE status=? ORDER BY created_at,id LIMIT ? OFFSET ?`, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]ReviewModerationItem, 0)
	for rows.Next() {
		var x ReviewModerationItem
		if err = rows.Scan(&x.ID, &x.GymID, &x.UserID, &x.Rating, &x.Comment, &x.Status, &x.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, x)
	}
	return items, total, rows.Err()
}

func (s *Service) ModerateReview(ctx context.Context, adminID, reviewID, decision, reason string) error {
	if !reviewState(decision, "approved", "rejected", "hidden") || len(reason) < 3 || len(reason) > 500 || strings.TrimSpace(reason) != reason {
		return ErrReviewInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var role string
	if err = tx.QueryRowContext(ctx, "SELECT role FROM users WHERE id=?", adminID).Scan(&role); err != nil || role != "admin" {
		return ErrReviewForbidden
	}
	var gymID, previous string
	if err = tx.QueryRowContext(ctx, "SELECT gym_id,status FROM gym_reviews WHERE id=?", reviewID).Scan(&gymID, &previous); errors.Is(err, sql.ErrNoRows) {
		return ErrReviewNotFound
	} else if err != nil {
		return err
	}
	if previous == decision {
		return ErrReviewConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, "UPDATE gym_reviews SET status=?,updated_at=? WHERE id=?", decision, now, reviewID); err != nil {
		return err
	}
	auditID, e := newReviewID()
	if e != nil {
		return e
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO gym_review_moderation_audit(id,review_id,moderator_user_id,previous_status,new_status,reason,created_at) VALUES(?,?,?,?,?,?,?)`, auditID, reviewID, adminID, previous, decision, reason, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE reference_data_revision SET revision=revision+1 WHERE id=1"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.InvalidateCache()
	return nil
}

func (s *Service) ApprovedRating(ctx context.Context, gymID string) (*float64, int, error) {
	var avg sql.NullFloat64
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT avg(rating),count(*) FROM gym_reviews WHERE gym_id=? AND status='approved'", gymID).Scan(&avg, &count); err != nil {
		return nil, 0, err
	}
	if !avg.Valid {
		return nil, count, nil
	}
	v := avg.Float64
	return &v, count, nil
}

func (s *Service) AddFavorite(ctx context.Context, userID, gymID string) error {
	if _, err := s.GetID(ctx, gymID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO favorite_gyms(user_id,gym_id,created_at) VALUES(?,?,?) ON CONFLICT(user_id,gym_id) DO NOTHING", userID, gymID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Service) RemoveFavorite(ctx context.Context, userID, gymID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM favorite_gyms WHERE user_id=? AND gym_id=?", userID, gymID)
	return err
}
func (s *Service) ListFavorites(ctx context.Context, userID string, limit, offset int) ([]Favorite, int, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, 0, ErrInvalidSearch
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM favorite_gyms f JOIN gyms g ON g.id=f.gym_id AND g.active=1 WHERE f.user_id=?", userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT g.id,g.name,g.address_json,g.latitude,g.longitude,g.verification_status,g.source_type,f.created_at FROM favorite_gyms f JOIN gyms g ON g.id=f.gym_id AND g.active=1 WHERE f.user_id=? ORDER BY f.created_at DESC,g.id LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]Favorite, 0)
	for rows.Next() {
		var x Favorite
		var address string
		if err = rows.Scan(&x.Gym.ID, &x.Gym.Name, &address, &x.Gym.Location.Latitude, &x.Gym.Location.Longitude, &x.Gym.VerificationStatus, &x.Gym.SourceType, &x.CreatedAt); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(address), &x.Gym.Address); err != nil {
			return nil, 0, err
		}
		items = append(items, x)
	}
	return items, total, rows.Err()
}

func reviewState(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func newReviewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[:8], h[8:12], h[12:16], h[16:20], h[20:]), nil
}
