package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type SaleResult struct {
	JobID     uuid.UUID
	SwapID    uuid.UUID
	CabalID   ids.CabalID
	BatchSize int
	Confirmed bool
	USDCOut   money.Micros
}

type CashOutSales struct {
	ledger Ledger
	ids    ids.Generator
}

func NewCashOutSales(ledger Ledger, g ids.Generator) CashOutSales {
	return CashOutSales{ledger: ledger, ids: g}
}

func (s CashOutSales) Started(ctx context.Context, tx db.Tx, e events.CashOutStarted, at time.Time) error {
	if e.SellUSDC.IsZero() {
		return nil
	}
	return s.sell(ctx, tx, ids.CabalIDFrom(e.CabalID), e.JobID, false, at)
}

func (s CashOutSales) NoLegs(ctx context.Context, tx db.Tx, cabal ids.CabalID, jobID uuid.UUID, at time.Time) error {
	return s.sell(ctx, tx, cabal, jobID, true, at)
}

func (s CashOutSales) sell(
	ctx context.Context, tx db.Tx, cabal ids.CabalID, jobID uuid.UUID, noLegs bool, at time.Time,
) error {
	job, err := s.lock(ctx, tx, cabal, jobID)
	if err == nil && job.Status == string(domain.CashOutStarted) {
		_, err = sqlc.New(tx.Queries()).MoveCashOutJob(ctx, sqlc.MoveCashOutJobParams{
			ID: jobID, FromStatus: string(domain.CashOutStarted), ToStatus: string(domain.CashOutSelling), At: at,
		})
		job.Status = string(domain.CashOutSelling)
	}
	if err == nil {
		err = s.settle(ctx, tx, cabal, jobID, job, noLegs, at)
	}
	return err
}

func (s CashOutSales) Result(ctx context.Context, tx db.Tx, r SaleResult, at time.Time) error {
	job, err := s.lock(ctx, tx, r.CabalID, r.JobID)
	if err != nil || (job.Status != string(domain.CashOutStarted) && job.Status != string(domain.CashOutSelling)) {
		return err
	}
	status := "failed"
	if r.Confirmed {
		status = "confirmed"
	}
	err = sqlc.New(tx.Queries()).InsertCashOutSell(ctx, sqlc.InsertCashOutSellParams{
		SwapID: r.SwapID, JobID: r.JobID, BatchSize: int64(r.BatchSize), Status: status,
		UsdcOutMicros: r.USDCOut.String(), At: at,
	})
	if err == nil {
		err = s.settle(ctx, tx, r.CabalID, r.JobID, job, false, at)
	}
	return err
}

func (s CashOutSales) lock(
	ctx context.Context, tx db.Tx, cabal ids.CabalID, job uuid.UUID,
) (sqlc.LockCashOutJobRow, error) {
	var row sqlc.LockCashOutJobRow
	err := s.ledger.LockCabal(ctx, tx, cabal)
	if err == nil {
		row, err = sqlc.New(tx.Queries()).LockCashOutJob(ctx, sqlc.LockCashOutJobParams{ID: job, CabalID: cabal.UUID()})
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = errs.Wrap(err, errs.CodeNotFound, "treasury.CashOutSales.lock", slog.String("job_id", job.String()))
	}
	return row, err
}

func (s CashOutSales) settle(
	ctx context.Context, tx db.Tx, cabal ids.CabalID, jobID uuid.UUID, job sqlc.LockCashOutJobRow, noLegs bool,
	at time.Time,
) error {
	if job.Status != string(domain.CashOutSelling) {
		return nil
	}
	q := sqlc.New(tx.Queries())
	tally, err := q.CashOutSaleTally(ctx, jobID)
	if err != nil || (!noLegs && (tally.BatchSize == 0 || tally.Results < tally.BatchSize)) {
		return err
	}
	settled, units, slice, err := sale(job, tally)
	if err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.AfterSellConfirm)
	to := settled.Status
	payout, code := settled.Paid, ""
	if !settled.Unpaid.IsZero() {
		code = string(errs.CodeSaleShort)
	}
	if to == domain.CashOutFailed {
		payout = slice
	}
	if _, err := q.SettleCashOutSale(ctx, sqlc.SettleCashOutSaleParams{
		ID: jobID, ToStatus: string(to), PayoutMicros: payout.String(), ReturnedUnits: settled.Returned.String(),
		ResultCode: code, At: at,
	}); err != nil {
		return err
	}
	if err := s.apply(ctx, tx, cabal, ids.UserIDFrom(job.UserID), jobID, units, to, settled, at); err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		observability.Info(ctx, observability.TreasuryCashOutSaleSettled,
			slog.String("job_id", jobID.String()), slog.String("cabal_id", cabal.String()),
			slog.String("before", string(domain.CashOutSelling)), slog.String("after", string(to)),
			slog.String("paid_micros", settled.Paid.String()), slog.String("returned_units", settled.Returned.String()))
	})
	return nil
}

func (s CashOutSales) apply(
	ctx context.Context, tx db.Tx, cabal ids.CabalID, user ids.UserID, job uuid.UUID, units money.SharesUnits,
	to domain.CashOutStatus, settled domain.SaleSettlement, at time.Time,
) error {
	if err := s.giveBack(ctx, tx, cabal, user, settled, at); err != nil || to != domain.CashOutFailed {
		return err
	}
	return s.fail(ctx, tx, cabal, user, job, units)
}

func sale(
	job sqlc.LockCashOutJobRow, tally sqlc.CashOutSaleTallyRow,
) (domain.SaleSettlement, money.SharesUnits, money.Micros, error) {
	units, unitsErr := money.ParseSharesUnits(job.ShareUnits)
	slice, sliceErr := money.ParseMicros(job.SliceMicros)
	paid, paidErr := money.ParseMicros(tally.Paid)
	if err := errors.Join(unitsErr, sliceErr, paidErr); err != nil {
		return domain.SaleSettlement{}, units, slice, errs.Wrap(
			err,
			errs.CodeDecodeFailed,
			"treasury.CashOutSales.sale",
		)
	}
	settled, err := domain.SettleSale(units, slice, paid)
	return settled, units, slice, err
}

func (s CashOutSales) giveBack(
	ctx context.Context, tx db.Tx, cabal ids.CabalID, user ids.UserID, settled domain.SaleSettlement, at time.Time,
) error {
	if settled.Unpaid.IsZero() {
		return nil
	}
	txn, err := domain.CashOutReturn(domain.UserTxnHeader{
		ID: s.ids.NewV7(), UserID: user, CabalID: cabal, Kind: domain.UserCashOut, Status: domain.TxnSettled,
	}, s.ledger.usdc, settled)
	if err != nil {
		return err
	}
	return s.ledger.At(at).PostUserTxn(ctx, tx, txn)
}

func (CashOutSales) fail(
	ctx context.Context, tx db.Tx, cabal ids.CabalID, user ids.UserID, job uuid.UUID, units money.SharesUnits,
) error {
	return tx.Events.Append(ctx, events.CashOutFailed{
		V: 1, JobID: job, CabalID: cabal.UUID(), UserID: user.UUID(), ShareUnits: units.Uint64(),
		Code: string(errs.CodeSaleShort),
	})
}
