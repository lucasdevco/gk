package task

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"gk/db/sqlc"
)

type PostgresRepository struct{ queries *sqlc.Queries }

func NewPostgresRepository(queries *sqlc.Queries) *PostgresRepository {
	return &PostgresRepository{queries: queries}
}

func (r *PostgresRepository) List(ctx context.Context) ([]Task, error) {
	rows, err := r.queries.ListTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	tasks := make([]Task, len(rows))
	for i, row := range rows {
		tasks[i] = fromRow(row)
	}
	return tasks, nil
}

func (r *PostgresRepository) Create(ctx context.Context, id, title string) (Task, error) {
	row, err := r.queries.CreateTask(ctx, sqlc.CreateTaskParams{ID: id, Title: title})
	if err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}
	return fromRow(row), nil
}

func (r *PostgresRepository) SetCompleted(ctx context.Context, id string, completed bool) (Task, error) {
	row, err := r.queries.UpdateTaskCompleted(ctx, sqlc.UpdateTaskCompletedParams{ID: id, Completed: completed})
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("update task: %w", err)
	}
	return fromRow(row), nil
}

func fromRow(row sqlc.Task) Task {
	return Task{
		ID: row.ID, Title: row.Title, Completed: row.Completed,
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}
