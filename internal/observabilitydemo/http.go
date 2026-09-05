// Package observabilitydemo provides removable, bounded development scenarios.
package observabilitydemo

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"gk/api"
	"gk/internal/platform/httpserver"
)

type Handler struct {
	begin    func(context.Context) (pgx.Tx, error)
	payment  *paymentSimulator
	enabled  bool
	logger   *slog.Logger
	slots    chan struct{}
	tracer   trace.Tracer
	runs     metric.Int64Counter
	attempts metric.Int64Counter
	duration metric.Float64Histogram
}

func New(logger *slog.Logger, environment string, pool *pgxpool.Pool) (*Handler, error) {
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
	if err != nil {
		return nil, err
	}
	if h.enabled {
		if pool != nil {
			h.begin = pool.Begin
		}
		h.payment, err = startPaymentSimulator(logger)
		if err != nil {
			return nil, err
		}
	}
	return h, nil
}

func (h *Handler) Close(ctx context.Context) error {
	if h.payment == nil {
		return nil
	}
	return h.payment.close(ctx)
}

func (h *Handler) RunDemoOrder(w http.ResponseWriter, r *http.Request, scenario string) {
	if !h.enabled {
		httpserver.WriteError(w, r, 404, "demo_disabled", "observability demonstrations are disabled")
		return
	}
	if scenario != "normal" && scenario != "slow-payment" && scenario != "payment-retry" && scenario != "out-of-stock" && scenario != "payment-declined" {
		httpserver.WriteError(w, r, 404, "scenario_not_found", "unknown observability scenario")
		return
	}
	var body *api.ObservabilityScenarioRequest
	if err := httpserver.DecodeJSON(w, r, &body); err != nil || body == nil {
		httpserver.WriteError(w, r, 400, "invalid_request", "expected a JSON object")
		return
	}
	delay, failures, quantity := 1500, 2, 1
	if body.Quantity != nil {
		quantity = *body.Quantity
	}
	if body.DelayMs != nil {
		delay = *body.DelayMs
	}
	if body.FailuresBeforeSuccess != nil {
		failures = *body.FailuresBeforeSuccess
	}
	if quantity < 1 || quantity > 10 || delay < 0 || delay > 3000 || failures < 0 || failures > 3 ||
		(body.DelayMs != nil && scenario != "slow-payment") ||
		(body.FailuresBeforeSuccess != nil && scenario != "payment-retry") {
		httpserver.WriteError(w, r, 400, "invalid_request", "quantity must be 1–10; delayMs (0–3000) is only for slow-payment; failuresBeforeSuccess (0–3) is only for payment-retry")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		httpserver.WriteError(w, r, 429, "demo_busy", "four demonstrations are already running")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ctx, span := h.tracer.Start(ctx, "demo.run", trace.WithAttributes(attribute.String("scenario", scenario)))
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
	order, count, err := h.checkout(ctx, scenario, quantity, delay, failures)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, "checkout failed")
		status, code, message := http.StatusServiceUnavailable, "checkout_unavailable", "checkout dependencies are unavailable"
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			outcome = "canceled"
			status, code, message = http.StatusRequestTimeout, "demo_canceled", "checkout canceled"
		case errors.Is(err, errOutOfStock):
			status, code, message = http.StatusConflict, "out_of_stock", "not enough inventory"
		case errors.Is(err, errPaymentDeclined):
			status, code, message = http.StatusUnprocessableEntity, "payment_declined", "simulated payment was declined"
		}
		h.logger.WarnContext(ctx, "order checkout failed", "scenario", scenario, "error", err)
		httpserver.WriteError(w, r, status, code, message)
		return
	}
	result := api.ObservabilityScenarioResult{Scenario: scenario, Outcome: api.Success, Attempts: count, DurationMs: time.Since(started).Milliseconds(), Order: api.DemoOrder{Id: order.ID, Quantity: order.Quantity, TotalCents: order.TotalCents, Status: api.Paid, StockRemaining: order.StockRemaining, RolledBack: order.RolledBack}}
	if traceID != "" {
		result.TraceId = &traceID
	}
	httpserver.WriteJSON(w, http.StatusOK, result)
}

// step measures real business/database operations without recording SQL or payloads.
func (h *Handler) step(ctx context.Context, name string, run func(context.Context) error) error {
	ctx, span := h.tracer.Start(ctx, name)
	defer span.End()
	err := run(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "operation failed")
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
