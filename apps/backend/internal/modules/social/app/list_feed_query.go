package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	FeedPageDefault = 30
	FeedPageMax     = 50
)

type FeedFilter struct {
	Kinds   []feed.Kind
	CabalID ids.CabalID
	Symbol  string
	Q       string
	Scope   feed.Scope
}

type FeedQuery struct {
	Viewer ids.UserID
	Filter FeedFilter
	After  *domain.Keyset
	Limit  int
}

type FeedItem struct {
	ID           uuid.UUID
	Kind         feed.Kind
	RefType      feed.RefType
	RefID        uuid.UUID
	CabalID      ids.CabalID
	ActorID      ids.UserID
	Symbol       string
	Title        string
	Detail       string
	Body         string
	Status       string
	Tone         feed.Tone
	CommentCount int32
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type FeedPage struct {
	Items []FeedItem
	Next  *domain.Keyset
}

func ListFeed(ctx context.Context, db sqlc.DBTX, q FeedQuery) (FeedPage, error) {
	const op = "social.ListFeed"
	if q.Limit < 1 || q.Limit > FeedPageMax {
		return FeedPage{}, errs.New(errs.CodeInvalidInput, op, slog.Int("limit", q.Limit))
	}
	params := sqlc.ListFeedParams{
		Kinds: kindStrings(q.Filter.Kinds), CabalID: q.Filter.CabalID.UUID(),
		Symbol: q.Filter.Symbol, Q: q.Filter.Q, Following: q.Filter.Scope == feed.ScopeFollowing,
		Viewer: q.Viewer.UUID(), RowLimit: int32(q.Limit) + 1,
	}
	if q.After != nil {
		params.HasCursor, params.AfterAt, params.AfterID = true, q.After.At, q.After.ID
	}
	rows, err := sqlc.New(db).ListFeed(ctx, params)
	if err != nil {
		return FeedPage{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	page := FeedPage{Items: make([]FeedItem, 0, min(len(rows), q.Limit))}
	for _, row := range rows[:min(len(rows), q.Limit)] {
		item, err := feedItemOf(row)
		if err != nil {
			return FeedPage{}, err
		}
		page.Items = append(page.Items, item)
	}
	if len(rows) > q.Limit {
		last := page.Items[q.Limit-1]
		page.Next = &domain.Keyset{At: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func feedItemOf(row sqlc.ListFeedRow) (FeedItem, error) {
	payload, err := feed.ParsePayload(row.Payload)
	if err != nil {
		return FeedItem{}, err
	}
	kind := feed.Kind(row.Kind)
	return FeedItem{
		ID: row.ID, Kind: kind, RefType: feed.RefType(row.RefType), RefID: row.RefID,
		CabalID: ids.CabalIDFrom(row.CabalID.Bytes), ActorID: ids.UserIDFrom(row.ActorID.Bytes),
		Symbol: row.Symbol.String, Title: row.Title, Detail: feed.RenderDetail(kind, payload), Body: row.Body.String,
		Status: row.Status.String, Tone: feed.RenderTone(kind, payload), CommentCount: row.CommentCount,
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}, nil
}

func kindStrings(kinds []feed.Kind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}
