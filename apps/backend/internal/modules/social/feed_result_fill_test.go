package social_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (r proposalRig) swap(batch int) events.TradeConfirmed {
	e := r.confirmed("buy", "proposal")
	e.SourceBatchSize = batch
	return e
}

func (r proposalRig) failedSwap(t *testing.T, e events.TradeConfirmed, code string) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		return r.feed.TradeFailed(ctx, tx, events.TradeFailed{
			V: 1, SwapID: e.SwapID, CabalID: e.CabalID, Source: e.Source, SourceBatchSize: e.SourceBatchSize,
			Action: e.Action, FailureCode: code,
		}, r.clock.Now())
	})
}

func (r proposalRig) passedRig(t *testing.T) proposalRig {
	t.Helper()
	r.mustCreate(t, func(*events.ProposalCreated) {})
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r proposalRig) resultPayload(t *testing.T) []byte {
	t.Helper()
	var raw []byte
	err := r.pool.QueryRow(t.Context(), `SELECT payload FROM feed_objects WHERE kind = 'result'`).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (r proposalRig) wantFill(t *testing.T, state, detail string) resultRow {
	t.Helper()
	got := r.onlyResult(t)
	if got.Payload.FillState != state || got.Body != detail {
		t.Fatalf("fill state %q body %q, want %q and %q", got.Payload.FillState, got.Body, state, detail)
	}
	if want := feed.RenderTitle(feed.KindResult, got.Payload); got.Title != want {
		t.Fatalf("title = %q, want %q", got.Title, want)
	}
	return got
}

func TestFeedResult_aTwoSwapOrderIsFilledOnceBothConfirmInEitherOrder(t *testing.T) {
	t.Parallel()
	for name, order := range map[string][2]int{"first then second": {0, 1}, "second then first": {1, 0}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newProposalRig(t).passedRig(t)
			swaps := [2]events.TradeConfirmed{r.swap(2), r.swap(2)}
			swaps[1].USDCMicros = money.MicrosFromUint64(250_000_000)
			sub := testkit.SubscribeCore(t, r.conn, hintSubject)
			if err := r.deliverTrade(t, swaps[order[0]]); err != nil {
				t.Fatal(err)
			}
			r.wantFill(t, feed.FillPending, "buying")
			if err := r.deliverTrade(t, swaps[order[1]]); err != nil {
				t.Fatal(err)
			}
			r.wantFilledOrder(t)
			wantHints(t, r.conn.Conn, sub, 2)
		})
	}
}

func (r proposalRig) wantFilledOrder(t *testing.T) {
	t.Helper()
	got := r.wantFill(t, feed.FillFilled, "filled")
	state, tokens, usdc, _, err := feed.DeriveFill(got.Payload)
	if err != nil || state != feed.FillFilled || tokens.Uint64() != 500_000_000 ||
		usdc.Uint64() != 750_000_000 {
		t.Fatalf("derived %s %s %s, %v, want filled 500000000 750000000", state, tokens, usdc, err)
	}
	if !strings.HasSuffix(got.Title, "· filled") {
		t.Fatalf("title = %q, want it to end filled", got.Title)
	}
	var found int
	err = r.pool.QueryRow(t.Context(), `SELECT count(*) FROM feed_objects
		WHERE kind = 'result' AND search @@ plainto_tsquery('english', 'buy apple filled')`).Scan(&found)
	if err != nil || found != 1 {
		t.Fatalf("search found %d rows, %v, want 1", found, err)
	}
	if rows := r.tradeRows(t); len(rows) != 0 {
		t.Fatalf("trade rows = %+v, want none", rows)
	}
}

func TestFeedResult_oneSwapConfirmedAndOneFailedIsPartialInEitherOrder(t *testing.T) {
	t.Parallel()
	for _, failFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirm then fail", true: "fail then confirm"}[failFirst], func(t *testing.T) {
			t.Parallel()
			r := newProposalRig(t).passedRig(t)
			ok, bad := r.swap(2), r.swap(2)
			confirm := func() error { return r.deliverTrade(t, ok) }
			fail := func() error { return r.failedSwap(t, bad, "price_moved") }
			first, second := confirm, fail
			if failFirst {
				first, second = fail, confirm
			}
			if err := errors.Join(first(), second()); err != nil {
				t.Fatal(err)
			}
			r.wantPartial(t, bad)
		})
	}
}

func (r proposalRig) wantPartial(t *testing.T, bad events.TradeConfirmed) {
	t.Helper()
	got := r.wantFill(t, feed.FillPartial, "partly filled · trade failed")
	if !strings.HasSuffix(got.Title, "· trade failed") {
		t.Fatalf("title = %q, want it to end trade failed", got.Title)
	}
	_, tokens, usdc, _, err := feed.DeriveFill(got.Payload)
	if err != nil || tokens.Uint64() != 250_000_000 || usdc.Uint64() != 500_000_000 {
		t.Fatalf("partial sums %s %s, %v, want the confirmed swap only", tokens, usdc, err)
	}
	if got.Payload.Failures[bad.SwapID.String()] != "price_moved" {
		t.Fatalf("failures = %v", got.Payload.Failures)
	}
}

func TestFeedResult_everySwapFailingIsFailed(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	for range 2 {
		if err := r.failedSwap(t, r.swap(2), "price_moved"); err != nil {
			t.Fatal(err)
		}
	}
	r.wantFill(t, feed.FillFailed, "trade failed")
}

func TestFeedResult_aTradeBeforeTheCloseRetriesAndLandsAfterIt(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	e := r.swap(1)
	if got := errs.CodeOf(r.deliverTrade(t, e)); got != errs.CodeFeedItemPending {
		t.Fatalf("confirmed before the proposal item = %s, want %s", got, errs.CodeFeedItemPending)
	}
	r.mustCreate(t, func(*events.ProposalCreated) {})
	if got := errs.CodeOf(r.deliverTrade(t, e)); got != errs.CodeFeedItemPending {
		t.Fatalf("confirmed before the close = %s, want %s", got, errs.CodeFeedItemPending)
	}
	if got := errs.CodeOf(r.failedSwap(t, r.swap(1), "x")); got != errs.CodeFeedItemPending {
		t.Fatalf("failed before the close = %s, want %s", got, errs.CodeFeedItemPending)
	}
	if rows := r.results(t); len(rows) != 0 {
		t.Fatalf("result rows = %d, want the fill never to create one", len(rows))
	}
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	if err := r.deliverTrade(t, e); err != nil {
		t.Fatal(err)
	}
	r.wantFill(t, feed.FillFilled, "filled")
}

func TestFeedResult_aReopenAndSecondCloseKeepTheFillAndTheFailure(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	ok, bad := r.swap(2), r.swap(2)
	if err := r.deliverTrade(t, ok); err != nil {
		t.Fatal(err)
	}
	if err := r.failedSwap(t, bad, "price_moved"); err != nil {
		t.Fatal(err)
	}
	before := r.onlyResult(t).Payload
	if err := r.failed(t); err != nil {
		t.Fatal(err)
	}
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	after := r.wantFill(t, feed.FillPartial, "partly filled · trade failed").Payload
	if len(after.Fills) != 1 || len(after.Failures) != 1 || after.ExpectedFills != 2 ||
		after.Fills[ok.SwapID.String()] != before.Fills[ok.SwapID.String()] {
		t.Fatalf("payload after the second close = %+v, want %+v", after, before)
	}
}

func TestFeedResult_redeliveringATradeEventChangesNothing(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	ok, bad := r.swap(2), r.swap(2)
	if err := r.deliverTrade(t, ok); err != nil {
		t.Fatal(err)
	}
	if err := r.failedSwap(t, bad, "price_moved"); err != nil {
		t.Fatal(err)
	}
	want := r.resultPayload(t)
	for _, redeliver := range []func() error{
		func() error { return r.deliverTrade(t, ok) },
		func() error { return r.failedSwap(t, bad, "price_moved") },
		func() error { return r.failedSwap(t, ok, "price_moved") },
		func() error { return r.deliverTrade(t, bad) },
	} {
		if err := redeliver(); err != nil {
			t.Fatal(err)
		}
	}
	if got := string(r.resultPayload(t)); got != string(want) {
		t.Fatalf("payload after redelivery = %s, want %s", got, want)
	}
}

func TestFeedResult_fillsThatOverflowFailTheDeliveryAndLeaveTheRow(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	first, second := r.swap(2), r.swap(2)
	first.USDCMicros = money.MicrosFromUint64(math.MaxUint64 - 1)
	second.USDCMicros = money.MicrosFromUint64(2)
	if err := r.deliverTrade(t, first); err != nil {
		t.Fatal(err)
	}
	want := r.resultPayload(t)
	if err := r.deliverTrade(t, second); err == nil {
		t.Fatal("overflowing fill = nil, want an error")
	}
	if got := string(r.resultPayload(t)); got != string(want) {
		t.Fatalf("payload after the overflow = %s, want %s", got, want)
	}
}

func TestFeedResult_aDifferentBatchSizeFailsTheDelivery(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	if err := r.deliverTrade(t, r.swap(2)); err != nil {
		t.Fatal(err)
	}
	if got := errs.CodeOf(r.deliverTrade(t, r.swap(3))); got != errs.CodeFeedFillBatchMismatch {
		t.Fatalf("batch of 3 after a batch of 2 = %s, want %s", got, errs.CodeFeedFillBatchMismatch)
	}
	if got := errs.CodeOf(r.failedSwap(t, r.swap(3), "x")); got != errs.CodeFeedFillBatchMismatch {
		t.Fatalf("failed batch of 3 after a batch of 2 = %s, want %s", got, errs.CodeFeedFillBatchMismatch)
	}
}

func TestFeedResult_aFailureWithNoSwapBatchIsFailed(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	if err := r.failedSwap(t, r.swap(0), "no_route"); err != nil {
		t.Fatal(err)
	}
	r.wantFill(t, feed.FillFailed, "trade failed")
}

func TestFeedResult_aFailedVoteHasNoFillState(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	if err := r.failed(t); err != nil {
		t.Fatal(err)
	}
	r.wantFill(t, "", "no trade")
}

func (r proposalRig) feedResult(t *testing.T) api.FeedItem {
	t.Helper()
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	routes := f.routesOptions(social.WithUsers(r.users), social.WithTallies(r.tallies))
	res, err := routes.GetFeed(asUser(t.Context(), r.bob), api.GetFeedRequestObject{Params: api.GetFeedParams{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range res.(api.GetFeed200JSONResponse).Items {
		if item.Kind == "result" {
			return item
		}
	}
	t.Fatal("no result item in the feed")
	return api.FeedItem{}
}

func wantPartialItem(t *testing.T, got api.FeedItem, filledAt time.Time) {
	t.Helper()
	if got.FillState == nil || *got.FillState != api.FeedItemFillStatePartial || !got.TradeFailed {
		t.Fatalf("partial item state = %v failed %t", got.FillState, got.TradeFailed)
	}
	if got.FilledAt == nil || !got.FilledAt.Equal(filledAt) {
		t.Fatalf("partial item filled_at = %v, want %v", got.FilledAt, filledAt)
	}
	wantPartialAmounts(t, got)
}

func wantPartialAmounts(t *testing.T, got api.FeedItem) {
	t.Helper()
	if got.FilledTokenAmount == nil || *got.FilledTokenAmount != 250_000_000 ||
		got.FilledUsdcMicros == nil || *got.FilledUsdcMicros != 500_000_000 {
		t.Fatalf("partial item amounts = %v %v", got.FilledTokenAmount, got.FilledUsdcMicros)
	}
	if got.PriceMicros == nil || *got.PriceMicros != 200_000_000 || got.TokenDecimals == nil ||
		*got.TokenDecimals != 8 {
		t.Fatalf("partial item price %v decimals %v", got.PriceMicros, got.TokenDecimals)
	}
}

func TestGetFeed_aResultCarriesItsFillAndWhetherTheTradeFailed(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	ok, bad := r.swap(2), r.swap(2)
	filledAt := r.clock.Now().UTC().Add(-5 * time.Hour).Truncate(time.Second)
	ok.ConfirmedAt = filledAt
	pending := r.feedResult(t)
	if pending.FillState == nil || *pending.FillState != api.FeedItemFillStatePending || pending.TradeFailed ||
		pending.FilledTokenAmount != nil || pending.FilledAt != nil {
		t.Fatalf("pending item = %+v", pending)
	}
	if err := errors.Join(r.deliverTrade(t, ok), r.failedSwap(t, bad, "price_moved")); err != nil {
		t.Fatal(err)
	}
	wantPartialItem(t, r.feedResult(t), filledAt)
}

func (r proposalRig) setResultPayload(t *testing.T, payload string) {
	t.Helper()
	_, err := r.pool.Exec(t.Context(), `UPDATE feed_objects SET payload = $1::jsonb WHERE kind = 'result'`, payload)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFeedResult_aStoredResultThatCannotBeReadFailsTheSwap(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	r.setResultPayload(t, `{"outcome":"passed","fills":{"a":"x"}}`)
	if err := r.deliverTrade(t, r.swap(1)); err == nil {
		t.Fatal("trade against an unreadable result = nil, want an error")
	}
}

func TestGetFeed_aStoredFillThatOverflowsFailsTheRead(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	r.setResultPayload(t, `{"outcome":"passed","expected_fills":2,"fills":{
		"a":{"token_amount":"1","token_decimals":8,"usdc_micros":"18446744073709551615","filled_at":"2026-10-03T08:41:00Z"},
		"b":{"token_amount":"1","token_decimals":8,"usdc_micros":"1","filled_at":"2026-10-03T08:41:00Z"}}}`)
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	routes := f.routesOptions(social.WithUsers(r.users), social.WithTallies(r.tallies))
	_, err := routes.GetFeed(asUser(t.Context(), r.bob), api.GetFeedRequestObject{Params: api.GetFeedParams{}})
	if err == nil {
		t.Fatal("feed with an overflowing stored fill = nil, want an error")
	}
}

func TestFeedResult_aSellFillRecordsTheTokensSold(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).passedRig(t)
	e := r.confirmed("sell", "proposal")
	e.SourceBatchSize = 1
	if err := r.deliverTrade(t, e); err != nil {
		t.Fatal(err)
	}
	got := r.wantFill(t, feed.FillFilled, "filled")
	_, tokens, _, _, err := feed.DeriveFill(got.Payload)
	if err != nil || tokens.Uint64() != e.InAmount {
		t.Fatalf("sold tokens = %s, %v, want %d", tokens, err, e.InAmount)
	}
}
