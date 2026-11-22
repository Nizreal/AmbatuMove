package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ambatumove/core/internal/calories"
	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/coach"
	"github.com/ambatumove/core/internal/config"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/httpapi"
	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/profiles"
	"github.com/ambatumove/core/internal/programs"
	"github.com/ambatumove/core/internal/saved"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/internal/tracking"
	"github.com/ambatumove/core/migrations"
)

type Application struct {
	Config  config.Config
	Logger  *slog.Logger
	DB      *sql.DB
	Handler http.Handler
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Application, error) {
	db, err := storage.OpenSQLite(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := storage.ApplyMigrations(ctx, db, migrations.FS); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply database migrations: %w", err)
	}
	if err := catalog.SeedReferenceData(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("seed reference catalog: %w", err)
	}
	if err := gyms.SeedFacilities(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("seed facility vocabulary: %w", err)
	}
	catalogService := catalog.NewService(db)
	catalogService.SetCacheEnabled(cfg.ReferenceCacheEnabled)
	if err := catalogService.Warm(ctx); err != nil && logger != nil {
		logger.WarnContext(ctx, "reference cache warmup failed; database-backed reads remain enabled", "error", err)
	}
	gymService := gyms.NewService(db)
	gymService.SetCacheEnabled(cfg.GymCacheEnabled)
	if cfg.SeedSyntheticGyms {
		if err := gymService.SeedSyntheticFixtures(ctx); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("seed synthetic gym directory: %w", err)
		}
	} else if err := gymService.DeactivateSynthetic(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("deactivate synthetic gym fixtures: %w", err)
	}
	calorieService := calories.NewService(db, catalogService, gymService)
	if err := calorieService.Seed(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("seed MET profiles: %w", err)
	}
	planningService := planning.NewService(catalogService, gymService)
	programService := programs.NewService(planningService)
	identityService, err := identity.NewService(db, cfg.JWTSigningKey, cfg.JWTIssuer, cfg.JWTAudience)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize identity service: %w", err)
	}
	profileService := profiles.NewService(db)
	savedService := saved.NewService(db)
	trackingService := tracking.NewService(db)
	var aiProvider coach.Provider
	if cfg.AIEnabled && cfg.GeminiAPIKey != "" {
		aiProvider = coach.NewGeminiClient(cfg.GeminiAPIKey, cfg.GeminiModel, time.Duration(cfg.AITimeoutSeconds)*time.Second)
	}
	coachService := coach.NewService(db, aiProvider, catalogService, profileService, savedService, coach.ServiceConfig{Enabled: cfg.AIEnabled, Timeout: time.Duration(cfg.AITimeoutSeconds) * time.Second, MaxOutputTokens: cfg.AIMaxOutputTokens, RateLimitRPM: cfg.AIRateLimitRPM, MaxConcurrent: cfg.AIMaxConcurrentRequests})
	return &Application{Config: cfg, Logger: logger, DB: db, Handler: httpapi.NewRouter(logger, db, httpapi.Dependencies{Catalog: catalogService, Planning: planningService, Programs: programService, Gyms: gymService, Calories: calorieService, Identity: identityService, Profiles: profileService, Saved: savedService, Tracking: trackingService, Coach: coachService, PublicRateLimitRPM: cfg.PublicRateLimitRPM, PartnerRateLimitRPM: cfg.PartnerRateLimitRPM, PlannerConcurrency: 2, PlannerMaxGeneration: time.Duration(cfg.PlannerMaxGenerationMS) * time.Millisecond, TrustedProxyCIDRs: cfg.TrustedProxyCIDRs, CORSAllowedOrigins: cfg.CORSAllowedOrigins})}, nil
}

func (a *Application) Close() error {
	if a.DB == nil {
		return nil
	}
	return a.DB.Close()
}
