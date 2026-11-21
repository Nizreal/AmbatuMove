package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ambatumove/core/internal/config"
	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/tracking"
)

func TestFreshApplicationServesCatalogPlanningCaloriesAndGyms(t *testing.T) {
	dbPath := t.TempDir() + "/integration.sqlite"
	cfg, err := config.Load(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return "test", true
		}
		if key == "DATABASE_PATH" {
			return dbPath, true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New() on fresh DB: %v", err)
	}
	defer application.Close()

	checks := []struct {
		method, path, body, want string
	}{
		{http.MethodGet, "/ready", "", `"status":"ready"`},
		{http.MethodGet, "/api/v1/exercises?category=cardio", "", `"id":"act_walk_outdoor"`},
		{http.MethodPost, "/api/v1/workouts/generate", `{"goal":"general_fitness","workout_type":"cardio","duration_minutes":25,"experience_level":"beginner","environment":"outdoor","preferred_activity_ids":["act_walk_outdoor"],"seed":42}`, `"estimated_duration_seconds":1500`},
		{http.MethodPost, "/api/v1/programs/generate", `{"goal":"endurance","experience_level":"beginner","weeks":1,"days_per_week":2,"start_date":"2026-11-23","timezone":"Asia/Jakarta","available_days":["monday","thursday"],"session_duration_minutes":20,"environment":"outdoor","preferred_activity_ids":["act_walk_outdoor"],"seed":42}`, `"kind":"rest"`},
		{http.MethodGet, "/api/v1/calories/estimate?activity=walking&intensity=moderate&duration_minutes=15&weight_kg=70", "", `"estimated_kcal":69.8`},
		{http.MethodGet, "/api/v1/calories/recommendations?target_kcal=200&weight_kg=70&max_duration_minutes=45&environment=outdoor&experience_level=intermediate&category=cardio&max_intensity=moderate&available_equipment=bicycle&excluded_exercise_ids=act_run_outdoor", "", `"exercise_id":"act_cycle_outdoor"`},
		{http.MethodGet, "/api/v1/gyms?latitude=-6.2&longitude=106.8167&radius_km=5&equipment=cable_machine&facility=shower", "", `"gym_demo_jakarta_001"`},
		{http.MethodPost, "/api/v1/workouts/generate", `{"goal":"strength","workout_type":"strength","duration_minutes":30,"experience_level":"beginner","environment":"gym","gym_id":"gym_demo_jakarta_001","seed":43}`, `"workout_type":"strength"`},
		{http.MethodPost, "/api/v1/workouts/generate", `{"goal":"general_fitness","workout_type":"mixed","duration_minutes":40,"experience_level":"beginner","environment":"indoor","available_equipment":["bodyweight","treadmill"],"preferred_activity_ids":["act_walk_treadmill"],"seed":44}`, `"workout_type":"mixed"`},
	}
	for _, path := range []string{
		"/api/v1/calories/recommendations?target_kcal=200&weight_kg=70&unsupported=true",
		"/api/v1/calories/recommendations?target_kcal=200&target_kcal=300&weight_kg=70",
		"/api/v1/calories/recommendations?target_kcal=200&weight_kg=70&max_duration_minutes=0",
		"/api/v1/calories/recommendations?target_kcal=200&weight_kg=70&limit=0",
	} {
		recorder := httptest.NewRecorder()
		application.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"VALIDATION_ERROR"`) {
			t.Fatalf("invalid recommendation query %q: %d %s", path, recorder.Code, recorder.Body.String())
		}
	}
	unknownRecommendationGym := httptest.NewRecorder()
	application.Handler.ServeHTTP(unknownRecommendationGym, httptest.NewRequest(http.MethodGet, "/api/v1/calories/recommendations?target_kcal=200&weight_kg=70&gym_id=missing-gym", nil))
	if unknownRecommendationGym.Code != http.StatusNotFound || !strings.Contains(unknownRecommendationGym.Body.String(), `"code":"RESOURCE_NOT_FOUND"`) {
		t.Fatalf("unknown recommendation gym response=%d %s", unknownRecommendationGym.Code, unknownRecommendationGym.Body.String())
	}
	for _, check := range checks {
		t.Run(check.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(check.method, check.path, strings.NewReader(check.body))
			application.Handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), check.want) {
				t.Fatalf("%s %s status=%d body=%s", check.method, check.path, recorder.Code, recorder.Body.String())
			}
		})
	}
	invalidGym := httptest.NewRecorder()
	application.Handler.ServeHTTP(invalidGym, httptest.NewRequest(http.MethodPost, "/api/v1/workouts/generate", strings.NewReader(`{"goal":"strength","workout_type":"strength","duration_minutes":30,"experience_level":"beginner","environment":"gym","gym_id":"missing-gym"}`)))
	if invalidGym.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidGym.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("unknown gym API response=%d %s", invalidGym.Code, invalidGym.Body.String())
	}
	partnerService, err := identity.NewService(application.DB, []byte("01234567890123456789012345678901"), "ambatumove", "ambatumove-mobile")
	if err != nil {
		t.Fatal(err)
	}
	_, partnerKey, err := partnerService.ProvisionPartnerKey(context.Background(), "calorie-consumer", []string{"calories:estimate"})
	if err != nil {
		t.Fatal(err)
	}
	partnerRequest := httptest.NewRequest(http.MethodGet, "/api/v1/calories/recommendations?target_kcal=100&weight_kg=70", nil)
	partnerRequest.Header.Set("X-API-Key", partnerKey)
	partnerResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(partnerResponse, partnerRequest)
	if partnerResponse.Code != http.StatusOK || !strings.Contains(partnerResponse.Body.String(), `"target_kcal":100`) {
		t.Fatalf("scoped partner could not access calorie recommendations: %d %s", partnerResponse.Code, partnerResponse.Body.String())
	}
}

func TestMixedWorkoutAPIReturnsStrengthAndRequestedCardio(t *testing.T) {
	cfg, err := config.Load(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return "test", true
		}
		if key == "DATABASE_PATH" {
			return t.TempDir() + "/mixed.sqlite", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	rec := httptest.NewRecorder()
	body := `{"goal":"general_fitness","workout_type":"mixed","duration_minutes":40,"experience_level":"beginner","environment":"indoor","available_equipment":["bodyweight","treadmill"],"preferred_activity_ids":["act_walk_treadmill"],"seed":44}`
	application.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/workouts/generate", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("mixed API status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Data struct {
			Sections []struct {
				Items []struct {
					ExerciseID string `json:"exercise_id"`
				} `json:"items"`
			} `json:"sections"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	strength, cardio := false, false
	for _, section := range result.Data.Sections {
		for _, item := range section.Items {
			if item.ExerciseID == "act_walk_treadmill" {
				cardio = true
				continue
			}
			exRec := httptest.NewRecorder()
			application.Handler.ServeHTTP(exRec, httptest.NewRequest(http.MethodGet, "/api/v1/exercises/"+item.ExerciseID, nil))
			var detail struct {
				Data struct {
					Category string `json:"category"`
				} `json:"data"`
			}
			if exRec.Code != http.StatusOK || json.Unmarshal(exRec.Body.Bytes(), &detail) != nil {
				t.Fatalf("exercise detail for %s: %d %s", item.ExerciseID, exRec.Code, exRec.Body.String())
			}
			if detail.Data.Category == "strength" {
				strength = true
			}
		}
	}
	if !strength || !cardio {
		t.Fatalf("mixed API modality coverage: strength=%v cardio=%v body=%s", strength, cardio, rec.Body.String())
	}
}

func TestRateLimitsAuthenticatedUsersIndependentlyOfSharedIP(t *testing.T) {
	cfg, err := config.Load(func(key string) (string, bool) {
		switch key {
		case "APP_ENV":
			return "test", true
		case "DATABASE_PATH":
			return t.TempDir() + "/rate.sqlite", true
		case "PUBLIC_RATE_LIMIT_RPM":
			return "1", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	register := func(email string) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"`+email+`","password":"a sufficiently long password"}`))
		application.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("register status=%d body=%s", rec.Code, rec.Body.String())
		}
		var response struct {
			Data struct {
				AccessToken string `json:"access_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data.AccessToken
	}
	tokens := []string{register("rate-one@example.test"), register("rate-two@example.test")}
	for _, token := range tokens {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "198.51.100.25:32000"
		application.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("independent user bucket status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokens[0])
	req.RemoteAddr = "198.51.100.25:32001"
	application.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("same-user limit status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPartnerRateLimitUsesVerifiedPartnerTier(t *testing.T) {
	cfg, err := config.Load(func(key string) (string, bool) {
		switch key {
		case "APP_ENV":
			return "test", true
		case "DATABASE_PATH":
			return t.TempDir() + "/partner-rate.sqlite", true
		case "PUBLIC_RATE_LIMIT_RPM":
			return "1", true
		case "PARTNER_RATE_LIMIT_RPM":
			return "2", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	provisioner, err := identity.NewService(application.DB, []byte("0123456789abcdef0123456789abcdef"), cfg.JWTIssuer, cfg.JWTAudience)
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := provisioner.ProvisionPartnerKey(context.Background(), "test group", []string{"gyms:read"})
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/gyms", nil)
		req.Header.Set("X-API-Key", key)
		req.RemoteAddr = "198.51.100.30:32000"
		application.Handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("partner request %d status=%d want=%d body=%s", index+1, rec.Code, want, rec.Body.String())
		}
	}
}

func TestAuthenticatedProfileAndSavedWorkoutOwnership(t *testing.T) {
	cfg, err := config.Load(func(key string) (string, bool) {
		switch key {
		case "APP_ENV":
			return "test", true
		case "DATABASE_PATH":
			return t.TempDir() + "/private.sqlite", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		application.Handler.ServeHTTP(rec, req)
		return rec
	}
	registered := call(http.MethodPost, "/api/v1/auth/register", `{"email":"demo@example.test","password":"a sufficiently long password"}`, "")
	if registered.Code != http.StatusCreated {
		t.Fatalf("register=%d %s", registered.Code, registered.Body.String())
	}
	var envelope struct {
		Data struct {
			AccessToken string `json:"access_token"`
			User        struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	if err = json.Unmarshal(registered.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	profile := call(http.MethodPut, "/api/v1/me/fitness-profile", `{"goal":"endurance","experience_level":"beginner","preferred_activity_ids":["act_walk_outdoor"],"equipment":[],"environment":"outdoor","space":"unrestricted","weight_kg":70,"excluded_ids":[]}`, envelope.Data.AccessToken)
	if profile.Code != http.StatusOK {
		t.Fatalf("profile=%d %s", profile.Code, profile.Body.String())
	}
	generated := call(http.MethodPost, "/api/v1/workouts/generate", `{"goal":"general_fitness","workout_type":"cardio","duration_minutes":20,"experience_level":"beginner","environment":"outdoor","preferred_activity_ids":["act_walk_outdoor"],"save":true,"seed":9}`, envelope.Data.AccessToken)
	if generated.Code != http.StatusOK || !strings.Contains(generated.Body.String(), `"saved":true`) {
		t.Fatalf("saved generation=%d %s", generated.Code, generated.Body.String())
	}
	var workout struct {
		Data struct {
			ID       *string `json:"id"`
			Sections []struct {
				Items []struct {
					ItemID     string `json:"item_id"`
					ExerciseID string `json:"exercise_id"`
				} `json:"items"`
			} `json:"sections"`
		} `json:"data"`
	}
	if err = json.Unmarshal(generated.Body.Bytes(), &workout); err != nil || workout.Data.ID == nil {
		t.Fatalf("saved response=%s err=%v", generated.Body.String(), err)
	}
	listed := call(http.MethodGet, "/api/v1/workouts", "", envelope.Data.AccessToken)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), *workout.Data.ID) {
		t.Fatalf("list=%d %s", listed.Code, listed.Body.String())
	}
	if len(workout.Data.Sections) == 0 || len(workout.Data.Sections[0].Items) == 0 {
		t.Fatalf("generated workout has no items: %s", generated.Body.String())
	}
	now := time.Now().UTC()
	completion := tracking.Completion{Status: "completed", StartedAt: now.Add(-20 * time.Minute).Format(time.RFC3339), FinishedAt: now.Format(time.RFC3339), Items: []tracking.PerformedItem{{ItemID: workout.Data.Sections[0].Items[0].ItemID, ExerciseID: workout.Data.Sections[0].Items[0].ExerciseID, ActualDurationSeconds: 60}}}
	payload, _ := json.Marshal(completion)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workouts/"+*workout.Data.ID+"/complete", strings.NewReader(string(payload)))
	request.Header.Set("Authorization", "Bearer "+envelope.Data.AccessToken)
	request.Header.Set("Idempotency-Key", "integration-retry-0001")
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("complete workout=%d %s", response.Code, response.Body.String())
	}
	logs := call(http.MethodGet, "/api/v1/workout-logs", "", envelope.Data.AccessToken)
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), *workout.Data.ID) {
		t.Fatalf("workout logs=%d %s", logs.Code, logs.Body.String())
	}
	unauthorized := call(http.MethodGet, "/api/v1/workouts", "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated private list=%d", unauthorized.Code)
	}
}

func TestFreshApplicationMigrationAndSeedSurviveRestart(t *testing.T) {
	dbPath := t.TempDir() + "/restart.sqlite"
	loadConfig := func() config.Config {
		cfg, err := config.Load(func(key string) (string, bool) {
			if key == "APP_ENV" {
				return "test", true
			}
			if key == "DATABASE_PATH" {
				return dbPath, true
			}
			return "", false
		})
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	first, err := New(context.Background(), loadConfig(), logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := New(context.Background(), loadConfig(), logger)
	if err != nil {
		t.Fatalf("restart on existing DB: %v", err)
	}
	defer second.Close()
	recorder := httptest.NewRecorder()
	second.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/exercises/ex_push_up", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"id":"ex_push_up"`) {
		t.Fatalf("catalog after restart status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
