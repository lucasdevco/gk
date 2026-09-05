// Package observabilitydemo provides removable, bounded development scenarios.
package observabilitydemo

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"gk/api"
	"gk/internal/platform/httpserver"
)

type Handler struct {
	enabled  bool
	logger   *slog.Logger
	slots    chan struct{}
	tracer   trace.Tracer
	runs     metric.Int64Counter
	attempts metric.Int64Counter
	duration metric.Float64Histogram
}

func New(logger *slog.Logger, environment string) (*Handler, error) {
	h := &Handler{enabled: environment == "development", logger: logger, slots: make(chan struct{}, 4), tracer: otel.Tracer("gk/observabilitydemo")}
	meter := otel.Meter("gk/observabilitydemo")
	var err error
	h.runs, err = meter.Int64Counter("gk.demo.runs")
	if err != nil {
		return nil, err
	}
	h.attempts, err = meter.Int64Counter("gk.demo.attempts")
	if err != nil {
		return nil, err
	}
	h.duration, err = meter.Float64Histogram("gk.demo.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.01, .05, .1, .25, .5, 1, 2, 4))
	return h, err
}

func (h *Handler) RunObservabilityScenario(w http.ResponseWriter, r *http.Request, scenario string) {
	if !h.enabled {
		httpserver.WriteError(w, r, 404, "demo_disabled", "observability demonstrations are disabled")
		return
	}
	if scenario != "baseline" && scenario != "slow-dependency" && scenario != "retry" {
		httpserver.WriteError(w, r, 404, "scenario_not_found", "unknown observability scenario")
		return
	}
	var body *api.ObservabilityScenarioRequest
	if err := httpserver.DecodeJSON(w, r, &body); err != nil || body == nil {
		httpserver.WriteError(w, r, 400, "invalid_request", "expected a JSON object")
		return
	}
	delay, failures := 1500, 2
	if body.DelayMs != nil {
		delay = *body.DelayMs
	}
	if body.FailuresBeforeSuccess != nil {
		failures = *body.FailuresBeforeSuccess
	}
	if delay < 0 || delay > 3000 || failures < 0 || failures > 3 ||
		(body.DelayMs != nil && scenario != "slow-dependency") ||
		(body.FailuresBeforeSuccess != nil && scenario != "retry") {
		httpserver.WriteError(w, r, 400, "invalid_request", "delayMs (0–3000) is only for slow-dependency; failuresBeforeSuccess (0–3) is only for retry")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		httpserver.WriteError(w, r, 429, "demo_busy", "four demonstrations are already running")
		return
	}
	ctx, span := h.tracer.Start(r.Context(), "demo.run", trace.WithAttributes(attribute.String("scenario", scenario)))
	defer span.End()
	traceID := ""
	if sc := span.SpanContext(); sc.IsValid() {
		traceID = sc.TraceID().String()
		w.Header().Set("X-Trace-Id", traceID)
	}
	started := time.Now()
	outcome := "success"
	defer func() {
		attrs := metric.WithAttributes(attribute.String("scenario", scenario), attribute.String("outcome", outcome))
		h.runs.Add(ctx, 1, attrs)
		h.duration.Record(ctx, time.Since(started).Seconds(), attrs)
		h.logger.InfoContext(ctx, "demo completed", "scenario", scenario, "outcome", outcome, "duration_ms", time.Since(started).Milliseconds())
	}()
	count := 0
	err := h.step(ctx, "demo.validate", 10*time.Millisecond)
	if err == nil {
		wait := 20 * time.Millisecond
		if scenario == "slow-dependency" {
			wait = time.Duration(delay) * time.Millisecond
		}
		if scenario != "retry" {
			failures = 0
		}
		for count = 1; count <= failures+1; count++ {
			attemptCtx, attemptSpan := h.tracer.Start(ctx, "demo.dependency", trace.WithAttributes(attribute.Int("attempt", count), attribute.Bool("simulated", true)))
			err = pause(attemptCtx, wait)
			result := "success"
			if err == nil && count <= failures {
				err = errors.New("simulated dependency unavailable")
			}
			if err != nil {
				result = "error"
				attemptSpan.RecordError(err)
				attemptSpan.SetStatus(codes.Error, "dependency attempt failed")
				h.logger.WarnContext(attemptCtx, "demo dependency failed", "scenario", scenario, "attempt", count, "error", err)
			}
			h.attempts.Add(attemptCtx, 1, metric.WithAttributes(attribute.String("scenario", scenario), attribute.String("outcome", result)))
			attemptSpan.End()
			if ctx.Err() != nil {
				err = ctx.Err()
				break
			}
			if err == nil {
				break
			}
			if err = h.step(ctx, "demo.backoff", 100*time.Millisecond); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = h.step(ctx, "demo.render", 10*time.Millisecond)
	}
	if err != nil {
		outcome = "canceled"
		span.RecordError(err)
		span.SetStatus(codes.Error, "demonstration canceled")
		httpserver.WriteError(w, r, http.StatusRequestTimeout, "demo_canceled", "demonstration canceled")
		return
	}
	result := api.ObservabilityScenarioResult{Scenario: scenario, Outcome: api.Success, Attempts: count, DurationMs: time.Since(started).Milliseconds()}
	if traceID != "" {
		result.TraceId = &traceID
	}
	httpserver.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) step(ctx context.Context, name string, duration time.Duration) error {
	ctx, span := h.tracer.Start(ctx, name)
	defer span.End()
	err := pause(ctx, duration)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "step canceled")
	}
	return err
}

func pause(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
