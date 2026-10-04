package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	FollowsPageDefault = 30
	FollowsPageMax     = 100
	followsBatchSize   = FollowsPageMax + 1
)

type FollowsQuery struct {
	Viewer ids.UserID
	User   ids.UserID
	After  *domain.Keyset
	Limit  int
}

type FollowUser struct {
	User       port.UserCard
	FollowedBy bool
}

type FollowsPage struct {
	Items []FollowUser
	Next  *domain.Keyset
}

func ListFollowers(ctx context.Context, db sqlc.DBTX, users Users, q FollowsQuery) (FollowsPage, error) {
	return listFollows(ctx, db, users, q, true)
}

func ListFollowing(ctx context.Context, db sqlc.DBTX, users Users, q FollowsQuery) (FollowsPage, error) {
	return listFollows(ctx, db, users, q, false)
}

func listFollows(
	ctx context.Context, db sqlc.DBTX, users Users, q FollowsQuery, followers bool,
) (FollowsPage, error) {
	if err := validateFollowsQuery(ctx, users, q); err != nil {
		return FollowsPage{}, err
	}
	const op = "social.ListFollows"
	query := sqlc.New(db)
	after, last := q.After, (*domain.Keyset)(nil)
	page := FollowsPage{Items: make([]FollowUser, 0, q.Limit)}
	for {
		rows, err := listFollowRows(ctx, query, q.User, after, followers)
		if err != nil {
			return FollowsPage{}, errs.Wrap(err, errs.CodeOf(err), op)
		}
		items, keys, err := visibleFollowUsers(ctx, query, users, q.Viewer, rows)
		if err != nil {
			return FollowsPage{}, errs.Wrap(err, errs.CodeOf(err), op)
		}
		for i, item := range items {
			if len(page.Items) == q.Limit {
				page.Next = last
				return page, nil
			}
			page.Items = append(page.Items, item)
			last = keys[i]
		}
		if len(rows) < followsBatchSize {
			return page, nil
		}
		after = rows[len(rows)-1].Key
	}
}

func validateFollowsQuery(ctx context.Context, users Users, q FollowsQuery) error {
	const op = "social.ListFollows"
	if q.Limit < 1 || q.Limit > FollowsPageMax {
		return errs.New(errs.CodeInvalidInput, op, slog.Int("limit", q.Limit))
	}
	return listedUser(ctx, users, q.User)
}

func visibleFollowUsers(
	ctx context.Context, query *sqlc.Queries, users Users, viewer ids.UserID, rows []followRow,
) ([]FollowUser, []*domain.Keyset, error) {
	userIDs, keys := followIDs(rows)
	cards, err := users.UsersByID(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	followed, err := followedAmong(ctx, query, viewer, userIDs)
	if err != nil {
		return nil, nil, err
	}
	items := make([]FollowUser, 0, len(rows))
	visibleKeys := make([]*domain.Keyset, 0, len(rows))
	for i, id := range userIDs {
		card, ok := cards[id]
		if ok && !card.Deleted && card.AccountStatus != "banned" {
			items = append(items, FollowUser{User: card, FollowedBy: followed[id]})
			visibleKeys = append(visibleKeys, keys[i])
		}
	}
	return items, visibleKeys, nil
}

func listedUser(ctx context.Context, users Users, id ids.UserID) error {
	const op = "social.ListFollows"
	status, err := statusOf(ctx, users, id)
	if err != nil {
		return err
	}
	if status == domain.AccountUnknown || status == domain.AccountDeleted || status == domain.AccountBanned {
		return errs.New(errs.CodeUserNotFound, op)
	}
	return nil
}

type followRow struct {
	ID  ids.UserID
	Key *domain.Keyset
}

func listFollowRows(
	ctx context.Context, query *sqlc.Queries, user ids.UserID, after *domain.Keyset, followers bool,
) ([]followRow, error) {
	params := sqlc.ListFollowersParams{UserID: user.UUID(), RowLimit: followsBatchSize}
	if after != nil {
		params.HasCursor, params.AfterAt, params.AfterID = true, after.At, after.ID
	}
	if followers {
		rows, err := query.ListFollowers(ctx, params)
		out := make([]followRow, len(rows))
		for i, row := range rows {
			out[i] = followRow{
				ID:  ids.UserIDFrom(row.FollowerID),
				Key: &domain.Keyset{At: row.CreatedAt.UTC(), ID: row.FollowerID},
			}
		}
		return out, err
	}
	rows, err := query.ListFollowing(ctx, sqlc.ListFollowingParams(params))
	out := make([]followRow, len(rows))
	for i, row := range rows {
		out[i] = followRow{
			ID:  ids.UserIDFrom(row.FolloweeID),
			Key: &domain.Keyset{At: row.CreatedAt.UTC(), ID: row.FolloweeID},
		}
	}
	return out, err
}

func followIDs(rows []followRow) ([]ids.UserID, []*domain.Keyset) {
	ids := make([]ids.UserID, len(rows))
	keys := make([]*domain.Keyset, len(rows))
	for i, row := range rows {
		ids[i], keys[i] = row.ID, row.Key
	}
	return ids, keys
}

func followedAmong(
	ctx context.Context,
	query *sqlc.Queries,
	viewer ids.UserID,
	targets []ids.UserID,
) (map[ids.UserID]bool, error) {
	raw := make([]uuid.UUID, len(targets))
	for i, id := range targets {
		raw[i] = id.UUID()
	}
	rows, err := query.FollowedAmong(ctx, sqlc.FollowedAmongParams{FollowerID: viewer.UUID(), FolloweeIds: raw})
	if err != nil {
		return nil, err
	}
	followed := make(map[ids.UserID]bool, len(rows))
	for _, id := range rows {
		followed[ids.UserIDFrom(id)] = true
	}
	return followed, nil
}
