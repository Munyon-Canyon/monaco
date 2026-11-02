package governance_test

import (
	"math/big"
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func robinhoodTokens(whole int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(whole), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

func TestProposeTrade_opensAnEVMBuyQuotedPastInt64(t *testing.T) {
	t.Parallel()
	for name, whole := range map[string]int64{"5 tokens": 5, "10 tokens": 10, "25 tokens": 25} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newTradeWorld(t)
			c := appleComparison(t)
			c.Ranked[0].GrossOutput = robinhoodTokens(whole)
			s := w.routedScenario(t, stubRouter{t: t, c: c}).Given(w.asMember())
			s.When(scenario.Post(w.proposals(), buyAAPL), scenario.ExpectStatus(http.StatusCreated))
			if n := proposalRows(t, s, w.cabal); n != 1 {
				t.Fatalf("proposals rows = %d, want 1", n)
			}
		})
	}
}

func TestPreview_ofAnEVMBuyQuotedPastInt64CarriesNoAdvisory(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	c := appleComparison(t)
	c.Ranked[0].GrossOutput = robinhoodTokens(10)
	w.routedScenario(t, stubRouter{t: t, c: c}).Given(w.asMember()).
		When(
			scenario.Get(w.preview("kind=buy&symbol=AAPLx&usdc_micros=5000000")),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("advisory_code", nil),
		)
}
