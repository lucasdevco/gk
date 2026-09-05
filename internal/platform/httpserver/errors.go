package httpserver

import (
	"log/slog"
	"net/http"
)

// ErrorBody is the public error contract. Never place a raw cause in Message or Details.
type ErrorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"requestId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

func WriteError(w http.ResponseWriter, r *http.Request, httpStatus int, code, message string) {
	WriteErrorDetails(w, r, httpStatus, code, message, nil)
}

func WriteErrorDetails(w http.ResponseWriter, r *http.Request, httpStatus int, code, message string, details map[string]any) {
	WriteJSON(w, httpStatus, struct {
		Error ErrorBody `json:"error"`
	}{
		Error: ErrorBody{Code: code, Message: message, RequestID: RequestID(r.Context()), Details: details},
	})
}

func WriteInternalError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	logger.ErrorContext(r.Context(), "http request failed", "error", err, "request_id", RequestID(r.Context()))
	WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
}
