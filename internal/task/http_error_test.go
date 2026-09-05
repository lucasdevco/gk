package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"gk/api"
)

func TestHTTPErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		httpStatus int
		code       string
	}{
		{"not found", fmt.Errorf("lookup: %w", ErrNotFound), 404, "task_not_found"},
		{"invalid title", fmt.Errorf("validate: %w", ErrInvalidTitle), 400, "task_invalid_title"},
		{"internal", errors.New("secret database connection detail"), 500, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			h := NewHTTPHandler(nil, slog.New(slog.NewTextHandler(&logs, nil)))
			response := httptest.NewRecorder()
			h.writeError(response, httptest.NewRequest("GET", "/", nil), tc.err)
			var body api.Error
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.httpStatus || body.Error.Code != tc.code {
				t.Fatalf("unexpected response: %s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), "secret") {
				t.Fatal("internal cause leaked")
			}
			if tc.httpStatus == 500 && !strings.Contains(logs.String(), tc.err.Error()) {
				t.Fatal("internal cause missing from log")
			}
		})
	}
}
