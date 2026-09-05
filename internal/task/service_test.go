package task

import (
	"context"
	"errors"
	"testing"
)

type memoryRepository struct{ tasks []Task }

func (m *memoryRepository) List(context.Context) ([]Task, error) {
	return append([]Task(nil), m.tasks...), nil
}

func (m *memoryRepository) Create(_ context.Context, id, title string) (Task, error) {
	created := Task{ID: id, Title: title}
	m.tasks = append(m.tasks, created)
	return created, nil
}

func (m *memoryRepository) SetCompleted(_ context.Context, id string, completed bool) (Task, error) {
	for i := range m.tasks {
		if m.tasks[i].ID == id {
			m.tasks[i].Completed = completed
			return m.tasks[i], nil
		}
	}
	return Task{}, ErrNotFound
}

func TestCreateTrimsAndValidatesTitle(t *testing.T) {
	repository := &memoryRepository{}
	service := NewService(repository)

	created, err := service.Create(context.Background(), "  ship it  ")
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "ship it" {
		t.Fatalf("title = %q", created.Title)
	}

	_, err = service.Create(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("error = %v, want ErrInvalidTitle", err)
	}
}
