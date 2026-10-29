package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CatchUp interface {
	Count(ctx context.Context, user ids.UserID) (int, error)
}
