package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/planning"
	"github.com/ambatumove/core/internal/profiles"
	"github.com/ambatumove/core/internal/programs"
	"github.com/ambatumove/core/internal/saved"
)

func registerPlanningRoutes(mux *http.ServeMux, service *planning.Service, auth *identity.Service, profileService *profiles.Service, savedService *saved.Service) {
	mux.HandleFunc("POST /api/v1/workouts/generate", func(w http.ResponseWriter, r *http.Request) {
		var request planning.GenerateRequest
		if !decodeJSON(w, r, 128<<10, &request) {
			return
		}
		principal, authErr := optionalPrincipal(r, auth)
		if authErr != nil {
			WriteError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The supplied credentials are invalid or expired.", nil)
			return
		}
		if principal.UserID != "" && profileService != nil {
			profile, profileErr := profileService.Get(r.Context(), principal.UserID)
			if profileErr != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Fitness preferences could not be loaded.", nil)
				return
			}
			if request.AvailableEquipment == nil {
				preset, presetErr := profileService.GetEquipmentPreset(r.Context(), principal.UserID)
				if presetErr != nil {
					WriteError(w, r, 500, "INTERNAL_ERROR", "Equipment preferences could not be loaded.", nil)
					return
				}
				if preset.Configured {
					request.AvailableEquipment = preset.Equipment
				}
			}
			applyWorkoutDefaults(&request, profile)
		}
		if request.Save && (auth == nil || savedService == nil) {
			WriteError(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication is required to save a workout.", nil)
			return
		}
		session, failure, err := service.Generate(r.Context(), request)
		if errors.Is(err, planning.ErrInvalidRequest) {
			WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Workout request constraints are invalid.", nil)
			return
		}
		if errors.Is(err, planning.ErrUnfeasible) {
			WriteError(w, r, http.StatusUnprocessableEntity, "UNFEASIBLE_PLAN", "No compatible activity fits the current constraints.", failure)
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Workout generation exceeded its configured time limit.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The workout could not be generated.", nil)
			return
		}
		if request.Save {
			if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Workout generation exceeded its configured time limit.", nil)
				return
			}
			principal, authErr := bearerPrincipal(r, auth)
			if authErr != nil {
				WriteError(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication is required to save a workout.", nil)
				return
			}
			session, err = savedService.SaveWorkout(r.Context(), principal.UserID, session)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
					WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Workout generation exceeded its configured time limit.", nil)
					return
				}
				WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The workout could not be saved.", nil)
				return
			}
		}
		WriteSuccess(w, r, http.StatusOK, session)
	})
	mux.HandleFunc("POST /api/v1/workouts/adapt-preview", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Workout planning.Session           `json:"workout"`
			Changes planning.ConstraintChanges `json:"changes"`
		}
		if !decodeJSON(w, r, 128<<10, &request) {
			return
		}
		result, failure, err := service.AdaptPreview(r.Context(), request.Workout, request.Changes)
		if errors.Is(err, planning.ErrInvalidRequest) {
			WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Workout adaptation constraints are invalid.", nil)
			return
		}
		if errors.Is(err, planning.ErrUnfeasible) {
			WriteError(w, r, http.StatusUnprocessableEntity, "UNFEASIBLE_PLAN", "The workout cannot be adapted to the new constraints.", failure)
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Workout adaptation exceeded its configured time limit.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The workout could not be adapted.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/v1/exercises/{exercise_id}/alternatives", func(w http.ResponseWriter, r *http.Request) {
		var request planning.AlternativeRequest
		if !decodeJSON(w, r, 128<<10, &request) {
			return
		}
		result, err := service.FindAlternatives(r.Context(), r.PathValue("exercise_id"), request)
		if errors.Is(err, planning.ErrNotFound) {
			WriteError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Exercise was not found.", nil)
			return
		}
		if errors.Is(err, planning.ErrInvalidRequest) {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Alternative constraints are invalid.", nil)
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Exercise alternative search exceeded its configured time limit.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Alternatives could not be calculated.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, result)
	})
}

func registerProgramRoutes(mux *http.ServeMux, service *programs.Service, auth *identity.Service, profileService *profiles.Service, savedService *saved.Service) {
	mux.HandleFunc("POST /api/v1/programs/generate", func(w http.ResponseWriter, r *http.Request) {
		var request programs.GenerateRequest
		if !decodeJSON(w, r, 128<<10, &request) {
			return
		}
		principal, authErr := optionalPrincipal(r, auth)
		if authErr != nil {
			WriteError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The supplied credentials are invalid or expired.", nil)
			return
		}
		if principal.UserID != "" && profileService != nil {
			profile, profileErr := profileService.Get(r.Context(), principal.UserID)
			if profileErr != nil {
				WriteError(w, r, 500, "INTERNAL_ERROR", "Fitness preferences could not be loaded.", nil)
				return
			}
			if request.AvailableEquipment == nil {
				preset, presetErr := profileService.GetEquipmentPreset(r.Context(), principal.UserID)
				if presetErr != nil {
					WriteError(w, r, 500, "INTERNAL_ERROR", "Equipment preferences could not be loaded.", nil)
					return
				}
				if preset.Configured {
					request.AvailableEquipment = preset.Equipment
				}
			}
			applyProgramDefaults(&request, profile)
		}
		if request.Save && (auth == nil || savedService == nil) {
			WriteError(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication is required to save a program.", nil)
			return
		}
		program, err := service.Generate(r.Context(), request)
		if errors.Is(err, programs.ErrInvalidRequest) {
			WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Program request constraints are invalid.", invalidRequestDetails(err))
			return
		}
		if errors.Is(err, programs.ErrUnfeasible) {
			WriteError(w, r, http.StatusUnprocessableEntity, "UNFEASIBLE_PLAN", "Program constraints cannot be satisfied.", map[string]string{"reason": err.Error()})
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Program generation exceeded its configured time limit.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The program could not be generated.", nil)
			return
		}
		if request.Save {
			if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Program generation exceeded its configured time limit.", nil)
				return
			}
			principal, authErr := bearerPrincipal(r, auth)
			if authErr != nil {
				WriteError(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication is required to save a program.", nil)
				return
			}
			program, err = savedService.SaveProgram(r.Context(), principal.UserID, program, request.DaysPerWeek)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
					WriteError(w, r, http.StatusGatewayTimeout, "PLANNER_TIMEOUT", "Program generation exceeded its configured time limit.", nil)
					return
				}
				WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The program could not be saved.", nil)
				return
			}
		}
		WriteSuccess(w, r, http.StatusOK, program)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			WriteError(w, r, http.StatusRequestEntityTooLarge, "VALIDATION_ERROR", "Request body is too large.", nil)
		} else {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Request body must contain valid JSON.", nil)
		}
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Request body must contain one JSON value.", nil)
		return false
	}
	return true
}

func invalidRequestDetails(err error) any {
	return map[string]string{"reason": fmt.Sprint(err)}
}

func applyWorkoutDefaults(request *planning.GenerateRequest, p profiles.Profile) {
	if request.Goal == "" {
		request.Goal = p.Goal
	}
	if request.ExperienceLevel == "" {
		request.ExperienceLevel = p.ExperienceLevel
	}
	if request.Environment == "" {
		request.Environment = p.Environment
	}
	if request.Space == "" {
		request.Space = p.Space
	}
	if request.AvailableEquipment == nil {
		request.AvailableEquipment = append([]string{}, p.Equipment...)
	}
	if request.PreferredActivityIDs == nil {
		request.PreferredActivityIDs = append([]string{}, p.PreferredActivityIDs...)
	}
	if request.ExcludedExerciseIDs == nil {
		request.ExcludedExerciseIDs = append([]string{}, p.ExcludedIDs...)
	}
}
func applyProgramDefaults(request *programs.GenerateRequest, p profiles.Profile) {
	if request.Goal == "" {
		request.Goal = p.Goal
	}
	if request.ExperienceLevel == "" {
		request.ExperienceLevel = p.ExperienceLevel
	}
	if request.Environment == "" {
		request.Environment = p.Environment
	}
	if request.Space == "" {
		request.Space = p.Space
	}
	if request.AvailableEquipment == nil {
		request.AvailableEquipment = append([]string{}, p.Equipment...)
	}
	if request.PreferredActivityIDs == nil {
		request.PreferredActivityIDs = append([]string{}, p.PreferredActivityIDs...)
	}
	if request.ExcludedExerciseIDs == nil {
		request.ExcludedExerciseIDs = append([]string{}, p.ExcludedIDs...)
	}
}
