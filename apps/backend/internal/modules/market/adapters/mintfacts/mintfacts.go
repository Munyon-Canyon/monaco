package mintfacts

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

var _ app.MintFacts = (*Chain)(nil)

type Chain struct {
	rpc *solana.Client
}

func New(rpc *solana.Client) *Chain { return &Chain{rpc: rpc} }

func (c *Chain) Facts(
	ctx context.Context, mint domain.Mint,
) (decimals uint8, multiplierNum, multiplierDen uint64, err error) {
	cfg, err := c.rpc.MintConfig(ctx, mint.Address())
	if err != nil {
		return 0, 0, 0, errs.Wrap(err, errs.CodeOf(err), "market.MintFacts.Facts", slog.String("mint", mint.String()))
	}
	return cfg.Mint.Decimals, cfg.UIMultiplier.Num, cfg.UIMultiplier.Den, nil
}
