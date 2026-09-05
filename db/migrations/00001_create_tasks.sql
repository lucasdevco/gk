-- +goose Up
CREATE TABLE tasks (
    id uuid PRIMARY KEY,
    title text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    completed boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tasks_created_at_idx ON tasks (created_at DESC);

-- +goose Down
DROP TABLE tasks;
