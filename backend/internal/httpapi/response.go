package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
)

type requestIDKey struct{}

type responseMeta struct {
	RequestID string `json:"request_id"`
}

type successEnvelope struct {
	Data any          `json:"data"`
	Meta responseMeta `json:"meta"`
}

type paginatedMeta struct {
	RequestID  string         `json:"request_id"`
	Pagination paginationMeta `json:"pagination"`
}

type paginationMeta struct {
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	Total   int  `json:"total"`
	HasMore bool `json:"has_more"`
}

type paginatedEnvelope struct {
	Data any           `json:"data"`
	Meta paginatedMeta `json:"meta"`
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details,omitempty"`
	} `json:"error"`
	Meta responseMeta `json:"meta"`
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func requestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

func WriteSuccess(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, successEnvelope{Data: data, Meta: responseMeta{RequestID: requestID(r)}})
}

func WritePaginated(w http.ResponseWriter, r *http.Request, data any, limit, offset, total int) {
	writeJSON(w, http.StatusOK, paginatedEnvelope{Data: data, Meta: paginatedMeta{
		RequestID:  requestID(r),
		Pagination: paginationMeta{Limit: limit, Offset: offset, Total: total, HasMore: offset+limit < total},
	}})
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	var body errorEnvelope
	body.Error.Code = code
	body.Error.Message = message
	body.Error.Details = details
	body.Meta.RequestID = requestID(r)
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
