package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func RetryAlreadyFailed(ctx context.Context, swaps Swaps, proposal uuid.UUID, retried ids.SwapID) (bool, error) {
	latest, found, err := swaps.LatestBySource(ctx, trading.Source{Kind: "proposal", ID: proposal})
	if err != nil {
		return false, err
	}
	return found && latest.ID != retried && latest.Status == "failed", nil
}
