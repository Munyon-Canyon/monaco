package adapters

import (
	"context"
	"log/slog"
	"maps"
	"math/big"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Balance struct {
	Owner  string
	Asset  string
	Amount money.SignedMicros
}

type BalanceRule func(payload []byte) ([]Balance, error)

func CheckLedger(
	usdc domain.Asset, rules map[events.Type]BalanceRule,
) func(context.Context, *pgxpool.Pool) ([]string, error) {
	return func(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
		q := sqlc.New(pool)
		diffs, err := invariants(ctx, q, usdc)
		if err != nil || len(rules) == 0 {
			return diffs, err
		}
		drift, err := balanceDrift(ctx, q, rules)
		return append(diffs, drift...), err
	}
}

func invariants(ctx context.Context, q *sqlc.Queries, usdc domain.Asset) ([]string, error) {
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
	costs, err := costBasisDrift(ctx, q, usdc)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(unbalanced)+len(split)+len(cabals)+len(costs)+len(users))
	for _, r := range unbalanced {
		out = append(out, r.Ledger+" "+r.TxnID+" sums to "+r.Total+" in "+r.Asset)
	}
	for _, r := range split {
		out = append(out, "transfer "+r.TransferID+" has headers in "+r.Statuses)
	}
	for _, r := range cabals {
		out = append(out, "cabal_positions "+r.CabalID+" "+r.Asset+": entries "+r.Entries+", position "+r.Position)
	}
	out = append(out, costs...)
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
	ledger, err := ledgerBalances(ctx, q, rules)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	applyLedgerBalances(rules, ledger, sum)
	var out []string
	for _, key := range slices.Sorted(maps.Keys(sums)) {
		if pair := sums[key]; pair[0].Cmp(pair[1]) != 0 {
			out = append(out, "balance "+key+": ledger "+pair[0].String()+", events "+pair[1].String())
		}
	}
	return out, nil
}

func applyLedgerBalances(
	rules map[events.Type]BalanceRule, ledger []ledgerBalance, sum func(string, int, *big.Int),
) {
	for _, r := range ledger {
		key := r.Owner + " " + r.Asset
		if includeLedgerBalance(rules, key) {
			v, _ := new(big.Int).SetString(r.Balance, 10)
			sum(key, 0, v)
		}
	}
}

type ledgerBalance struct {
	Owner   string
	Asset   string
	Balance string
}

func ledgerBalances(ctx context.Context, q *sqlc.Queries, rules map[events.Type]BalanceRule) ([]ledgerBalance, error) {
	if !walletRulesOnly(rules) {
		rows, err := q.LedgerBalances(ctx)
		return mapLedgerBalances(rows), err
	}
	rows, err := q.DepositLedgerBalances(ctx)
	return mapDepositLedgerBalances(rows), err
}

func mapLedgerBalances(rows []sqlc.LedgerBalancesRow) []ledgerBalance {
	balances := make([]ledgerBalance, len(rows))
	for i, row := range rows {
		balances[i] = ledgerBalance{Owner: row.Owner, Asset: row.Asset, Balance: row.Balance}
	}
	return balances
}

func mapDepositLedgerBalances(rows []sqlc.DepositLedgerBalancesRow) []ledgerBalance {
	balances := make([]ledgerBalance, len(rows))
	for i, row := range rows {
		balances[i] = ledgerBalance{Owner: row.Owner, Asset: row.Asset, Balance: row.Balance}
	}
	return balances
}

func includeLedgerBalance(rules map[events.Type]BalanceRule, key string) bool {
	if !walletRulesOnly(rules) {
		return true
	}
	return strings.HasPrefix(key, "wallet:")
}

func walletRulesOnly(rules map[events.Type]BalanceRule) bool {
	for t := range rules {
		if t != events.TypeDepositCredited && t != events.TypeWithdrawalConfirmed {
			return false
		}
	}
	return len(rules) > 0
}
