package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	CloseReminderInterval = 30 * time.Second
	CloseReminderBatch    = 100
)

type ReminderMembers interface {
	IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error)
}

type CloseReminderPoller struct {
	uow     *db.UnitOfWork
	reads   sqlc.DBTX
	members ReminderMembers
	clock   clock.Clock
	Every   time.Duration
}

func NewCloseReminderPoller(
	uow *db.UnitOfWork, reads sqlc.DBTX, members ReminderMembers, c clock.Clock,
) *CloseReminderPoller {
	return &CloseReminderPoller{uow: uow, reads: reads, members: members, clock: c}
}

func (*CloseReminderPoller) Name() string { return "governance.close_reminders" }

func (p *CloseReminderPoller) Interval() time.Duration {
	if p.Every > 0 {
		return p.Every
	}
	return CloseReminderInterval
}

func (p *CloseReminderPoller) Tick(ctx context.Context) (poller.Report, error) {
	now := p.clock.Now()
	due, err := sqlc.New(p.reads).DueReminders(ctx, sqlc.DueRemindersParams{Now: now, Batch: CloseReminderBatch})
	if err != nil {
		return poller.Report{}, err
	}
	report := poller.Report{Scanned: len(due)}
	var failed []error
	for _, row := range due {
		raised, err := p.remind(ctx, row)
		failed = append(failed, err)
		if raised {
			report.Changed++
		}
	}
	return report, errors.Join(failed...)
}

func (p *CloseReminderPoller) remind(ctx context.Context, row sqlc.DueRemindersRow) (bool, error) {
	var raised bool
	now := p.clock.Now()
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		prop, claimed, err := claimReminder(ctx, sqlc.New(tx.Queries()), row, now)
		if err != nil || !claimed {
			return err
		}
		raised, err = raiseCloseReminder(ctx, tx, p.members, prop, row.UserID, now)
		return err
	})
	if err != nil {
		return false, err
	}
	return raised, nil
}

func claimReminder(
	ctx context.Context, q *sqlc.Queries, row sqlc.DueRemindersRow, now time.Time,
) (sqlc.LockProposalRow, bool, error) {
	const op = "governance.CloseReminders"
	prop, err := q.LockProposal(ctx, sqlc.LockProposalParams{ID: row.ProposalID, VoterID: row.UserID})
	if err != nil {
		return sqlc.LockProposalRow{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	held, err := q.LockReminder(ctx, sqlc.LockReminderParams(row))
	if errors.Is(err, sql.ErrNoRows) {
		return sqlc.LockProposalRow{}, false, nil
	}
	if err != nil {
		return sqlc.LockProposalRow{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	if held.SentAt.Valid {
		return sqlc.LockProposalRow{}, false, nil
	}
	n, err := q.MarkReminderSent(
		ctx,
		sqlc.MarkReminderSentParams{ProposalID: row.ProposalID, UserID: row.UserID, Now: now},
	)
	if err != nil {
		return sqlc.LockProposalRow{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	return prop, n > 0, nil
}

func raiseCloseReminder(
	ctx context.Context, tx db.Tx, members ReminderMembers, prop sqlc.LockProposalRow, user uuid.UUID, now time.Time,
) (bool, error) {
	const op = "governance.CloseReminders"
	if prop.Status != string(domain.StatusOpen) || !now.Before(prop.ExpiresAt) {
		return false, nil
	}
	member, err := members.IsMember(ctx, ids.CabalIDFrom(prop.CabalID), ids.UserIDFrom(user))
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if !member {
		return false, nil
	}
	q := sqlc.New(tx.Queries())
	voted, err := q.HasBallot(ctx, sqlc.HasBallotParams{ProposalID: prop.ID, VoterID: user})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, op)
	}
	if voted {
		return false, nil
	}
	tally, err := q.CountBallots(ctx, prop.ID)
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, op)
	}
	err = tx.Events.Append(ctx, events.ProposalCloseReminderDue{
		V: 1, ProposalID: prop.ID, CabalID: prop.CabalID, UserID: user, Kind: prop.Kind, Symbol: prop.Symbol.String,
		USDCMicros:  money.MicrosFromUint64(unsigned(prop.UsdcMicros.Int64)),
		TokenAmount: unsigned(prop.TokenAmount.Int64), ExpiresAt: prop.ExpiresAt,
		Yes: int(tally.Yes), No: int(tally.No), Voters: int(tally.Voters),
	})
	return err == nil, err
}
