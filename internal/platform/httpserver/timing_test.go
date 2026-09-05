package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestServerTiming(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		code    int
	}{
		{"body", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }, 200},
		{"empty", func(http.ResponseWriter, *http.Request) {}, 200},
		{"explicit", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }, 204},
		{"error", func(w http.ResponseWriter, r *http.Request) {
			WriteError(w, r, 400, "invalid_request", "invalid request")
		}, 400},
		{"panic", func(http.ResponseWriter, *http.Request) { panic("test") }, 500},
		{"flush", func(w http.ResponseWriter, _ *http.Request) {
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Error(err)
			}
			_, _ = w.Write([]byte("stream"))
		}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			handler := CORS(tc.handler, "https://example.com")
			handler = Recover(handler, logger)
			handler = ServerTiming(handler)
			response := httptest.NewRecorder()
			response.Header().Add("Server-Timing", "db;dur=1")
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
			result := response.Result()
			defer result.Body.Close()
			values := result.Header.Values("Server-Timing")
			if result.StatusCode != tc.code || len(values) != 2 || values[0] != "db;dur=1" {
				t.Fatalf("response: %d %v", result.StatusCode, values)
			}
			duration, err := strconv.ParseFloat(strings.TrimPrefix(values[1], "app;dur="), 64)
			if err != nil || duration < 0 {
				t.Fatalf("invalid duration %q", values[1])
			}
			if !strings.Contains(result.Header.Get("Access-Control-Expose-Headers"), "Server-Timing") || result.Header.Get("Timing-Allow-Origin") != "https://example.com" {
				t.Fatal("timing headers not exposed")
			}
		})
	}
}
