package social_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func (r proposalRig) moved() events.AssetPriceMoved {
	return events.AssetPriceMoved{
		V: 1, AssetID: marketfake.AAPLx().ID.UUID(), Symbol: "AAPLx", AssetName: "Apple",
		ThresholdBps: 1000, ChangeBps: 1000, MarkMicros: money.MicrosFromUint64(212_400_000),
		PrevCloseMicros: money.MicrosFromUint64(193_100_000), TradingDay: "2026-03-02", ObservedAt: r.now,
	}
}

func (r proposalRig) deliverMove(t *testing.T, e events.AssetPriceMoved) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		return r.feed.PriceMoved(ctx, tx, e, r.clock.Now())
	})
}

func TestFeedPriceMove_InsertsOnce(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	sub := testkit.SubscribeCore(t, r.conn, hintSubject)
	e := r.moved()
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(r.gen.NewV7()))
	for range 2 {
		err := db.New(r.pool, r.gen, r.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
			return r.feed.PriceMoved(ctx, tx, e, r.clock.Now())
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var (
		n            int
		ref, id      [16]byte
		cabal, actor *[16]byte
		title        string
		raw          json.RawMessage
	)
	err := r.pool.QueryRow(t.Context(), `SELECT count(*) OVER (), id, ref_id, cabal_id, actor_id, title, payload
		FROM feed_objects WHERE kind = 'price_move' AND ref_type = 'asset_price_moves'`).
		Scan(&n, &id, &ref, &cabal, &actor, &title, &raw)
	if err != nil || n != 1 || ref != id || title != "AAPLx is up 10% today" {
		t.Fatalf("rows = %d id %x ref %x title %q, %v", n, id, ref, title, err)
	}
	if cabal != nil || actor != nil {
		t.Fatalf("cabal %v actor %v, want both null", cabal, actor)
	}
	wantPriceMovePayload(t, raw)
	payload, err := feed.ParsePayload(raw)
	if err != nil || feed.RenderDetail(feed.KindPriceMove, payload) != "$212.40, previous close $193.10" {
		t.Fatalf("detail = %q, %v", feed.RenderDetail(feed.KindPriceMove, payload), err)
	}
	wantHints(t, r.conn.Conn, sub, 2)
}

func TestFeedPriceMove_returnsAWriteFailure(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if _, err := r.pool.Exec(t.Context(), "ALTER TABLE feed_objects RENAME TO feed_objects_gone"); err != nil {
		t.Fatal(err)
	}
	if err := r.deliverMove(t, r.moved()); err == nil || errs.CodeOf(err) == errs.CodeFeedItemPending {
		t.Fatalf("move = %v, want a database failure", err)
	}
}

func TestGetFeed_aPriceMoveMatchesTheSymbolAndNotACabal(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if err := r.deliverMove(t, r.moved()); err != nil {
		t.Fatal(err)
	}
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	viewer := ids.NewUserID(r.gen)
	got := f.getFeed(t, viewer, api.GetFeedParams{Symbol: ptr("aaplx")})
	if len(got.Items) != 1 || got.Items[0].Kind != "price_move" || got.Items[0].CabalId != nil ||
		got.Items[0].Title != "AAPLx is up 10% today" ||
		got.Items[0].Detail == nil || *got.Items[0].Detail != "$212.40, previous close $193.10" ||
		got.Items[0].Tone != "positive" {
		t.Fatalf("symbol filter = %+v, want the price move", got.Items)
	}
	byCabal := f.getFeed(t, viewer, api.GetFeedParams{Symbol: ptr("aaplx"), CabalId: ptr(r.cabal)})
	if len(byCabal.Items) != 0 {
		t.Fatalf("cabal filter = %+v, want none", byCabal.Items)
	}
}

func wantPriceMovePayload(t *testing.T, raw json.RawMessage) {
	t.Helper()
	want := map[string]any{
		"symbol": "AAPLx", "asset_name": "Apple", "threshold_bps": 1000.0, "change_bps": 1000.0,
		"mark_micros": "212400000", "prev_close_micros": "193100000", "trading_day": "2026-03-02",
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("payload = %s, want %v", raw, want)
	}
}
