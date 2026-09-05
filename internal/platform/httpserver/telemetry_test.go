package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRouteTelemetry(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()); _ = tp.Shutdown(context.Background()) })
	inner := http.NewServeMux()
	inner.HandleFunc("PATCH /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400) })
	inner.HandleFunc("GET /api/panic", func(w http.ResponseWriter, r *http.Request) { panic("test") })
	mux := http.NewServeMux()
	mux.Handle("/api/", inner)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := otelhttp.NewHandler(Chain(mux, WithRequestID, Recover(logger), RouteTelemetry), "http.server", otelhttp.WithMeterProvider(mp), otelhttp.WithTracerProvider(tp))
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"PATCH", "/api/tasks/first?secret=one", 400},
		{"PATCH", "/api/tasks/second?secret=two", 400},
		{"GET", "/api/missing-one", 404},
		{"GET", "/api/missing-two", 404},
		{"GET", "/api/panic", 500},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		spans := recorder.Ended()
		if len(spans) == 0 || w.Header().Get("X-Trace-Id") != spans[len(spans)-1].SpanContext().TraceID().String() {
			t.Fatalf("response trace ID does not match recorded trace: %q", w.Header().Get("X-Trace-Id"))
		}
		if w.Code != tc.status {
			t.Fatalf("status=%d want=%d", w.Code, tc.status)
		}
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	counts := map[string]uint64{}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("unexpected histogram %T", m.Data)
			}
			for _, p := range hist.DataPoints {
				route, ok := p.Attributes.Value("http.route")
				if !ok {
					t.Fatal("missing route")
				}
				counts[route.AsString()] += p.Count
			}
		}
	}
	if counts["/api/tasks/{id}"] != 2 || counts["unmatched"] != 2 || counts["/api/panic"] != 1 || len(counts) != 3 {
		t.Fatalf("routes=%v", counts)
	}
	for _, span := range recorder.Ended() {
		switch span.Name() {
		case "PATCH /api/tasks/{id}", "GET unmatched", "GET /api/panic":
		default:
			t.Fatalf("unexpected span name: %s", span.Name())
		}
	}
}
