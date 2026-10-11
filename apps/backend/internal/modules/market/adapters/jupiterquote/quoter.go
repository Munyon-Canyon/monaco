package jupiterquote

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const quoteTTL = 30 * time.Second

var _ mc.Quoter = Quoter{}

type Quoter struct {
	client *jupiter.Client
	clock  clock.Clock
}

func NewQuoter(client *jupiter.Client, clk clock.Clock) Quoter {
	return Quoter{client: client, clock: clk}
}

func (q Quoter) Quote(ctx context.Context, l mc.Listing, side mc.Side, amount money.Micros) (mc.Quote, error) {
	const op = "jupiterquote.Quoter.Quote"
	attrs := []slog.Attr{slog.String("network", string(l.Network)), slog.String("side", string(side))}
	if side != mc.SideBuy || l.Network != chain.NetworkSolana || amount.IsZero() {
		return mc.Quote{}, errs.New(errs.CodeInvalidInput, op, attrs...)
	}
	cash := chain.NetworkSolana.Cash()
	got, err := q.client.Quote(ctx, jupiter.QuoteSpec{
		In:     jupiter.Mint{Address: cash.Address, Decimals: cash.Decimals},
		Out:    jupiter.Mint{Address: l.TokenAddress, Decimals: l.Decimals},
		Amount: money.NewBaseUnits(amount.Uint64(), cash.Decimals),
	})
	if err != nil {
		return mc.Quote{}, err
	}
	if !got.Routable {
		return mc.Quote{}, errs.New(errs.CodeNoRoute, op, attrs...)
	}
	now := q.clock.Now()
	return mc.Quote{
		ListingID: l.ID, Network: l.Network, Venue: mc.VenueJupiter, Side: side,
		InputMicros: money.MicrosFromUint64(got.InAmount.Uint64()),
		GrossOutput: new(big.Int).SetUint64(got.OutAmount.Uint64()),
		Costs:       []mc.Cost{},
		QuotedAt:    now, ExpiresAt: now.Add(quoteTTL),
	}, nil
}
