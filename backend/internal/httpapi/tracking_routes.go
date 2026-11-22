package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/tracking"
)

func registerTrackingRoutes(mux *http.ServeMux, auth *identity.Service, service *tracking.Service) {
	protect := func(handler http.HandlerFunc) http.HandlerFunc { return authenticated(auth, false, handler) }
	mux.HandleFunc("POST /api/v1/workout-logs/manual", protect(func(w http.ResponseWriter, r *http.Request) {
		var request tracking.ManualRequest
		if !decodeJSON(w, r, 64<<10, &request) {
			return
		}
		p, _ := bearerPrincipal(r, auth)
		log, err := service.CreateManual(r.Context(), p.UserID, r.Header.Get("Idempotency-Key"), request)
		switch {
		case errors.Is(err, tracking.ErrInvalid):
			WriteError(w, r, 422, "VALIDATION_ERROR", "Manual activity fields or measurements are invalid.", nil)
		case errors.Is(err, tracking.ErrConflict):
			WriteError(w, r, 409, "IDEMPOTENCY_CONFLICT", "This idempotency key was already used with different content.", nil)
		case err != nil:
			WriteError(w, r, 500, "INTERNAL_ERROR", "Manual activity could not be recorded.", nil)
		default:
			WriteSuccess(w, r, http.StatusCreated, log)
		}
	}))
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
	mux.HandleFunc("PATCH /api/v1/workout-logs/{log_id}", protect(func(w http.ResponseWriter, r *http.Request) {
		var raw struct {
			ExpectedVersion int             `json:"expected_version"`
			Reason          string          `json:"reason"`
			Notes           json.RawMessage `json:"notes"`
			Status          json.RawMessage `json:"status"`
			StartedAt       json.RawMessage `json:"started_at"`
			FinishedAt      json.RawMessage `json:"finished_at"`
			Items           json.RawMessage `json:"items"`
		}
		if !decodeJSON(w, r, 128<<10, &raw) {
			return
		}
		request := tracking.Correction{ExpectedVersion: raw.ExpectedVersion, Reason: raw.Reason}
		for _, field := range []struct {
			name   string
			raw    json.RawMessage
			target **string
		}{{"notes", raw.Notes, &request.Notes}, {"status", raw.Status, &request.Status}, {"started_at", raw.StartedAt, &request.StartedAt}, {"finished_at", raw.FinishedAt, &request.FinishedAt}} {
			if len(field.raw) == 0 {
				continue
			}
			if string(field.raw) == "null" {
				WriteError(w, r, 422, "VALIDATION_ERROR", field.name+" must be omitted or a string.", nil)
				return
			}
			var value string
			if err := json.Unmarshal(field.raw, &value); err != nil {
				WriteError(w, r, 400, "VALIDATION_ERROR", field.name+" must be a string.", nil)
				return
			}
			*field.target = &value
		}
		if len(raw.Items) > 0 {
			if string(raw.Items) == "null" {
				WriteError(w, r, 422, "VALIDATION_ERROR", "items must be omitted or an array.", nil)
				return
			}
			var items []tracking.PerformedItem
			if err := json.Unmarshal(raw.Items, &items); err != nil {
				WriteError(w, r, 400, "VALIDATION_ERROR", "items must be a valid array.", nil)
				return
			}
			request.Items = &items
		}
		p, _ := bearerPrincipal(r, auth)
		result, err := service.Correct(r.Context(), p.UserID, r.PathValue("log_id"), request)
		switch {
		case errors.Is(err, tracking.ErrInvalid):
			WriteError(w, r, 422, "VALIDATION_ERROR", "Correction fields are invalid or make the log inconsistent.", nil)
		case errors.Is(err, tracking.ErrNotFound):
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout log was not found.", nil)
		case errors.Is(err, tracking.ErrVersion):
			WriteError(w, r, 412, "VERSION_CONFLICT", "Workout log version has changed.", nil)
		case err != nil:
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout log could not be corrected.", nil)
		default:
			WriteSuccess(w, r, 200, result)
		}
	}))
	mux.HandleFunc("GET /api/v1/workout-logs/{log_id}/corrections", protect(func(w http.ResponseWriter, r *http.Request) {
		limit, err := queryInt(r.URL.Query().Get("limit"), 20, 1, 100)
		if err != nil {
			WriteError(w, r, 400, "VALIDATION_ERROR", "limit must be between 1 and 100.", nil)
			return
		}
		offset, err := queryInt(r.URL.Query().Get("offset"), 0, 0, 1000000)
		if err != nil {
			WriteError(w, r, 400, "VALIDATION_ERROR", "offset must be non-negative.", nil)
			return
		}
		p, _ := bearerPrincipal(r, auth)
		items, total, err := service.CorrectionHistory(r.Context(), p.UserID, r.PathValue("log_id"), limit, offset)
		if errors.Is(err, tracking.ErrNotFound) {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout log was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Correction history could not be loaded.", nil)
			return
		}
		WritePaginated(w, r, items, limit, offset, total)
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
	mux.HandleFunc("GET /api/v1/me/progression", protect(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if len(q) > 4 {
			WriteError(w, r, 400, "VALIDATION_ERROR", "Unsupported progression query parameter.", nil)
			return
		}
		days, err := queryInt(q.Get("days"), 90, 28, 365)
		if err != nil {
			WriteError(w, r, 400, "VALIDATION_ERROR", "days must be between 28 and 365.", nil)
			return
		}
		limit, err := queryInt(q.Get("limit"), 20, 1, 50)
		if err != nil {
			WriteError(w, r, 400, "VALIDATION_ERROR", "limit must be between 1 and 50.", nil)
			return
		}
		exercise := q.Get("exercise_id")
		if len(exercise) > 80 {
			WriteError(w, r, 400, "VALIDATION_ERROR", "exercise_id is too long.", nil)
			return
		}
		timezone := q.Get("timezone")
		if timezone == "" {
			timezone = "UTC"
		}
		if len(timezone) > 100 {
			WriteError(w, r, 400, "VALIDATION_ERROR", "timezone is invalid.", nil)
			return
		}
		p, _ := bearerPrincipal(r, auth)
		result, err := service.Progression(r.Context(), p.UserID, days, exercise, timezone, limit)
		if errors.Is(err, tracking.ErrHistoryLimit) {
			WriteError(w, r, 413, "HISTORY_LIMIT_EXCEEDED", "The bounded analytics history limit was exceeded.", nil)
			return
		}
		if errors.Is(err, tracking.ErrInvalid) {
			WriteError(w, r, 400, "VALIDATION_ERROR", "Progression parameters are invalid.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Progression could not be calculated.", nil)
			return
		}
		WriteSuccess(w, r, 200, result)
	}))
	mux.HandleFunc("GET /api/v1/stats/trends", protect(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if len(q) > 2 {
			WriteError(w, r, 400, "VALIDATION_ERROR", "Unsupported trends query parameter.", nil)
			return
		}
		days, err := queryInt(q.Get("days"), 90, 7, 365)
		if err != nil {
			WriteError(w, r, 400, "VALIDATION_ERROR", "days must be between 7 and 365.", nil)
			return
		}
		timezone := q.Get("timezone")
		if timezone == "" {
			timezone = "UTC"
		}
		if len(timezone) > 100 {
			WriteError(w, r, 400, "VALIDATION_ERROR", "timezone is invalid.", nil)
			return
		}
		p, _ := bearerPrincipal(r, auth)
		result, err := service.Trends(r.Context(), p.UserID, days, timezone)
		if errors.Is(err, tracking.ErrHistoryLimit) {
			WriteError(w, r, 413, "HISTORY_LIMIT_EXCEEDED", "The bounded analytics history limit was exceeded.", nil)
			return
		}
		if errors.Is(err, tracking.ErrInvalid) {
			WriteError(w, r, 400, "VALIDATION_ERROR", "Trend parameters are invalid.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Activity trends could not be calculated.", nil)
			return
		}
		WriteSuccess(w, r, 200, result)
	}))
}
