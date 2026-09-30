package adapters

import (
	"context"
	"log/slog"
	"maps"
	"math/big"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Balance struct {
	Owner  string
	Asset  string
	Amount money.SignedMicros
}

type BalanceRule func(payload []byte) ([]Balance, error)

func CheckLedger(rules map[events.Type]BalanceRule) func(context.Context, *pgxpool.Pool) ([]string, error) {
	return func(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
		q := sqlc.New(pool)
		diffs, err := invariants(ctx, q)
		if err != nil || len(rules) == 0 {
			return diffs, err
		}
		drift, err := balanceDrift(ctx, q, rules)
		return append(diffs, drift...), err
	}
}

func invariants(ctx context.Context, q *sqlc.Queries) ([]string, error) {
	const op = "treasury.CheckLedger"
	unbalanced, err := q.UnbalancedTxns(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	split, err := q.SplitTransfers(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	cabals, err := q.CabalPositionDrift(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	users, err := q.UserPositionDrift(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := make([]string, 0, len(unbalanced)+len(split)+len(cabals)+len(users))
	for _, r := range unbalanced {
		out = append(out, r.Ledger+" "+r.TxnID+" sums to "+r.Total+" in "+r.Asset)
	}
	for _, r := range split {
		out = append(out, "transfer "+r.TransferID+" has headers in "+r.Statuses)
	}
	for _, r := range cabals {
		out = append(out, "cabal_positions "+r.CabalID+" "+r.Asset+": entries "+r.Entries+", position "+r.Position)
	}
	for _, r := range users {
		out = append(out, "user_positions "+r.UserID+" "+r.CabalID+": entries "+r.Entries+", position "+r.Position)
	}
	return out, nil
}

func balanceDrift(ctx context.Context, q *sqlc.Queries, rules map[events.Type]BalanceRule) ([]string, error) {
	const op = "treasury.CheckLedger"
	types := make([]string, 0, len(rules))
	for t := range rules {
		types = append(types, string(t))
	}
	rows, err := q.MoneyEvents(ctx, types)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	sums := map[string][2]*big.Int{}
	sum := func(key string, side int, v *big.Int) {
		pair := sums[key]
		if pair[0] == nil {
			pair = [2]*big.Int{new(big.Int), new(big.Int)}
		}
		pair[side].Add(pair[side], v)
		sums[key] = pair
	}
	for _, r := range rows {
		moves, err := rules[events.Type(r.Type)](r.Payload)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeOf(err), op, slog.String("type", r.Type))
		}
		for _, m := range moves {
			sum(m.Owner+" "+m.Asset, 1, big.NewInt(m.Amount.Int64()))
		}
	}
	ledger, err := q.LedgerBalances(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	for _, r := range ledger {
		v, _ := new(big.Int).SetString(r.Balance, 10)
		sum(r.Owner+" "+r.Asset, 0, v)
	}
	var out []string
	for _, key := range slices.Sorted(maps.Keys(sums)) {
		if pair := sums[key]; pair[0].Cmp(pair[1]) != 0 {
			out = append(out, "balance "+key+": ledger "+pair[0].String()+", events "+pair[1].String())
		}
	}
	return out, nil
}
