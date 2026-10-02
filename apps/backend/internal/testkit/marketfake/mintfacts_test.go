package marketfake_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestMintFacts_answersWhatWasPutAndFailsWhereFaultsSay(t *testing.T) {
	t.Parallel()
	var f marketfake.MintFacts
	aapl, tsla := marketfake.AAPLx().Mint, marketfake.TSLAx().Mint
	f.Put(aapl, 6, 3, 2)
	answers, failures, err := f.Facts(t.Context(), []market.Mint{aapl})
	if got := answers[aapl]; err != nil || len(failures) != 0 || !isAAPLFact(got) {
		t.Fatalf("Facts(AAPLx) = %+v, %v, %v, want 6 decimals at 3/2", answers, failures, err)
	}
	_, failures, err = f.Facts(t.Context(), []market.Mint{tsla})
	if err != nil || errs.CodeOf(failures[tsla]) != errs.CodeNotFound {
		t.Fatalf("Facts(TSLAx) never put = %v, %v, want not_found like a missing account", failures, err)
	}
}

func TestMintFacts_returnsScriptedFailures(t *testing.T) {
	t.Parallel()
	var f marketfake.MintFacts
	aapl := marketfake.AAPLx().Mint
	f.Put(aapl, 6, 3, 2)
	down := errs.New(errs.CodeRPCUnavailable, "test")
	f.FailOnce("Facts:"+aapl.String(), down)
	_, failures, err := f.Facts(t.Context(), []market.Mint{aapl})
	if err != nil || errs.CodeOf(failures[aapl]) != errs.CodeRPCUnavailable {
		t.Fatalf("Facts(AAPLx) after FailOnce on its mint = %v, %v, want the scripted fault", failures, err)
	}
	answers, failures, err := f.Facts(t.Context(), []market.Mint{aapl})
	if err != nil || len(failures) != 0 || !isAAPLFact(answers[aapl]) {
		t.Fatalf("Facts(AAPLx) after the one fault = %+v, %v, %v, want the put facts again", answers, failures, err)
	}
	f.Fail("Facts", down)
	_, _, err = f.Facts(t.Context(), []market.Mint{aapl})
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Facts(AAPLx) with every mint failing = %v, want the scripted fault", err)
	}
	if got := f.Asked(); got != 3 {
		t.Fatalf("Asked = %d, want every faulted call counted", got)
	}
}

func isAAPLFact(f app.MintFact) bool {
	return f.Decimals == 6 && f.MultiplierNum == 3 && f.MultiplierDen == 2
}
