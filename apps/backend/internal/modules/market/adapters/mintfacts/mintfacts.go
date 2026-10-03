package mintfacts

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

var _ app.MintFacts = (*Chain)(nil)

type Chain struct {
	rpc *solana.Client
}

func New(rpc *solana.Client) *Chain { return &Chain{rpc: rpc} }

func (c *Chain) Facts(
	ctx context.Context, mints []domain.Mint,
) (map[domain.Mint]app.MintFact, map[domain.Mint]error, error) {
	addresses := make([]chain.SolanaAddress, len(mints))
	byAddress := make(map[chain.SolanaAddress]domain.Mint, len(mints))
	for i, mint := range mints {
		addresses[i] = mint.Address()
		byAddress[addresses[i]] = mint
	}
	configs, failures, err := c.rpc.MintConfigs(ctx, addresses)
	facts := make(map[domain.Mint]app.MintFact, len(configs))
	for address, cfg := range configs {
		facts[byAddress[address]] = app.MintFact{
			Decimals: cfg.Mint.Decimals, MultiplierNum: cfg.UIMultiplier.Num, MultiplierDen: cfg.UIMultiplier.Den,
			NextMultiplierNum: cfg.NextUIMultiplier.Num, NextMultiplierDen: cfg.NextUIMultiplier.Den,
			NextMultiplierAt: cfg.NextUIMultiplierAt,
		}
	}
	errsByMint := make(map[domain.Mint]error, len(failures))
	for address, failure := range failures {
		mint := byAddress[address]
		errsByMint[mint] = errs.Wrap(
			failure, errs.CodeOf(failure), "market.MintFacts.Facts", slog.String("mint", mint.String()),
		)
	}
	if err != nil {
		err = errs.Wrap(err, errs.CodeOf(err), "market.MintFacts.Facts")
	}
	return facts, errsByMint, err
}
