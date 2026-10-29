package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type SetReminder struct {
	ProposalID ids.ProposalID
	UserID     ids.UserID
}

type ClearReminder struct {
	ProposalID ids.ProposalID
	UserID     ids.UserID
}

type ReminderResult struct {
	ProposalID  ids.ProposalID
	ReminderSet bool
	DueAt       *time.Time
}

type ReminderHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewReminderHandler(uow *db.UnitOfWork, c clock.Clock) *ReminderHandler {
	return &ReminderHandler{uow: uow, clock: c}
}

func (h *ReminderHandler) Set(ctx context.Context, cmd SetReminder) (ReminderResult, error) {
	var out ReminderResult
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		out, err = h.set(ctx, tx, cmd)
		return err
	})
	if err != nil {
		return ReminderResult{}, err
	}
	return out, nil
}

func (h *ReminderHandler) set(ctx context.Context, tx db.Tx, cmd SetReminder) (ReminderResult, error) {
	const op = "governance.SetReminder"
	q := sqlc.New(tx.Queries())
	p, err := q.LockProposal(ctx, sqlc.LockProposalParams{ID: cmd.ProposalID.UUID(), VoterID: cmd.UserID.UUID()})
	if err != nil {
		return ReminderResult{}, lookupFailed(err, op)
	}
	now := h.clock.Now()
	if err := mayVote(p, now, op); err != nil {
		return ReminderResult{}, err
	}
	out := ReminderResult{ProposalID: cmd.ProposalID}
	_, err = q.LockBallot(ctx, sqlc.LockBallotParams{ProposalID: p.ID, VoterID: cmd.UserID.UUID()})
	switch {
	case err == nil:
		return out, nil
	case !errors.Is(err, sql.ErrNoRows):
		return ReminderResult{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	due := domain.ReminderDue(p.ExpiresAt, now)
	err = q.UpsertReminder(ctx, sqlc.UpsertReminderParams{
		ProposalID: p.ID, UserID: cmd.UserID.UUID(), DueAt: due, CreatedAt: now,
	})
	if err != nil {
		return ReminderResult{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	out.ReminderSet, out.DueAt = true, &due
	return out, nil
}

func (h *ReminderHandler) Clear(ctx context.Context, cmd ClearReminder) (ReminderResult, error) {
	const op = "governance.ClearReminder"
	var out ReminderResult
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		found, err := q.ProposalExists(ctx, cmd.ProposalID.UUID())
		switch {
		case err != nil:
			return errs.Wrap(err, errs.CodeInternal, op)
		case !found:
			return errs.New(errs.CodeProposalNotFound, op)
		}
		if _, err := q.DeleteReminder(ctx, sqlc.DeleteReminderParams{
			ProposalID: cmd.ProposalID.UUID(), UserID: cmd.UserID.UUID(),
		}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		out = ReminderResult{ProposalID: cmd.ProposalID}
		return nil
	})
	if err != nil {
		return ReminderResult{}, err
	}
	return out, nil
}
