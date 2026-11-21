package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteSuccessUsesCommonEnvelopeAndRequestID(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/example", nil)
	request = request.WithContext(withRequestID(request.Context(), "req-test"))
	WriteSuccess(recorder, request, 200, map[string]string{"id": "example"})

	var got struct {
		Data map[string]string `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("response JSON: %v", err)
	}
	if got.Data["id"] != "example" || got.Meta.RequestID != "req-test" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestWriteErrorUsesCommonEnvelope(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/example", nil)
	request = request.WithContext(withRequestID(request.Context(), "req-fail"))
	WriteError(recorder, request, 503, "SERVICE_UNAVAILABLE", "not ready", nil)

	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("response JSON: %v", err)
	}
	if recorder.Code != 503 || got.Error.Code != "SERVICE_UNAVAILABLE" || got.Meta.RequestID != "req-fail" {
		t.Fatalf("unexpected response: status=%d body=%+v", recorder.Code, got)
	}
}
