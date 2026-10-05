package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	WithdrawalPollInterval = 5 * time.Second
	withdrawalBatch        = 100
)

type SignatureStatuses interface {
	SignatureStatuses(context.Context, []chain.Signature) ([]solana.Status, error)
}

type WithdrawalPollerDeps struct {
	UoW       *db.UnitOfWork
	Reads     sqlc.DBTX
	Clock     clock.Clock
	Chain     SignatureStatuses
	Transfers func() (Transfers, error)
	Hints     HintPublisher
	UnsentAge time.Duration
}

type WithdrawalPoller struct{ d WithdrawalPollerDeps }

func NewWithdrawalPoller(d WithdrawalPollerDeps) *WithdrawalPoller { return &WithdrawalPoller{d: d} }

func (*WithdrawalPoller) Name() string { return "funding.withdrawals" }

func (*WithdrawalPoller) Interval() time.Duration { return WithdrawalPollInterval }

type withdrawalMove struct {
	id     uuid.UUID
	user   ids.UserID
	amount money.Micros
	from   domain.WithdrawalStatus
	to     domain.WithdrawalStatus
	code   string
	event  events.Event
}

func (p *WithdrawalPoller) Tick(ctx context.Context) (poller.Report, error) {
	now := p.d.Clock.Now()
	q := sqlc.New(p.d.Reads)
	created, createdErr := q.ListStaleCreatedWithdrawals(ctx, sqlc.ListStaleCreatedWithdrawalsParams{
		OlderThan: now.Add(-p.d.UnsentAge), MaxRows: withdrawalBatch,
	})
	submitted, submittedErr := q.ListSubmittedWithdrawals(ctx, withdrawalBatch)
	if err := errors.Join(createdErr, submittedErr); err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, "funding.WithdrawalPoller.list")
	}
	report := poller.Report{Scanned: len(created) + len(submitted)}
	var failures []error
	for _, row := range created {
		amount, err := money.ParseMicros(row.AmountMicros)
		if err != nil {
			failures = append(failures, errs.Wrap(err, errs.CodeDecodeFailed, "funding.WithdrawalPoller.amount"))
			continue
		}
		moved, err := p.move(ctx, withdrawalMove{
			id: row.ID, user: ids.UserIDFrom(row.UserID), amount: amount,
			from: domain.WithdrawalCreated, to: domain.WithdrawalFailed, code: domain.WithdrawalNotSent,
		})
		report.Changed += moved
		failures = append(failures, err)
	}
	if len(submitted) > 0 {
		changed, err := p.resolveSubmitted(ctx, submitted)
		report.Changed += changed
		failures = append(failures, err)
	}
	return report, errors.Join(failures...)
}

func (p *WithdrawalPoller) resolveSubmitted(
	ctx context.Context, rows []sqlc.ListSubmittedWithdrawalsRow,
) (int, error) {
	sigs := make([]chain.Signature, len(rows))
	for i, row := range rows {
		sigs[i] = chain.Signature(row.TxSignature.String)
	}
	statuses, err := p.d.Chain.SignatureStatuses(ctx, sigs)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.WithdrawalPoller.statuses")
	}
	if len(statuses) != len(rows) {
		return 0, errs.New(errs.CodeDecodeFailed, "funding.WithdrawalPoller.statuses",
			slog.Int("statuses", len(statuses)), slog.Int("withdrawals", len(rows)))
	}
	changed := 0
	var failures []error
	for i, row := range rows {
		moved, err := p.resolve(ctx, row, statuses[i])
		changed += moved
		failures = append(failures, err)
	}
	return changed, errors.Join(failures...)
}

func (p *WithdrawalPoller) resolve(
	ctx context.Context, row sqlc.ListSubmittedWithdrawalsRow, status solana.Status,
) (int, error) {
	amount, err := money.ParseMicros(row.AmountMicros)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeDecodeFailed, "funding.WithdrawalPoller.amount")
	}
	if row.LastValidBlockHeight.Int64 < 0 {
		return 0, errs.New(errs.CodeDecodeFailed, "funding.WithdrawalPoller.height",
			slog.String("withdrawal_id", row.ID.String()))
	}
	m := withdrawalMove{id: row.ID, user: ids.UserIDFrom(row.UserID), amount: amount, from: domain.WithdrawalSubmitted}
	signature := chain.Signature(row.TxSignature.String)
	lastValid := uint64(row.LastValidBlockHeight.Int64)
	switch {
	case status.State == solana.StateFinalized && !status.Failed:
		m.to = domain.WithdrawalConfirmed
		m.event = events.WithdrawalConfirmed{
			V: 1, WithdrawalID: row.ID, UserID: row.UserID, AmountMicros: amount,
			ToAddress: chain.SolanaAddress(row.ToAddress), TxSignature: signature,
		}
	case status.State == solana.StateFinalized:
		m.to, m.code = domain.WithdrawalFailed, domain.WithdrawalTransactionFailed
	case status.BlockhashExpired(lastValid):
		m.to, m.code = domain.WithdrawalFailed, domain.WithdrawalBlockhashExpired
	case status.State == solana.StateNotFound:
		return 0, p.rebroadcast(ctx, row, signature, lastValid)
	default:
		return 0, nil
	}
	if m.to == domain.WithdrawalFailed {
		m.event = events.WithdrawalFailed{
			V: 1, WithdrawalID: row.ID, UserID: row.UserID, AmountMicros: amount, Code: m.code,
		}
	}
	return p.move(ctx, m)
}

func (p *WithdrawalPoller) rebroadcast(
	ctx context.Context, row sqlc.ListSubmittedWithdrawalsRow, signature chain.Signature, lastValid uint64,
) error {
	transfers, err := p.d.Transfers()
	if err != nil {
		return err
	}
	err = transfers.Broadcast(ctx, relayer.SignedTx{
		Bytes: row.SignedTx, Signature: signature, LastValidBlockHeight: lastValid,
	})
	if err != nil {
		observability.Degraded(ctx, observability.FundingWithdrawalBroadcastFailed,
			slog.String("withdrawal_id", row.ID.String()), slog.String("code", string(errs.CodeOf(err))))
	}
	return nil
}

func (p *WithdrawalPoller) move(ctx context.Context, m withdrawalMove) (int, error) {
	moved := 0
	err := p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		var n int64
		var err error
		if m.to == domain.WithdrawalConfirmed {
			n, err = q.ConfirmWithdrawal(ctx, sqlc.ConfirmWithdrawalParams{ID: m.id, CompletedAt: p.d.Clock.Now()})
		} else {
			n, err = q.FailWithdrawal(ctx, sqlc.FailWithdrawalParams{
				ID: m.id, FailCode: m.code, CompletedAt: p.d.Clock.Now(),
			})
		}
		if err != nil || n == 0 {
			return err
		}
		moved = 1
		tx.AfterCommit(func(ctx context.Context) {
			p.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(m.user), nil)
			observability.Info(ctx, observability.FundingWithdrawalMoved,
				slog.String("withdrawal_id", m.id.String()), slog.String("user_id", m.user.String()),
				slog.String("amount_micros", m.amount.String()),
				slog.String("status_before", string(m.from)), slog.String("status_after", string(m.to)))
		})
		if m.event == nil {
			return nil
		}
		return tx.Events.Append(ctx, m.event)
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.WithdrawalPoller.move")
	}
	return moved, nil
}
