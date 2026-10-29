package adapters

import (
	"context"
	"log/slog"
	"math"
	"slices"
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
	q.Filter, err = feedFilter(p.Kind, p.CabalId, p.ActorId, p.Symbol, p.Q, (*string)(p.Scope))
	if err != nil {
		return nil, err
	}
	if q.Filter.Status, err = feedStatus(p.Status); err != nil {
		return nil, err
	}
	if q.Sort, err = feedSort(p.Sort); err != nil {
		return nil, err
	}
	q.Now = h.Clock.Now()
	if p.Limit != nil {
		q.Limit = *p.Limit
	}
	if p.Cursor != nil {
		after, err := parseFeedCursor(q.Sort, *p.Cursor)
		if err != nil {
			return nil, err
		}
		q.After = &after
	}
	page, err := app.ListFeed(ctx, h.Reads, q)
	if err != nil {
		return nil, err
	}
	items, err := h.wireFeedItems(ctx, page.Items)
	if err != nil {
		return nil, err
	}
	body := api.FeedPage{Items: items}
	if page.Next != nil {
		next := page.Next.Encode()
		body.NextCursor = &next
	}
	return api.GetFeed200JSONResponse(body), nil
}

func feedSort(raw *api.GetFeedParamsSort) (feed.Sort, error) {
	if raw == nil {
		return feed.SortNew, nil
	}
	if !raw.Valid() {
		return "", errs.New(errs.CodeInvalidInput, "social.GetFeed", slog.String("sort", string(*raw)))
	}
	return feed.Sort(*raw), nil
}

func feedStatus(raw *api.GetFeedParamsStatus) (string, error) {
	if raw == nil {
		return "", nil
	}
	if !raw.Valid() {
		return "", errs.New(errs.CodeInvalidInput, "social.GetFeed", slog.String("status", string(*raw)))
	}
	return string(*raw), nil
}

func parseFeedCursor(sort feed.Sort, raw string) (domain.Keyset, error) {
	if sort == feed.SortTop {
		return domain.ParseRankedKeyset(raw)
	}
	return domain.ParseKeyset(raw)
}

func (h HTTP) GetFeedItem(
	ctx context.Context, req api.GetFeedItemRequestObject,
) (api.GetFeedItemResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	p := req.Params
	filter, err := feedFilter(p.Kind, p.CabalId, p.ActorId, p.Symbol, p.Q, (*string)(p.Scope))
	if err != nil {
		return nil, err
	}
	view, err := app.GetFeedItem(ctx, h.Reads, me, req.Id, filter)
	if err != nil {
		return nil, err
	}
	items, err := h.wireFeedItems(ctx, []app.FeedItem{view.Item})
	if err != nil {
		return nil, err
	}
	return api.GetFeedItem200JSONResponse(api.FeedItemDetail{
		Item: items[0], Visible: view.Visible, CanComment: view.Item.CanReply,
	}), nil
}

func feedFilter(kinds *string, cabal, actor *uuid.UUID, symbol, q, scope *string) (app.FeedFilter, error) {
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
	if actor != nil {
		f.ActorID = ids.UserIDFrom(*actor)
	}
	f.Symbol, f.Q = strings.TrimSpace(deref(symbol)), strings.TrimSpace(deref(q))
	return f, nil
}

type feedRefs struct {
	actors []ids.UserID
	assets []uuid.UUID
	cabals []ids.CabalID
	logos  map[uuid.UUID]string
	covers map[ids.CabalID]string
}

func collectFeedRefs(items []app.FeedItem) feedRefs {
	var r feedRefs
	for _, item := range items {
		if item.ActorID.UUID() != uuid.Nil && !slices.Contains(r.actors, item.ActorID) {
			r.actors = append(r.actors, item.ActorID)
		}
		if item.AssetID != uuid.Nil && !slices.Contains(r.assets, item.AssetID) {
			r.assets = append(r.assets, item.AssetID)
		}
		if item.CabalID.UUID() != uuid.Nil && !slices.Contains(r.cabals, item.CabalID) {
			r.cabals = append(r.cabals, item.CabalID)
		}
	}
	return r
}

func (h HTTP) loadFeedImages(ctx context.Context, r *feedRefs) error {
	const op = "social.wireFeedItems"
	logos, err := h.Logos.AssetLogos(ctx, r.assets)
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), op)
	}
	views, err := h.Pictures.Cabals(ctx, r.cabals)
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), op)
	}
	r.logos, r.covers = logos, make(map[ids.CabalID]string, len(views))
	for id, view := range views {
		r.covers[id] = view.PictureURL
	}
	return nil
}

func (h HTTP) wireFeedItems(ctx context.Context, items []app.FeedItem) ([]api.FeedItem, error) {
	refs := collectFeedRefs(items)
	who, err := loadPeople(ctx, h.Users, h.Returns, refs.actors, "social.wireFeedItems")
	if err != nil {
		return nil, err
	}
	if err := h.loadFeedImages(ctx, &refs); err != nil {
		return nil, err
	}
	tallies, err := h.loadTallies(ctx, items)
	if err != nil {
		return nil, err
	}
	out := make([]api.FeedItem, len(items))
	for i, item := range items {
		out[i] = wireFeedItem(item)
		wireTally(item, tallies, &out[i])
		if card, ok := who.cards[item.ActorID]; ok && !card.Deleted {
			out[i].ActorPhotoUrl = optionalWireText(card.PhotoURL)
		}
		out[i].Caller = wireCaller(item.Caller, who)
		out[i].AssetLogoUrl = optionalWireText(refs.logos[item.AssetID])
		out[i].CabalPictureUrl = optionalWireText(refs.covers[item.CabalID])
		out[i].Return1mBps, out[i].ReturnPrivate = who.authorReturn(item.ActorID)
	}
	return out, nil
}

func wireFeedItem(item app.FeedItem) api.FeedItem {
	tokens := optionalWireAmount(item.TokenAmount)
	filledTokens := optionalWireAmount(item.FilledTokens.Uint64())
	return api.FeedItem{
		Id:            item.ID,
		Kind:          string(item.Kind),
		RefType:       string(item.RefType),
		RefId:         item.RefID,
		CabalId:       optionalWireID(item.CabalID.UUID()),
		CabalName:     optionalWireText(item.CabalName),
		ActorId:       optionalWireID(item.ActorID.UUID()),
		ActorName:     optionalWireText(item.ActorName),
		AssetId:       optionalWireID(item.AssetID),
		Symbol:        optionalWireText(item.Symbol),
		Title:         item.Title,
		Detail:        optionalWireText(item.Detail),
		Body:          optionalWireText(item.Body),
		Status:        optionalWireText(item.Status),
		Tone:          string(item.Tone),
		Action:        optionalWireText(string(item.Action)),
		UsdcMicros:    optionalWireAmount(item.USDCMicros.Uint64()),
		AssetName:     optionalWireText(item.AssetName),
		TokenAmount:   tokens,
		TokenDecimals: optionalWireDecimals(item.TokenDecimals, tokens != nil || filledTokens != nil),
		PriceMicros: optionalWireAmount(
			item.PriceMicros.Uint64(),
		),
		ShareOfPotBps:     optionalWirePositive(item.ShareOfPotBps),
		ChangeBps:         item.ChangeBps,
		MarkMicros:        optionalWireAmount(item.MarkMicros.Uint64()),
		PrevCloseMicros:   optionalWireAmount(item.PrevClose.Uint64()),
		ProposalId:        optionalWireID(item.ProposalID),
		Outcome:           optionalWireEnum[api.FeedItemOutcome](string(item.Outcome)),
		Shape:             optionalWireEnum[api.FeedItemShape](string(item.Shape)),
		AssetKind:         optionalWireEnum[api.FeedItemAssetKind](string(item.AssetKind)),
		FillState:         optionalWireEnum[api.FeedItemFillState](item.FillState),
		FilledTokenAmount: filledTokens,
		FilledUsdcMicros:  optionalWireAmount(item.FilledUSDCMicros.Uint64()),
		FilledAt:          item.FilledAt,
		TradeFailed:       item.TradeFailed,
		CommentCount:      item.CommentCount,
		ThreadItemId:      item.ThreadItemID,
		ReplyCount:        item.ReplyCount,
		CanReply:          item.CanReply,
		CanReact:          item.CanReact,
		ReplyRule:         api.FeedItemReplyRule(item.ReplyRule),
		ReplyRuleCabalName: optionalWireText(
			item.ReplyRuleCabalName,
		),
		LikeCount:     item.LikeCount,
		LikedByMe:     item.LikedByMe,
		Reactions:     wireReactions(item.Reactions),
		ReactionCount: item.ReactionCount,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

func wireCaller(caller *app.FeedCaller, who people) *api.FeedCaller {
	if caller == nil {
		return nil
	}
	out := api.FeedCaller{Id: caller.ID.UUID(), Name: caller.Name}
	if card, ok := who.cards[caller.ID]; ok && !card.Deleted {
		out.PhotoUrl = optionalWireText(card.PhotoURL)
	}
	return &out
}

func wireReactions(reactions []app.Reaction) []api.Reaction {
	out := make([]api.Reaction, len(reactions))
	for i, r := range reactions {
		out[i] = api.Reaction{Emoji: r.Emoji.String(), Count: r.Count, Mine: r.Mine}
	}
	return out
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

func optionalWireAmount(n uint64) *int64 {
	if n > math.MaxInt64 {
		return nil
	}
	return optionalWirePositive(int64(n))
}

func optionalWirePositive(n int64) *int64 {
	if n <= 0 {
		return nil
	}
	return &n
}

func optionalWireDecimals(d *uint8, withAmount bool) *int {
	if d == nil || !withAmount {
		return nil
	}
	v := int(*d)
	return &v
}

func optionalWireEnum[T ~string](s string) *T {
	if s == "" {
		return nil
	}
	v := T(s)
	return &v
}

func optionalWireText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h HTTP) PutFeedLike(
	ctx context.Context, req api.PutFeedLikeRequestObject,
) (api.PutFeedLikeResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.LikeFeedItem.Handle(ctx, app.LikeFeedItem{FeedObjectID: req.Id, Viewer: me}); err != nil {
		return nil, err
	}
	return api.PutFeedLike204Response{}, nil
}

func (h HTTP) DeleteFeedLike(
	ctx context.Context, req api.DeleteFeedLikeRequestObject,
) (api.DeleteFeedLikeResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.UnlikeFeedItem.Handle(ctx, app.UnlikeFeedItem{FeedObjectID: req.Id, Viewer: me}); err != nil {
		return nil, err
	}
	return api.DeleteFeedLike204Response{}, nil
}
