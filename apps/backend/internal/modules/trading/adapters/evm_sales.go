package adapters

import (
	"context"
	"log/slog"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const evmSaleProbeMicros = 1_000_000

var _ app.EVMSales = EVMSales{}

type EVMSales struct {
	Listings app.Listings
	Quoter   mc.Quoter
}

func (s EVMSales) Sale(ctx context.Context, asset string, units uint64) (app.EVMSale, error) {
	l, err := s.Listings.ListingByAsset(ctx, asset)
	if err != nil || !l.Enabled || units == 0 {
		return app.EVMSale{Listing: l}, err
	}
	estimate, err := s.proceeds(ctx, l, evmSaleProbeMicros, units)
	if err != nil || estimate == 0 {
		return app.EVMSale{Listing: l}, err
	}
	proceeds, err := s.proceeds(ctx, l, estimate, units)
	return app.EVMSale{Listing: l, ProceedsMicros: min(estimate, proceeds)}, err
}

func (s EVMSales) proceeds(ctx context.Context, l mc.Listing, micros, units uint64) (uint64, error) {
	q, err := s.Quoter.Quote(ctx, l, mc.SideSell, money.MicrosFromUint64(micros))
	if err != nil {
		return 0, err
	}
	scaled := new(big.Int).Mul(new(big.Int).SetUint64(micros), new(big.Int).SetUint64(units))
	if q.GrossOutput == nil || q.GrossOutput.Sign() <= 0 || !scaled.Quo(scaled, q.GrossOutput).IsUint64() {
		return 0, errs.New(errs.CodeDecodeFailed, "trading.EVMSales.Sale", slog.String("listing_id", l.ID.String()))
	}
	return scaled.Uint64(), nil
}
