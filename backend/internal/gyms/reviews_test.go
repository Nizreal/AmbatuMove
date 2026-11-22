package gyms

import (
	"context"
	"testing"
)

func TestReviewModerationControlsPublicVisibilityAndAudit(t *testing.T) {
	s := newGymService(t)
	ctx := context.Background()
	for _, u := range []string{"reviewer", "admin", "other"} {
		if _, err := s.db.Exec("INSERT INTO users(id,email_normalized,password_hash,role,created_at) VALUES(?,?,?,'user',?)", u, u+"@example.test", "hash", "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec("UPDATE users SET role='admin' WHERE id='admin'"); err != nil {
		t.Fatal(err)
	}
	review, err := s.SubmitReview(ctx, "reviewer", "gym_demo_jakarta_001", 5, "Good equipment and clean.")
	if err != nil || review.Status != "pending" {
		t.Fatalf("review=%+v err=%v", review, err)
	}
	public, total, err := s.ListApprovedReviews(ctx, "gym_demo_jakarta_001", 10, 0)
	if err != nil || total != 0 || len(public) != 0 {
		t.Fatalf("pending leaked: %+v total=%d err=%v", public, total, err)
	}
	if _, err = s.SubmitReview(ctx, "reviewer", "gym_demo_jakarta_001", 3, "duplicate"); err != ErrReviewConflict {
		t.Fatalf("duplicate review error=%v", err)
	}
	if err = s.ModerateReview(ctx, "admin", review.ID, "approved", "meets policy"); err != nil {
		t.Fatal(err)
	}
	public, total, err = s.ListApprovedReviews(ctx, "gym_demo_jakarta_001", 10, 0)
	if err != nil || total != 1 || len(public) != 1 || public[0].Rating != 5 {
		t.Fatalf("approved reviews=%+v total=%d err=%v", public, total, err)
	}
	detail, err := s.Get(ctx, "gym_demo_jakarta_001")
	if err != nil || detail.Summary.AverageRating == nil || *detail.Summary.AverageRating != 5 || detail.Summary.ApprovedReviewCount != 1 {
		t.Fatalf("approved aggregate=%+v err=%v", detail.Summary, err)
	}
	if err = s.ModerateReview(ctx, "admin", review.ID, "hidden", "reported concern"); err != nil {
		t.Fatal(err)
	}
	public, total, err = s.ListApprovedReviews(ctx, "gym_demo_jakarta_001", 10, 0)
	if err != nil || total != 0 || len(public) != 0 {
		t.Fatalf("hidden review leaked: %+v total=%d err=%v", public, total, err)
	}
	detail, err = s.Get(ctx, "gym_demo_jakarta_001")
	if err != nil || detail.Summary.AverageRating != nil || detail.Summary.ApprovedReviewCount != 0 {
		t.Fatalf("hidden aggregate remained: %+v err=%v", detail.Summary, err)
	}
	var audits int
	if err = s.db.QueryRow("SELECT count(*) FROM gym_review_moderation_audit WHERE review_id=?", review.ID).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("audit records=%d err=%v", audits, err)
	}
}

func TestFavoriteGymsAreIdempotentOwnerScopedAndRetainedWhenInactive(t *testing.T) {
	s := newGymService(t)
	ctx := context.Background()
	for _, u := range []string{"u1", "u2"} {
		if _, err := s.db.Exec("INSERT INTO users(id,email_normalized,password_hash,created_at) VALUES(?,?,?,?)", u, u+"@example.test", "hash", "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddFavorite(ctx, "u1", "gym_demo_jakarta_001"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddFavorite(ctx, "u1", "gym_demo_jakarta_001"); err != nil {
		t.Fatal(err)
	}
	items, total, err := s.ListFavorites(ctx, "u1", 10, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("favorites=%+v total=%d err=%v", items, total, err)
	}
	other, total, err := s.ListFavorites(ctx, "u2", 10, 0)
	if err != nil || total != 0 || len(other) != 0 {
		t.Fatalf("other favorites=%+v total=%d err=%v", other, total, err)
	}
	if _, err = s.db.Exec("UPDATE gyms SET active=0 WHERE id='gym_demo_jakarta_001'"); err != nil {
		t.Fatal(err)
	}
	items, total, err = s.ListFavorites(ctx, "u1", 10, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("inactive gym should be hidden from list: %+v total=%d err=%v", items, total, err)
	}
	if err = s.RemoveFavorite(ctx, "u1", "gym_demo_jakarta_001"); err != nil {
		t.Fatal(err)
	}
}
