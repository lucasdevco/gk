-- +goose Up
-- Isolated observability fixtures; every demo rolls its transaction back.
CREATE TABLE demo_inventory (
    id uuid PRIMARY KEY,
    available integer NOT NULL CHECK (available >= 0)
);
CREATE TABLE demo_orders (
    id uuid PRIMARY KEY,
    inventory_id uuid NOT NULL REFERENCES demo_inventory(id),
    quantity integer NOT NULL CHECK (quantity BETWEEN 1 AND 10),
    total_cents integer NOT NULL CHECK (total_cents > 0),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid'))
);

-- +goose Down
DROP TABLE demo_orders;
DROP TABLE demo_inventory;
