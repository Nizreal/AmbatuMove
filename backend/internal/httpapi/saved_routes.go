package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/programs"
	"github.com/ambatumove/core/internal/saved"
)

func registerSavedRoutes(mux *http.ServeMux, planner *planning.Service, programmer *programs.Service, auth *identity.Service, store *saved.Service) {
	if auth == nil || store == nil {
		return
	}
	protect := func(handler http.HandlerFunc) http.HandlerFunc { return authenticated(auth, false, handler) }
	mux.HandleFunc("GET /api/v1/workouts", protect(func(w http.ResponseWriter, r *http.Request) {
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
		q := r.URL.Query()
		filters := saved.WorkoutFilters{Status: q.Get("status"), ProgramID: q.Get("program_id"), From: q.Get("from"), To: q.Get("to")}
		items, total, err := store.ListWorkouts(r.Context(), p.UserID, limit, offset, filters)
		if errors.Is(err, saved.ErrVersion) {
			WriteError(w, r, 400, "VALIDATION_ERROR", "Workout filters are invalid.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workouts could not be loaded.", nil)
			return
		}
		WritePaginated(w, r, items, limit, offset, total)
	}))
	mux.HandleFunc("GET /api/v1/workouts/{workout_id}", protect(func(w http.ResponseWriter, r *http.Request) {
		p, _ := bearerPrincipal(r, auth)
		item, err := store.GetWorkout(r.Context(), p.UserID, r.PathValue("workout_id"))
		if errors.Is(err, saved.ErrNotFound) {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, 200, item)
	}))
	mux.HandleFunc("GET /api/v1/workouts/{workout_id}/revisions", protect(func(w http.ResponseWriter, r *http.Request) {
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
		items, total, err := store.WorkoutRevisions(r.Context(), p.UserID, r.PathValue("workout_id"), limit, offset)
		if errors.Is(err, saved.ErrNotFound) {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout was not found.", nil)
			return
		}
		if errors.Is(err, saved.ErrVersion) {
			WriteError(w, r, 400, "VALIDATION_ERROR", "Revision pagination is invalid.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout revisions could not be loaded.", nil)
			return
		}
		WritePaginated(w, r, items, limit, offset, total)
	}))
	mux.HandleFunc("PATCH /api/v1/workouts/{workout_id}", protect(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ExpectedVersion int    `json:"expected_version"`
			Status          string `json:"status"`
		}
		if !decodeJSON(w, r, 4096, &req) {
			return
		}
		if req.Status != "planned" && req.Status != "cancelled" {
			WriteError(w, r, 422, "VALIDATION_ERROR", "Only planned or cancelled status may be set through metadata updates.", nil)
			return
		}
		p, _ := bearerPrincipal(r, auth)
		id := r.PathValue("workout_id")
		v, err := store.GetWorkout(r.Context(), p.UserID, id)
		if err != nil {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout was not found.", nil)
			return
		}
		if v.Status == "completed" || req.ExpectedVersion != v.Version {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Completed workout facts cannot be changed, or the version is stale.", nil)
			return
		}
		v.Status = req.Status
		updated, err := store.UpdateWorkout(r.Context(), p.UserID, id, v.Version, v, "metadata")
		if errors.Is(err, saved.ErrVersion) {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Workout version has changed.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout metadata could not be updated.", nil)
			return
		}
		WriteSuccess(w, r, 200, updated)
	}))
	mux.HandleFunc("DELETE /api/v1/workouts/{workout_id}", protect(func(w http.ResponseWriter, r *http.Request) {
		p, _ := bearerPrincipal(r, auth)
		err := store.ArchiveWorkout(r.Context(), p.UserID, r.PathValue("workout_id"))
		if errors.Is(err, saved.ErrNotFound) {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout could not be archived.", nil)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/v1/workouts/{workout_id}/adapt", protect(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Changes         planning.ConstraintChanges `json:"changes"`
			ExpectedVersion int                        `json:"expected_version"`
		}
		if !decodeJSON(w, r, 16<<10, &req) {
			return
		}
		p, _ := bearerPrincipal(r, auth)
		current, err := store.GetWorkout(r.Context(), p.UserID, r.PathValue("workout_id"))
		if err != nil {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout was not found.", nil)
			return
		}
		if req.ExpectedVersion != current.Version {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Workout version has changed.", nil)
			return
		}
		result, _, err := planner.AdaptPreview(r.Context(), current, req.Changes)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Workout adaptation exceeded its configured time limit.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 422, "VALIDATION_ERROR", "Workout cannot be adapted to the requested constraints.", nil)
			return
		}
		result.AdaptedWorkout.ID = current.ID
		updated, err := store.UpdateWorkout(r.Context(), p.UserID, r.PathValue("workout_id"), current.Version, result.AdaptedWorkout, "adaptation")
		if errors.Is(err, saved.ErrVersion) {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Workout version has changed.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout could not be adapted.", nil)
			return
		}
		WriteSuccess(w, r, 200, updated)
	}))
	mux.HandleFunc("POST /api/v1/workouts/{workout_id}/items/{item_id}/replace", protect(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ReplacementExerciseID string `json:"replacement_exercise_id"`
			ExpectedVersion       int    `json:"expected_version"`
		}
		if !decodeJSON(w, r, 4096, &req) {
			return
		}
		p, _ := bearerPrincipal(r, auth)
		id := r.PathValue("workout_id")
		v, err := store.GetWorkout(r.Context(), p.UserID, id)
		if err != nil {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout was not found.", nil)
			return
		}
		if req.ExpectedVersion != v.Version {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Workout version has changed.", nil)
			return
		}
		originalID := ""
		found := false
		for si := range v.Sections {
			for ii := range v.Sections[si].Items {
				item := &v.Sections[si].Items[ii]
				if item.ItemID == r.PathValue("item_id") {
					originalID = item.ExerciseID
					found = true
				}
			}
		}
		if !found {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout item was not found.", nil)
			return
		}
		alternatives, altErr := planner.FindAlternatives(r.Context(), originalID, planning.AlternativeRequest{AvailableEquipment: v.Constraints.AvailableEquipment, ExperienceLevel: v.Constraints.ExperienceLevel, Environment: v.Constraints.Environment, Space: v.Constraints.Space, Goal: v.Goal, MaxResults: 10})
		if errors.Is(altErr, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Exercise substitution exceeded its configured time limit.", nil)
			return
		}
		if altErr != nil {
			WriteError(w, r, 422, "VALIDATION_ERROR", "Replacement constraints could not be validated.", nil)
			return
		}
		compatible := false
		for _, candidate := range alternatives.Alternatives {
			if candidate.ExerciseID == req.ReplacementExerciseID {
				compatible = true
				break
			}
		}
		if !compatible {
			WriteError(w, r, 422, "INCOMPATIBLE_REPLACEMENT", "Replacement exercise is not compatible with the workout constraints.", nil)
			return
		}
		for si := range v.Sections {
			for ii := range v.Sections[si].Items {
				if v.Sections[si].Items[ii].ItemID == r.PathValue("item_id") {
					v.Sections[si].Items[ii].ExerciseID = req.ReplacementExerciseID
				}
			}
		}
		updated, err := store.UpdateWorkout(r.Context(), p.UserID, id, v.Version, v, "item_replacement")
		if errors.Is(err, saved.ErrVersion) {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Workout version has changed.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Workout item could not be replaced.", nil)
			return
		}
		WriteSuccess(w, r, 200, updated)
	}))
	mux.HandleFunc("GET /api/v1/programs", protect(func(w http.ResponseWriter, r *http.Request) {
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
		items, total, err := store.ListPrograms(r.Context(), p.UserID, limit, offset)
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Programs could not be loaded.", nil)
			return
		}
		WritePaginated(w, r, items, limit, offset, total)
	}))
	mux.HandleFunc("GET /api/v1/programs/{program_id}", protect(func(w http.ResponseWriter, r *http.Request) {
		p, _ := bearerPrincipal(r, auth)
		item, err := store.GetProgram(r.Context(), p.UserID, r.PathValue("program_id"))
		if errors.Is(err, saved.ErrNotFound) {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Program was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Program could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, 200, item)
	}))
	mux.HandleFunc("POST /api/v1/programs/{program_id}/adapt", protect(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Changes         planning.ConstraintChanges `json:"changes"`
			ExpectedVersion int                        `json:"expected_version"`
		}
		if !decodeJSON(w, r, 16<<10, &req) {
			return
		}
		p, _ := bearerPrincipal(r, auth)
		id := r.PathValue("program_id")
		program, err := store.GetProgram(r.Context(), p.UserID, id)
		if err != nil {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Program was not found.", nil)
			return
		}
		if req.ExpectedVersion != program.Version {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Program version has changed.", nil)
			return
		}
		today := time.Now().UTC().Format("2006-01-02")
		changed := false
		for i := range program.Schedule {
			entry := &program.Schedule[i]
			if entry.Date < today || entry.Session == nil {
				continue
			}
			adapted, _, adaptErr := planner.AdaptPreview(r.Context(), *entry.Session, req.Changes)
			if errors.Is(adaptErr, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Program adaptation exceeded its configured time limit.", nil)
				return
			}
			if adaptErr != nil {
				WriteError(w, r, 422, "UNFEASIBLE_PLAN", "A future session cannot satisfy the updated constraints.", nil)
				return
			}
			entry.Session = &adapted.AdaptedWorkout
			changed = true
		}
		if !changed {
			WriteError(w, r, 409, "NO_FUTURE_SESSIONS", "The program has no future sessions to adapt.", nil)
			return
		}
		updated, err := store.UpdateProgram(r.Context(), p.UserID, id, program.Version, program)
		if errors.Is(err, saved.ErrVersion) {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Program version has changed.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Program could not be adapted.", nil)
			return
		}
		WriteSuccess(w, r, 200, updated)
	}))
	mux.HandleFunc("PATCH /api/v1/programs/{program_id}/schedule", protect(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ExpectedVersion int    `json:"expected_version"`
			FromDate        string `json:"from_date"`
			ToDate          string `json:"to_date"`
		}
		if !decodeJSON(w, r, 4096, &req) {
			return
		}
		p, _ := bearerPrincipal(r, auth)
		id := r.PathValue("program_id")
		program, err := store.GetProgram(r.Context(), p.UserID, id)
		if err != nil {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Program was not found.", nil)
			return
		}
		if req.ExpectedVersion != program.Version {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Program version has changed.", nil)
			return
		}
		from, e1 := time.Parse("2006-01-02", req.FromDate)
		to, e2 := time.Parse("2006-01-02", req.ToDate)
		if e1 != nil || e2 != nil || !to.After(from) || req.FromDate < time.Now().UTC().Format("2006-01-02") {
			WriteError(w, r, 422, "VALIDATION_ERROR", "Schedule dates must identify a future session and a different target date.", nil)
			return
		}
		fromIndex, toIndex := -1, -1
		for i := range program.Schedule {
			if program.Schedule[i].Date == req.FromDate {
				fromIndex = i
			}
			if program.Schedule[i].Date == req.ToDate {
				toIndex = i
			}
		}
		if fromIndex < 0 || toIndex < 0 || program.Schedule[fromIndex].Session == nil || program.Schedule[toIndex].Session != nil {
			WriteError(w, r, 422, "VALIDATION_ERROR", "Move one scheduled workout onto an existing rest day in this program.", nil)
			return
		}
		program.Schedule[fromIndex], program.Schedule[toIndex] = program.Schedule[toIndex], program.Schedule[fromIndex]
		programStart, _ := time.Parse("2006-01-02", program.StartDate)
		for i := range program.Schedule {
			day, _ := time.Parse("2006-01-02", program.Schedule[i].Date)
			program.Schedule[i].Day = strings.ToLower(day.Weekday().String())
			program.Schedule[i].Week = int(day.Sub(programStart)/(7*24*time.Hour)) + 1
		}
		sort.Slice(program.Schedule, func(i, j int) bool { return program.Schedule[i].Date < program.Schedule[j].Date })
		updated, err := store.UpdateProgram(r.Context(), p.UserID, id, program.Version, program)
		if errors.Is(err, saved.ErrVersion) {
			WriteError(w, r, 412, "VERSION_CONFLICT", "Program version has changed.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Program schedule could not be updated.", nil)
			return
		}
		WriteSuccess(w, r, 200, updated)
	}))
	mux.HandleFunc("DELETE /api/v1/programs/{program_id}", protect(func(w http.ResponseWriter, r *http.Request) {
		p, _ := bearerPrincipal(r, auth)
		err := store.ArchiveProgram(r.Context(), p.UserID, r.PathValue("program_id"))
		if errors.Is(err, saved.ErrNotFound) {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Program was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Program could not be archived.", nil)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	_ = programmer
}
