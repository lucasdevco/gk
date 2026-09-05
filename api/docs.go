package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var specification []byte

// RegisterDocs serves the same contract used for code generation.
func RegisterDocs(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(specification)
	})
	mux.HandleFunc("GET /api/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(docsHTML))
	})
	mux.HandleFunc("GET /api/docs/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/docs", http.StatusTemporaryRedirect)
	})
}

// Pin CDN assets to a specific release; the contract itself is served locally.
const docsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>GK API Documentation</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css">
</head>
<body>
  <p><a href="/api/openapi.yaml">OpenAPI specification</a></p>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: '/api/openapi.yaml',
      dom_id: '#swagger-ui',
      deepLinking: true,
      validatorUrl: null,
      queryConfigEnabled: false,
      presets: [SwaggerUIBundle.presets.apis]
    });
  </script>
</body>
</html>`
