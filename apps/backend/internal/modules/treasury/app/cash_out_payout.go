package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	CashOutPayoutWait = 60 * time.Second
	CashOutPayoutPoll = 2 * time.Second
)

type PayoutChain interface {
	PayoutStatus(ctx context.Context, sig chain.Signature, lastValid uint64) (domain.PayoutReading, error)
}

type Hints interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type CashOutPayoutDeps struct {
	UoW       *db.UnitOfWork
	Reads     sqlc.DBTX
	Ledger    Ledger
	IDs       ids.Generator
	Clock     clock.Clock
	Chain     PayoutChain
	Transfers func() (PayoutTransfers, error)
	Wallets   PayoutWallets
	USDC      chain.Mint
	Hints     Hints
}

type CashOutPayouts struct{ d CashOutPayoutDeps }

func NewCashOutPayouts(d CashOutPayoutDeps) *CashOutPayouts { return &CashOutPayouts{d: d} }

type payoutJob struct {
	id      uuid.UUID
	cabal   ids.CabalID
	user    ids.UserID
	units   money.SharesUnits
	kept    money.SharesUnits
	payout  money.Micros
	slice   money.Micros
	status  domain.CashOutStatus
	selling bool
	latest  domain.PayoutAttempt
	signed  relayer.SignedTx
}

type pace uint8

const (
	paceStop pace = iota + 1
	paceAgain
	paceWait
)

func (p *CashOutPayouts) Advance(ctx context.Context, jobID uuid.UUID, wait time.Duration, heartbeat func()) error {
	deadline := p.d.Clock.Now().Add(wait)
	for {
		job, err := p.load(ctx, p.d.Reads, jobID)
		if err != nil {
			return err
		}
		next, err := p.step(ctx, job)
		if err != nil || next == paceStop {
			return err
		}
		if next == paceAgain {
			continue
		}
		if !p.d.Clock.Now().Before(deadline) {
			return nil
		}
		if heartbeat != nil {
			heartbeat()
		}
		select {
		case <-ctx.Done():
			return errs.Wrap(ctx.Err(), errs.CodeUpstreamUnavailable, "treasury.CashOutPayouts.Advance")
		case <-p.d.Clock.After(CashOutPayoutPoll):
		}
	}
}

func (p *CashOutPayouts) EndedShort(ctx context.Context, jobID uuid.UUID) bool {
	job, err := p.load(ctx, p.d.Reads, jobID)
	return err == nil && job.status == domain.CashOutPartial
}

func (p *CashOutPayouts) step(ctx context.Context, job payoutJob) (pace, error) {
	switch domain.NextPayoutStep(job.status, job.selling, job.latest) {
	case domain.PayoutIdle:
		return paceStop, nil
	case domain.PayoutAwaitSale:
		return paceWait, nil
	case domain.PayoutGiveUp:
		return paceStop, p.fail(ctx, job, 0)
	case domain.PayoutSign:
		return paceAgain, p.sign(ctx, job)
	case domain.PayoutSend:
		return paceAgain, p.send(ctx, job)
	case domain.PayoutCheck:
	}
	return p.check(ctx, job)
}

func (p *CashOutPayouts) check(ctx context.Context, job payoutJob) (pace, error) {
	reading, err := p.d.Chain.PayoutStatus(ctx, job.signed.Signature, job.signed.LastValidBlockHeight)
	if err != nil {
		return paceStop, err
	}
	switch reading.Verdict() {
	case domain.PayoutLanded:
		return paceStop, p.complete(ctx, job)
	case domain.PayoutRejected:
		return paceStop, p.fail(ctx, job, job.latest.Number)
	case domain.PayoutLapsed:
		return paceAgain, p.lapse(ctx, job)
	case domain.PayoutMissing:
		p.broadcast(ctx, job)
	case domain.PayoutInFlight:
	}
	return paceWait, nil
}

func (p *CashOutPayouts) lapse(ctx context.Context, job payoutJob) error {
	return p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := sqlc.New(tx.Queries()).MoveCashOutPayout(ctx, sqlc.MoveCashOutPayoutParams{
			JobID: job.id, Attempt: job.latest.Number, ToStatus: string(domain.PayoutExpired),
			FromStatuses: []string{string(domain.PayoutSigned), string(domain.PayoutBroadcast)},
		})
		return err
	})
}

func (p *CashOutPayouts) complete(ctx context.Context, job payoutJob) error {
	return p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		now, err := p.lock(ctx, tx, &job)
		if err != nil || now.status != domain.CashOutPaying || now.latest.Number != job.latest.Number {
			return err
		}
		q := sqlc.New(tx.Queries())
		end := domain.CashOutEnd(now.payout, now.slice)
		if err := p.move(ctx, q, now, end, ""); err != nil {
			return err
		}
		if err := p.settleAttempt(ctx, q, now, domain.PayoutConfirmed); err != nil {
			return err
		}
		if err := p.postPaid(ctx, tx, now); err != nil {
			return err
		}
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
		to, _ := domain.NextCashOut(domain.CashOutPaying, end)
		p.moved(tx, now, to)
		tx.AfterCommit(func(ctx context.Context) {
			p.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(now.user), nil)
			p.d.Hints.PublishHint(ctx, events.CabalActivityChangedHint(now.cabal), nil)
		})
		return tx.Events.Append(ctx, ended(now, end))
	})
}

func ended(job payoutJob, end domain.CashOutEvent) events.Event {
	if end == domain.CashOutCompletePartial {
		return events.CashOutPartial{
			V: 1, JobID: job.id, CabalID: job.cabal.UUID(), UserID: job.user.UUID(),
			ShareUnitsBurned: job.units.Uint64(), ShareUnitsReturned: job.kept.Uint64(),
			PayoutMicros: job.payout, Signature: job.signed.Signature,
		}
	}
	return events.CashOutCompleted{
		V: 1, JobID: job.id, CabalID: job.cabal.UUID(), UserID: job.user.UUID(), ShareUnits: job.units.Uint64(),
		PayoutMicros: job.payout, Signature: job.signed.Signature,
	}
}

func (p *CashOutPayouts) postPaid(ctx context.Context, tx db.Tx, job payoutJob) error {
	const op = "treasury.CashOutPayouts.postPaid"
	paid, err := job.payout.Delta(money.Micros{})
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), op)
	}
	usdc := domain.MintAsset(p.d.USDC.Address)
	txn, err := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: p.d.IDs.NewV7(), CabalID: job.cabal, Kind: domain.CabalCashOut, Status: domain.TxnPending,
		TransferID: job.id, TxSignature: job.signed.Signature,
	}, []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: usdc, Amount: money.SignedMicrosFromInt64(-paid.Int64())},
		{Account: domain.CabalMembers, Asset: usdc, Amount: paid},
	})
	if err != nil {
		return err
	}
	if err := p.d.Ledger.At(p.d.Clock.Now()).PostCabalTxn(ctx, tx, txn); err != nil {
		return err
	}
	settled, err := p.d.Ledger.SetStatus(ctx, tx, job.id, domain.TxnPending, domain.TxnSettled)
	if err == nil && !settled {
		err = errs.New(errs.CodeInternal, op, slog.String("job_id", job.id.String()))
	}
	return err
}

func (p *CashOutPayouts) fail(ctx context.Context, job payoutJob, rejected int16) error {
	return p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		now, err := p.lock(ctx, tx, &job)
		if err != nil || now.status != job.status || now.latest != job.latest {
			return err
		}
		q := sqlc.New(tx.Queries())
		if err := p.move(ctx, q, now, domain.CashOutFail, string(errs.CodePayoutFailed)); err != nil {
			return err
		}
		if rejected > 0 {
			if err := p.settleAttempt(ctx, q, now, domain.PayoutFailed); err != nil {
				return err
			}
		}
		if err := p.giveBack(ctx, tx, now); err != nil {
			return err
		}
		p.moved(tx, now, domain.CashOutFailed)
		tx.AfterCommit(func(ctx context.Context) {
			p.d.Hints.PublishHint(ctx, events.CabalActivityChangedHint(now.cabal), nil)
		})
		return tx.Events.Append(ctx, events.CashOutFailed{
			V: 1, JobID: now.id, CabalID: now.cabal.UUID(), UserID: now.user.UUID(), ShareUnits: now.units.Uint64(),
			Code: string(errs.CodePayoutFailed),
		})
	})
}

func (p *CashOutPayouts) giveBack(ctx context.Context, tx db.Tx, job payoutJob) error {
	const op = "treasury.CashOutPayouts.giveBack"
	failed, err := p.d.Ledger.FailUnpaired(ctx, tx, job.id)
	if err != nil {
		return err
	}
	if !failed {
		return errs.New(errs.CodeInternal, op, slog.String("job_id", job.id.String()))
	}
	txn, err := domain.CashOutReturn(domain.UserTxnHeader{
		ID: p.d.IDs.NewV7(), UserID: job.user, CabalID: job.cabal, Kind: domain.UserCashOut, Status: domain.TxnSettled,
	}, domain.MintAsset(p.d.USDC.Address), domain.SaleSettlement{Unpaid: job.payout, Returned: job.units})
	if err != nil {
		return err
	}
	return p.d.Ledger.At(p.d.Clock.Now()).PostUserTxn(ctx, tx, txn)
}

func (p *CashOutPayouts) settleAttempt(
	ctx context.Context, q *sqlc.Queries, job payoutJob, to domain.PayoutStatus,
) error {
	n, err := q.MoveCashOutPayout(ctx, sqlc.MoveCashOutPayoutParams{
		JobID: job.id, Attempt: job.latest.Number, ToStatus: string(to),
		FromStatuses: []string{string(domain.PayoutSigned), string(domain.PayoutBroadcast)},
	})
	if err == nil && n == 0 {
		err = errs.New(errs.CodeVersionConflict, "treasury.CashOutPayouts.settleAttempt",
			slog.String("job_id", job.id.String()), slog.Int("attempt", int(job.latest.Number)))
	}
	return err
}

func (p *CashOutPayouts) move(
	ctx context.Context, q *sqlc.Queries, job payoutJob, event domain.CashOutEvent, code string,
) error {
	to, err := domain.NextCashOut(job.status, event)
	if err != nil {
		return err
	}
	n, err := q.EndCashOutJob(ctx, sqlc.EndCashOutJobParams{
		ID: job.id, FromStatus: string(job.status), ToStatus: string(to), ResultCode: code, At: p.d.Clock.Now(),
	})
	if err == nil && n == 0 {
		err = errs.New(errs.CodeVersionConflict, "treasury.CashOutPayouts.move", slog.String("job_id", job.id.String()))
	}
	return err
}

func (p *CashOutPayouts) moved(tx db.Tx, job payoutJob, to domain.CashOutStatus) {
	payload, _ := json.Marshal(map[string]string{"job_id": job.id.String()})
	tx.AfterCommit(func(ctx context.Context) {
		p.d.Hints.PublishHint(ctx, events.UserCashOutChangedHint(job.user), payload)
		observability.Info(ctx, observability.TreasuryCashOutMoved,
			slog.String("job_id", job.id.String()), slog.String("before", string(job.status)),
			slog.String("after", string(to)), slog.String("payout_micros", job.payout.String()))
	})
}

func (p *CashOutPayouts) lock(ctx context.Context, tx db.Tx, job *payoutJob) (payoutJob, error) {
	if err := p.d.Ledger.LockCabal(ctx, tx, job.cabal); err != nil {
		return payoutJob{}, err
	}
	return p.load(ctx, tx.Queries(), job.id)
}

func (p *CashOutPayouts) load(ctx context.Context, dbtx sqlc.DBTX, id uuid.UUID) (payoutJob, error) {
	const op = "treasury.CashOutPayouts.load"
	row, err := sqlc.New(dbtx).CashOutPayoutJob(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return payoutJob{}, errs.Wrap(err, errs.CodeNotFound, op, slog.String("job_id", id.String()))
	}
	if err != nil {
		return payoutJob{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return parsePayoutJob(row)
}

func parsePayoutJob(row sqlc.CashOutPayoutJobRow) (payoutJob, error) {
	units, unitsErr := money.ParseSharesUnits(row.ShareUnits)
	returned, returnedErr := money.ParseSharesUnits(row.ReturnedUnits)
	payout, payoutErr := money.ParseMicros(row.PayoutMicros)
	slice, sliceErr := money.ParseMicros(row.SliceMicros)
	status, statusErr := domain.ParseCashOutStatus(row.Status)
	attempt, attemptErr := domain.ParsePayoutStatus(row.PayoutStatus)
	if err := errors.Join(unitsErr, returnedErr, payoutErr, sliceErr, statusErr, attemptErr); err != nil {
		return payoutJob{}, errs.Wrap(err, errs.CodeDecodeFailed, "treasury.CashOutPayouts.parse")
	}
	burned, err := units.Sub(returned)
	if err != nil || row.LastValidBlockHeight < 0 {
		return payoutJob{}, errs.New(errs.CodeDecodeFailed, "treasury.CashOutPayouts.parse",
			slog.String("job_id", row.ID.String()))
	}
	return payoutJob{
		id: row.ID, cabal: ids.CabalIDFrom(row.CabalID), user: ids.UserIDFrom(row.UserID), units: burned,
		kept: returned, payout: payout, slice: slice, status: status, selling: row.Selling,
		latest: domain.PayoutAttempt{Number: row.Attempt, Status: attempt},
		signed: relayer.SignedTx{
			Bytes: row.SignedTx, Signature: chain.Signature(row.Signature),
			LastValidBlockHeight: uint64(row.LastValidBlockHeight),
		},
	}, nil
}
