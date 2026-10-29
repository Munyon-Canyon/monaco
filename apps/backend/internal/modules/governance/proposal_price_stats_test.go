package governance_test

import (
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	livePrice  = 220_000_000
	filledOut  = 11_400_000
	filledCost = 219_298_245
)

func (d readsDB) priceBook(micros uint64) *marketfake.PricesFake {
	book := &marketfake.PricesFake{}
	if micros > 0 {
		book.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(micros), d.now)
	}
	return book
}

func (d readsDB) detailWith(t *testing.T, book *marketfake.PricesFake, p uuid.UUID) api.ProposalDetail {
	t.Helper()
	h := adapters.HTTP{Reads: d.reads().Priced(book, testkit.NewClock(d.now))}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	res, err := h.GetProposal(ctx, api.GetProposalRequestObject{Id: p})
	got, ok := res.(api.GetProposal200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetProposal = %#v, %v, want 200", res, err)
	}
	return api.ProposalDetail(got)
}

func (d readsDB) putSwap(source uuid.UUID, s trading.SwapView, fee *money.Micros) {
	s.ID, s.Source, s.Action = ids.SwapIDFrom(d.ids.NewV7()), trading.Source{Kind: "proposal", ID: source}, "buy"
	s.InAmount, s.CreatedAt, s.FeeMicros = 25_000_000, d.now, fee
	if s.Status == "confirmed" {
		s.OutAmount, s.ConfirmedAt = filledOut, d.now
	}
	d.swaps.Put(s)
}

func equalPtr[T comparable](got *T, want T) bool { return got != nil && *got == want }

func TestGetProposal_openPriceAndEstimate(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.seed(t, d.now)
	book := d.priceBook(livePrice)
	got := d.detailWith(t, book, p).Stats
	if !equalPtr(got.PriceMicros, livePrice) || !equalPtr(got.SharesAtomics, 11_363_636) ||
		!equalPtr(got.SharesExact, false) || !equalPtr(got.TokenDecimals, 8) || got.SinceBps != nil {
		t.Fatalf("open stats = %+v, want price 220000000, estimate 11363636 inexact, no since", got)
	}
	if asked := book.Asked(); len(asked) != 1 || len(asked[0]) != 1 || asked[0][0] != marketfake.AAPLx().ID {
		t.Fatalf("price book asked %v, want the proposal's asset once", asked)
	}
}

func TestGetProposal_filledStatsAndReceipt(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.seed(t, d.now)
	fee := money.MicrosFromUint64(400_000)
	d.putSwap(p, trading.SwapView{Status: "confirmed"}, &fee)
	got := d.detailWith(t, d.priceBook(livePrice), p)
	if got.Swap == nil || !equalPtr(got.Swap.PriceMicros, filledCost) || !equalPtr(got.Swap.FeeMicros, 400_000) ||
		got.Swap.FilledAt == nil || !got.Swap.FilledAt.Equal(d.now) {
		t.Fatalf("swap = %+v, want fill %d, fee 400000, filled at %s", got.Swap, filledCost, d.now)
	}
	s := got.Stats
	if !equalPtr(s.PriceMicros, livePrice) || !equalPtr(s.SharesAtomics, filledOut) || !equalPtr(s.SharesExact, true) ||
		!equalPtr(s.SinceBps, 32) {
		t.Fatalf("stats = %+v, want live 220000000, exact %d shares, +32 bps", s, filledOut)
	}
}

func TestGetProposal_pendingSwapHasNoFill(t *testing.T) {
	t.Parallel()
	for _, swap := range []trading.SwapView{{Status: "created"}, {Status: "submitted"}, {Status: "failed"}} {
		status := string(swap.Status)
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			d := newReadsDB(t)
			p := d.seed(t, d.now)
			fee := money.MicrosFromUint64(400_000)
			d.putSwap(p, swap, &fee)
			got := d.detailWith(t, d.priceBook(livePrice), p)
			noFill := got.Swap != nil && got.Swap.FilledAt == nil && got.Swap.FeeMicros == nil &&
				got.Swap.PriceMicros == nil
			if !noFill || got.Stats.SinceBps != nil || !equalPtr(got.Stats.SharesExact, false) ||
				!equalPtr(got.Stats.SharesAtomics, 11_363_636) {
				t.Fatalf("%s detail = swap %+v, stats %+v, want no fill, fee or since and the estimate",
					status, got.Swap, got.Stats)
			}
		})
	}
}

func TestGetProposal_fallingPriceSinceIsNegative(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		live uint64
		want int64
	}{"falling": {200_000_000, -879}, "unchanged": {filledCost, 0}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newReadsDB(t)
			p := d.seed(t, d.now)
			d.putSwap(p, trading.SwapView{Status: "confirmed"}, nil)
			got := d.detailWith(t, d.priceBook(tc.live), p)
			if !equalPtr(got.Stats.SinceBps, tc.want) {
				t.Fatalf("since = %v, want %d", got.Stats.SinceBps, tc.want)
			}
			if got.Swap.FeeMicros != nil {
				t.Fatalf("fee = %v, want null when the swap recorded none", got.Swap.FeeMicros)
			}
		})
	}
}

func TestGetProposal_sellSharesExact(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.buy(d.caller)
	p.CabalID, p.Kind, p.UsdcMicros = d.cabal, "sell", pgtype.Int8{}
	p.TokenAmount, p.QuoteOutAmount = pgtype.Int8{Int64: 5_000_000, Valid: true}, 40_000_000
	d.insert(t, p)
	got := d.detailWith(t, d.priceBook(livePrice), p.ID).Stats
	if !equalPtr(got.SharesAtomics, 5_000_000) || !equalPtr(got.SharesExact, true) ||
		!equalPtr(got.PriceMicros, livePrice) {
		t.Fatalf("sell stats = %+v, want exactly 5000000 shares", got)
	}
}

func TestGetProposal_unpricedIsNull(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.seed(t, d.now)
	got := d.detailWith(t, d.priceBook(0), p).Stats
	if got.PriceMicros != nil || got.SharesAtomics != nil || got.SinceBps != nil || !equalPtr(got.SharesExact, false) {
		t.Fatalf("unpriced stats = %+v, want null price, shares and since", got)
	}
	d.putSwap(p, trading.SwapView{Status: "confirmed"}, nil)
	filled := d.detailWith(t, d.priceBook(0), p).Stats
	if filled.PriceMicros != nil || filled.SinceBps != nil || !equalPtr(filled.SharesExact, true) ||
		!equalPtr(filled.SharesAtomics, filledOut) {
		t.Fatalf("unpriced fill stats = %+v, want exact shares and no price or since", filled)
	}
}

func TestGetProposal_priceBookFailureIsAnError(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.seed(t, d.now)
	book := d.priceBook(livePrice)
	book.FailOnce("PricesAsOf", errs.New(errs.CodeDBUnavailable, "t"))
	h := adapters.HTTP{Reads: d.reads().Priced(book, testkit.NewClock(d.now))}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	if _, err := h.GetProposal(ctx, api.GetProposalRequestObject{Id: p}); err == nil {
		t.Fatal("GetProposal with a failing price book succeeded")
	}
}

func TestGetProposal_overflowingStatIsNullAndTheReadSucceeds(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.seed(t, d.now)
	d.putSwap(p, trading.SwapView{Status: "confirmed"}, nil)
	got := d.detailWith(t, d.priceBook(math.MaxUint64), p)
	if got.Stats.SinceBps != nil || !equalPtr(got.Stats.SharesAtomics, filledOut) || got.Swap == nil {
		t.Fatalf("detail with an overflowing price = stats %+v, swap %+v, want null since and exact shares",
			got.Stats, got.Swap)
	}
}

func TestSinceBps_aChangeBeyondInt64BpsIsAnErrorNotAValue(t *testing.T) {
	t.Parallel()
	got, err := app.SinceBps(money.MicrosFromUint64(5_000), money.MicrosFromUint64(math.MaxInt64-1_000))
	if got != nil || err == nil {
		t.Fatalf("SinceBps(5000, near MaxInt64) = %v, %v, want nil and an error", got, err)
	}
}

func TestSinceBps_failsWhenTheMoveOverflowsBasisPoints(t *testing.T) {
	t.Parallel()
	got, err := app.SinceBps(money.MicrosFromUint64(1), money.MicrosFromUint64(math.MaxInt64))
	if err == nil || got != nil {
		t.Fatalf("SinceBps = %v, %v, want an overflow error", got, err)
	}
}
