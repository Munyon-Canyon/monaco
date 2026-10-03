package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Via string

const (
	ViaOpen    Via = "open"
	ViaRequest Via = "request"
	ViaInvite  Via = "invite"
)

func lockCabal(
	ctx context.Context, q *sqlc.Queries, cabalID ids.CabalID, user ids.UserID, op string,
) (domain.Cabal, domain.Actor, error) {
	cabal, actors, err := lockCabalFor(ctx, q, cabalID, op, user)
	if err != nil {
		return domain.Cabal{}, domain.Actor{}, err
	}
	return cabal, actors[0], nil
}

func lockCabalFor(
	ctx context.Context, q *sqlc.Queries, cabalID ids.CabalID, op string, users ...ids.UserID,
) (domain.Cabal, []domain.Actor, error) {
	raw := make([]uuid.UUID, 0, len(users))
	for _, user := range users {
		raw = append(raw, user.UUID())
	}
	row, err := q.LockCabalForAccess(ctx, sqlc.LockCabalForAccessParams{CabalID: cabalID.UUID(), UserIds: raw})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Cabal{}, nil, errs.New(errs.CodeCabalNotFound, op)
	}
	if err != nil {
		return domain.Cabal{}, nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	rules, err := domain.NewRules(
		row.JoinMode, row.VoterMode, row.Threshold, row.ProposalExpirySeconds, row.SlippageBps,
	)
	if err != nil {
		return domain.Cabal{}, nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	cabal := domain.Cabal{
		CreatorID: ids.UserIDFrom(row.CreatorID), Rules: rules, Banned: row.Status == string(port.StatusBanned),
	}
	actors := make([]domain.Actor, 0, len(users))
	for _, user := range users {
		actors = append(actors, domain.Actor{UserID: user, Member: slices.Contains(row.MemberIds, user.UUID())})
	}
	return cabal, actors, nil
}

type newMember struct {
	cabalID   ids.CabalID
	userID    ids.UserID
	rules     domain.Rules
	via       Via
	requestID uuid.UUID
	at        time.Time
}

func addMember(ctx context.Context, tx db.Tx, m newMember, op string) error {
	n, err := sqlc.New(tx.Queries()).InsertMember(ctx, sqlc.InsertMemberParams{
		CabalID: m.cabalID.UUID(), UserID: m.userID.UUID(), Role: string(domain.RoleMember),
		CanVote: domain.VoterFor(m.rules, domain.RoleMember), JoinedAt: m.at,
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	if n == 0 {
		return errs.New(errs.CodeAlreadyMember, op, slog.String("via", string(m.via)))
	}
	return tx.Events.Append(ctx, events.CabalMemberJoined{
		V: 1, CabalID: m.cabalID.UUID(), UserID: m.userID.UUID(), Role: string(domain.RoleMember),
		Via: string(m.via), RequestID: m.requestID,
	})
}

func findRequest(
	ctx context.Context, q *sqlc.Queries, cabalID ids.CabalID, requestID ids.AccessRequestID, op string,
) (sqlc.CabalAccessRequest, domain.AccessRequest, error) {
	row, err := q.FindAccessRequest(ctx, sqlc.FindAccessRequestParams{CabalID: cabalID.UUID(), ID: requestID.UUID()})
	if errors.Is(err, sql.ErrNoRows) {
		return sqlc.CabalAccessRequest{}, domain.AccessRequest{}, errs.New(errs.CodeNotFound, op)
	}
	if err != nil {
		return sqlc.CabalAccessRequest{}, domain.AccessRequest{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return row, domain.AccessRequest{
		ID: requestID, Direction: domain.Direction(row.Direction), UserID: ids.UserIDFrom(row.UserID),
		InvitedBy: ids.UserIDFrom(uuid.UUID(row.InvitedBy.Bytes)), ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

type dueInvite struct {
	id, cabalID, userID uuid.UUID
}

func expireInvite(ctx context.Context, tx db.Tx, invite dueInvite, at time.Time, op string) (bool, error) {
	n, err := sqlc.New(tx.Queries()).ExpireAccessRequest(ctx, sqlc.ExpireAccessRequestParams{
		CabalID: invite.cabalID, ID: invite.id, Now: at,
	})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, op)
	}
	if n == 0 {
		return false, nil
	}
	return true, tx.Events.Append(ctx, events.CabalAccessDecided{
		V: 1, RequestID: invite.id, CabalID: invite.cabalID, UserID: invite.userID,
		Direction: string(domain.DirectionInvite), Decision: string(domain.AccessExpired),
	})
}

type settlement struct {
	row   sqlc.CabalAccessRequest
	event domain.AccessEvent
	actor ids.UserID
	at    time.Time
}

func settleRequest(ctx context.Context, tx db.Tx, s settlement, op string) (Access, error) {
	next, err := domain.Next(domain.AccessStatus(s.row.Status), s.event)
	if err != nil {
		return Access{}, err
	}
	n, err := sqlc.New(tx.Queries()).DecideAccessRequest(ctx, sqlc.DecideAccessRequestParams{
		Status: string(next), DecidedBy: s.actor.UUID(), Now: s.at, CabalID: s.row.CabalID, ID: s.row.ID,
	})
	if err != nil {
		return Access{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	if n == 0 {
		return Access{}, errs.New(errs.CodeAccessRequestNotPending, op)
	}
	err = tx.Events.Append(ctx, events.CabalAccessDecided{
		V: 1, RequestID: s.row.ID, CabalID: s.row.CabalID, UserID: s.row.UserID, Direction: s.row.Direction,
		Decision: string(next), ActorID: s.actor.UUID(),
	})
	return Access{ID: s.row.ID, Direction: s.row.Direction, Status: string(next)}, err
}
