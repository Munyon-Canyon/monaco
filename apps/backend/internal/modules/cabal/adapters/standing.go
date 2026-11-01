package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const (
	accountBanned    = "banned"
	lastMemberClosed = "The last member deleted their account"
)

type Standing struct{}

func (Standing) Deleted(ctx context.Context, tx db.Tx, e events.UserDeleted, at time.Time) error {
	const op = "cabal.Standing.Deleted"
	q := sqlc.New(tx.Queries())
	gone, err := q.DeleteMembershipsOf(ctx, e.UserID)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	cabals := make([]uuid.UUID, 0, len(gone))
	for _, row := range gone {
		cabals = append(cabals, row.CabalID)
		left := events.CabalMemberLeft{V: 1, CabalID: row.CabalID, UserID: e.UserID, WasVoter: row.CanVote}
		if err := tx.Events.Append(ctx, left); err != nil {
			return err
		}
	}
	open, err := closeEmptyCabals(ctx, tx, q, cabals, at, op)
	if err != nil {
		return err
	}
	return keepAnAdmin(ctx, tx, q, open, op)
}

func (Standing) StatusChanged(ctx context.Context, tx db.Tx, e events.UserAccountStatusChanged, _ time.Time) error {
	const op = "cabal.Standing.StatusChanged"
	q := sqlc.New(tx.Queries())
	switch {
	case e.To == accountBanned:
		cabals, err := q.SuspendMembershipsOf(ctx, e.UserID)
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		return keepAnAdmin(ctx, tx, q, cabals, op)
	case e.From == accountBanned:
		if _, err := q.RestoreMembershipsOf(ctx, e.UserID); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
	}
	return nil
}

func closeEmptyCabals(
	ctx context.Context, tx db.Tx, q *sqlc.Queries, cabals []uuid.UUID, at time.Time, op string,
) ([]uuid.UUID, error) {
	open := make([]uuid.UUID, 0, len(cabals))
	for _, cabal := range cabals {
		if _, err := q.LockCabalExclusive(ctx, cabal); err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		closed, err := q.CloseEmptyCabal(ctx, sqlc.CloseEmptyCabalParams{ID: cabal, Now: at})
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		if closed == 0 {
			open = append(open, cabal)
			continue
		}
		if err := tx.Events.Append(
			ctx,
			events.CabalBanned{V: 1, CabalID: cabal, Reason: lastMemberClosed},
		); err != nil {
			return nil, err
		}
	}
	return open, nil
}

func keepAnAdmin(ctx context.Context, tx db.Tx, q *sqlc.Queries, cabals []uuid.UUID, op string) error {
	for _, cabal := range cabals {
		promoted, err := q.PromoteLongestStandingMember(ctx, cabal)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		updated := events.CabalUpdated{
			V: 1, CabalID: cabal, ActorID: promoted, Changes: events.CabalChanges{AdminIDs: []uuid.UUID{promoted}},
		}
		if err := tx.Events.Append(ctx, updated); err != nil {
			return err
		}
	}
	return nil
}
