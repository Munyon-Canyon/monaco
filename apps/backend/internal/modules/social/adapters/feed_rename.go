package adapters

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const renameBatch = 500

type staleRow = sqlc.StaleFeedByActorRow

type staleRows func(ctx context.Context, q *sqlc.Queries, after uuid.UUID) ([]staleRow, error)

func (h Feed) FetchProfile(ctx context.Context, e events.UserProfileUpdated) (string, error) {
	if !slices.ContainsFunc(e.Fields, changesName) {
		return "", nil
	}
	user := ids.UserIDFrom(e.UserID)
	cards, err := h.Users.UsersByID(ctx, []ids.UserID{user})
	if err != nil {
		return "", err
	}
	return feedName(cards[user].DisplayName, cards[user].Handle), nil
}

func changesName(field string) bool { return field == "display_name" || field == "handle" }

func (h Feed) ApplyProfile(
	ctx context.Context, tx db.Tx, e events.UserProfileUpdated, name string, at time.Time,
) error {
	if name == "" {
		return nil
	}
	stale := func(ctx context.Context, q *sqlc.Queries, after uuid.UUID) ([]staleRow, error) {
		return q.StaleFeedByActor(ctx, sqlc.StaleFeedByActorParams{
			ActorID: e.UserID, Name: name, After: after, RowLimit: renameBatch,
		})
	}
	return h.rewrite(ctx, tx, at, stale, func(p *feed.Payload) { p.ActorName = name })
}

func (h Feed) CabalUpdated(ctx context.Context, tx db.Tx, e events.CabalUpdated, at time.Time) error {
	if e.Changes.Name == nil {
		return nil
	}
	name := *e.Changes.Name
	err := h.UoW.Do(ctx, func(ctx context.Context, step db.Tx) error {
		n, err := sqlc.New(step.Queries()).RenameFeedCabal(ctx, sqlc.RenameFeedCabalParams{
			Name: name, At: at, CabalID: e.CabalID,
		})
		if err == nil && n == 0 {
			err = errs.New(errs.CodeFeedItemPending, "social.Feed.CabalUpdated")
		}
		return err
	})
	if err != nil {
		return err
	}
	stale := func(ctx context.Context, q *sqlc.Queries, after uuid.UUID) ([]staleRow, error) {
		rows, err := q.StaleFeedByCabal(ctx, sqlc.StaleFeedByCabalParams{
			CabalID: e.CabalID, Name: name, After: after, RowLimit: renameBatch,
		})
		out := make([]staleRow, len(rows))
		for i, row := range rows {
			out[i] = staleRow(row)
		}
		return out, err
	}
	return h.rewrite(ctx, tx, at, stale, func(p *feed.Payload) { p.CabalName = name })
}

func (h Feed) rewrite(ctx context.Context, tx db.Tx, at time.Time, stale staleRows, edit func(*feed.Payload)) error {
	defer bus.KeepAlive(ctx)()
	rewritten, after := 0, uuid.Nil
	for {
		rows, err := h.rewriteBatch(ctx, at, after, stale, edit)
		if err != nil {
			return err
		}
		rewritten += len(rows)
		if len(rows) < renameBatch {
			break
		}
		after = rows[len(rows)-1].ID
	}
	if rewritten > 0 {
		tx.AfterCommit(func(ctx context.Context) { h.Bus.PublishHint(ctx, "global.feed", nil) })
	}
	return nil
}

func (h Feed) rewriteBatch(
	ctx context.Context, at time.Time, after uuid.UUID, stale staleRows, edit func(*feed.Payload),
) ([]staleRow, error) {
	var rows []staleRow
	err := h.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) (err error) {
		q := sqlc.New(tx.Queries())
		if rows, err = stale(ctx, q, after); err != nil || len(rows) == 0 {
			return err
		}
		update := sqlc.RewriteFeedRowsParams{At: at}
		for _, row := range rows {
			p, err := feed.ParsePayload(row.Payload)
			if err != nil {
				return err
			}
			edit(&p)
			update.Ids = append(update.Ids, row.ID)
			update.Titles = append(update.Titles, feed.RenderTitle(feed.Kind(row.Kind), p))
			update.Payloads = append(update.Payloads, p.JSON())
		}
		return q.RewriteFeedRows(ctx, update)
	})
	return rows, err
}
