package domain

import (
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Cabal struct {
	CreatorID ids.UserID
	Rules     Rules
}

func (c Cabal) IsCreator(user ids.UserID) bool { return c.CreatorID == user }

func (c Cabal) RoleOf(user ids.UserID) Role {
	if c.IsCreator(user) {
		return RoleCreator
	}
	return RoleMember
}

type Actor struct {
	UserID ids.UserID
	Member bool
}

type AccessRequest struct {
	ID        ids.AccessRequestID
	Direction Direction
	UserID    ids.UserID
	InvitedBy ids.UserID
	ExpiresAt time.Time
}

func require(ok bool, code errs.Code, op string) error {
	if ok {
		return nil
	}
	return errs.New(code, op)
}

func CanInvite(actor Actor, cabal Cabal) error {
	const op = "cabal.CanInvite"
	if !actor.Member {
		return errs.New(errs.CodeNotCabalMember, op)
	}
	switch cabal.Rules.JoinMode() {
	case JoinOpen:
		return nil
	case JoinRequest:
		return require(cabal.IsCreator(actor.UserID), errs.CodeNotCabalCreator, op)
	}
	return errs.New(errs.CodeInternal, op, slog.String("join_mode", string(cabal.Rules.JoinMode())))
}

func CanDecide(actor Actor, cabal Cabal, req AccessRequest) error {
	const op = "cabal.CanDecide"
	switch req.Direction {
	case DirectionRequest:
		return require(cabal.IsCreator(actor.UserID), errs.CodeNotCabalCreator, op)
	case DirectionInvite:
		return require(actor.UserID == req.UserID, errs.CodeForbidden, op)
	}
	return errs.New(errs.CodeInternal, op, slog.String("direction", string(req.Direction)))
}

func CanRevoke(actor Actor, cabal Cabal, req AccessRequest) error {
	const op = "cabal.CanRevoke"
	switch req.Direction {
	case DirectionRequest:
		return require(actor.UserID == req.UserID, errs.CodeCannotRevokeAccess, op)
	case DirectionInvite:
		return require(actor.UserID == req.InvitedBy || cabal.IsCreator(actor.UserID), errs.CodeCannotRevokeAccess, op)
	}
	return errs.New(errs.CodeInternal, op, slog.String("direction", string(req.Direction)))
}
