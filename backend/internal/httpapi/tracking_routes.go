package httpapi

import (
	"errors"
	"net/http"

	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/tracking"
)

func registerTrackingRoutes(mux *http.ServeMux, auth *identity.Service, service *tracking.Service) {
	protect := func(handler http.HandlerFunc) http.HandlerFunc { return authenticated(auth, false, handler) }
	mux.HandleFunc("POST /api/v1/workouts/{workout_id}/complete", protect(func(w http.ResponseWriter, r *http.Request) {
		var request tracking.Completion
		if !decodeJSON(w, r, 128<<10, &request) {
			return
		}
		principal, _ := bearerPrincipal(r, auth)
		log, err := service.Complete(r.Context(), principal.UserID, r.PathValue("workout_id"), r.Header.Get("Idempotency-Key"), request)
		switch {
		case errors.Is(err, tracking.ErrInvalid):
			WriteError(w, r, 422, "VALIDATION_ERROR", "Workout completion fields are invalid.", nil)
		case errors.Is(err, tracking.ErrNotFound):
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout was not found.", nil)
		case errors.Is(err, tracking.ErrConflict):
			WriteError(w, r, 409, "IDEMPOTENCY_CONFLICT", "This idempotency key was already used with different content.", nil)
		case err != nil:
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout completion could not be recorded.", nil)
		default:
			WriteSuccess(w, r, http.StatusCreated, log)
		}
	}))
	mux.HandleFunc("GET /api/v1/workout-logs", protect(func(w http.ResponseWriter, r *http.Request) {
		limit, err := queryInt(r.URL.Query().Get("limit"), 20, 1, 100)
		if err != nil {
			WriteError(w, r, 400, "VALIDATION_ERROR", "limit must be between 1 and 100.", nil)
			return
		}
		offset, err := queryInt(r.URL.Query().Get("offset"), 0, 0, 1000000)
		if err != nil {
			WriteError(w, r, 400, "VALIDATION_ERROR", "offset must be a non-negative integer.", nil)
			return
		}
		p, _ := bearerPrincipal(r, auth)
		items, total, err := service.List(r.Context(), p.UserID, limit, offset)
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout history could not be loaded.", nil)
			return
		}
		WritePaginated(w, r, items, limit, offset, total)
	}))
	mux.HandleFunc("GET /api/v1/workout-logs/{log_id}", protect(func(w http.ResponseWriter, r *http.Request) {
		p, _ := bearerPrincipal(r, auth)
		item, err := service.Get(r.Context(), p.UserID, r.PathValue("log_id"))
		if errors.Is(err, tracking.ErrNotFound) {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout log was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout log could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, 200, item)
	}))
	mux.HandleFunc("GET /api/v1/stats/summary", protect(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		from, to := q.Get("from"), q.Get("to")
		if from == "" || to == "" {
			WriteError(w, r, 400, "VALIDATION_ERROR", "from and to dates are required.", nil)
			return
		}
		p, _ := bearerPrincipal(r, auth)
		result, err := service.Summary(r.Context(), p.UserID, from, to, q.Get("timezone"), q.Get("period"))
		if errors.Is(err, tracking.ErrInvalid) {
			WriteError(w, r, 400, "VALIDATION_ERROR", "Date range is invalid or exceeds 366 days.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Activity summary could not be calculated.", nil)
			return
		}
		WriteSuccess(w, r, 200, result)
	}))
}
