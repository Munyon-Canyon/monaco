package app

import (
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const leaveCabalOp = "cabal.LeaveCabal"

type TreasuryReads interface {
	ShareUnits(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (money.SharesUnits, error)
	PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error)
}

type LeaveCabal struct {
	ActorID ids.UserID
	CabalID ids.CabalID
}

type LeaveCabalHandler struct {
	uow      *db.UnitOfWork
	reads    sqlc.DBTX
	treasury TreasuryReads
}

func NewLeaveCabalHandler(uow *db.UnitOfWork, reads sqlc.DBTX, treasury TreasuryReads) *LeaveCabalHandler {
	return &LeaveCabalHandler{uow: uow, reads: reads, treasury: treasury}
}

type leaveCheck struct {
	cabalID ids.CabalID
	in      domain.LeaveInput
	potRead bool
}

func (h *LeaveCabalHandler) Handle(ctx context.Context, cmd LeaveCabal) error {
	members, err := sqlc.New(h.reads).ListMembers(ctx, cmd.CabalID.UUID())
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, leaveCabalOp)
	}
	i := slices.IndexFunc(members, func(m sqlc.ListMembersRow) bool { return m.UserID == cmd.ActorID.UUID() })
	if i < 0 {
		return errs.New(errs.CodeNotCabalMember, leaveCabalOp)
	}
	shares, err := h.treasury.ShareUnits(ctx, cmd.CabalID, cmd.ActorID)
	if err != nil {
		return err
	}
	check := &leaveCheck{cabalID: cmd.CabalID, in: domain.LeaveInput{
		IsCreator: members[i].Role == string(domain.RoleCreator), MemberCount: len(members), ShareUnits: shares,
	}}
	if err := h.check(ctx, check); err != nil {
		return err
	}
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return h.leave(ctx, tx, cmd, check)
	})
}

func (h *LeaveCabalHandler) leave(ctx context.Context, tx db.Tx, cmd LeaveCabal, check *leaveCheck) error {
	q := sqlc.New(tx.Queries())
	locked, err := q.LockMembers(ctx, cmd.CabalID.UUID())
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, leaveCabalOp)
	}
	i := slices.IndexFunc(locked, func(m sqlc.LockMembersRow) bool { return m.UserID == cmd.ActorID.UUID() })
	if i < 0 {
		return errs.New(errs.CodeNotCabalMember, leaveCabalOp)
	}
	check.in.IsCreator, check.in.MemberCount = locked[i].Role == string(domain.RoleCreator), len(locked)
	if err := h.check(ctx, check); err != nil {
		return err
	}
	gone, err := q.DeleteMember(ctx, sqlc.DeleteMemberParams{CabalID: cmd.CabalID.UUID(), UserID: cmd.ActorID.UUID()})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, leaveCabalOp)
	}
	return tx.Events.Append(ctx, events.CabalMemberLeft{
		V: 1, CabalID: cmd.CabalID.UUID(), UserID: cmd.ActorID.UUID(), WasVoter: gone.CanVote,
	})
}

func (h *LeaveCabalHandler) check(ctx context.Context, c *leaveCheck) error {
	if c.in.MemberCount == 1 && c.in.ShareUnits.IsZero() && !c.potRead {
		pot, err := h.treasury.PotValue(ctx, c.cabalID)
		if err != nil {
			return err
		}
		c.in.PotValue, c.potRead = pot, true
	}
	return domain.CheckLeave(c.in)
}
