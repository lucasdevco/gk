-- name: ListTasks :many
SELECT id, title, completed, created_at, updated_at
FROM tasks
ORDER BY created_at DESC;

-- name: CreateTask :one
INSERT INTO tasks (id, title)
VALUES ($1, $2)
RETURNING id, title, completed, created_at, updated_at;

-- name: UpdateTaskCompleted :one
UPDATE tasks
SET completed = $2, updated_at = now()
WHERE id = $1
RETURNING id, title, completed, created_at, updated_at;
