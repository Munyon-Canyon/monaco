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
	row, err := q.LockCabalForAccess(ctx, sqlc.LockCabalForAccessParams{CabalID: cabalID.UUID(), UserID: user.UUID()})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Cabal{}, domain.Actor{}, errs.New(errs.CodeCabalNotFound, op)
	}
	if err != nil {
		return domain.Cabal{}, domain.Actor{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	rules, err := domain.NewRules(
		row.JoinMode, row.VoterMode, row.Threshold, row.ProposalExpirySeconds, row.SlippageBps,
	)
	if err != nil {
		return domain.Cabal{}, domain.Actor{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	cabal := domain.Cabal{
		CreatorID: ids.UserIDFrom(row.CreatorID), Rules: rules, Banned: row.Status == string(port.StatusBanned),
	}
	return cabal, domain.Actor{UserID: user, Member: row.IsMember}, nil
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
