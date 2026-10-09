package port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Action struct {
	ID      uuid.UUID
	AdminID uuid.UUID
	Kind    string
	Reason  string
	At      time.Time
}

type Actions interface {
	RecentActions(ctx context.Context, targetType, targetID string, limit int) ([]Action, error)
}
