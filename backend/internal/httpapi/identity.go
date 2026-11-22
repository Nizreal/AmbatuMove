package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/profiles"
)

type principalContextKey struct{}

func principalFrom(ctx context.Context) identity.Principal {
	principal, _ := ctx.Value(principalContextKey{}).(identity.Principal)
	return principal
}

func authenticated(service *identity.Service, admin bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := bearerPrincipal(r, service)
		if err != nil || (admin && principal.Role != "admin") {
			WriteError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The supplied credentials are invalid or expired.", nil)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
	}
}

func bearerPrincipal(r *http.Request, service *identity.Service) (identity.Principal, error) {
	if service == nil {
		return identity.Principal{}, identity.ErrInvalidCredentials
	}
	header := r.Header.Get("Authorization")
	if len(header) < 8 || header[:7] != "Bearer " {
		return identity.Principal{}, identity.ErrInvalidCredentials
	}
	return service.Authenticate(r.Context(), header[7:])
}

func optionalPrincipal(r *http.Request, service *identity.Service) (identity.Principal, error) {
	if r.Header.Get("Authorization") == "" {
		return identity.Principal{}, nil
	}
	return bearerPrincipal(r, service)
}

func registerIdentityRoutes(mux *http.ServeMux, service *identity.Service, profileService *profiles.Service) {
	mux.HandleFunc("POST /api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, 4096, &body) {
			return
		}
		tokens, err := service.Register(r.Context(), body.Email, body.Password)
		if errors.Is(err, identity.ErrInvalidRequest) {
			WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Email or password does not meet the required format.", nil)
			return
		}
		if errors.Is(err, identity.ErrInvalidCredentials) {
			WriteError(w, r, http.StatusConflict, "ACCOUNT_EXISTS", "An account with this email already exists.", nil)
			return
		}
		if errors.Is(err, identity.ErrHashBusy) {
			WriteError(w, r, http.StatusTooManyRequests, "AUTH_BUSY", "Credential processing is busy; retry shortly.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The account could not be created.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusCreated, tokens)
	})
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, 4096, &body) {
			return
		}
		tokens, err := service.Login(r.Context(), body.Email, body.Password)
		if errors.Is(err, identity.ErrHashBusy) {
			WriteError(w, r, http.StatusTooManyRequests, "AUTH_BUSY", "Credential processing is busy; retry shortly.", nil)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, tokens)
	})
	mux.HandleFunc("POST /api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if !decodeJSON(w, r, 4096, &body) {
			return
		}
		tokens, err := service.Refresh(r.Context(), body.RefreshToken)
		if err != nil {
			WriteError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The refresh credential is invalid, expired, or revoked.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, tokens)
	})
	mux.HandleFunc("POST /api/v1/auth/logout", authenticated(service, false, func(w http.ResponseWriter, r *http.Request) {
		if err := service.Logout(r.Context(), principalFrom(r.Context())); err != nil {
			WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The session could not be revoked.", nil)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/v1/me", authenticated(service, false, func(w http.ResponseWriter, r *http.Request) {
		user, err := service.User(r.Context(), principalFrom(r.Context()).UserID)
		if err != nil {
			WriteError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "User was not found.", nil)
			return
		}
		WriteSuccess(w, r, http.StatusOK, user)
	}))
	if profileService != nil {
		mux.HandleFunc("GET /api/v1/me/equipment", authenticated(service, false, func(w http.ResponseWriter, r *http.Request) {
			preset, err := profileService.GetEquipmentPreset(r.Context(), principalFrom(r.Context()).UserID)
			if err != nil {
				WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The equipment preset could not be loaded.", nil)
				return
			}
			WriteSuccess(w, r, http.StatusOK, preset)
		}))
		mux.HandleFunc("PUT /api/v1/me/equipment", authenticated(service, false, func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				Equipment []string `json:"equipment"`
			}
			if !decodeJSON(w, r, 8<<10, &request) {
				return
			}
			if request.Equipment == nil {
				WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "equipment must be an array, including [] to explicitly clear the preset.", nil)
				return
			}
			if err := profileService.PutEquipmentPreset(r.Context(), principalFrom(r.Context()).UserID, request.Equipment); errors.Is(err, profiles.ErrInvalid) {
				WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Equipment IDs must refer to active catalog equipment and contain at most 32 entries.", nil)
				return
			} else if err != nil {
				WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The equipment preset could not be saved.", nil)
				return
			}
			preset, err := profileService.GetEquipmentPreset(r.Context(), principalFrom(r.Context()).UserID)
			if err != nil {
				WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The equipment preset could not be loaded.", nil)
				return
			}
			WriteSuccess(w, r, http.StatusOK, preset)
		}))
		mux.HandleFunc("GET /api/v1/me/fitness-profile", authenticated(service, false, func(w http.ResponseWriter, r *http.Request) {
			profile, err := profileService.Get(r.Context(), principalFrom(r.Context()).UserID)
			if err != nil {
				WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The fitness profile could not be loaded.", nil)
				return
			}
			WriteSuccess(w, r, http.StatusOK, profile)
		}))
		mux.HandleFunc("PUT /api/v1/me/fitness-profile", authenticated(service, false, func(w http.ResponseWriter, r *http.Request) {
			var profile profiles.Profile
			if !decodeJSON(w, r, 16<<10, &profile) {
				return
			}
			if err := profileService.Put(r.Context(), principalFrom(r.Context()).UserID, profile); errors.Is(err, profiles.ErrInvalid) {
				WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Fitness profile fields are invalid.", nil)
				return
			} else if err != nil {
				WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The fitness profile could not be saved.", nil)
				return
			}
			saved, err := profileService.Get(r.Context(), principalFrom(r.Context()).UserID)
			if err != nil {
				WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The fitness profile could not be loaded.", nil)
				return
			}
			WriteSuccess(w, r, http.StatusOK, saved)
		}))
	}
}
