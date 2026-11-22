package httpapi

import (
	"errors"
	"net/http"

	"github.com/ambatumove/core/internal/coach"
	"github.com/ambatumove/core/internal/identity"
)

func registerCoachRoutes(mux *http.ServeMux, auth *identity.Service, service *coach.Service) {
	mux.HandleFunc("POST /api/v1/coach/chat", authenticated(auth, false, func(w http.ResponseWriter, r *http.Request) {
		var request coach.ChatRequest
		if !decodeJSON(w, r, 16<<10, &request) {
			return
		}
		principal, _ := bearerPrincipal(r, auth)
		result, err := service.Chat(r.Context(), principal.UserID, requestID(r), request)
		if err == nil {
			WriteSuccess(w, r, http.StatusOK, result)
			return
		}
		if retryAfter := coach.RetryAfterSeconds(coach.ProviderRetryAfter(err)); retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		switch {
		case errors.Is(err, coach.ErrInvalid):
			WriteError(w, r, 422, "VALIDATION_ERROR", "Coach request fields are invalid.", nil)
		case errors.Is(err, coach.ErrConsentRequired):
			WriteError(w, r, 422, "AI_CONSENT_REQUIRED", "Explicit consent is required before sending the request to an external AI provider.", nil)
		case errors.Is(err, coach.ErrNotFound):
			WriteError(w, r, 404, "RESOURCE_NOT_FOUND", "Workout context was not found.", nil)
		case errors.Is(err, coach.ErrRateLimited), errors.Is(err, coach.ErrBusy):
			WriteError(w, r, 429, "RATE_LIMITED", "AI request limit reached; retry later.", nil)
		case errors.Is(err, coach.ErrProviderQuota):
			WriteError(w, r, 429, "AI_QUOTA_EXHAUSTED", "The AI provider quota is temporarily exhausted.", nil)
		case errors.Is(err, coach.ErrTimeout):
			WriteError(w, r, 504, "AI_TIMEOUT", "The AI provider request timed out.", nil)
		case errors.Is(err, coach.ErrMalformedResponse):
			WriteError(w, r, 502, "AI_PROVIDER_ERROR", "The AI provider returned an unusable response.", nil)
		case errors.Is(err, coach.ErrProviderConfig), errors.Is(err, coach.ErrUnavailable):
			WriteError(w, r, 503, "AI_UNAVAILABLE", "The exercise coach is currently unavailable.", nil)
		default:
			WriteError(w, r, 500, "INTERNAL_ERROR", "The exercise coach request failed.", nil)
		}
	}))
}
