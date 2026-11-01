package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type RevokeScope string

const (
	RevokeThisSession RevokeScope = "this_session"
	RevokeAllSessions RevokeScope = "all_sessions"
)

type RevokeSessions struct {
	UserID    ids.UserID
	SessionID string
	Scope     RevokeScope
}

type RevokeSessionsDeps struct {
	UoW   *db.UnitOfWork
	Clock clock.Clock
}

type RevokeSessionsHandler struct{ d RevokeSessionsDeps }

func NewRevokeSessionsHandler(d RevokeSessionsDeps) *RevokeSessionsHandler {
	return &RevokeSessionsHandler{d: d}
}

func (h *RevokeSessionsHandler) Handle(ctx context.Context, cmd RevokeSessions) error {
	const op = "identity.RevokeSessions.Handle"
	if cmd.Scope != RevokeThisSession && cmd.Scope != RevokeAllSessions {
		return errs.New(errs.CodeInvalidInput, op, slog.String("scope", string(cmd.Scope)))
	}
	if cmd.Scope == RevokeThisSession && cmd.SessionID == "" {
		return nil
	}
	now := h.d.Clock.Now()
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		if cmd.Scope == RevokeAllSessions {
			if err := revokeAll(ctx, q, cmd, now); err != nil {
				return err
			}
			return tx.Events.Append(ctx, events.UserSessionsRevoked{
				V: 1, UserID: cmd.UserID.UUID(), Scope: string(cmd.Scope), At: now,
			})
		}
		_, err := q.RevokeSession(ctx, sqlc.RevokeSessionParams{
			PrivySessionID: cmd.SessionID, UserID: cmd.UserID.UUID(), RevokedAt: now,
		})
		return err
	})
	if err != nil {
		return err
	}
	observability.Info(ctx, observability.IdentitySessionsRevoked,
		slog.String("user_id", cmd.UserID.String()), slog.String("scope", string(cmd.Scope)))
	return nil
}

func revokeAll(ctx context.Context, q *sqlc.Queries, cmd RevokeSessions, now time.Time) error {
	if _, err := q.RevokeSessionsIssuedBefore(ctx, sqlc.RevokeSessionsIssuedBeforeParams{
		UserID: cmd.UserID.UUID(), ValidAfter: now,
	}); err != nil {
		return err
	}
	if _, err := q.RevokeKnownSessions(ctx, sqlc.RevokeKnownSessionsParams{
		UserID: cmd.UserID.UUID(), RevokedAt: now,
	}); err != nil {
		return err
	}
	if cmd.SessionID == "" {
		return nil
	}
	_, err := q.RevokeSession(ctx, sqlc.RevokeSessionParams{
		PrivySessionID: cmd.SessionID, UserID: cmd.UserID.UUID(), RevokedAt: now,
	})
	return err
}
