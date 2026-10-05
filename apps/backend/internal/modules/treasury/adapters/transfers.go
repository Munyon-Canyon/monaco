package adapters

import (
	"context"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

type Transfers struct {
	transfers func() (*relayer.Transfers, error)
}

func NewTransfers(cfg config.Config, clk clock.Clock) Transfers {
	return Transfers{transfers: sync.OnceValues(func() (*relayer.Transfers, error) {
		const op = "treasury.Transfers"
		signer, err := privy.New(cfg, clk)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		r, err := relayer.New(cfg, solana.New(cfg, clk))
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		return relayer.NewTransfers(r, signer), nil
	})}
}

func (t Transfers) Build(ctx context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error) {
	transfers, err := t.transfers()
	if err != nil {
		return relayer.SignedTx{}, err
	}
	return transfers.Build(ctx, spec)
}

func (t Transfers) Broadcast(ctx context.Context, tx relayer.SignedTx) error {
	transfers, err := t.transfers()
	if err != nil {
		return err
	}
	return transfers.Broadcast(ctx, tx)
}
