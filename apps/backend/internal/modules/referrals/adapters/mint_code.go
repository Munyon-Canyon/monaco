package adapters

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const MintRetries = 5

type MintCode struct {
	Entropy func(context.Context) io.Reader
}

func (h MintCode) Handle(ctx context.Context, tx db.Tx, e events.UserCreated, at time.Time) error {
	q, random := sqlc.New(tx.Queries()), h.Entropy(ctx)
	for range 1 + MintRetries {
		code, err := domain.NewRandomCode(random)
		if err != nil {
			return err
		}
		minted, err := q.MintReferralCode(ctx, sqlc.MintReferralCodeParams{
			Owner: e.UserID, Code: string(code), CreatedAt: at,
		})
		if err != nil || minted {
			return err
		}
	}
	return errs.New(errs.CodeInternal, "referrals.MintCode", slog.Int("retries", MintRetries))
}
