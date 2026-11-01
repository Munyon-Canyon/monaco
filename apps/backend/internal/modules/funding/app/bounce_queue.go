package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type PendingBounce struct {
	ExternalDepositID uuid.UUID
	CabalID           ids.CabalID
	Status            string
	Amount            string
	Mint              string
	ReturnAddress     string
	Attempts          int32
	Since             time.Time
}

func OpenBounceStatuses() []string {
	return []string{
		string(domain.ExternalDetected), string(domain.ExternalBouncing),
		string(domain.ExternalBounceFailed),
	}
}

func ExternalDepositStatuses() []string {
	return []string{
		string(domain.ExternalDetected), string(domain.ExternalBouncing), string(domain.ExternalReturned),
		string(domain.ExternalBounceFailed), string(domain.ExternalHeld), string(domain.ExternalIgnoredDust),
		string(domain.ExternalIgnoredUnknown),
	}
}

func BounceQueue(ctx context.Context, q sqlc.DBTX, statuses []string, limit int) ([]PendingBounce, error) {
	rows, err := sqlc.New(q).ListExternalDepositsByStatus(ctx, sqlc.ListExternalDepositsByStatusParams{
		Statuses: statuses, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "funding.BounceQueue")
	}
	out := make([]PendingBounce, len(rows))
	for i, r := range rows {
		out[i] = PendingBounce{
			ExternalDepositID: r.ID, CabalID: ids.CabalIDFrom(r.CabalID), Status: r.Status, Amount: r.Amount,
			Mint: r.Mint, ReturnAddress: r.ReturnAddress, Attempts: r.BounceAttempts, Since: r.DetectedAt,
		}
	}
	return out, nil
}
