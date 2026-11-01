package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	sideBuy  = "buy"
	sideSell = "sell"
)

type TradeMarks struct {
	q      *sqlc.Queries
	assets AssetNames
}

func NewTradeMarks(db sqlc.DBTX, assets AssetNames) *TradeMarks {
	return &TradeMarks{q: sqlc.New(db), assets: assets}
}

type pairKey struct {
	cabal  uuid.UUID
	symbol string
}

func (t *TradeMarks) TradeMarks(
	ctx context.Context,
	pairs []port.CabalAsset,
) (map[port.CabalAsset][]port.TradeMark, error) {
	out := make(map[port.CabalAsset][]port.TradeMark, len(pairs))
	if len(pairs) == 0 {
		return out, nil
	}
	names, err := t.assets.AssetNames(ctx)
	if err != nil {
		return nil, err
	}
	var params sqlc.ListTradesForPairsParams
	byMint := map[pairKey]port.CabalAsset{}
	for _, pair := range pairs {
		mint, ok := mintOf(names, pair.Symbol)
		if !ok {
			continue
		}
		key := pairKey{cabal: pair.CabalID.UUID(), symbol: string(mint)}
		if _, dup := byMint[key]; dup {
			continue
		}
		byMint[key] = pair
		params.CabalIds = append(params.CabalIds, key.cabal)
		params.Assets = append(params.Assets, key.symbol)
	}
	if len(params.Assets) == 0 {
		return out, nil
	}
	rows, err := t.q.ListTradesForPairs(ctx, params)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "treasury.TradeMarks")
	}
	grouped := map[pairKey][]trade{}
	for _, row := range rows {
		tr, err := tradeOf(row)
		if err != nil {
			return nil, err
		}
		key := pairKey{cabal: row.CabalID, symbol: row.Asset}
		grouped[key] = append(grouped[key], tr)
	}
	for key, pair := range byMint {
		name := names[domain.Asset(key.symbol)]
		marks, err := replayTrades(grouped[key], name)
		if err != nil {
			return nil, err
		}
		out[pair] = marks
	}
	return out, nil
}

type trade struct {
	id    uuid.UUID
	at    time.Time
	side  string
	units uint64
	usdc  uint64

	cashOut bool
}

func tradeOf(row sqlc.ListTradesForPairsRow) (trade, error) {
	const op = "treasury.TradeMarks.row"
	usdc, usdcErr := row.UsdcMicros.Int64Value()
	units, unitsErr := row.Units.Int64Value()
	if usdcErr != nil || unitsErr != nil || !usdc.Valid || !units.Valid || usdc.Int64 < 0 || units.Int64 <= 0 {
		return trade{}, errs.New(errs.CodeInternal, op, slog.String("trade_id", row.ID.String()))
	}
	return trade{
		id: row.ID, at: row.OccurredAt.UTC(), side: row.Kind, units: uint64(units.Int64), usdc: uint64(usdc.Int64),
		cashOut: row.CashoutSale,
	}, nil
}

type replayed struct {
	held, cost uint64
	cycle      time.Time
}

func (r *replayed) buy(tr trade) error {
	cost, err := money.MicrosFromUint64(r.cost).Add(money.MicrosFromUint64(tr.usdc))
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "treasury.TradeMarks.buy")
	}
	if r.held == 0 {
		r.cycle = tr.at
	}
	r.held, r.cost = r.held+tr.units, cost.Uint64()
	return nil
}

func (r *replayed) sell(tr trade) (money.Micros, error) {
	removed, err := money.MulDiv(r.cost, tr.units, r.held)
	if tr.units > r.held || err != nil {
		return money.Micros{}, errs.New(
			errs.CodeInternal,
			"treasury.TradeMarks.sell",
			slog.String("trade_id", tr.id.String()),
		)
	}
	r.held, r.cost = r.held-tr.units, r.cost-removed
	return money.MicrosFromUint64(removed), nil
}

func replayTrades(trades []trade, name AssetName) ([]port.TradeMark, error) {
	marks := make([]port.TradeMark, 0, len(trades))
	var state replayed
	for _, tr := range trades {
		num, den := int64(1), int64(1)
		if name.MultiplierAt != nil {
			num, den = name.MultiplierAt(tr.at)
		}
		units := money.NewBaseUnits(tr.units, name.Decimals)
		price, _, err := domain.TradePriceMicros(money.MicrosFromUint64(tr.usdc), units, num, den)
		if err != nil {
			return nil, err
		}
		mark := port.TradeMark{
			SwapID: tr.id, At: tr.at, Side: tr.side, Units: units,
			USDCMicros: money.MicrosFromUint64(tr.usdc), PriceMicros: price,
		}
		switch tr.side {
		case sideBuy:
			err = state.buy(tr)
		case sideSell:
			mark.CostOfSoldMicros, err = state.sell(tr)
		default:
			err = errs.New(errs.CodeInternal, "treasury.TradeMarks.replay", slog.String("side", tr.side))
		}
		if err != nil {
			return nil, err
		}
		mark.CycleStart = state.cycle
		if !tr.cashOut {
			marks = append(marks, mark)
		}
	}
	return marks, nil
}
