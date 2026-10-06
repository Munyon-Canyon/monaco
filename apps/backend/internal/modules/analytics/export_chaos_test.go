//go:build faultpoints

package analytics_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

const lookupFailures = 2

type chaosCase struct {
	event events.Event
	name  string
}

func proposalChaosCases(s scene) []chaosCase {
	return []chaosCase{
		{
			events.ProposalPassed{
				V: 1, ProposalID: s.proposal, CabalID: s.cabal, ProposerID: s.proposer, Kind: "buy", Symbol: "AAPLx",
				USDCMicros: money.MicrosFromUint64(25_000_000),
			},
			"proposal_passed",
		},
		{events.ProposalFailed{V: 1, ProposalID: s.proposal, CabalID: s.cabal}, "proposal_failed"},
		{events.ProposalExpired{V: 1, ProposalID: s.proposal, CabalID: s.cabal}, "proposal_expired"},
		{
			events.TradeConfirmed{
				V: 1, SwapID: s.swap, CabalID: s.cabal, Source: s.source(), SourceBatchSize: 1, Action: "buy",
				Symbol: "AAPLx", InMint: usdcMint, InAmount: 25_000_000, OutMint: aaplxMint, OutAmount: 105_000_000,
				USDCMicros: money.MicrosFromUint64(25_000_000),
			},
			"trade_executed",
		},
		{
			events.TradeBlocked{
				V: 1, CabalID: s.cabal, Source: s.source(), SourceBatchSize: 1, Action: "buy", Symbol: "AAPLx",
				Code: errs.CodeInsufficientFunds, Have: 24_999_999, Need: 25_000_000,
			},
			"trade_blocked",
		},
		{
			events.TradeFailed{
				V: 1, SwapID: s.swap, CabalID: s.cabal, Source: s.source(), SourceBatchSize: 1, Action: "buy",
				Symbol: "AAPLx", InMint: usdcMint, InAmount: 25_000_000, FailureCode: "jupiter_failed",
			},
			"trade_failed",
		},
	}
}

type chaosWorld struct {
	e     *env
	s     scene
	names map[uuid.UUID]string
	msgs  []*chaos.Msg
}

func newChaosWorld(t *testing.T) chaosWorld {
	t.Helper()
	g := &governanceFake{}
	for range lookupFailures {
		g.FailOnce("Proposer", errs.New(errs.CodeDBUnavailable, "governanceFake.Proposer"))
	}
	e := newEnv(t, func(r *analytics.Registry) { analytics.RegisterProposalExports(r, g) })
	w := chaosWorld{e: e, s: newScene(e), names: map[uuid.UUID]string{}}
	g.opened(ids.ProposalIDFrom(w.s.proposal), ids.UserIDFrom(w.s.proposer))
	for _, c := range proposalChaosCases(w.s) {
		a := e.appendEvent(t, userActor(w.s.voter), c.event)
		w.names[a.id] = c.name
		w.msgs = append(w.msgs, chaos.NewMsg(e.bus.Conn, c.event.Type(), ids.EventIDFrom(a.id), a.payload))
	}
	return w
}

func (w chaosWorld) problems(res chaos.Result) []string {
	var out []string
	captures := w.e.fake.Captures()
	if len(captures) != len(w.names) {
		out = append(out, fmt.Sprintf("%d distinct captures, want %d", len(captures), len(w.names)))
	}
	if terms := res.Terms(); len(terms) > 0 {
		out = append(out, fmt.Sprintf("terms %v", terms))
	}
	for _, c := range captures {
		if want, ok := w.names[c.UUID]; !ok || c.Event != want || c.DistinctID != w.s.proposer.String() {
			out = append(out, fmt.Sprintf("capture %+v, want one of %v for the proposer %s", c, w.names, w.s.proposer))
		}
	}
	return out
}

func TestAnalytics_ProposalExports_ExportEachEventOnceUnderChaos(t *testing.T) {
	t.Parallel()
	for _, seed := range chaos.Seeds(t) {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			t.Parallel()
			w := newChaosWorld(t)
			res := chaos.Dispatch(w.e.ctx(t), t, seed, w.e.reg, w.e.consumer, w.msgs)
			if problems := w.problems(res); len(problems) > 0 {
				t.Fatalf("seed %d: %s\n%v", seed, strings.Join(problems, "\n"), res.Trace)
			}
		})
	}
}
