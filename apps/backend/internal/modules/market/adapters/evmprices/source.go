package evmprices

import (
	"context"
	"errors"
	"log/slog"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const ReferenceMicros = 100_000_000

var (
	_ multichain.EVMPriceSource     = Source{}
	_ multichain.EVMSellPriceSource = Source{}
)

type Source struct {
	quoter    multichain.Quoter
	reference money.Micros
}

func New(quoter multichain.Quoter) Source {
	return Source{quoter: quoter, reference: money.MicrosFromUint64(ReferenceMicros)}
}

func (s Source) Prices(
	ctx context.Context, listings []multichain.Listing,
) (map[multichain.ListingID]multichain.Price, error) {
	return s.prices(ctx, listings, multichain.SideBuy)
}

func (s Source) SellPrices(
	ctx context.Context, listings []multichain.Listing,
) (map[multichain.ListingID]multichain.Price, error) {
	return s.prices(ctx, listings, multichain.SideSell)
}

func (s Source) prices(
	ctx context.Context, listings []multichain.Listing, side multichain.Side,
) (map[multichain.ListingID]multichain.Price, error) {
	out := make(map[multichain.ListingID]multichain.Price, len(listings))
	var failed []error
	for _, l := range listings {
		q, err := s.quoter.Quote(ctx, l, side, s.reference)
		if err == nil {
			err = withinImpact(q)
		}
		if err == nil {
			var perShare money.Micros
			if perShare, err = PerShare(q, l); err == nil {
				out[l.ID] = multichain.Price{PerShareMicros: perShare, AsOf: q.QuotedAt}
				continue
			}
		}
		failed = append(failed, errs.Wrap(err, errs.CodeOf(err), "evmprices.Source.Prices",
			slog.String("listing", l.ID.String()), slog.String("symbol", l.Symbol)))
	}
	return out, errors.Join(failed...)
}

func PerShare(q multichain.Quote, l multichain.Listing) (money.Micros, error) {
	const op = "evmprices.PerShare"
	if q.GrossOutput == nil || q.GrossOutput.Sign() <= 0 || l.SharesPerToken == nil || l.SharesPerToken.Sign() <= 0 {
		return money.Micros{}, errs.New(errs.CodeDecodeFailed, op, slog.String("listing", l.ID.String()))
	}
	price := new(big.Rat).SetInt(new(big.Int).SetUint64(q.InputMicros.Uint64()))
	price.Mul(price, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(l.Decimals)), nil)))
	price.Quo(price, new(big.Rat).SetInt(q.GrossOutput))
	price.Quo(price, l.SharesPerToken)
	floor := new(big.Int).Quo(price.Num(), price.Denom())
	if !floor.IsUint64() || floor.Sign() == 0 {
		return money.Micros{}, errs.New(errs.CodeDecodeFailed, op,
			slog.String("listing", l.ID.String()), slog.String("price", floor.String()))
	}
	return money.MicrosFromUint64(floor.Uint64()), nil
}

func withinImpact(q multichain.Quote) error {
	var impact uint64
	for _, c := range q.Costs {
		if c.Kind == multichain.CostPriceImpact {
			impact += c.Micros.Uint64()
		}
	}
	bps := new(big.Int).Mul(new(big.Int).SetUint64(impact), big.NewInt(10_000))
	if bps.Cmp(
		new(big.Int).Mul(new(big.Int).SetUint64(q.InputMicros.Uint64()), big.NewInt(multichain.MaxImpactBps)),
	) > 0 {
		return errs.New(errs.CodeSlippageExceeded, "evmprices.withinImpact",
			slog.Uint64("impact_micros", impact), slog.Uint64("input_micros", q.InputMicros.Uint64()))
	}
	return nil
}
