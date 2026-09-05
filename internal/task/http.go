package task

import (
	"errors"
	"log/slog"
	"net/http"

	"gk/api"
	"gk/internal/platform/httpserver"
)

type HTTPHandler struct {
	service *Service
	logger  *slog.Logger
}

func NewHTTPHandler(service *Service, logger *slog.Logger) *HTTPHandler {
	return &HTTPHandler{service: service, logger: logger}
}

func (h *HTTPHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.service.List(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	items := make([]api.Task, len(tasks))
	for i, item := range tasks {
		items[i] = toAPI(item)
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *HTTPHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var body api.CreateTaskJSONRequestBody
	if err := httpserver.DecodeJSON(w, r, &body); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request body")
		return
	}
	created, err := h.service.Create(r.Context(), body.Title)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, toAPI(created))
}

func (h *HTTPHandler) UpdateTask(w http.ResponseWriter, r *http.Request, id string) {
	var body api.UpdateTaskJSONRequestBody
	if err := httpserver.DecodeJSON(w, r, &body); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request body")
		return
	}
	updated, err := h.service.SetCompleted(r.Context(), id, body.Completed)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, toAPI(updated))
}

func (h *HTTPHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidTitle):
		httpserver.WriteError(w, r, http.StatusBadRequest, "task_invalid_title", ErrInvalidTitle.Error())
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, r, http.StatusNotFound, "task_not_found", "task not found")
	default:
		httpserver.WriteInternalError(w, r, h.logger, err)
	}
}

func toAPI(value Task) api.Task {
	return api.Task{
		Id: value.ID, Title: value.Title, Completed: value.Completed,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

var _ api.ServerInterface = (*HTTPHandler)(nil)
