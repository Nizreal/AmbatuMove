package httpapi

import (
	"net/http"

	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/identity"
)

func registerCacheStatsRoute(mux *http.ServeMux, auth *identity.Service, reference *catalog.Service, gym *gyms.Service) {
	mux.HandleFunc("GET /api/v1/admin/cache-stats", authenticated(auth, true, func(w http.ResponseWriter, r *http.Request) {
		WriteSuccess(w, r, http.StatusOK, map[string]any{"reference": reference.CacheStats(), "gyms": gym.CacheStats()})
	}))
}
