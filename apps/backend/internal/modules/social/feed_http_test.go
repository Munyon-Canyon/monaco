package social_test

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func (f feedFixture) routes() adapters.HTTP { return f.routesWith(nil) }

func (f feedFixture) routesWith(cards []identity.UserCard) adapters.HTTP {
	return f.routesUsing(fakes.NewIdentity(cards, nil))
}

func (f feedFixture) routesUsing(users *fakes.Identity) adapters.HTTP {
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	return social.HTTPOf(social.New(deps, social.WithUsers(users)))
}

func ptr[T any](v T) *T { return &v }

func (f feedFixture) getFeed(t *testing.T, viewer ids.UserID, p api.GetFeedParams) api.FeedPage {
	t.Helper()
	res, err := f.routes().GetFeed(asUser(t.Context(), viewer), api.GetFeedRequestObject{Params: p})
	if err != nil {
		t.Fatal(err)
	}
	page, ok := res.(api.GetFeed200JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return api.FeedPage(page)
}

func TestGetFeed_pagesNewestFirstWithAnOpaqueCursor(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	cabal := ids.CabalIDFrom(f.gen.NewV7())
	trade := f.gen.NewV7()
	olderAt := f.clock.Now()
	older := f.item(t, func(it *feed.Item) { it.RefID, it.CabalID, it.ActorID = trade, cabal, viewer })
	f.clock.Advance(time.Millisecond)
	move := f.gen.NewV7()
	newer := f.item(t, func(it *feed.Item) {
		it.Kind, it.RefID, it.Payload = feed.KindPriceMove, move, feed.Payload{Symbol: "AAPLx"}
	})
	first := f.getFeed(t, viewer, api.GetFeedParams{Limit: ptr(1), Sort: ptr(api.GetFeedParamsSort("new"))})
	want := api.FeedItem{
		Id: newer, Kind: "price_move", RefType: "asset_price_moves", RefId: move, Symbol: ptr("AAPLx"),
		Title: "AAPLx is flat today", Tone: "neutral", CreatedAt: f.clock.Now(), UpdatedAt: f.clock.Now(),
	}
	if len(first.Items) != 1 || !reflect.DeepEqual(first.Items[0], want) || first.NextCursor == nil {
		t.Fatalf("first page = %+v, want [%+v] and a cursor", first, want)
	}
	second := f.getFeed(t, viewer, api.GetFeedParams{Limit: ptr(1), Cursor: first.NextCursor})
	want = api.FeedItem{
		Id: older, Kind: "trade", RefType: "swaps", RefId: trade, CabalId: ptr(cabal.UUID()),
		CabalName: ptr("Alpha Cabal"), ActorId: ptr(viewer.UUID()), Symbol: ptr("AAPLx"),
		Title: "Alpha Cabal bought $500 of Apple", Action: ptr("buy"), UsdcMicros: ptr(int64(500_000_000)),
		AssetName: ptr("Apple"), Tone: "neutral", CreatedAt: olderAt, UpdatedAt: olderAt,
	}
	if len(second.Items) != 1 || !reflect.DeepEqual(second.Items[0], want) || second.NextCursor != nil {
		t.Fatalf("second page = %+v, want [%+v] and no cursor", second, want)
	}
}

func TestGetFeed_returnsTheActorsCurrentPhotoOrNull(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	withPhoto, without := ids.NewUserID(f.gen), ids.NewUserID(f.gen)
	photoItem := f.item(t, func(it *feed.Item) { it.ActorID = withPhoto })
	f.clock.Advance(time.Millisecond)
	bareItem := f.item(t, func(it *feed.Item) { it.RefID, it.ActorID = f.gen.NewV7(), without })
	f.clock.Advance(time.Millisecond)
	systemItem := f.item(t, func(it *feed.Item) { it.RefID, it.ActorID = f.gen.NewV7(), ids.UserID{} })
	cards := []identity.UserCard{
		{ID: withPhoto, Handle: "maya", PhotoURL: "https://img.example/maya.png"},
		{ID: without, Handle: "sam"},
	}
	res, err := f.routesWith(cards).GetFeed(asUser(t.Context(), viewer), api.GetFeedRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	page := api.FeedPage(res.(api.GetFeed200JSONResponse))
	got := map[uuid.UUID]*string{}
	for _, item := range page.Items {
		got[item.Id] = item.ActorPhotoUrl
	}
	if len(got) != 3 || got[photoItem] == nil || *got[photoItem] != "https://img.example/maya.png" {
		t.Fatalf("photo item = %v in %v, want the member's photo", got[photoItem], got)
	}
	if got[bareItem] != nil || got[systemItem] != nil {
		t.Fatalf("photoless photos = %v and %v, want null", got[bareItem], got[systemItem])
	}
}

func TestGetFeed_failsWhenTheUserLookupFails(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	f.item(t, func(it *feed.Item) { it.ActorID = viewer })
	users := fakes.NewIdentity(nil, nil)
	users.Fail("UsersByID", errs.New(errs.CodeUpstreamUnavailable, "test"))
	_, err := f.routesUsing(users).GetFeed(asUser(t.Context(), viewer), api.GetFeedRequestObject{})
	wantCode(t, err, errs.CodeUpstreamUnavailable)
}

func TestGetFeedItem_failsWhenTheUserLookupFails(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	id := f.item(t, func(it *feed.Item) { it.ActorID = viewer })
	users := fakes.NewIdentity(nil, nil)
	users.Fail("UsersByID", errs.New(errs.CodeUpstreamUnavailable, "test"))
	_, err := f.routesUsing(users).GetFeedItem(asUser(t.Context(), viewer), api.GetFeedItemRequestObject{Id: id})
	wantCode(t, err, errs.CodeUpstreamUnavailable)
}

func TestGetFeed_appliesTheFiltersItParses(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, cabal := ids.NewUserID(f.gen), ids.CabalIDFrom(f.gen.NewV7())
	want := f.item(t, func(it *feed.Item) { it.Kind, it.CabalID = feed.KindProposal, cabal })
	f.item(t)
	page := f.getFeed(t, viewer, api.GetFeedParams{
		Kind: ptr(" proposal , trade"), CabalId: ptr(cabal.UUID()), Symbol: ptr(" aaplx "), Q: ptr(" alpha "),
		Scope: ptr(api.GetFeedParamsScope("all")),
	})
	if len(page.Items) != 1 || page.Items[0].Id != want {
		t.Fatalf("page = %+v, want only %s", page, want)
	}
	if empty := f.getFeed(t, viewer, api.GetFeedParams{Kind: ptr("")}); len(empty.Items) != 2 {
		t.Fatalf("kind= read %d items, want every item", len(empty.Items))
	}
}

func TestGetFeed_refusesUnknownParamValues(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	for name, p := range map[string]api.GetFeedParams{
		"scope":  {Scope: ptr(api.GetFeedParamsScope("bogus"))},
		"kind":   {Kind: ptr("proposal,news")},
		"sort":   {Sort: ptr(api.GetFeedParamsSort("bogus"))},
		"cursor": {Cursor: ptr("not a cursor")},
		"limit":  {Limit: ptr(51)},
	} {
		_, err := f.routes().GetFeed(asUser(t.Context(), viewer), api.GetFeedRequestObject{Params: p})
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func TestGetFeedItem_returnsTheItemAndWhetherTheFiltersPassIt(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	id := f.item(t)
	for kind, visible := range map[string]bool{"trade": true, "proposal": false} {
		res, err := f.routes().GetFeedItem(asUser(t.Context(), viewer), api.GetFeedItemRequestObject{
			Id: id, Params: api.GetFeedItemParams{Kind: ptr(kind)},
		})
		if err != nil {
			t.Fatal(err)
		}
		got, ok := res.(api.GetFeedItem200JSONResponse)
		if !ok || got.Item.Id != id || got.Visible != visible || got.Item.Title != "Alpha Cabal bought $500 of Apple" {
			t.Fatalf("kind=%s: response = %+v, want visible %v", kind, res, visible)
		}
	}
}

func TestGetFeedItem_refusesBadFiltersAndUnknownItems(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	ctx := asUser(t.Context(), ids.NewUserID(f.gen))
	_, err := f.routes().GetFeedItem(ctx, api.GetFeedItemRequestObject{
		Id: f.gen.NewV7(), Params: api.GetFeedItemParams{Scope: ptr(api.GetFeedItemParamsScope("bogus"))},
	})
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.routes().GetFeedItem(ctx, api.GetFeedItemRequestObject{Id: f.gen.NewV7()})
	wantCode(t, err, errs.CodeFeedItemNotFound)
}

func TestFeedRoutes_requireASignedInUser(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	_, err := f.routes().GetFeed(context.Background(), api.GetFeedRequestObject{})
	wantCode(t, err, errs.CodeUnauthorized)
	_, err = f.routes().GetFeedItem(context.Background(), api.GetFeedItemRequestObject{Id: f.gen.NewV7()})
	wantCode(t, err, errs.CodeUnauthorized)
}

func TestGetFeed_sortTopRanksByCommentsOverTheLastDayOnTheFakeClock(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	quiet := f.item(t)
	f.clock.Advance(time.Minute)
	busy := f.commented(t, 3)
	top := api.GetFeedParamsSort("top")
	first := f.getFeed(t, viewer, api.GetFeedParams{Sort: &top, Limit: ptr(1)})
	if len(first.Items) != 1 || first.Items[0].Id != busy || first.NextCursor == nil {
		t.Fatalf("first top page = %+v, want %s and a cursor", first, busy)
	}
	second := f.getFeed(t, viewer, api.GetFeedParams{Sort: &top, Limit: ptr(1), Cursor: first.NextCursor})
	if len(second.Items) != 1 || second.Items[0].Id != quiet || second.NextCursor != nil {
		t.Fatalf("second top page = %+v, want %s and the end", second, quiet)
	}
	f.clock.Advance(feed.TopWindow)
	if aged := f.getFeed(t, viewer, api.GetFeedParams{Sort: &top}); len(aged.Items) != 1 || aged.Items[0].Id != busy {
		t.Fatalf("aged top page = %+v, want only the item still inside the window", aged)
	}
}

func TestGetFeed_keepsAPlainAndARankedCursorApart(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	f.item(t)
	f.clock.Advance(time.Second)
	f.item(t)
	top, fresh := api.GetFeedParamsSort("top"), api.GetFeedParamsSort("new")
	ranked := f.getFeed(t, viewer, api.GetFeedParams{Sort: &top, Limit: ptr(1)}).NextCursor
	plain := f.getFeed(t, viewer, api.GetFeedParams{Sort: &fresh, Limit: ptr(1)}).NextCursor
	for name, p := range map[string]api.GetFeedParams{
		"ranked on new": {Sort: &fresh, Cursor: ranked},
		"plain on top":  {Sort: &top, Cursor: plain},
	} {
		_, err := f.routes().GetFeed(asUser(t.Context(), viewer), api.GetFeedRequestObject{Params: p})
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func (f feedFixture) wireItem(t *testing.T, edit func(*feed.Item)) api.FeedItem {
	t.Helper()
	viewer := ids.NewUserID(f.gen)
	id := f.item(t, edit)
	for _, item := range f.getFeed(t, viewer, api.GetFeedParams{}).Items {
		if item.Id == id {
			return item
		}
	}
	t.Fatalf("item %s is not in the feed", id)
	return api.FeedItem{}
}

func TestGetFeed_sendsTheFiguresOfProposalsAndTrades(t *testing.T) {
	t.Parallel()
	gen := testkit.NewIDs(5613)
	proposal, ref := gen.NewV7(), gen.NewV7()
	buy := feed.Payload{
		Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy, USDCMicros: money.MicrosFromUint64(500_000_000),
		ShareOfPotBps: 1000, Status: "open",
	}
	sell := feed.Payload{
		Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionSell, TokenAmount: 7, TokenDecimals: 8,
		ShareOfPotBps: 250, Status: "open",
	}
	trade := func(action feed.Action, usdc, tokens uint64) feed.Payload {
		return feed.Payload{
			Symbol: "AAPLx", AssetName: "Apple", Action: action, USDCMicros: money.MicrosFromUint64(usdc),
			TokenAmount: tokens, TokenDecimals: 8, ProposalID: proposal,
			PriceMicros: feed.FillPrice(money.MicrosFromUint64(usdc), tokens, 8),
		}
	}
	for name, tc := range map[string]struct {
		kind    feed.Kind
		payload feed.Payload
		want    func(api.FeedItem) api.FeedItem
	}{
		"a buy proposal": {feed.KindProposal, buy, func(i api.FeedItem) api.FeedItem {
			i.Action, i.UsdcMicros, i.AssetName = ptr("buy"), ptr(int64(500_000_000)), ptr("Apple")
			i.ShareOfPotBps, i.ProposalId = ptr(int64(1000)), ptr(ref)
			return i
		}},
		"a sell proposal": {feed.KindProposal, sell, func(i api.FeedItem) api.FeedItem {
			i.Action, i.AssetName, i.TokenAmount, i.TokenDecimals = ptr("sell"), ptr("Apple"), ptr(int64(7)), ptr(8)
			i.ShareOfPotBps, i.ProposalId = ptr(int64(250)), ptr(ref)
			return i
		}},
		"a buy trade": {feed.KindTrade, trade(feed.ActionBuy, 500_000_000, 250_000_000), func(i api.FeedItem) api.FeedItem {
			i.Action, i.UsdcMicros, i.AssetName = ptr("buy"), ptr(int64(500_000_000)), ptr("Apple")
			i.TokenAmount, i.TokenDecimals, i.PriceMicros = ptr(int64(250_000_000)), ptr(8), ptr(int64(200_000_000))
			i.ProposalId = ptr(proposal)
			return i
		}},
		"a sell trade": {feed.KindTrade, trade(feed.ActionSell, 300_000_000, 100_000_000), func(i api.FeedItem) api.FeedItem {
			i.Action, i.UsdcMicros, i.AssetName = ptr("sell"), ptr(int64(300_000_000)), ptr("Apple")
			i.TokenAmount, i.TokenDecimals, i.PriceMicros = ptr(int64(100_000_000)), ptr(8), ptr(int64(300_000_000))
			i.ProposalId = ptr(proposal)
			return i
		}},
		"a proposal stored before the share shipped": {
			feed.KindProposal,
			feed.Payload{Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy, Status: "open"},
			func(i api.FeedItem) api.FeedItem {
				i.Action, i.AssetName, i.ProposalId = ptr("buy"), ptr("Apple"), ptr(ref)
				return i
			},
		},
		"a price move": {
			feed.KindPriceMove,
			feed.Payload{Symbol: "AAPLx", AssetName: "Apple"},
			func(i api.FeedItem) api.FeedItem { i.AssetName = ptr("Apple"); return i },
		},
		"a cabal created": {feed.KindCabalCreated, feed.Payload{CabalName: "Alpha"}, func(i api.FeedItem) api.FeedItem { return i }},
		"a member joined": {feed.KindMemberJoined, feed.Payload{CabalName: "Alpha"}, func(i api.FeedItem) api.FeedItem { return i }},
		"an oversize fill": {feed.KindTrade, trade(feed.ActionBuy, 1, math.MaxUint64), func(i api.FeedItem) api.FeedItem {
			i.Action, i.UsdcMicros, i.AssetName = ptr("buy"), ptr(int64(1)), ptr("Apple")
			i.ProposalId = ptr(proposal)
			return i
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFeedFixture(t)
			got := f.wireItem(t, func(it *feed.Item) { it.Kind, it.RefID, it.Payload = tc.kind, ref, tc.payload })
			assertFigures(t, got, tc.want(api.FeedItem{}))
		})
	}
}

func assertFigures(t *testing.T, got, want api.FeedItem) {
	t.Helper()
	type figures struct {
		Action, AssetName                                   *string
		UsdcMicros, TokenAmount, PriceMicros, ShareOfPotBps *int64
		TokenDecimals                                       *int
		ProposalID                                          *uuid.UUID
	}
	pick := func(i api.FeedItem) figures {
		return figures{
			i.Action, i.AssetName, i.UsdcMicros, i.TokenAmount, i.PriceMicros, i.ShareOfPotBps, i.TokenDecimals,
			i.ProposalId,
		}
	}
	if !reflect.DeepEqual(pick(got), pick(want)) {
		t.Fatalf("figures = %+v, want %+v", pick(got), pick(want))
	}
}
