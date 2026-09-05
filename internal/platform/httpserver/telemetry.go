package httpserver

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RouteTelemetry must wrap the mux directly, inside otelhttp and any middleware
// that clones the request. Read the final matched pattern after nested muxes
// finish, never the raw URL: IDs and query strings must not become metric labels.
func RouteTelemetry(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
			w.Header().Set("X-Trace-Id", sc.TraceID().String())
		}
		defer func() {
			route := r.Pattern
			if _, path, ok := strings.Cut(route, " "); ok {
				route = path
			}
			if route == "" {
				route = "unmatched"
			}
			attr := attribute.String("http.route", route)
			labeler, _ := otelhttp.LabelerFromContext(r.Context())
			labeler.Add(attr)
			span := trace.SpanFromContext(r.Context())
			span.SetAttributes(attr)
			method := r.Method
			switch method {
			case "GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH":
			default:
				method = "_OTHER"
			}
			span.SetName(method + " " + route)
		}()
		next.ServeHTTP(w, r)
	})
}
