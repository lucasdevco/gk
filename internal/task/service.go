// Package task is a complete, removable example business module.
package task

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidTitle = errors.New("task title must contain between 1 and 200 characters")
	ErrNotFound     = errors.New("task not found")
)

type Task struct {
	ID        string
	Title     string
	Completed bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository is the persistence seam. Production and test adapters both use it.
type Repository interface {
	List(context.Context) ([]Task, error)
	Create(context.Context, string, string) (Task, error)
	SetCompleted(context.Context, string, bool) (Task, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context) ([]Task, error) {
	return s.repository.List(ctx)
}

func (s *Service) Create(ctx context.Context, title string) (Task, error) {
	title = strings.TrimSpace(title)
	if len([]rune(title)) == 0 || len([]rune(title)) > 200 {
		return Task{}, ErrInvalidTitle
	}
	return s.repository.Create(ctx, uuid.NewString(), title)
}

func (s *Service) SetCompleted(ctx context.Context, id string, completed bool) (Task, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Task{}, ErrNotFound
	}
	return s.repository.SetCompleted(ctx, id, completed)
}
