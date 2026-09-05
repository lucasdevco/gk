package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDocumentationRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterDocs(mux)
	for _, tc := range []struct {
		path, contentType string
		content           []byte
	}{
		{"/openapi.yaml", "application/yaml; charset=utf-8", specification},
		{"/docs", "text/html; charset=utf-8", []byte("url: '/openapi.yaml'")},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != 200 || w.Header().Get("Content-Type") != tc.contentType || !bytes.Contains(w.Body.Bytes(), tc.content) {
				t.Fatalf("unexpected docs response: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
