package adapters

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) GetFeed(ctx context.Context, req api.GetFeedRequestObject) (api.GetFeedResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	p := req.Params
	q := app.FeedQuery{Viewer: me, Limit: app.FeedPageDefault}
	q.Filter, err = feedFilter(p.Kind, p.CabalId, p.Symbol, p.Q, (*string)(p.Scope))
	if err != nil {
		return nil, err
	}
	if p.Sort != nil && !p.Sort.Valid() {
		return nil, errs.New(errs.CodeInvalidInput, "social.GetFeed", slog.String("sort", string(*p.Sort)))
	}
	if p.Limit != nil {
		q.Limit = *p.Limit
	}
	if p.Cursor != nil {
		after, err := domain.ParseKeyset(*p.Cursor)
		if err != nil {
			return nil, err
		}
		q.After = &after
	}
	page, err := app.ListFeed(ctx, h.Reads, q)
	if err != nil {
		return nil, err
	}
	body := api.FeedPage{Items: make([]api.FeedItem, len(page.Items))}
	for i, item := range page.Items {
		body.Items[i] = wireFeedItem(item)
	}
	if page.Next != nil {
		next := page.Next.Encode()
		body.NextCursor = &next
	}
	return api.GetFeed200JSONResponse(body), nil
}

func (h HTTP) GetFeedItem(
	ctx context.Context, req api.GetFeedItemRequestObject,
) (api.GetFeedItemResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	p := req.Params
	filter, err := feedFilter(p.Kind, p.CabalId, p.Symbol, p.Q, (*string)(p.Scope))
	if err != nil {
		return nil, err
	}
	view, err := app.GetFeedItem(ctx, h.Reads, me, req.Id, filter)
	if err != nil {
		return nil, err
	}
	return api.GetFeedItem200JSONResponse(api.FeedItemDetail{Item: wireFeedItem(view.Item), Visible: view.Visible}), nil
}

func feedFilter(kinds *string, cabal *uuid.UUID, symbol, q, scope *string) (app.FeedFilter, error) {
	var f app.FeedFilter
	var err error
	if f.Scope, err = feed.ParseScope(deref(scope)); err != nil {
		return app.FeedFilter{}, err
	}
	if raw := deref(kinds); raw != "" {
		for part := range strings.SplitSeq(raw, ",") {
			k, err := feed.ParseKind(strings.TrimSpace(part))
			if err != nil {
				return app.FeedFilter{}, err
			}
			f.Kinds = append(f.Kinds, k)
		}
	}
	if cabal != nil {
		f.CabalID = ids.CabalIDFrom(*cabal)
	}
	f.Symbol, f.Q = strings.TrimSpace(deref(symbol)), strings.TrimSpace(deref(q))
	return f, nil
}

func wireFeedItem(item app.FeedItem) api.FeedItem {
	return api.FeedItem{
		Id: item.ID, Kind: string(item.Kind), RefType: string(item.RefType), RefId: item.RefID,
		CabalId: optionalWireID(item.CabalID.UUID()), CabalName: optionalWireText(item.CabalName),
		ActorId: optionalWireID(item.ActorID.UUID()), ActorName: optionalWireText(item.ActorName),
		AssetId: optionalWireID(item.AssetID), Symbol: optionalWireText(item.Symbol), Title: item.Title,
		Detail: optionalWireText(item.Detail), Body: optionalWireText(item.Body),
		Status: optionalWireText(item.Status), Tone: string(item.Tone),
		CommentCount: item.CommentCount, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optionalWireID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func optionalWireText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
