package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type pauseScope struct{ cabal *ids.CabalID }

func scopeFromRow(cabalID uuid.UUID) pauseScope {
	if cabalID == uuid.Nil {
		return pauseScope{}
	}
	id := ids.CabalIDFrom(cabalID)
	return pauseScope{cabal: &id}
}

func (s pauseScope) row() uuid.UUID {
	if s.cabal == nil {
		return uuid.Nil
	}
	return s.cabal.UUID()
}

func (s pauseScope) eventCabalID() *uuid.UUID {
	if s.cabal == nil {
		return nil
	}
	id := s.cabal.UUID()
	return &id
}

func (s pauseScope) name() string {
	if s.cabal == nil {
		return "global"
	}
	return "cabal"
}

func (s pauseScope) key() string {
	if s.cabal == nil {
		return "global"
	}
	return s.cabal.String()
}

func optionalUser(user *ids.UserID) uuid.UUID {
	if user == nil {
		return uuid.Nil
	}
	return user.UUID()
}

func lockScope(ctx context.Context, q *sqlc.Queries, s pauseScope) ([]string, error) {
	if err := q.LockPauseScope(ctx, s.row()); err != nil {
		return nil, err
	}
	return q.OpenScopePauses(ctx, s.row())
}

func settleResolved(ctx context.Context, tx db.Tx, s pauseScope, before, after []string, hints HintPublisher) error {
	if len(after) == 0 {
		resumed := events.CabalResumed{V: 1, CabalID: s.eventCabalID(), Scope: s.name()}
		if err := tx.Events.Append(ctx, resumed); err != nil {
			return err
		}
	}
	afterCommitPauseChanged(tx, s, before, after, hints)
	return nil
}

func afterCommitPauseChanged(tx db.Tx, s pauseScope, before, after []string, hints HintPublisher) {
	tx.AfterCommit(func(ctx context.Context) {
		hints.PublishHint(ctx, "cabal."+s.key()+".pause_changed", nil)
		observability.Info(ctx, observability.FundingPauseChanged,
			slog.String("scope", s.name()), slog.String("cabal_id", s.key()),
			slog.String("reasons_before", strings.Join(before, ",")),
			slog.String("reasons_after", strings.Join(after, ",")))
	})
}
