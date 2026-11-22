package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/coach"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/profiles"
	"github.com/ambatumove/core/internal/saved"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/internal/tracking"
	"github.com/ambatumove/core/migrations"
)

func TestP2AuthenticatedRoutesEndToEndAndOwnerIsolation(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "p2-api.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err = catalog.SeedReferenceData(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = gyms.SeedFacilities(ctx, db); err != nil {
		t.Fatal(err)
	}
	catalogSvc := catalog.NewService(db)
	gymSvc := gyms.NewService(db)
	if err = gymSvc.SeedSyntheticFixtures(ctx); err != nil {
		t.Fatal(err)
	}
	profileSvc := profiles.NewService(db)
	savedSvc := saved.NewService(db)
	trackingSvc := tracking.NewService(db)
	auth, err := identity.NewService(db, []byte("01234567890123456789012345678901"), "ambatumove", "ambatumove-mobile")
	if err != nil {
		t.Fatal(err)
	}
	user, err := auth.Register(ctx, "p2user@example.test", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.Register(ctx, "other@example.test", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if err = auth.CreateAdmin(ctx, "p2admin@example.test", "another sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	admin, err := auth.Login(ctx, "p2admin@example.test", "another sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	coachSvc := coach.NewService(db, routeCoachProvider{}, catalogSvc, profileSvc, savedSvc, coach.ServiceConfig{Enabled: true, Timeout: time.Second, MaxOutputTokens: 128, RateLimitRPM: 10, MaxConcurrent: 1})
	handler := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), db, Dependencies{Catalog: catalogSvc, Gyms: gymSvc, Identity: auth, Profiles: profileSvc, Saved: savedSvc, Tracking: trackingSvc, Coach: coachSvc})
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	// Preset is independent of profile and stores an explicitly empty value.
	if got := call(http.MethodPut, "/api/v1/me/equipment", `{"equipment":["dumbbell","bodyweight","dumbbell"]}`, user.AccessToken); got.Code != 200 || !strings.Contains(got.Body.String(), `"configured":true`) || !strings.Contains(got.Body.String(), `"equipment":["bodyweight","dumbbell"]`) {
		t.Fatalf("equipment preset=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPut, "/api/v1/me/equipment", `{"equipment":[]}`, user.AccessToken); got.Code != 200 || !strings.Contains(got.Body.String(), `"equipment":[]`) {
		t.Fatalf("empty preset=%d %s", got.Code, got.Body.String())
	}
	manual := `{"status":"completed","started_at":"2026-11-21T10:00:00Z","finished_at":"2026-11-21T10:40:00Z","items":[{"exercise_id":"ex_push_up","performed_sets":[{"reps":12,"load_kg":0}],"perceived_effort":6},{"exercise_id":"act_walk_outdoor","actual_duration_seconds":2400,"actual_distance_km":2.1,"perceived_effort":4}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workout-logs/manual", strings.NewReader(manual))
	req.Header.Set("Authorization", "Bearer "+user.AccessToken)
	req.Header.Set("Idempotency-Key", "manual-route-key-001")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"source":"manual"`) {
		t.Fatalf("manual log=%d %s", rec.Code, rec.Body.String())
	}
	var manualEnvelope struct {
		Data struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"data"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &manualEnvelope); err != nil {
		t.Fatal(err)
	}
	patch := call(http.MethodPatch, "/api/v1/workout-logs/"+manualEnvelope.Data.ID, `{"expected_version":1,"reason":"Corrected session note","notes":"felt good"}`, user.AccessToken)
	if patch.Code != 200 || !strings.Contains(patch.Body.String(), `"version":2`) {
		t.Fatalf("correction=%d %s", patch.Code, patch.Body.String())
	}
	if got := call(http.MethodPatch, "/api/v1/workout-logs/"+manualEnvelope.Data.ID, `{"expected_version":2,"reason":"Explicit null must be rejected","notes":null}`, user.AccessToken); got.Code != 422 {
		t.Fatalf("explicit null correction status=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodGet, "/api/v1/workout-logs/"+manualEnvelope.Data.ID+"/corrections?limit=10", "", other.AccessToken); got.Code != 404 {
		t.Fatalf("cross-user correction history=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodGet, "/api/v1/workout-logs/"+manualEnvelope.Data.ID+"/corrections?limit=10", "", user.AccessToken); got.Code != 200 || !strings.Contains(got.Body.String(), `"previous_state"`) {
		t.Fatalf("correction history=%d %s", got.Code, got.Body.String())
	}
	planned, err := savedSvc.SaveWorkout(ctx, user.User.ID, planning.Session{Goal: "strength", WorkoutType: "strength", Sections: []planning.Section{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := call(http.MethodPatch, "/api/v1/workouts/"+*planned.ID, `{"expected_version":1,"status":"cancelled"}`, user.AccessToken); got.Code != 200 {
		t.Fatalf("metadata revision update=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodGet, "/api/v1/workouts/"+*planned.ID+"/revisions?limit=10", "", other.AccessToken); got.Code != 404 {
		t.Fatalf("cross-user revisions=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodGet, "/api/v1/workouts/"+*planned.ID+"/revisions?limit=10", "", user.AccessToken); got.Code != 200 || !strings.Contains(got.Body.String(), `"reason":"metadata"`) {
		t.Fatalf("revisions=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPost, "/api/v1/gyms/gym_demo_jakarta_001/reviews", `{"rating":5,"comment":"Helpful review."}`, user.AccessToken); got.Code != 201 || !strings.Contains(got.Body.String(), `"status":"pending"`) {
		t.Fatalf("review submission=%d %s", got.Code, got.Body.String())
	}
	var reviewEnvelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal([]byte(call(http.MethodPost, "/api/v1/gyms/gym_demo_jakarta_001/reviews", `{"rating":4,"comment":"duplicate"}`, other.AccessToken).Body.String()), &reviewEnvelope); err == nil && reviewEnvelope.Data.ID != "" { /* an unrelated user's review is valid */
	}
	adminQueue := call(http.MethodGet, "/api/v1/admin/gym-reviews?status=pending", "", admin.AccessToken)
	if adminQueue.Code != 200 || !strings.Contains(adminQueue.Body.String(), `"status":"pending"`) {
		t.Fatalf("admin review queue=%d %s", adminQueue.Code, adminQueue.Body.String())
	}
	var queue struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(adminQueue.Body.Bytes(), &queue); err != nil || len(queue.Data) < 1 {
		t.Fatalf("review queue parse=%+v err=%v", queue, err)
	}
	moderate := call(http.MethodPost, "/api/v1/admin/gym-reviews/"+queue.Data[0].ID+"/moderate", `{"decision":"approved","reason":"meets moderation rules"}`, admin.AccessToken)
	if moderate.Code != 200 {
		t.Fatalf("moderate=%d %s", moderate.Code, moderate.Body.String())
	}
	if got := call(http.MethodGet, "/api/v1/gyms/gym_demo_jakarta_001/reviews?limit=20", "", other.AccessToken); got.Code != 200 || !strings.Contains(got.Body.String(), `"rating":5`) {
		t.Fatalf("public approved review=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPost, "/api/v1/me/favorite-gyms", `{"gym_id":"gym_demo_jakarta_001"}`, user.AccessToken); got.Code != 204 {
		t.Fatalf("favorite add=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodGet, "/api/v1/me/favorite-gyms?limit=20", "", user.AccessToken); got.Code != 200 || !strings.Contains(got.Body.String(), "gym_demo_jakarta_001") {
		t.Fatalf("favorite list=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodGet, "/api/v1/me/favorite-gyms?limit=20", "", other.AccessToken); got.Code != 200 || strings.Contains(got.Body.String(), "gym_demo_jakarta_001") {
		t.Fatalf("other user's favorite list=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodGet, "/api/v1/me/progression?days=90&timezone=UTC", "", user.AccessToken); got.Code != 200 || !strings.Contains(got.Body.String(), "insufficient_data") {
		t.Fatalf("progression=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodGet, "/api/v1/stats/trends?days=90&timezone=UTC", "", user.AccessToken); got.Code != 200 || !strings.Contains(got.Body.String(), "manual_activities") {
		t.Fatalf("trends=%d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPost, "/api/v1/coach/chat", `{"message":"Why do I feel my back?","exercise_id":"ex_lat_pulldown","consent_external_processing":true}`, user.AccessToken); got.Code != 200 || !strings.Contains(got.Body.String(), `"provider":"mock"`) {
		t.Fatalf("coach=%d %s", got.Code, got.Body.String())
	}
}
