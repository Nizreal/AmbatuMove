package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func TestRateLimitAndCORSBounds(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	limited := rateLimitMiddleware(1, 1, newLimiter(), nil, nil, next)
	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exercises", nil)
	req.RemoteAddr = "127.0.0.1:1000"
	limited.ServeHTTP(first, req)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status=%d", first.Code)
	}
	second := httptest.NewRecorder()
	limited.ServeHTTP(second, req)
	if second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") == "" {
		t.Fatalf("limited status=%d headers=%v", second.Code, second.Header())
	}
	allowed := corsMiddleware([]string{"https://demo.example.test"}, next)
	preflight := httptest.NewRecorder()
	corsReq := httptest.NewRequest(http.MethodOptions, "/api/v1/workouts", nil)
	corsReq.Header.Set("Origin", "https://demo.example.test")
	allowed.ServeHTTP(preflight, corsReq)
	if preflight.Code != http.StatusNoContent || preflight.Header().Get("Access-Control-Allow-Origin") != "https://demo.example.test" {
		t.Fatalf("allowed preflight=%d %v", preflight.Code, preflight.Header())
	}
	denied := httptest.NewRecorder()
	corsReq.Header.Set("Origin", "https://other.example.test")
	allowed.ServeHTTP(denied, corsReq)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("denied preflight=%d", denied.Code)
	}
}

func TestRateLimitTrustsForwardedIPOnlyFromConfiguredProxy(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	newRequest := func(peer, forwarded string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/exercises", nil)
		r.RemoteAddr = peer
		r.Header.Set("X-Forwarded-For", forwarded)
		return r
	}
	trusted := rateLimitMiddleware(1, 1, newLimiter(), []string{"10.20.0.0/16"}, nil, next)
	for _, ip := range []string{"198.51.100.1", "198.51.100.2"} {
		rec := httptest.NewRecorder()
		trusted.ServeHTTP(rec, newRequest("10.20.0.4:8080", ip))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("trusted client %s status=%d", ip, rec.Code)
		}
	}
	untrusted := rateLimitMiddleware(1, 1, newLimiter(), []string{"10.20.0.0/16"}, nil, next)
	first, second := httptest.NewRecorder(), httptest.NewRecorder()
	untrusted.ServeHTTP(first, newRequest("198.51.100.9:3456", "203.0.113.1"))
	untrusted.ServeHTTP(second, newRequest("198.51.100.9:3457", "203.0.113.2"))
	if first.Code != http.StatusNoContent || second.Code != http.StatusTooManyRequests {
		t.Fatalf("untrusted forwarded header changed identity buckets: %d, %d", first.Code, second.Code)
	}
}

func TestPlannerDeadlinePropagatesAndMapsToGatewayTimeout(t *testing.T) {
	handler := plannerDeadlineMiddleware(10*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "planning timeout", nil)
		}
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/workouts/generate", nil))
	if rec.Code != http.StatusGatewayTimeout || !strings.Contains(rec.Body.String(), "PLANNER_TIMEOUT") {
		t.Fatalf("deadline response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestWorkoutGenerationReturnsConsistentTimeoutEnvelope(t *testing.T) {
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "timeout.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err = catalog.SeedReferenceData(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	handler := NewRouter(nil, db, Dependencies{Planning: planning.NewService(catalog.NewService(db)), PlannerMaxGeneration: time.Second})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workouts/generate", strings.NewReader(`{"goal":"general_fitness","workout_type":"strength","duration_minutes":30,"experience_level":"beginner","environment":"home"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusGatewayTimeout || !strings.Contains(rec.Body.String(), `"code":"PLANNER_TIMEOUT"`) {
		t.Fatalf("expired API request status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDocsRoutesServeOpenAPIAndUI(t *testing.T) {
	mux := http.NewServeMux()
	registerDocsRoutes(mux)
	for _, path := range []string{"/openapi.json", "/docs"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, rec.Code)
		}
		if path == "/openapi.json" && rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("openapi content type=%s", rec.Header().Get("Content-Type"))
		}
		if path == "/openapi.json" {
			var spec struct {
				Paths      map[string]map[string]json.RawMessage `json:"paths"`
				Components struct {
					Schemas map[string]json.RawMessage `json:"schemas"`
				} `json:"components"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
				t.Fatalf("runtime OpenAPI is invalid JSON: %v", err)
			}
			for _, required := range []string{"/workouts/generate", "/programs/generate", "/gyms/{gym_id}", "/openapi.json", "/ready"} {
				if _, ok := spec.Paths[required]; !ok {
					t.Errorf("OpenAPI missing runtime path %s", required)
				}
			}
			for _, required := range []string{"WorkoutGenerateRequest", "WorkoutSession", "ProgramGenerateRequest", "Error"} {
				if _, ok := spec.Components.Schemas[required]; !ok {
					t.Errorf("OpenAPI missing schema %s", required)
				}
			}
		}
	}
}
