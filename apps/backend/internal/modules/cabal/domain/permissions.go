package domain

import (
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Cabal struct {
	Creators []ids.UserID
	Admins   []ids.UserID
	Rules    Rules
	Banned   bool
}

func (c Cabal) IsCreator(user ids.UserID) bool { return slices.Contains(c.Creators, user) }

func (c Cabal) IsAdmin(user ids.UserID) bool {
	return c.IsCreator(user) || slices.Contains(c.Admins, user)
}

func (c Cabal) RoleOf(user ids.UserID) Role {
	switch {
	case c.IsCreator(user):
		return RoleCreator
	case c.IsAdmin(user):
		return RoleAdmin
	}
	return RoleMember
}

func (c Cabal) VotersWithAdmins(requested []ids.UserID) []ids.UserID {
	voters := slices.Clone(requested)
	for _, admin := range c.Admins {
		if !c.IsCreator(admin) && !slices.Contains(voters, admin) {
			voters = append(voters, admin)
		}
	}
	return voters
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
	Blocked   bool
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
	return require(cabal.IsAdmin(actor.UserID), errs.CodeNotCabalAdmin, op)
}

func CanInviteUser(inviter, invitee Actor, cabal Cabal, blocked bool) error {
	const op = "cabal.CanInviteUser"
	if err := CanInvite(inviter, cabal); err != nil {
		return err
	}
	switch {
	case cabal.Banned:
		return errs.New(errs.CodeCabalBanned, op)
	case invitee.Member:
		return errs.New(errs.CodeAlreadyMember, op)
	case blocked:
		return errs.New(errs.CodeUserNotFound, op)
	}
	return nil
}

func CanJoin(actor Actor, cabal Cabal) error {
	const op = "cabal.CanJoin"
	switch {
	case cabal.Banned:
		return errs.New(errs.CodeCabalBanned, op)
	case actor.Member:
		return errs.New(errs.CodeAlreadyMember, op)
	case cabal.Rules.JoinMode() == JoinRequest:
		return errs.New(errs.CodeJoinNeedsRequest, op)
	}
	return nil
}

func CanRequest(actor Actor, cabal Cabal) error {
	const op = "cabal.CanRequest"
	switch {
	case cabal.Banned:
		return errs.New(errs.CodeCabalBanned, op)
	case actor.Member:
		return errs.New(errs.CodeAlreadyMember, op)
	case cabal.Rules.JoinMode() != JoinRequest:
		return errs.New(errs.CodeRequestNotNeeded, op)
	}
	return nil
}

func CanDecide(actor Actor, cabal Cabal, req AccessRequest) error {
	const op = "cabal.CanDecide"
	switch req.Direction {
	case DirectionRequest:
		return require(cabal.IsAdmin(actor.UserID), errs.CodeNotCabalAdmin, op)
	case DirectionInvite:
		return require(actor.UserID == req.UserID, errs.CodeForbidden, op)
	}
	return errs.New(errs.CodeInternal, op, slog.String("direction", string(req.Direction)))
}

func CanAdmit(cabal Cabal, req AccessRequest, now time.Time) error {
	const op = "cabal.CanAdmit"
	if cabal.Banned {
		return errs.New(errs.CodeCabalBanned, op)
	}
	if req.Direction == DirectionInvite && (req.Blocked || !cabal.IsAdmin(req.InvitedBy)) {
		return errs.New(errs.CodeAccessRequestNotPending, op)
	}
	return req.CheckNotExpired(now)
}

func CanRevoke(actor Actor, cabal Cabal, req AccessRequest) error {
	const op = "cabal.CanRevoke"
	switch req.Direction {
	case DirectionRequest:
		return require(actor.UserID == req.UserID, errs.CodeCannotRevokeAccess, op)
	case DirectionInvite:
		return require(cabal.IsAdmin(actor.UserID), errs.CodeCannotRevokeAccess, op)
	}
	return errs.New(errs.CodeInternal, op, slog.String("direction", string(req.Direction)))
}
