package httpapi

import (
	"context"
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
	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/profiles"
	"github.com/ambatumove/core/internal/saved"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

type routeCoachProvider struct{ calls *int }

func (routeCoachProvider) Name() string { return "mock" }
func (p routeCoachProvider) Generate(context.Context, string, []coach.Message, int) (coach.ProviderResult, error) {
	if p.calls != nil {
		*p.calls++
	}
	return coach.ProviderResult{Text: "Use a controlled range and pause briefly at the top.", Model: "mock-v1"}, nil
}

func TestCoachRouteRequiresUserJWTConsentAndReturnsEnvelope(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "coach-api.sqlite"))
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
	auth, err := identity.NewService(db, []byte("01234567890123456789012345678901"), "ambatumove", "ambatumove-mobile")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.Register(ctx, "coach@example.test", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	_, partnerKey, err := auth.ProvisionPartnerKey(ctx, "consumer", []string{"catalog:read"})
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	provider := &routeCoachProvider{calls: &providerCalls}
	coachService := coach.NewService(db, provider, catalog.NewService(db), profiles.NewService(db), saved.NewService(db), coach.ServiceConfig{Enabled: true, Timeout: time.Second, MaxOutputTokens: 128, RateLimitRPM: 5, MaxConcurrent: 1})
	handler := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), db, Dependencies{Identity: auth, Catalog: catalog.NewService(db), Gyms: nil, Coach: coachService})
	call := func(token, key, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/coach/chat", strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if key != "" {
			req.Header.Set("X-API-Key", key)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if got := call("", "", `{"message":"form question","consent_external_processing":true}`); got.Code != 401 {
		t.Fatalf("anonymous status=%d %s", got.Code, got.Body.String())
	}
	if got := call("", partnerKey, `{"message":"form question","consent_external_processing":true}`); got.Code != 401 {
		t.Fatalf("partner access status=%d %s", got.Code, got.Body.String())
	}
	got := call(tokens.AccessToken, "", `{"message":"My wrist hurts during push-ups."}`)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"safety_classification":"caution"`) || strings.Contains(got.Body.String(), `"provider"`) || providerCalls != 0 {
		t.Fatalf("nonconsented caution status=%d calls=%d %s", got.Code, providerCalls, got.Body.String())
	}
	if got := call(tokens.AccessToken, "", `{"message":"form question"}`); got.Code != 422 || !strings.Contains(got.Body.String(), "AI_CONSENT_REQUIRED") {
		t.Fatalf("missing consent status=%d %s", got.Code, got.Body.String())
	}
	got = call(tokens.AccessToken, "", `{"message":"Why do I feel my biceps?","exercise_id":"ex_lat_pulldown","consent_external_processing":true}`)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"provider":"mock"`) || !strings.Contains(got.Body.String(), `"request_id"`) {
		t.Fatalf("coach response status=%d %s", got.Code, got.Body.String())
	}
	if providerCalls != 1 {
		t.Fatalf("general request provider calls=%d, want 1", providerCalls)
	}
	got = call(tokens.AccessToken, "", `{"message":"My wrist hurts during push-ups.","consent_external_processing":true}`)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"safety_classification":"caution"`) || !strings.Contains(got.Body.String(), "Stop the movement that causes pain") || !strings.Contains(got.Body.String(), `"provider":"mock"`) {
		t.Fatalf("consented caution route status=%d %s", got.Code, got.Body.String())
	}
	if providerCalls != 2 {
		t.Fatalf("caution request provider calls=%d, want 2", providerCalls)
	}
	got = call(tokens.AccessToken, "", `{"message":"I have severe chest pain right now."}`)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"safety_classification":"urgent"`) || providerCalls != 2 {
		t.Fatalf("urgent route status=%d calls=%d %s", got.Code, providerCalls, got.Body.String())
	}
}
