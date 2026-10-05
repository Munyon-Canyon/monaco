package adapters

import (
	"cmp"
	"context"
	"log/slog"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const unitsDisplayDecimals = 4

type potStore interface {
	CabalPotSnapshot(context.Context, uuid.UUID) ([]sqlc.CabalPotSnapshotRow, error)
	CabalMemberShares(context.Context, uuid.UUID) ([]sqlc.CabalMemberSharesRow, error)
}

func (q *Queries) CabalPot(
	ctx context.Context, cabalID ids.CabalID, viewer ids.UserID, member bool,
) (port.CabalPot, error) {
	const op = "treasury.Queries.CabalPot"
	rows, err := q.pot.CabalPotSnapshot(ctx, cabalID.UUID())
	if err != nil {
		return port.CabalPot{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	head := rows[0]
	pot, err := q.potHoldings(ctx, cabalID, rows)
	if err != nil {
		return port.CabalPot{}, err
	}
	if pot.CashMicros, err = subtractCashOutReservation(pot.CashMicros, head.CashOutReservedMicros); err != nil {
		return port.CabalPot{}, err
	}
	if err := pot.total(); err != nil {
		return port.CabalPot{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	net, err := signed(head.NetContributedMicros)
	if err != nil {
		return port.CabalPot{}, err
	}
	if pot.PnLMicros, err = signedSub(microsInt(pot.PotValueMicros), big.NewInt(net.Int64())); err != nil {
		return port.CabalPot{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if bps, ok := domain.ReturnBps(pot.PnLMicros, net); ok {
		pot.ReturnBps = &bps
	}
	if !member {
		return pot.CabalPot, nil
	}
	totalShares, err := shares(head.TotalShares)
	if err != nil {
		return port.CabalPot{}, err
	}
	me, err := q.slice(ctx, cabalID, viewer, totalShares, pot.PotValueMicros)
	if err != nil {
		return port.CabalPot{}, err
	}
	pot.Me = &me
	return pot.CabalPot, nil
}

type potBuilder struct{ port.CabalPot }

func (q *Queries) potHoldings(
	ctx context.Context, cabalID ids.CabalID, rows []sqlc.CabalPotSnapshotRow,
) (potBuilder, error) {
	pot := potBuilder{port.CabalPot{CabalID: cabalID, PricesAsOf: q.clock.Now(), Holdings: []port.Holding{}}}
	assets := map[chain.SolanaAddress]app.Asset{}
	positions := make([]port.Position, 0, len(rows))
	var err error
	for _, row := range rows {
		if !row.Asset.Valid {
			continue
		}
		var position port.Position
		position, err = q.position(ctx, row.Asset.String, row.Units, row.CostBasisMicros, assets)
		if err != nil {
			return potBuilder{}, err
		}
		if position.Mint == q.usdc {
			pot.CashMicros = money.MicrosFromUint64(position.Units.Uint64())
			continue
		}
		positions = append(positions, position)
	}
	if len(positions) == 0 {
		return pot, nil
	}
	if pot.Holdings, pot.PricesAsOf, err = q.priceHoldings(ctx, positions, assets, pot.PricesAsOf); err != nil {
		return potBuilder{}, err
	}
	slices.SortFunc(pot.Holdings, func(a, b port.Holding) int {
		if c := b.ValueMicros.Cmp(a.ValueMicros); c != 0 {
			return c
		}
		return cmp.Compare(a.Symbol, b.Symbol)
	})
	return pot, nil
}

func (q *Queries) priceHoldings(
	ctx context.Context, positions []port.Position, assets map[chain.SolanaAddress]app.Asset, asOf time.Time,
) ([]port.Holding, time.Time, error) {
	prices, err := q.prices.LatestPrices(ctx)
	if err != nil {
		return nil, time.Time{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.CabalPot")
	}
	holdings := make([]port.Holding, 0, len(positions))
	for _, position := range positions {
		holding, observedAt, err := q.holding(ctx, position, prices, assets)
		if err != nil {
			return nil, time.Time{}, err
		}
		if observedAt.Before(asOf) {
			asOf = observedAt
		}
		holdings = append(holdings, holding)
	}
	return holdings, asOf, nil
}

func (q *Queries) holding(
	ctx context.Context,
	position port.Position,
	prices map[uuid.UUID]app.Price,
	assets map[chain.SolanaAddress]app.Asset,
) (port.Holding, time.Time, error) {
	value, err := q.positionValue(ctx, position, prices, assets)
	if err != nil {
		return port.Holding{}, time.Time{}, err
	}
	asset := assets[position.Mint]
	price := prices[asset.ID]
	pnl, err := signedSub(microsInt(value), microsInt(position.CostBasis))
	if err != nil {
		return port.Holding{}, time.Time{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.holding")
	}
	num, den := asset.UIMultiplierAt(q.clock.Now())
	return port.Holding{
		Symbol: asset.Symbol, DisplayName: asset.DisplayName, Kind: asset.Kind, TokenAmount: position.Units,
		Units:       displayUnits(position.Units, num, den),
		PriceMicros: price.Micros, ValueMicros: value, CostBasisMicros: position.CostBasis, PnLMicros: pnl,
	}, price.ObservedAt, nil
}

func (p *potBuilder) total() error {
	values := make([]money.Micros, 0, len(p.Holdings)+1)
	values = append(values, p.CashMicros)
	total := p.CashMicros.Uint64()
	for _, holding := range p.Holdings {
		values = append(values, holding.ValueMicros)
		total += holding.ValueMicros.Uint64()
	}
	weights, err := domain.WeightsBps(values)
	if err != nil {
		return err
	}
	p.PotValueMicros, p.CashWeightBps = money.MicrosFromUint64(total), weights[0]
	for i := range p.Holdings {
		p.Holdings[i].WeightBps = weights[i+1]
	}
	return nil
}

func (q *Queries) slice(
	ctx context.Context, cabalID ids.CabalID, viewer ids.UserID, total money.SharesUnits, pot money.Micros,
) (port.Slice, error) {
	const op = "treasury.Queries.slice"
	rows, err := q.pot.CabalMemberShares(ctx, cabalID.UUID())
	if err != nil {
		return port.Slice{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	units := make([]money.SharesUnits, len(rows))
	for i, row := range rows {
		if units[i], err = shares(row.ShareUnits); err != nil {
			return port.Slice{}, err
		}
	}
	bps, err := domain.SlicesBps(units)
	if err != nil {
		return port.Slice{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	i := slices.IndexFunc(rows, func(row sqlc.CabalMemberSharesRow) bool { return row.UserID == viewer.UUID() })
	if i < 0 || units[i].IsZero() {
		return port.Slice{}, nil
	}
	contributed, err := micros(rows[i].ContributedMicros)
	if err != nil {
		return port.Slice{}, err
	}
	withdrawn, err := micros(rows[i].WithdrawnMicros)
	if err != nil {
		return port.Slice{}, err
	}
	value, err := domain.PayoutFor(units[i], total, pot)
	if err != nil {
		return port.Slice{}, err
	}
	net, err := signedSub(microsInt(contributed), microsInt(withdrawn))
	if err != nil {
		return port.Slice{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	pnl, err := signedSub(microsInt(value), big.NewInt(net.Int64()))
	if err != nil {
		return port.Slice{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return port.Slice{
		ShareUnits: units[i], ValueMicros: value, SliceBps: bps[i], NetContributedMicros: net, PnLMicros: pnl,
	}, nil
}

func displayUnits(units money.BaseUnits, num, den int64) string {
	scaled := new(big.Int).SetUint64(units.Uint64())
	scaled.Mul(scaled, big.NewInt(num))
	scaled.Mul(scaled, new(big.Int).Exp(big.NewInt(10), big.NewInt(unitsDisplayDecimals), nil))
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(units.Decimals())), nil)
	scaled.Quo(scaled, divisor.Mul(divisor, big.NewInt(den)))
	return new(big.Rat).SetFrac(scaled, big.NewInt(10_000)).FloatString(unitsDisplayDecimals)
}

func microsInt(m money.Micros) *big.Int { return new(big.Int).SetUint64(m.Uint64()) }

func signedSub(a, b *big.Int) (money.SignedMicros, error) {
	d := new(big.Int).Sub(a, b)
	if !d.IsInt64() {
		return money.SignedMicros{}, errs.New(
			errs.CodeInvalidInput,
			"treasury.signedSub",
			slog.String("diff", d.String()),
		)
	}
	return money.SignedMicrosFromInt64(d.Int64()), nil
}

func signed(raw string) (money.SignedMicros, error) {
	value, err := money.ParseSignedMicros(raw)
	if err != nil {
		return money.SignedMicros{}, errs.Wrap(err, errs.CodeDecodeFailed, "treasury.Queries.signed")
	}
	return value, nil
}
