package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ambatumove/core/internal/catalog"
)

func registerCatalogRoutes(mux *http.ServeMux, service *catalog.Service) {
	mux.HandleFunc("GET /api/v1/exercises", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		limit, err := queryInt(query.Get("limit"), 20, 1, 100)
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be between 1 and 100.", nil)
			return
		}
		offset, err := queryInt(query.Get("offset"), 0, 0, 1000000)
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "offset must be a non-negative integer.", nil)
			return
		}
		filters := catalog.Filters{
			Query: query.Get("q"), Category: query.Get("category"), MuscleGroup: query.Get("muscle_group"),
			Equipment: query["equipment"], Difficulty: query.Get("difficulty"), Environment: query.Get("environment"),
			MeasurementType: query.Get("measurement_type"), Sort: query.Get("sort"), Limit: limit, Offset: offset,
		}
		if len(filters.Query) > 120 {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "q must be at most 120 characters.", nil)
			return
		}
		page, err := service.ListExercises(r.Context(), filters)
		if errors.Is(err, catalog.ErrInvalidFilter) {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "One or more catalog filters are invalid.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The catalog could not be loaded.", nil)
			return
		}
		WritePaginated(w, r, page.Items, page.Limit, page.Offset, page.Total)
	})
	mux.HandleFunc("GET /api/v1/exercises/{exercise_id}", func(w http.ResponseWriter, r *http.Request) {
		item, err := service.GetExercise(r.Context(), r.PathValue("exercise_id"))
		if errors.Is(err, catalog.ErrNotFound) {
			WriteError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Exercise was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The exercise could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, item)
	})
	mux.HandleFunc("GET /api/v1/equipment", func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListEquipment(r.Context(), r.URL.Query().Get("category"))
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Equipment could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, items)
	})
	mux.HandleFunc("GET /api/v1/muscle-groups", func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListMuscles(r.Context())
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Muscle groups could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, items)
	})
	mux.HandleFunc("GET /api/v1/exercise-categories", func(w http.ResponseWriter, r *http.Request) {
		WriteSuccess(w, r, http.StatusOK, map[string]any{
			"categories":        catalog.Categories(),
			"measurement_types": []map[string]string{{"id": "repetitions", "label": "Repetitions"}, {"id": "duration", "label": "Duration"}, {"id": "duration_distance", "label": "Duration and distance"}},
		})
	})
}

func queryInt(raw string, fallback, min, max int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return 0, errors.New("out of range")
	}
	return n, nil
}
