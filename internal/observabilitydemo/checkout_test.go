package observabilitydemo

import (
	"context"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Fake only the SQL operations used by checkout; payment still uses real HTTP.
type fakeTx struct {
	pgx.Tx
	id, inventoryID, status string
	stock, quantity, total  int32
	rolledBack              bool
	failCreate              bool
	rollbackErr             error
}

func (tx *fakeTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if err := ctx.Err(); err != nil {
		return pgconn.CommandTag{}, err
	}
	switch {
	case strings.Contains(query, "CreateDemoInventory"):
		tx.inventoryID = args[0].(string)
		tx.stock = args[1].(int32)
	case strings.Contains(query, "CreateDemoOrder"):
		if tx.failCreate {
			return pgconn.CommandTag{}, errors.New("secret database failure")
		}
		tx.id = args[0].(string)
		tx.quantity = args[2].(int32)
		tx.total = args[3].(int32)
		tx.status = "pending"
	case strings.Contains(query, "ConfirmDemoOrder"):
		tx.status = "paid"
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (tx *fakeTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	if err := ctx.Err(); err != nil {
		return fakeRow{err: err}
	}
	if strings.Contains(query, "ReserveDemoInventory") {
		quantity := args[0].(int32)
		if tx.stock < quantity {
			return fakeRow{err: pgx.ErrNoRows}
		}
		tx.stock -= quantity
		return fakeRow{values: []any{tx.stock}}
	}
	return fakeRow{values: []any{tx.id, tx.inventoryID, tx.quantity, tx.total, tx.status}}
}
func (tx *fakeTx) Rollback(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx.rolledBack = true
	return tx.rollbackErr
}

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	for i, v := range row.values {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(v))
	}
	return nil
}

func TestCheckoutBusinessOutcomes(t *testing.T) {
	for _, tc := range []struct {
		scenario                 string
		failCreate, failRollback bool
		status                   int
		paid                     bool
	}{
		{"normal", false, false, 200, true},
		{"out-of-stock", false, false, 409, false},
		{"payment-declined", false, false, 422, false},
		{"normal", true, false, 503, false},
		{"normal", false, true, 503, true},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			h := newTestHandler(t)
			tx := &fakeTx{failCreate: tc.failCreate}
			if tc.failRollback {
				tx.rollbackErr = errors.New("rollback failed")
			}
			h.begin = func(context.Context) (pgx.Tx, error) { return tx, nil }
			w := httptest.NewRecorder()
			h.RunDemoOrder(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"quantity":2}`)), tc.scenario)
			if w.Code != tc.status || !tx.rolledBack || (tx.status == "paid") != tc.paid {
				t.Fatalf("status=%d rollback=%v order=%s body=%s", w.Code, tx.rolledBack, tx.status, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("database detail leaked")
			}
			if tc.status == 200 && (tx.stock != 8 || tx.total != 3980) {
				t.Fatalf("stock=%d total=%d", tx.stock, tx.total)
			}
		})
	}
}

func TestCanceledCheckoutRollsBack(t *testing.T) {
	h := newTestHandler(t)
	tx := &fakeTx{}
	ctx, cancel := context.WithCancel(context.Background())
	h.begin = func(context.Context) (pgx.Tx, error) { cancel(); return tx, nil }
	w := httptest.NewRecorder()
	h.RunDemoOrder(w, httptest.NewRequest("POST", "/", strings.NewReader("{}")).WithContext(ctx), "normal")
	if w.Code != 408 || !tx.rolledBack {
		t.Fatalf("status=%d rolledBack=%v", w.Code, tx.rolledBack)
	}
}
