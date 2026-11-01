package governance_test

import (
	"math/big"
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func robinhoodTokens(whole int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(whole), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

func TestProposeTrade_refusesAnEVMBuyQuotedPastInt64AndOpensOneWithin(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		whole int64
		fits  bool
	}{"5 tokens": {5, true}, "10 tokens": {10, false}, "25 tokens": {25, false}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newTradeWorld(t)
			c := appleComparison(t)
			c.Ranked[0].GrossOutput = robinhoodTokens(tc.whole)
			s := w.routedScenario(t, stubRouter{t: t, c: c}).Given(w.asMember())
			if tc.fits {
				s.When(scenario.Post(w.proposals(), buyAAPL), scenario.ExpectStatus(http.StatusCreated))
				return
			}
			s.When(scenario.Post(w.proposals(), buyAAPL)).
				Then(
					scenario.ExpectProblem(errs.CodeTradeTooLarge),
					scenario.ExpectEvents(events.TypeProposalCreated, 0),
				)
			if n := proposalRows(t, s, w.cabal); n != 0 {
				t.Fatalf("proposals rows = %d, want 0", n)
			}
		})
	}
}

func TestProposeTrade_aSolanaWinnerIsNeverRefusedForItsTokenAmount(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	c := solanaFirstComparison(t)
	c.Ranked[0].GrossOutput = robinhoodTokens(25)
	w.routedScenario(t, stubRouter{t: t, c: c}).Given(w.asMember()).
		When(scenario.Post(w.proposals(), buyAAPL), scenario.ExpectStatus(http.StatusCreated))
}

func TestPreview_advisesAnEVMBuyQuotedPastInt64(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	c := appleComparison(t)
	c.Ranked[0].GrossOutput = robinhoodTokens(10)
	w.routedScenario(t, stubRouter{t: t, c: c}).Given(w.asMember()).
		When(
			scenario.Get(w.preview("kind=buy&symbol=AAPLx&usdc_micros=5000000")),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("advisory_code", "trade_too_large"),
			scenario.ExpectJSON("advisory_message", errs.Message(errs.CodeTradeTooLarge)),
		)
}
