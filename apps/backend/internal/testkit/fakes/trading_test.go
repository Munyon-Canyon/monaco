package fakes_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestTrading_answersTheQueriesPortFromItsSwaps(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(7)
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	src := trading.Source{Kind: domain.SourceProposal, ID: g.NewV7()}
	failed := trading.SwapView{
		ID: ids.SwapIDFrom(g.NewV7()), Source: src, Status: domain.StatusFailed, CreatedAt: at, Retryable: true,
	}
	retry := trading.SwapView{
		ID: ids.SwapIDFrom(g.NewV7()), Source: src, Status: domain.StatusCreated, CreatedAt: at.Add(time.Minute),
	}
	f := fakes.NewTrading(failed)
	ctx := t.Context()
	if live, err := f.HasLiveSwap(ctx, src); err != nil || live {
		t.Fatalf("HasLiveSwap with only a failed swap = %t, %v", live, err)
	}
	f.Put(retry)
	retry.Status, retry.TxSignature = domain.StatusSubmitted, "sig-1"
	f.Put(retry)
	if got, err := f.Swap(ctx, retry.ID); err != nil || got != retry {
		t.Fatalf("Swap = %+v, %v, want the replaced row", got, err)
	}
	if got, err := f.SwapBySignature(ctx, "sig-1"); err != nil || got != retry {
		t.Fatalf("SwapBySignature = %+v, %v", got, err)
	}
	if got, ok, err := f.LatestBySource(ctx, src); err != nil || !ok || got != retry {
		t.Fatalf("LatestBySource = %+v, %t, %v, want the retry", got, ok, err)
	}
	if live, err := f.HasLiveSwap(ctx, src); err != nil || !live {
		t.Fatalf("HasLiveSwap with a submitted swap = %t, %v", live, err)
	}
}

func TestTrading_findsNothingItWasNotGiven(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(9)
	src := trading.Source{Kind: domain.SourceProposal, ID: g.NewV7()}
	f := fakes.NewTrading(trading.SwapView{ID: ids.SwapIDFrom(g.NewV7()), Source: src, TxSignature: "sig-1"})
	ctx := t.Context()
	for sig, want := range map[chain.Signature]bool{"sig-1": true, "sig-2": false, "": false} {
		if owns, err := f.OwnsSignature(ctx, sig); err != nil || owns != want {
			t.Errorf("OwnsSignature(%q) = %t, %v, want %t", sig, owns, err, want)
		}
	}
	if _, err := f.Swap(ctx, ids.SwapIDFrom(g.NewV7())); errs.CodeOf(err) != errs.CodeSwapNotFound {
		t.Fatalf("Swap of an unknown id err = %v, want swap_not_found", err)
	}
	other := trading.Source{Kind: domain.SourceCashout, ID: src.ID}
	if _, ok, err := f.LatestBySource(ctx, other); ok || err != nil {
		t.Fatalf("LatestBySource of another source = %t, %v, want nothing", ok, err)
	}
}

func TestTrading_failAndFailOnce(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(8)
	src := trading.Source{Kind: domain.SourceProposal, ID: g.NewV7()}
	f := fakes.NewTrading()
	ctx := t.Context()
	down := errs.New(errs.CodeDBUnavailable, "test")
	f.FailOnce(down)
	if _, err := f.HasLiveSwap(ctx, src); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("HasLiveSwap after FailOnce err = %v, want db_unavailable", err)
	}
	if _, err := f.HasLiveSwap(ctx, src); err != nil {
		t.Fatalf("second HasLiveSwap after FailOnce err = %v, want nil", err)
	}
	f.Fail(down)
	_, swapErr := f.Swap(ctx, ids.SwapIDFrom(g.NewV7()))
	_, _, latestErr := f.LatestBySource(ctx, src)
	_, liveErr := f.HasLiveSwap(ctx, src)
	_, ownsErr := f.OwnsSignature(ctx, "sig-1")
	for name, err := range map[string]error{
		"Swap": swapErr, "LatestBySource": latestErr, "HasLiveSwap": liveErr, "OwnsSignature": ownsErr,
	} {
		if errs.CodeOf(err) != errs.CodeDBUnavailable {
			t.Errorf("%s after Fail err = %v, want db_unavailable", name, err)
		}
	}
	f.Fail(nil)
	if _, err := f.HasLiveSwap(ctx, src); err != nil {
		t.Fatalf("HasLiveSwap after Fail(nil) err = %v", err)
	}
}
