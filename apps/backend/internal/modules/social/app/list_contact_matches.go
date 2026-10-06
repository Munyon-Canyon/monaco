package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	ContactPageDefault = 30
	ContactPageMax     = 50
)

type ContactMatchQuery struct {
	User  ids.UserID
	After *domain.Keyset
	Limit int
}

type ContactMatch struct {
	UserID       ids.UserID
	Handle       string
	DisplayName  string
	PhotoURL     string
	FollowedByMe bool
}

type ContactMatchPage struct {
	Items []ContactMatch
	Next  *domain.Keyset
}

func ListContactMatches(ctx context.Context, db sqlc.DBTX, users Users, q ContactMatchQuery) (ContactMatchPage, error) {
	const op = "social.ListContactMatches"
	if q.Limit < 1 || q.Limit > ContactPageMax {
		return ContactMatchPage{}, errs.New(errs.CodeInvalidInput, op, slog.Int("limit", q.Limit))
	}
	params := sqlc.ListContactMatchesParams{UserID: q.User.UUID(), RowLimit: int32(q.Limit) + 1}
	if q.After != nil {
		params.HasCursor, params.AfterAt, params.AfterID = true, q.After.At, q.After.ID
	}
	rows, err := sqlc.New(db).ListContactMatches(ctx, params)
	if err != nil {
		return ContactMatchPage{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	window := rows[:min(len(rows), q.Limit)]
	page := ContactMatchPage{Items: make([]ContactMatch, 0, len(window))}
	if len(rows) > q.Limit {
		last := window[len(window)-1]
		page.Next = &domain.Keyset{At: last.CreatedAt.UTC(), ID: last.MatchedUserID}
	}
	if len(window) == 0 {
		return page, nil
	}
	idsToLoad := make([]ids.UserID, len(window))
	for i, row := range window {
		idsToLoad[i] = ids.UserIDFrom(row.MatchedUserID)
	}
	cards, err := users.UsersByID(ctx, idsToLoad)
	if err != nil {
		return ContactMatchPage{}, err
	}
	for _, row := range window {
		id := ids.UserIDFrom(row.MatchedUserID)
		card, ok := cards[id]
		if !liveContact(card, ok) {
			continue
		}
		page.Items = append(page.Items, ContactMatch{
			UserID: id, Handle: card.Handle, DisplayName: card.DisplayName, PhotoURL: card.PhotoURL,
			FollowedByMe: row.FollowedByMe,
		})
	}
	return page, nil
}

func liveContact(card port.UserCard, ok bool) bool {
	return ok && !card.Deleted && string(card.AccountStatus) != string(domain.AccountBanned)
}
