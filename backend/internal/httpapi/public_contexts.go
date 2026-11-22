package httpapi

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ambatumove/core/internal/calories"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/profiles"
)

func registerGymRoutes(mux *http.ServeMux, service *gyms.Service, auth *identity.Service) {
	mux.HandleFunc("GET /api/v1/gyms", func(w http.ResponseWriter, r *http.Request) {
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
		latitude, err := optionalFloat(query.Get("latitude"))
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "latitude must be a valid number.", nil)
			return
		}
		longitude, err := optionalFloat(query.Get("longitude"))
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "longitude must be a valid number.", nil)
			return
		}
		radius, err := optionalFloat(query.Get("radius_km"))
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "radius_km must be a valid number.", nil)
			return
		}
		page, err := service.Search(r.Context(), gyms.Search{Query: query.Get("q"), Latitude: latitude, Longitude: longitude, RadiusKm: radius, Equipment: query["equipment"], Facilities: query["facility"], VerificationStatus: query.Get("verification_status"), Limit: limit, Offset: offset})
		if errors.Is(err, gyms.ErrInvalidSearch) {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Gym search filters are invalid.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Gyms could not be loaded.", nil)
			return
		}
		WritePaginated(w, r, page.Items, page.Limit, page.Offset, page.Total)
	})
	mux.HandleFunc("GET /api/v1/gyms/{gym_id}", func(w http.ResponseWriter, r *http.Request) {
		item, err := service.Get(r.Context(), r.PathValue("gym_id"))
		if errors.Is(err, gyms.ErrNotFound) {
			WriteError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Gym was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Gym details could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, item)
	})
	mux.HandleFunc("GET /api/v1/gyms/{gym_id}/equipment", func(w http.ResponseWriter, r *http.Request) {
		items, err := service.Equipment(r.Context(), r.PathValue("gym_id"))
		if errors.Is(err, gyms.ErrNotFound) {
			WriteError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Gym was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Gym equipment could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, items)
	})
	mux.HandleFunc("GET /api/v1/gyms/{gym_id}/facilities", func(w http.ResponseWriter, r *http.Request) {
		items, err := service.Facilities(r.Context(), r.PathValue("gym_id"))
		if errors.Is(err, gyms.ErrNotFound) {
			WriteError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Gym was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Gym facilities could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, items)
	})
	mux.HandleFunc("GET /api/v1/gyms/{gym_id}/reviews", func(w http.ResponseWriter, r *http.Request) {
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
		items, total, err := service.ListApprovedReviews(r.Context(), r.PathValue("gym_id"), limit, offset)
		if errors.Is(err, gyms.ErrNotFound) {
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Gym was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, 500, "INTERNAL_ERROR", "Gym reviews could not be loaded.", nil)
			return
		}
		WritePaginated(w, r, items, limit, offset, total)
	})
	mux.HandleFunc("GET /api/v1/facilities", func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListFacilities(r.Context())
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Facilities could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, items)
	})
	if auth != nil {
		mux.HandleFunc("POST /api/v1/gyms/{gym_id}/reviews", authenticated(auth, false, func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Rating  int    `json:"rating"`
				Comment string `json:"comment"`
			}
			if !decodeJSON(w, r, 8<<10, &req) {
				return
			}
			p, _ := bearerPrincipal(r, auth)
			item, err := service.SubmitReview(r.Context(), p.UserID, r.PathValue("gym_id"), req.Rating, req.Comment)
			if errors.Is(err, gyms.ErrReviewInvalid) {
				WriteError(w, r, 422, "VALIDATION_ERROR", "Rating or comment is invalid.", nil)
				return
			}
			if errors.Is(err, gyms.ErrReviewConflict) {
				WriteError(w, r, 409, "DUPLICATE_REVIEW", "A review already exists for this user and gym.", nil)
				return
			}
			if errors.Is(err, gyms.ErrNotFound) {
				WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Gym was not found.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Gym review could not be submitted.", nil)
				return
			}
			WriteSuccess(w, r, 201, item)
		}))
		mux.HandleFunc("POST /api/v1/me/favorite-gyms", authenticated(auth, false, func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				GymID string `json:"gym_id"`
			}
			if !decodeJSON(w, r, 4096, &req) {
				return
			}
			p, _ := bearerPrincipal(r, auth)
			err := service.AddFavorite(r.Context(), p.UserID, req.GymID)
			if errors.Is(err, gyms.ErrNotFound) {
				WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Gym was not found.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Gym could not be favorited.", nil)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		mux.HandleFunc("GET /api/v1/me/favorite-gyms", authenticated(auth, false, func(w http.ResponseWriter, r *http.Request) {
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
			items, total, err := service.ListFavorites(r.Context(), p.UserID, limit, offset)
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Favorite gyms could not be loaded.", nil)
				return
			}
			WritePaginated(w, r, items, limit, offset, total)
		}))
		mux.HandleFunc("DELETE /api/v1/me/favorite-gyms/{gym_id}", authenticated(auth, false, func(w http.ResponseWriter, r *http.Request) {
			p, _ := bearerPrincipal(r, auth)
			if err := service.RemoveFavorite(r.Context(), p.UserID, r.PathValue("gym_id")); err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Favorite could not be removed.", nil)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		mux.HandleFunc("POST /api/v1/gyms/submissions", authenticated(auth, false, func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				GymID    string         `json:"gym_id"`
				Type     string         `json:"submission_type"`
				Changes  map[string]any `json:"changes"`
				Evidence []string       `json:"evidence"`
			}
			if !decodeJSON(w, r, 32<<10, &req) {
				return
			}
			p, _ := bearerPrincipal(r, auth)
			item, err := service.Submit(r.Context(), p.UserID, req.Type, req.GymID, req.Changes, req.Evidence)
			if errors.Is(err, gyms.ErrSubmissionInvalid) {
				WriteError(w, r, 422, "VALIDATION_ERROR", "Gym submission fields are invalid.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Gym submission could not be stored.", nil)
				return
			}
			WriteSuccess(w, r, http.StatusCreated, item)
		}))
		submissionGet := authenticated(auth, false, func(w http.ResponseWriter, r *http.Request) {
			p, _ := bearerPrincipal(r, auth)
			item, err := service.GetSubmission(r.Context(), r.PathValue("submission_id"), p.UserID, p.Role == "admin")
			if errors.Is(err, gyms.ErrSubmissionNotFound) {
				WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Gym submission was not found.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Gym submission could not be loaded.", nil)
				return
			}
			WriteSuccess(w, r, 200, item)
		})
		mux.HandleFunc("/api/v1/gyms/", func(w http.ResponseWriter, r *http.Request) {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if r.Method == http.MethodGet && len(parts) == 5 && parts[2] == "gyms" && parts[3] == "submissions" {
				r.SetPathValue("submission_id", parts[4])
				submissionGet(w, r)
				return
			}
			http.NotFound(w, r)
		})
		mux.HandleFunc("GET /api/v1/admin/gym-submissions", authenticated(auth, true, func(w http.ResponseWriter, r *http.Request) {
			items, err := service.ListSubmissions(r.Context(), r.URL.Query().Get("status"))
			if errors.Is(err, gyms.ErrSubmissionInvalid) {
				WriteError(w, r, 400, "VALIDATION_ERROR", "Submission status filter is invalid.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Gym submissions could not be loaded.", nil)
				return
			}
			WriteSuccess(w, r, 200, items)
		}))
		mux.HandleFunc("GET /api/v1/admin/gym-reviews", authenticated(auth, true, func(w http.ResponseWriter, r *http.Request) {
			status := r.URL.Query().Get("status")
			if status == "" {
				status = "pending"
			}
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
			items, total, err := service.ListReviewModeration(r.Context(), p.UserID, status, limit, offset)
			if errors.Is(err, gyms.ErrReviewInvalid) {
				WriteError(w, r, 400, "VALIDATION_ERROR", "Review status or pagination is invalid.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Review queue could not be loaded.", nil)
				return
			}
			WritePaginated(w, r, items, limit, offset, total)
		}))
		mux.HandleFunc("POST /api/v1/admin/gym-reviews/{review_id}/moderate", authenticated(auth, true, func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Decision string `json:"decision"`
				Reason   string `json:"reason"`
			}
			if !decodeJSON(w, r, 4096, &req) {
				return
			}
			p, _ := bearerPrincipal(r, auth)
			err := service.ModerateReview(r.Context(), p.UserID, r.PathValue("review_id"), req.Decision, req.Reason)
			if errors.Is(err, gyms.ErrReviewNotFound) {
				WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Review was not found.", nil)
				return
			}
			if errors.Is(err, gyms.ErrReviewInvalid) {
				WriteError(w, r, 422, "VALIDATION_ERROR", "Moderation decision or reason is invalid.", nil)
				return
			}
			if errors.Is(err, gyms.ErrReviewForbidden) {
				WriteError(w, r, 403, "FORBIDDEN", "Administrator access is required.", nil)
				return
			}
			if errors.Is(err, gyms.ErrReviewConflict) {
				WriteError(w, r, 409, "VERSION_CONFLICT", "Review already has this moderation state.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Review could not be moderated.", nil)
				return
			}
			WriteSuccess(w, r, 200, map[string]string{"status": req.Decision})
		}))
		mux.HandleFunc("POST /api/v1/admin/gym-submissions/{submission_id}/review", authenticated(auth, true, func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Decision string `json:"decision"`
				Reason   string `json:"reason"`
			}
			if !decodeJSON(w, r, 4096, &req) {
				return
			}
			p, _ := bearerPrincipal(r, auth)
			err := service.Review(r.Context(), r.PathValue("submission_id"), p.UserID, req.Decision, req.Reason)
			if errors.Is(err, gyms.ErrSubmissionNotFound) {
				WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Pending gym submission was not found.", nil)
				return
			}
			if errors.Is(err, gyms.ErrSubmissionInvalid) {
				WriteError(w, r, 422, "VALIDATION_ERROR", "Review decision or submitted changes are invalid.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Gym submission could not be reviewed.", nil)
				return
			}
			status := "approved"
			if req.Decision == "reject" {
				status = "rejected"
			}
			WriteSuccess(w, r, 200, map[string]string{"status": status})
		}))
		mux.HandleFunc("PATCH /api/v1/admin/gyms/{gym_id}", authenticated(auth, true, func(w http.ResponseWriter, r *http.Request) {
			var changes map[string]any
			if !decodeJSON(w, r, 16<<10, &changes) {
				return
			}
			p, _ := bearerPrincipal(r, auth)
			err := service.AdminUpdate(r.Context(), r.PathValue("gym_id"), p.UserID, changes)
			if errors.Is(err, gyms.ErrNotFound) {
				WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Gym was not found.", nil)
				return
			}
			if errors.Is(err, gyms.ErrSubmissionInvalid) {
				WriteError(w, r, 422, "VALIDATION_ERROR", "Curated gym updates contain unsupported fields.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Gym could not be updated.", nil)
				return
			}
			item, err := service.Get(r.Context(), r.PathValue("gym_id"))
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Updated gym could not be loaded.", nil)
				return
			}
			WriteSuccess(w, r, 200, item)
		}))
	}
}

func registerCalorieRoutes(mux *http.ServeMux, service *calories.Service, auth *identity.Service, profileService *profiles.Service) {
	mux.HandleFunc("GET /api/v1/calories/activities", func(w http.ResponseWriter, r *http.Request) {
		activity := r.URL.Query().Get("activity")
		if activity != "" && activity != "walking" && activity != "running" && activity != "cycling" && activity != "strength_training" && activity != "bodyweight" && activity != "mobility" {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "activity must name a supported activity category.", nil)
			return
		}
		profiles, err := service.ListProfiles(r.Context(), activity)
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Activity references could not be loaded.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, profiles)
	})
	mux.HandleFunc("GET /api/v1/calories/recommendations", func(w http.ResponseWriter, r *http.Request) {
		request, err := parseRecommendationQuery(r.URL.Query())
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Calorie recommendation query parameters are invalid or unsupported.", nil)
			return
		}
		result, err := service.Recommend(r.Context(), request)
		if errors.Is(err, calories.ErrInvalidRequest) {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Calorie recommendation inputs are invalid.", nil)
			return
		}
		if errors.Is(err, calories.ErrGymNotFound) {
			WriteError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "The selected gym was not found.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Activity recommendations could not be calculated.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, result)
	})
	mux.HandleFunc("GET /api/v1/calories/estimate", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		duration, durationErr := strconv.ParseFloat(query.Get("duration_minutes"), 64)
		weight := 0.0
		weightErr := error(nil)
		if query.Get("weight_kg") != "" {
			weight, weightErr = strconv.ParseFloat(query.Get("weight_kg"), 64)
		} else {
			principal, authErr := optionalPrincipal(r, auth)
			if authErr != nil {
				WriteError(w, r, 401, "INVALID_CREDENTIALS", "The supplied credentials are invalid or expired.", nil)
				return
			}
			if principal.UserID != "" && profileService != nil {
				profile, err := profileService.Get(r.Context(), principal.UserID)
				if err != nil {
					WriteError(w, r, 500, "INTERNAL_ERROR", "Fitness preferences could not be loaded.", nil)
					return
				}
				if profile.WeightKg != nil {
					weight = *profile.WeightKg
				} else {
					weightErr = errors.New("weight missing")
				}
			} else {
				weightErr = errors.New("weight missing")
			}
		}
		if durationErr != nil || weightErr != nil {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "duration_minutes and weight_kg must be supplied or available from the signed-in profile.", nil)
			return
		}
		speed, err := optionalFloat(query.Get("speed_kmh"))
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "speed_kmh must be a valid number.", nil)
			return
		}
		pace, err := optionalFloat(query.Get("pace_seconds_per_km"))
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "pace_seconds_per_km must be a valid number.", nil)
			return
		}
		result, err := service.Estimate(r.Context(), calories.EstimateRequest{ActivityID: query.Get("activity_id"), Activity: query.Get("activity"), Intensity: query.Get("intensity"), DurationMinutes: duration, WeightKg: weight, SpeedKmh: speed, PaceSecondsPerKm: pace, EstimateType: query.Get("estimate_type")})
		if errors.Is(err, calories.ErrInvalidRequest) {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Calorie estimate inputs are invalid.", nil)
			return
		}
		if errors.Is(err, calories.ErrUnsupported) {
			code := "UNSUPPORTED_ACTIVITY"
			if query.Get("intensity") != "" || speed != nil || pace != nil {
				code = "UNSUPPORTED_INTENSITY"
			}
			WriteError(w, r, http.StatusUnprocessableEntity, code, "No Compendium activity mapping matches the requested activity and intensity.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The estimate could not be calculated.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/v1/calories/estimate-workout", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == "" {
			if _, err := bearerPrincipal(r, auth); err != nil {
				WriteError(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "A user token or scoped partner key is required.", nil)
				return
			}
		}
		var request struct {
			WeightKg     *float64 `json:"weight_kg"`
			EstimateType string   `json:"estimate_type"`
			Segments     []struct {
				ActivityID      string  `json:"activity_id"`
				DurationMinutes float64 `json:"duration_minutes"`
				Intensity       string  `json:"intensity,omitempty"`
			} `json:"segments"`
		}
		if !decodeJSON(w, r, 128<<10, &request) {
			return
		}
		principal, authErr := optionalPrincipal(r, auth)
		if authErr != nil {
			WriteError(w, r, 401, "INVALID_CREDENTIALS", "The supplied credentials are invalid or expired.", nil)
			return
		}
		if request.WeightKg == nil && principal.UserID != "" && profileService != nil {
			profile, err := profileService.Get(r.Context(), principal.UserID)
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Fitness preferences could not be loaded.", nil)
				return
			}
			request.WeightKg = profile.WeightKg
		}
		if request.WeightKg == nil {
			WriteError(w, r, 422, "VALIDATION_ERROR", "weight_kg is required unless the signed-in profile contains body mass.", nil)
			return
		}
		estimateType := request.EstimateType
		if estimateType == "" {
			estimateType = "gross"
		}
		if len(request.Segments) == 0 || len(request.Segments) > 100 {
			WriteError(w, r, 400, "VALIDATION_ERROR", "segments must contain between 1 and 100 entries.", nil)
			return
		}
		type segmentResult struct {
			ActivityID      string            `json:"activity_id"`
			DurationMinutes float64           `json:"duration_minutes"`
			Estimate        calories.Estimate `json:"estimate"`
		}
		results := make([]segmentResult, 0, len(request.Segments))
		total := 0.0
		for _, segment := range request.Segments {
			estimate, err := service.Estimate(r.Context(), calories.EstimateRequest{ActivityID: segment.ActivityID, Intensity: segment.Intensity, DurationMinutes: segment.DurationMinutes, WeightKg: *request.WeightKg, EstimateType: estimateType})
			if errors.Is(err, calories.ErrInvalidRequest) || errors.Is(err, calories.ErrUnsupported) {
				WriteError(w, r, 422, "VALIDATION_ERROR", "One or more workout segments are unsupported or invalid.", nil)
				return
			}
			if err != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Workout energy estimate could not be calculated.", nil)
				return
			}
			total += estimate.EstimatedKcal
			results = append(results, segmentResult{segment.ActivityID, segment.DurationMinutes, estimate})
		}
		WriteSuccess(w, r, 200, map[string]any{"segments": results, "total_estimated_kcal": math.Round(total*10) / 10, "estimate_type": estimateType, "limitations": []string{"Population-based estimates; individual expenditure may differ."}})
	})
}

func parseRecommendationQuery(query url.Values) (calories.RecommendationRequest, error) {
	allowed := map[string]bool{
		"target_kcal": true, "weight_kg": true, "max_duration_minutes": true,
		"available_equipment": true, "excluded_equipment": true, "gym_id": true,
		"environment": true, "experience_level": true, "category": true,
		"max_intensity": true, "excluded_exercise_ids": true, "limit": true,
	}
	for key := range query {
		if !allowed[key] {
			return calories.RecommendationRequest{}, errors.New("unsupported query parameter")
		}
	}
	single := func(key string, required bool) (string, error) {
		values, exists := query[key]
		if !exists {
			if required {
				return "", errors.New("missing required query parameter")
			}
			return "", nil
		}
		if len(values) != 1 {
			return "", errors.New("query parameter must appear once")
		}
		return values[0], nil
	}
	floatValue := func(key string, required bool) (float64, error) {
		raw, err := single(key, required)
		if err != nil || raw == "" && !required {
			return 0, err
		}
		if raw == "" {
			return 0, errors.New("query parameter must not be empty")
		}
		return strconv.ParseFloat(raw, 64)
	}
	intValue := func(key string) (int, error) {
		raw, err := single(key, false)
		if err != nil {
			return 0, err
		}
		if raw == "" {
			if query.Has(key) {
				return 0, errors.New("integer query parameter must not be empty")
			}
			return 0, err
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, err
		}
		if value < 1 {
			return 0, errors.New("integer query parameter must be positive")
		}
		return value, nil
	}
	target, err := floatValue("target_kcal", true)
	if err != nil {
		return calories.RecommendationRequest{}, err
	}
	weight, err := floatValue("weight_kg", true)
	if err != nil {
		return calories.RecommendationRequest{}, err
	}
	duration, err := intValue("max_duration_minutes")
	if err != nil {
		return calories.RecommendationRequest{}, err
	}
	limit, err := intValue("limit")
	if err != nil {
		return calories.RecommendationRequest{}, err
	}
	getArray := func(key string, max int) ([]string, bool, error) {
		values, exists := query[key]
		if !exists {
			return nil, false, nil
		}
		if len(values) > max {
			return nil, true, errors.New("too many array query values")
		}
		result := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				if len(values) == 1 {
					continue // An explicitly empty list is meaningful for available_equipment.
				}
				return nil, true, errors.New("array query values must not be empty")
			}
			result = append(result, value)
		}
		return result, true, nil
	}
	available, availableSet, err := getArray("available_equipment", 20)
	if err != nil {
		return calories.RecommendationRequest{}, err
	}
	excludedEquipment, _, err := getArray("excluded_equipment", 20)
	if err != nil {
		return calories.RecommendationRequest{}, err
	}
	excludedExercises, _, err := getArray("excluded_exercise_ids", 100)
	if err != nil {
		return calories.RecommendationRequest{}, err
	}
	var fields [5]string
	for index, key := range []string{"gym_id", "environment", "experience_level", "category", "max_intensity"} {
		fields[index], err = single(key, false)
		if err != nil {
			return calories.RecommendationRequest{}, err
		}
	}
	return calories.RecommendationRequest{TargetKcal: target, WeightKg: weight, MaxDurationMinutes: duration,
		AvailableEquipment: available, AvailableEquipmentSet: availableSet, ExcludedEquipment: excludedEquipment,
		GymID: fields[0], Environment: fields[1], ExperienceLevel: fields[2], Category: fields[3], MaxIntensity: fields[4],
		ExcludedExerciseIDs: excludedExercises, Limit: limit}, nil
}

func optionalFloat(raw string) (*float64, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
