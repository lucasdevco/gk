package task

import "errors"

// Domain errors are independent of HTTP and can be matched through wrapped causes.
var (
	ErrInvalidTitle = errors.New("task title must contain between 1 and 200 characters")
	ErrNotFound     = errors.New("task not found")
)
