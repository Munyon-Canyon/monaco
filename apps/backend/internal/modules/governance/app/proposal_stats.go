package app

import (
	"context"
	"log/slog"
	"math"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	bpsPerWhole = 10_000
	maxDecimals = 19
)

type Stats struct {
	TreasuryShareBps *int64
	ClosesAt         time.Time
	ClosedAt         *time.Time
	PriceMicros      *money.Micros
	Shares           *money.BaseUnits
	SharesExact      bool
	TokenDecimals    uint8
	SinceBps         *int64
}

type MyVote struct {
	Order, Of, YesOrder, NoOrder int
	Decided                      bool
}

func TreasuryShareBps(value, pot money.Micros) (*int64, error) {
	share, ok, err := shareBps(value, pot)
	if err != nil || !ok {
		return nil, err
	}
	return &share, nil
}

func EstimatedShares(usdc, price money.Micros, decimals uint8) (*money.BaseUnits, error) {
	units, ok, err := estimatedUnits(usdc, price, decimals)
	if err != nil || !ok {
		return nil, err
	}
	shares := money.NewBaseUnits(units, decimals)
	return &shares, nil
}

func estimatedUnits(usdc, price money.Micros, decimals uint8) (uint64, bool, error) {
	if price.IsZero() || decimals > maxDecimals {
		return 0, false, nil
	}
	units, err := money.MulDiv(usdc.Uint64(), money.OneWhole(decimals), price.Uint64())
	return units, err == nil, err
}

func SinceBps(fill, now money.Micros) (*int64, error) {
	bps, ok, err := sinceBps(fill, now)
	if err != nil || !ok {
		return nil, err
	}
	return &bps, nil
}

func sinceBps(fill, now money.Micros) (int64, bool, error) {
	if fill.IsZero() || now.IsZero() {
		return 0, false, nil
	}
	delta, err := now.Delta(fill)
	if err != nil {
		return 0, false, err
	}
	raw := delta.Int64()
	magnitude := unsigned(raw)
	if raw < 0 {
		magnitude = unsigned(-(raw + 1)) + 1
	}
	bps, err := money.MulDiv(magnitude, bpsPerWhole, fill.Uint64())
	if err != nil {
		return 0, false, err
	}
	if bps > math.MaxInt64 {
		return 0, false, errs.New(errs.CodeInvalidInput, "governance.SinceBps", slog.Uint64("bps", bps))
	}
	if raw < 0 {
		return -int64(bps), true, nil
	}
	return int64(bps), true, nil
}

func shareBps(value, pot money.Micros) (int64, bool, error) {
	if pot.IsZero() {
		return 0, false, nil
	}
	share, err := money.MulDiv(value.Uint64(), bpsPerWhole, pot.Uint64())
	if err != nil || share > math.MaxInt64 {
		return 0, false, err
	}
	return new(big.Int).SetUint64(share).Int64(), true, nil
}

func rowShareValue(kind string, usdc, quoteOut int64) money.Micros {
	if kind == string(domain.KindSell) {
		return money.MicrosFromUint64(unsigned(quoteOut))
	}
	return money.MicrosFromUint64(unsigned(usdc))
}

func rowStats(r sqlc.GetProposalRow) (Stats, error) {
	out := Stats{ClosesAt: r.ExpiresAt}
	if r.ClosedAt.Valid {
		closed := r.ClosedAt.Time
		out.ClosedAt = &closed
	}
	if !r.PotValueMicros.Valid {
		return out, nil
	}
	value := rowShareValue(r.Kind, r.UsdcMicros.Int64, r.QuoteOutAmount.Int64)
	share, err := TreasuryShareBps(value, money.MicrosFromUint64(unsigned(r.PotValueMicros.Int64)))
	out.TreasuryShareBps = share
	return out, err
}

type priceInputs struct {
	id       ids.ProposalID
	kind     string
	usdc     money.Micros
	token    uint64
	decimals uint8
	live     money.Micros
	fill     Fill
}

func withPrices(ctx context.Context, s Stats, in priceInputs) Stats {
	s.TokenDecimals = in.decimals
	if !in.live.IsZero() {
		live := in.live
		s.PriceMicros = &live
	}
	s.Shares, s.SharesExact = sharesOf(ctx, in)
	if in.fill.Confirmed {
		fill := money.MicrosFromUint64(in.fill.PriceMicros)
		since, err := SinceBps(fill, in.live)
		overflowed(ctx, in.id, "since_bps", err)
		s.SinceBps = since
	}
	return s
}

func QuotedStats(s Stats, d domain.Draft) Stats {
	decimals := d.TokenAmount.Decimals()
	s.TokenDecimals = decimals
	usdc, tokens := d.USDCMicros, d.TokenAmount.Uint64()
	if d.Kind == domain.KindSell {
		usdc = money.MicrosFromUint64(d.QuoteOut)
	} else {
		tokens = d.QuoteOut
	}
	if price := money.PricePerUnit(usdc, tokens, decimals); !price.IsZero() {
		s.PriceMicros = &price
	}
	if tokens > 0 {
		shares := money.NewBaseUnits(tokens, decimals)
		s.Shares, s.SharesExact = &shares, d.Kind == domain.KindSell
	}
	return s
}

func sharesOf(ctx context.Context, in priceInputs) (*money.BaseUnits, bool) {
	exact := func(units uint64) (*money.BaseUnits, bool) {
		if units == 0 {
			return nil, false
		}
		shares := money.NewBaseUnits(units, in.decimals)
		return &shares, true
	}
	switch {
	case in.kind == string(domain.KindSell):
		return exact(in.token)
	case in.fill.Confirmed:
		return exact(in.fill.TokenAmount)
	}
	shares, err := EstimatedShares(in.usdc, in.live, in.decimals)
	overflowed(ctx, in.id, "shares", err)
	return shares, false
}

func overflowed(ctx context.Context, id ids.ProposalID, stat string, err error) {
	if err != nil {
		observability.Degraded(ctx, observability.GovernanceStatsOverflow,
			slog.String("proposal_id", id.String()), slog.String("stat", stat))
	}
}

func myVoteOf(row sqlc.BallotOrderRow, decided bool) *MyVote {
	return &MyVote{
		Order: int(row.Order), Of: int(row.Voters), YesOrder: int(row.YesOrder), NoOrder: int(row.NoOrder),
		Decided: decided,
	}
}
