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

	"github.com/jackc/pgx/v5"
	"gk/api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	h, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), "development", nil)
	if err != nil {
		t.Fatal(err)
	}
	h.begin = func(context.Context) (pgx.Tx, error) { return &fakeTx{}, nil }
	t.Cleanup(func() { _ = h.Close(context.Background()) })
	return h
}

func TestValidationAndAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, scenario, body string
		disabled, busy       bool
		status               int
	}{
		{"disabled", "normal", "{}", true, false, 404},
		{"unknown", "unknown", "{}", false, false, 404},
		{"null", "normal", "null", false, false, 400},
		{"unknown field", "normal", `{"extra":1}`, false, false, 400},
		{"trailing JSON", "normal", "{} {}", false, false, 400},
		{"negative delay", "slow-payment", `{"delayMs":-1}`, false, false, 400},
		{"excess delay", "slow-payment", `{"delayMs":3001}`, false, false, 400},
		{"excess retries", "payment-retry", `{"failuresBeforeSuccess":4}`, false, false, 400},
		{"wrong parameter", "normal", `{"delayMs":1}`, false, false, 400},
		{"bad quantity", "normal", `{"quantity":0}`, false, false, 400},
		{"large quantity", "normal", `{"quantity":11}`, false, false, 400},
		{"busy", "normal", "{}", false, true, 429},
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
			h.RunDemoOrder(w, httptest.NewRequest("POST", "/", strings.NewReader(tc.body)), tc.scenario)
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
		{"normal", "{}", 1}, {"slow-payment", `{"delayMs":0}`, 1},
		{"payment-retry", `{"failuresBeforeSuccess":0}`, 1}, {"payment-retry", "{}", 3},
	} {
		t.Run(tc.scenario+tc.body, func(t *testing.T) {
			h := newTestHandler(t)
			w := httptest.NewRecorder()
			h.RunDemoOrder(w, httptest.NewRequest("POST", "/", strings.NewReader(tc.body)), tc.scenario)
			var result api.ObservabilityScenarioResult
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || result.Attempts != tc.attempts || result.TraceId != nil || !result.Order.RolledBack || result.Order.Status != api.Paid || result.Order.StockRemaining != 9 {
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
	h.RunDemoOrder(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"delayMs":3000}`)).WithContext(ctx), "slow-payment")
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
	oldPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		otel.SetTextMapPropagator(oldPropagator)
		otel.SetTracerProvider(oldTP)
		otel.SetMeterProvider(oldMP)
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})
	h := newTestHandler(t)
	w := httptest.NewRecorder()
	h.RunDemoOrder(w, httptest.NewRequest("POST", "/", strings.NewReader("{}")), "payment-retry")
	if w.Code != 200 || w.Header().Get("X-Trace-Id") == "" {
		t.Fatal(w.Body.String())
	}
	var root sdktrace.ReadOnlySpan
	errors, attempts := 0, 0
	for _, span := range recorder.Ended() {
		if span.Name() == "demo.run" {
			root = span
		}
		if span.Name() == "payment.authorize" {
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
		if span.Name() != "demo.run" && !span.Parent().IsValid() {
			t.Fatal("missing trace parent")
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
				if m.Name != "gk.demo.duration" {
					continue
				}
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
			h, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), environment, nil)
			if err != nil {
				t.Fatal(err)
			}
			h.begin = func(context.Context) (pgx.Tx, error) { return &fakeTx{}, nil }
			t.Cleanup(func() { _ = h.Close(context.Background()) })
			w := httptest.NewRecorder()
			h.RunDemoOrder(w, httptest.NewRequest("POST", "/", strings.NewReader("{}")), "normal")
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
