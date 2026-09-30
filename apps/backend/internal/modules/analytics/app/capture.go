package app

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Capture struct {
	UUID       uuid.UUID
	Event      string
	DistinctID string
	Timestamp  time.Time
	Properties map[string]any
	Set        map[string]any
}

type PostHog interface {
	Capture(ctx context.Context, batch []Capture) error
}
