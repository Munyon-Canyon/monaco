package adapters

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type heldRow struct{ asset, units, cost string }

type evmPosition struct {
	key   string
	units *big.Int
	cost  money.Micros
}

type chainRows struct {
	solana  []heldRow
	evmCash money.Micros
	evm     []evmPosition
	first   firstBuys
}

type evmHolding struct {
	evmPosition
	quote app.ListingQuote
	value money.Micros
}

func (q *Queries) byChain(rows []heldRow) (chainRows, error) {
	var out chainRows
	for _, row := range rows {
		if err := q.place(&out, row); err != nil {
			return chainRows{}, err
		}
	}
	return out, nil
}

func (q *Queries) place(out *chainRows, row heldRow) error {
	const op = "treasury.Queries.place"
	switch domain.PlaceAsset(row.asset, q.usdc).Place {
	case domain.EVMCash:
		cash, err := micros(row.units)
		if err != nil {
			return err
		}
		if out.evmCash, err = out.evmCash.Add(cash); err != nil {
			return errs.Wrap(err, errs.CodeOf(err), op)
		}
	case domain.EVMToken:
		units, ok := new(big.Int).SetString(row.units, 10)
		if !ok || units.Sign() < 0 {
			return errs.New(errs.CodeDecodeFailed, op, slog.String("asset", row.asset))
		}
		cost, err := micros(row.cost)
		if err != nil {
			return err
		}
		out.evm = append(out.evm, evmPosition{key: row.asset, units: units, cost: cost})
	case domain.SolanaCash, domain.SolanaToken:
		out.solana = append(out.solana, row)
	}
	return nil
}

func (q *Queries) valueEVM(
	ctx context.Context, positions []evmPosition, extra ...string,
) ([]evmHolding, map[string]app.ListingQuote, time.Time, error) {
	asOf := q.clock.Now()
	if len(positions) == 0 {
		return nil, nil, asOf, nil
	}
	if q.listings == nil {
		return nil, nil, time.Time{}, priceUnavailable(nil, chain.SolanaAddress(positions[0].key))
	}
	keys := make([]string, 0, len(positions)+len(extra))
	for _, p := range positions {
		keys = append(keys, p.key)
	}
	quotes, err := q.listings(ctx, append(keys, extra...), asOf)
	if err != nil {
		return nil, nil, time.Time{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.valueEVM")
	}
	out := make([]evmHolding, 0, len(positions))
	for _, p := range positions {
		quote, ok := quotes[p.key]
		if !ok || quote.Price == nil || q.clock.Now().Sub(quote.Price.ObservedAt) > multichain.ListingPriceMaxAge {
			return nil, nil, time.Time{}, priceUnavailable(nil, chain.SolanaAddress(p.key))
		}
		value, err := quote.Value(p.units)
		if err != nil {
			return nil, nil, time.Time{}, priceUnavailable(err, chain.SolanaAddress(p.key))
		}
		if quote.Price.ObservedAt.Before(asOf) {
			asOf = quote.Price.ObservedAt
		}
		out = append(out, evmHolding{evmPosition: p, quote: quote, value: value})
	}
	return out, quotes, asOf, nil
}

func (q *Queries) offSolana(ctx context.Context, transit money.Micros, split chainRows) (money.Micros, error) {
	const op = "treasury.Queries.offSolana"
	total, err := split.evmCash.Add(transit)
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	held, _, _, err := q.valueEVM(ctx, split.evm)
	if err != nil {
		return money.Micros{}, err
	}
	for _, h := range held {
		if total, err = total.Add(h.value); err != nil {
			return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), op)
		}
	}
	return total, nil
}

func (q *Queries) offSolanaPosition(p *port.CabalPositions, asset, unitsRaw string) (bool, error) {
	const op = "treasury.Queries.offSolanaPosition"
	switch domain.PlaceAsset(asset, q.usdc).Place {
	case domain.EVMCash:
		cash, err := micros(unitsRaw)
		if err != nil {
			return true, err
		}
		if p.EVMCash, err = p.EVMCash.Add(cash); err != nil {
			return true, errs.Wrap(err, errs.CodeOf(err), op)
		}
		return true, nil
	case domain.EVMToken:
		units, ok := new(big.Int).SetString(unitsRaw, 10)
		if !ok || units.Sign() < 0 {
			return true, errs.New(errs.CodeDecodeFailed, op, slog.String("asset", asset))
		}
		p.Listings = append(p.Listings, port.ListingPosition{Key: asset, Units: units})
		return true, nil
	case domain.SolanaCash, domain.SolanaToken:
	}
	return false, nil
}
