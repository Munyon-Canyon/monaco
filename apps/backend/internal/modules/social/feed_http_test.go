package social_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func (f feedFixture) routes() adapters.HTTP {
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	return social.HTTPOf(social.New(deps, social.WithUsers(fakes.NewIdentity(nil, nil))))
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
		ActorId: ptr(viewer.UUID()), Symbol: ptr("AAPLx"), Title: "Alpha Cabal bought $500 of AAPLx",
		Detail: ptr("Apple"), Tone: "neutral", CreatedAt: olderAt, UpdatedAt: olderAt,
	}
	if len(second.Items) != 1 || !reflect.DeepEqual(second.Items[0], want) || second.NextCursor != nil {
		t.Fatalf("second page = %+v, want [%+v] and no cursor", second, want)
	}
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
		"sort":   {Sort: ptr(api.GetFeedParamsSort("top"))},
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
		if !ok || got.Item.Id != id || got.Visible != visible || got.Item.Title != "Alpha Cabal bought $500 of AAPLx" {
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
