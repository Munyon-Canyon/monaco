package marketfake_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestMintFacts_answersWhatWasPutAndFailsWhereFaultsSay(t *testing.T) {
	t.Parallel()
	var f marketfake.MintFacts
	aapl, tsla := marketfake.AAPLx().Mint, marketfake.TSLAx().Mint
	f.Put(aapl, 6, 3, 2)
	if decimals, num, den, err := f.Facts(t.Context(), aapl); err != nil || decimals != 6 || num != 3 || den != 2 {
		t.Fatalf("Facts(AAPLx) = %d, %d/%d, %v, want 6 decimals at 3/2", decimals, num, den, err)
	}
	if _, _, _, err := f.Facts(t.Context(), tsla); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("Facts(TSLAx) never put = %v, want not_found like a missing account", err)
	}
	down := errs.New(errs.CodeRPCUnavailable, "test")
	f.FailOnce("Facts:"+aapl.String(), down)
	if _, _, _, err := f.Facts(t.Context(), aapl); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Facts(AAPLx) after FailOnce on its mint = %v, want the scripted fault", err)
	}
	if _, _, _, err := f.Facts(t.Context(), aapl); err != nil {
		t.Fatalf("Facts(AAPLx) after the one fault = %v, want the put facts again", err)
	}
	f.Fail("Facts", down)
	if _, _, _, err := f.Facts(t.Context(), aapl); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Facts(AAPLx) with every mint failing = %v, want the scripted fault", err)
	}
	if got := f.Asked(); got != 5 {
		t.Fatalf("Asked = %d, want every one of the 5 calls counted, failed or not", got)
	}
}
