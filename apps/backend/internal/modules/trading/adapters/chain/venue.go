package chain

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Jupiter interface {
	Quote(ctx context.Context, spec jupiter.QuoteSpec) (jupiter.Quote, error)
	Order(ctx context.Context, spec jupiter.OrderSpec) (jupiter.Order, error)
	ExecuteUntilTerminal(ctx context.Context, requestID string, signed []byte) (jupiter.ExecuteResult, error)
}

type Venue struct {
	jupiter Jupiter
}

var (
	_ app.Venue = Venue{}
	_ Jupiter   = (*jupiter.Client)(nil)
)

func NewVenue(j Jupiter) Venue { return Venue{jupiter: j} }

func (v Venue) Quote(ctx context.Context, spec app.QuoteSpec) (app.Quote, error) {
	q, err := v.jupiter.Quote(ctx, jupiter.QuoteSpec{
		In: mint(spec.InMint), Out: mint(spec.OutMint), Amount: money.NewBaseUnits(spec.InAmount, spec.InMint.Decimals),
	})
	if err != nil {
		return app.Quote{}, err
	}
	return app.Quote{
		InAmount: q.InAmount.Uint64(), OutAmount: q.OutAmount.Uint64(),
		PriceImpactBps: q.PriceImpactBps, Routable: q.Routable,
	}, nil
}

func (v Venue) Order(ctx context.Context, spec app.OrderSpec) (app.Order, error) {
	o, err := v.jupiter.Order(ctx, jupiter.OrderSpec{
		In: mint(spec.InMint), Out: mint(spec.OutMint), Taker: jupiter.SolanaAddress(spec.Taker),
		Payer:  jupiter.SolanaAddress(spec.Payer),
		Amount: money.NewBaseUnits(spec.InAmount, spec.InMint.Decimals), SlippageBps: spec.SlippageBps,
	})
	if err != nil {
		return app.Order{}, err
	}
	return app.Order{RequestID: o.RequestID, Transaction: o.Transaction}, nil
}

func (v Venue) ExecuteUntilTerminal(ctx context.Context, requestID string, signed []byte) (app.ExecuteResult, error) {
	const op = "trading.Venue.ExecuteUntilTerminal"
	r, err := v.jupiter.ExecuteUntilTerminal(ctx, requestID, signed)
	if err != nil {
		return app.ExecuteResult{}, err
	}
	status, ok := map[jupiter.Status]app.ExecuteStatus{
		jupiter.StatusSuccess: app.ExecuteSuccess,
		jupiter.StatusFailed:  app.ExecuteFailed,
		jupiter.StatusPending: app.ExecutePending,
	}[r.Status]
	if !ok {
		return app.ExecuteResult{}, errs.New(errs.CodeDecodeFailed, op, slog.Int("status", int(r.Status)))
	}
	return app.ExecuteResult{Status: status, OutAmount: r.OutAmount, ErrorCode: r.ErrorCode}, nil
}

func mint(m platform.Mint) jupiter.Mint {
	return jupiter.Mint{Address: string(m.Address), Decimals: m.Decimals}
}
