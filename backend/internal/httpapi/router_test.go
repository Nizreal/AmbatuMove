package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) PingContext(context.Context) error { return f.err }

func TestHealthDoesNotRequireDatabase(t *testing.T) {
	handler := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "{\"service\":\"ambatumove-core\",\"status\":\"ok\"}\n" {
		t.Fatalf("health response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestReadinessReturnsUnavailableWhenDatabaseCannotPing(t *testing.T) {
	handler := NewRouter(nil, fakePinger{err: errors.New("database unavailable")})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}
