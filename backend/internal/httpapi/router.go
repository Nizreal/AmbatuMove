package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ambatumove/core/internal/calories"
	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/profiles"
	"github.com/ambatumove/core/internal/programs"
	"github.com/ambatumove/core/internal/saved"
	"github.com/ambatumove/core/internal/tracking"
)

const serviceName = "ambatumove-core"

type Pinger interface {
	PingContext(context.Context) error
}

type Dependencies struct {
	Catalog              *catalog.Service
	Planning             *planning.Service
	Programs             *programs.Service
	Gyms                 *gyms.Service
	Calories             *calories.Service
	Identity             *identity.Service
	Profiles             *profiles.Service
	Saved                *saved.Service
	PublicRateLimitRPM   int
	PartnerRateLimitRPM  int
	PlannerConcurrency   int
	Tracking             *tracking.Service
	CORSAllowedOrigins   []string
	TrustedProxyCIDRs    []string
	PlannerMaxGeneration time.Duration
}

func NewRouter(logger *slog.Logger, database Pinger, dependencies ...Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": serviceName})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if database == nil || database.PingContext(ctx) != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "service": serviceName})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "service": serviceName})
	})
	registerDocsRoutes(mux)
	var deps Dependencies
	if len(dependencies) > 0 {
		deps = dependencies[0]
	}
	if deps.PublicRateLimitRPM < 1 {
		deps.PublicRateLimitRPM = 60
	}
	if deps.PartnerRateLimitRPM < 1 {
		deps.PartnerRateLimitRPM = 120
	}
	if deps.Catalog != nil {
		registerCatalogRoutes(mux, deps.Catalog)
	}
	if deps.Planning != nil {
		registerPlanningRoutes(mux, deps.Planning, deps.Identity, deps.Profiles, deps.Saved)
	}
	if deps.Programs != nil {
		registerProgramRoutes(mux, deps.Programs, deps.Identity, deps.Profiles, deps.Saved)
	}
	if deps.Gyms != nil {
		registerGymRoutes(mux, deps.Gyms, deps.Identity)
	}
	if deps.Calories != nil {
		registerCalorieRoutes(mux, deps.Calories, deps.Identity, deps.Profiles)
	}
	if deps.Identity != nil {
		registerIdentityRoutes(mux, deps.Identity, deps.Profiles)
	}
	if deps.Saved != nil && deps.Identity != nil {
		registerSavedRoutes(mux, deps.Planning, deps.Programs, deps.Identity, deps.Saved)
	}
	if deps.Tracking != nil && deps.Identity != nil {
		registerTrackingRoutes(mux, deps.Identity, deps.Tracking)
	}
	if deps.Identity != nil && deps.Catalog != nil && deps.Gyms != nil {
		registerCacheStatsRoute(mux, deps.Identity, deps.Catalog, deps.Gyms)
	}
	var handler http.Handler = mux
	handler = plannerLimitMiddleware(deps.PlannerConcurrency, handler)
	handler = plannerDeadlineMiddleware(deps.PlannerMaxGeneration, handler)
	handler = rateLimitMiddleware(deps.PublicRateLimitRPM, deps.PartnerRateLimitRPM, newLimiter(), deps.TrustedProxyCIDRs, deps.Identity, handler)
	handler = partnerAccessMiddleware(deps.Identity, handler)
	handler = corsMiddleware(deps.CORSAllowedOrigins, handler)
	return chain(handler, requestIDMiddleware, securityHeaders, accessLog(logger), recoverPanic(logger))
}

type middleware func(http.Handler) http.Handler

func chain(next http.Handler, middleware ...middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		next = middleware[i](next)
	}
	return next
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" || len(id) > 128 || strings.ContainsAny(id, "\r\n") {
			var raw [16]byte
			if _, err := rand.Read(raw[:]); err == nil {
				id = hex.EncodeToString(raw[:])
			} else {
				id = "request-id-unavailable"
			}
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(withRequestID(r.Context(), id)))
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func accessLog(logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			wrapped := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(wrapped, r)
			if logger != nil {
				status := wrapped.status
				if status == 0 {
					status = http.StatusOK
				}
				logger.InfoContext(r.Context(), "http request", "request_id", requestID(r), "method", r.Method, "path", r.URL.Path, "status", status, "duration_ms", time.Since(started).Milliseconds())
			}
		})
	}
}

func recoverPanic(logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if logger != nil {
						logger.ErrorContext(r.Context(), "request panic", "request_id", requestID(r))
					}
					WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
