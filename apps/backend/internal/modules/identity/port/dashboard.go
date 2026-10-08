package port

import "context"

type StatusCount struct {
	AuthState     string
	AccountStatus string
	Users         int64
}

type Dashboard interface {
	StatusCounts(ctx context.Context) ([]StatusCount, error)
}
