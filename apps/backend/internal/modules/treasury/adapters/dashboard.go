package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Dashboard struct {
	q    *sqlc.Queries
	usdc chain.SolanaAddress
}

var _ port.Dashboard = Dashboard{}

func NewDashboard(db sqlc.DBTX, usdc chain.SolanaAddress) Dashboard {
	return Dashboard{q: sqlc.New(db), usdc: usdc}
}

func (d Dashboard) LedgerTotals(
	ctx context.Context, from, to time.Time, size bucket.Size,
) ([]port.LedgerBucket, error) {
	const op = "treasury.Dashboard.LedgerTotals"
	rows, err := d.q.LedgerTotals(ctx, sqlc.LedgerTotalsParams{
		Bucket: string(size), Usdc: string(d.usdc), FromAt: from, ToAt: to,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out := make([]port.LedgerBucket, len(rows))
	for i, row := range rows {
		out[i], err = ledgerBucket(row)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op)
		}
	}
	return out, nil
}

func ledgerBucket(row sqlc.LedgerTotalsRow) (port.LedgerBucket, error) {
	deposits, depositErr := flow(row.DepositCount, row.DepositMicros)
	funds, fundErr := flow(row.FundCount, row.FundMicros)
	cashOuts, cashOutErr := flow(row.CashOutCount, row.CashOutMicros)
	withdrawals, withdrawalErr := flow(row.WithdrawalCount, row.WithdrawalMicros)
	buy, buyErr := money.ParseMicros(row.SwapBuyMicros)
	sell, sellErr := money.ParseMicros(row.SwapSellMicros)
	if err := errors.Join(depositErr, fundErr, cashOutErr, withdrawalErr, buyErr, sellErr); err != nil {
		return port.LedgerBucket{}, err
	}
	return port.LedgerBucket{
		Start: row.BucketStart.UTC(), Deposits: deposits, Funds: funds, CashOuts: cashOuts,
		Withdrawals: withdrawals, SwapBuy: buy, SwapSell: sell,
	}, nil
}

func flow(count int64, micros string) (port.Flow, error) {
	usdc, err := money.ParseMicros(micros)
	return port.Flow{Count: count, USDC: usdc}, err
}

func (d Dashboard) PlatformBalanceTotal(ctx context.Context) (money.Micros, error) {
	const op = "treasury.Dashboard.PlatformBalanceTotal"
	raw, err := d.q.PlatformBalanceTotal(ctx, string(d.usdc))
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	total, err := money.ParseSignedMicros(raw)
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	micros, err := total.Micros()
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return micros, nil
}
