package observabilitydemo

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gk/api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	h, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), "development")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestValidationAndAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, scenario, body string
		disabled, busy       bool
		status               int
	}{
		{"disabled", "baseline", "{}", true, false, 404},
		{"unknown", "unknown", "{}", false, false, 404},
		{"null", "baseline", "null", false, false, 400},
		{"unknown field", "baseline", `{"extra":1}`, false, false, 400},
		{"trailing JSON", "baseline", "{} {}", false, false, 400},
		{"negative delay", "slow-dependency", `{"delayMs":-1}`, false, false, 400},
		{"excess delay", "slow-dependency", `{"delayMs":3001}`, false, false, 400},
		{"excess retries", "retry", `{"failuresBeforeSuccess":4}`, false, false, 400},
		{"wrong parameter", "baseline", `{"delayMs":1}`, false, false, 400},
		{"busy", "baseline", "{}", false, true, 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			h.enabled = !tc.disabled
			if tc.busy {
				for i := 0; i < cap(h.slots); i++ {
					h.slots <- struct{}{}
				}
			}
			w := httptest.NewRecorder()
			h.RunObservabilityScenario(w, httptest.NewRequest("POST", "/", strings.NewReader(tc.body)), tc.scenario)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestScenarios(t *testing.T) {
	for _, tc := range []struct {
		scenario, body string
		attempts       int
	}{
		{"baseline", "{}", 1}, {"slow-dependency", `{"delayMs":0}`, 1},
		{"retry", `{"failuresBeforeSuccess":0}`, 1}, {"retry", "{}", 3},
	} {
		t.Run(tc.scenario+tc.body, func(t *testing.T) {
			h := newTestHandler(t)
			w := httptest.NewRecorder()
			h.RunObservabilityScenario(w, httptest.NewRequest("POST", "/", strings.NewReader(tc.body)), tc.scenario)
			var result api.ObservabilityScenarioResult
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || result.Attempts != tc.attempts || result.TraceId != nil {
				t.Fatalf("unexpected result: %s", w.Body.String())
			}
			if len(h.slots) != 0 {
				t.Fatal("admission slot leaked")
			}
		})
	}
}

func TestCancellationReleasesSlot(t *testing.T) {
	h := newTestHandler(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	started := time.Now()
	h.RunObservabilityScenario(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"delayMs":3000}`)).WithContext(ctx), "slow-dependency")
	if w.Code != 408 || time.Since(started) > time.Second || len(h.slots) != 0 {
		t.Fatalf("cancellation failed: %d", w.Code)
	}
}

func TestRetryTelemetry(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	oldTP, oldMP := otel.GetTracerProvider(), otel.GetMeterProvider()
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		otel.SetTracerProvider(oldTP)
		otel.SetMeterProvider(oldMP)
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})
	h := newTestHandler(t)
	w := httptest.NewRecorder()
	h.RunObservabilityScenario(w, httptest.NewRequest("POST", "/", strings.NewReader("{}")), "retry")
	if w.Code != 200 || w.Header().Get("X-Trace-Id") == "" {
		t.Fatal(w.Body.String())
	}
	var root sdktrace.ReadOnlySpan
	errors, attempts := 0, 0
	for _, span := range recorder.Ended() {
		if span.Name() == "demo.run" {
			root = span
		}
		if span.Name() == "demo.dependency" {
			attempts++
			if span.Status().Code == codes.Error {
				errors++
			}
		}
	}
	if root == nil || root.Status().Code == codes.Error || attempts != 3 || errors != 2 {
		t.Fatalf("root=%v attempts=%d errors=%d", root, attempts, errors)
	}
	for _, span := range recorder.Ended() {
		if span.Name() != "demo.run" && span.Parent().SpanID() != root.SpanContext().SpanID() {
			t.Fatal("broken trace parent")
		}
		if span.SpanContext().TraceID().String() != w.Header().Get("X-Trace-Id") {
			t.Fatal("trace ID mismatch")
		}
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	durations := uint64(0)
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch value := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range value.DataPoints {
					counts[m.Name] += point.Value
				}
			case metricdata.Histogram[float64]:
				for _, point := range value.DataPoints {
					durations += point.Count
				}
			}
		}
	}
	if counts["gk.demo.runs"] != 1 || counts["gk.demo.attempts"] != 3 || durations != 1 {
		t.Fatalf("metrics=%v durations=%d", counts, durations)
	}
}

func TestEnvironment(t *testing.T) {
	for _, environment := range []string{"development", "production", "staging", ""} {
		t.Run(environment, func(t *testing.T) {
			h, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), environment)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.RunObservabilityScenario(w, httptest.NewRequest("POST", "/", strings.NewReader("{}")), "baseline")
			want := 404
			if environment == "development" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("status=%d want=%d", w.Code, want)
			}
		})
	}
}
