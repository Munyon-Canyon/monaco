package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

type SignatureStatuses interface {
	SignatureStatuses(ctx context.Context, sigs []chain.Signature) ([]solana.Status, error)
}

type PayoutChain struct{ Solana func() SignatureStatuses }

func (c PayoutChain) PayoutStatus(
	ctx context.Context, sig chain.Signature, lastValid uint64,
) (domain.PayoutReading, error) {
	const op = "treasury.PayoutChain.PayoutStatus"
	statuses, err := c.Solana().SignatureStatuses(ctx, []chain.Signature{sig})
	if err != nil {
		return domain.PayoutReading{}, err
	}
	if len(statuses) != 1 {
		return domain.PayoutReading{}, errs.New(errs.CodeDecodeFailed, op, slog.Int("statuses", len(statuses)))
	}
	status := statuses[0]
	state, ok := map[solana.State]domain.PayoutState{
		solana.StateNotFound:   domain.PayoutNotFound,
		solana.StateProcessing: domain.PayoutProcessing,
		solana.StateFinalized:  domain.PayoutFinalized,
	}[status.State]
	if !ok {
		return domain.PayoutReading{}, errs.New(errs.CodeDecodeFailed, op, slog.Int("state", int(status.State)))
	}
	return domain.PayoutReading{
		State: state, Failed: status.Failed, Expired: status.BlockhashExpired(lastValid),
	}, nil
}
