package observabilitydemo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"gk/db/sqlc"
)

type checkoutOrder struct {
	ID             string
	Quantity       int
	TotalCents     int
	StockRemaining int
	RolledBack     bool
}

// checkout exercises real SQL in isolated fixtures. Rollback even after payment
// success is deliberate for this demo, not a production checkout transaction model.
func (h *Handler) checkout(ctx context.Context, scenario string, quantity, delay, failures int) (order checkoutOrder, attempts int, err error) {
	if h.begin == nil || h.payment == nil {
		return order, 0, errors.New("checkout dependencies not configured")
	}
	var tx pgx.Tx
	err = h.step(ctx, "db.begin", func(ctx context.Context) error { var e error; tx, e = h.begin(ctx); return e })
	if err != nil {
		return order, 0, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		rollbackErr := h.step(cleanup, "db.rollback", tx.Rollback)
		if rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback demo: %w", rollbackErr))
		}
		order.RolledBack = rollbackErr == nil
	}()
	q := sqlc.New(tx)
	inventoryID, orderID := uuid.NewString(), uuid.NewString()
	stock := int32(10)
	if scenario == "out-of-stock" {
		stock = 0
	}
	if err = h.step(ctx, "inventory.fixture", func(ctx context.Context) error {
		if _, e := tx.Exec(ctx, "SET LOCAL statement_timeout = '4s'"); e != nil {
			return e
		}
		return q.CreateDemoInventory(ctx, sqlc.CreateDemoInventoryParams{ID: inventoryID, Available: stock})
	}); err != nil {
		return
	}
	if err = h.step(ctx, "order.create", func(ctx context.Context) error {
		e := q.CreateDemoOrder(ctx, sqlc.CreateDemoOrderParams{ID: orderID, InventoryID: inventoryID, Quantity: int32(quantity), TotalCents: int32(quantity * 1990)})
		if e == nil {
			h.logger.InfoContext(ctx, "demo order created", "order_id", orderID, "quantity", quantity, "total_cents", quantity*1990)
		}
		return e
	}); err != nil {
		return
	}
	var remaining int32
	if err = h.step(ctx, "inventory.reserve", func(ctx context.Context) error {
		var e error
		remaining, e = q.ReserveDemoInventory(ctx, sqlc.ReserveDemoInventoryParams{ID: inventoryID, Quantity: int32(quantity)})
		if errors.Is(e, pgx.ErrNoRows) {
			return errOutOfStock
		}
		return e
	}); err != nil {
		return
	}
	if scenario != "slow-payment" {
		delay = 20
	}
	if scenario != "payment-retry" {
		failures = 0
	}
	for attempts = 1; attempts <= 4; attempts++ {
		err = h.step(ctx, "payment.authorize", func(ctx context.Context) error {
			e := h.payment.authorize(ctx, paymentRequest{OrderID: orderID, AmountCents: quantity * 1990, Attempt: attempts, Failures: failures, DelayMS: delay, Declined: scenario == "payment-declined"})
			outcome := "success"
			if e != nil {
				outcome = "error"
				h.logger.WarnContext(ctx, "demo payment attempt failed", "order_id", orderID, "attempt", attempts, "error", e)
			}
			h.attempts.Add(ctx, 1, metric.WithAttributes(attribute.String("scenario", scenario), attribute.String("outcome", outcome)))
			return e
		})
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			return
		}
		if !errors.Is(err, errPaymentUnavailable) || attempts == 4 {
			return
		}
		if err = h.step(ctx, "payment.backoff", func(ctx context.Context) error { return pause(ctx, 100*time.Millisecond) }); err != nil {
			return
		}
	}
	if err = h.step(ctx, "order.confirm", func(ctx context.Context) error {
		affected, e := q.ConfirmDemoOrder(ctx, orderID)
		if e == nil && affected != 1 {
			return errors.New("order is not pending")
		}
		return e
	}); err != nil {
		return
	}
	if err = h.step(ctx, "order.read", func(ctx context.Context) error {
		saved, e := q.GetDemoOrder(ctx, orderID)
		if e != nil {
			return e
		}
		if saved.Status != "paid" {
			return errors.New("order was not confirmed")
		}
		order = checkoutOrder{ID: saved.ID, Quantity: int(saved.Quantity), TotalCents: int(saved.TotalCents), StockRemaining: int(remaining)}
		h.logger.InfoContext(ctx, "demo order confirmed", "order_id", saved.ID, "stock_remaining", remaining)
		return nil
	}); err != nil {
		return
	}
	return
}
