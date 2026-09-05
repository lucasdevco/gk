-- name: CreateDemoInventory :exec
INSERT INTO demo_inventory (id, available) VALUES ($1, $2);

-- name: CreateDemoOrder :exec
INSERT INTO demo_orders (id, inventory_id, quantity, total_cents)
VALUES ($1, $2, $3, $4);

-- name: ReserveDemoInventory :one
UPDATE demo_inventory SET available = available - sqlc.arg(quantity)::integer
WHERE id = sqlc.arg(id) AND available >= sqlc.arg(quantity)::integer
RETURNING available;

-- name: ConfirmDemoOrder :execrows
UPDATE demo_orders SET status = 'paid' WHERE id = $1 AND status = 'pending';

-- name: GetDemoOrder :one
SELECT id, inventory_id, quantity, total_cents, status FROM demo_orders WHERE id = $1;
