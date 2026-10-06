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
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
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
	e        *env
	distinct string
	names    map[uuid.UUID]string
	msgs     []*chaos.Msg
}

func newChaosWorld(t *testing.T) chaosWorld {
	t.Helper()
	g := &governanceFake{}
	for range lookupFailures {
		g.FailOnce("Proposer", errs.New(errs.CodeDBUnavailable, "governanceFake.Proposer"))
	}
	e := newEnv(t, func(r *analytics.Registry) { analytics.RegisterProposalExports(r, g) })
	s := newScene(e)
	g.opened(ids.ProposalIDFrom(s.proposal), ids.UserIDFrom(s.proposer))
	return appendedWorld(t, e, userActor(s.voter), s.proposer, proposalChaosCases(s))
}

func moneyChaosCases(s moneyScene) []chaosCase {
	signature := keyOf(9, 64)
	return []chaosCase{
		{depositOf(s, keyOf(7, 32), signature), "deposit_credited"},
		{
			events.Funded{
				V: 1, TransferID: s.id, CabalID: s.cabal, UserID: s.user,
				AmountMicros: money.MicrosFromUint64(5_250_001), ShareUnits: money.SharesUnitsFromUint64(5_000_000),
				TxSignature: chain.Signature(signature),
			},
			"cabal_funded",
		},
		{
			events.CashOutCompleted{
				V: 1, JobID: s.id, CabalID: s.cabal, UserID: s.user, ShareUnits: 2_000_000,
				PayoutMicros: money.MicrosFromUint64(1_234_567), Signature: chain.Signature(signature),
			},
			"cash_out_completed",
		},
		{
			events.CashOutPartial{
				V: 1, JobID: s.id, CabalID: s.cabal, UserID: s.user, ShareUnitsBurned: 1_500_000,
				ShareUnitsReturned: 500_000, PayoutMicros: money.MicrosFromUint64(750_000),
				Signature: chain.Signature(signature),
			},
			"cash_out_partial",
		},
		{
			events.CashOutFailed{
				V: 1, JobID: s.id, CabalID: s.cabal, UserID: s.user, ShareUnits: 2_000_000,
				Code: string(errs.CodePayoutFailed),
			},
			"cash_out_failed",
		},
	}
}

func newMoneyChaosWorld(t *testing.T) chaosWorld {
	t.Helper()
	e := newEnv(t, func(r *analytics.Registry) {
		analytics.RegisterFundingExports(r)
		analytics.RegisterTreasuryExports(r)
	})
	s := newMoneyScene(e)
	return appendedWorld(t, e, "system:chaos", s.user, moneyChaosCases(s))
}

func appendedWorld(t *testing.T, e *env, actor string, distinct uuid.UUID, cases []chaosCase) chaosWorld {
	t.Helper()
	w := chaosWorld{e: e, distinct: distinct.String(), names: map[uuid.UUID]string{}}
	for _, c := range cases {
		a := e.appendEvent(t, actor, c.event)
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
		if want, ok := w.names[c.UUID]; !ok || c.Event != want || c.DistinctID != w.distinct {
			out = append(out, fmt.Sprintf("capture %+v, want one of %v for %s", c, w.names, w.distinct))
		}
	}
	return out
}

func exportsEachEventOnceUnderChaos(t *testing.T, build func(*testing.T) chaosWorld) {
	t.Helper()
	for _, seed := range chaos.Seeds(t) {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			t.Parallel()
			w := build(t)
			res := chaos.Dispatch(w.e.ctx(t), t, seed, w.e.reg, w.e.consumer, w.msgs)
			if problems := w.problems(res); len(problems) > 0 {
				t.Fatalf("seed %d: %s\n%v", seed, strings.Join(problems, "\n"), res.Trace)
			}
		})
	}
}

func TestAnalytics_ProposalExports_ExportEachEventOnceUnderChaos(t *testing.T) {
	t.Parallel()
	exportsEachEventOnceUnderChaos(t, newChaosWorld)
}

func TestAnalytics_MoneyExports_ExportEachEventOnceUnderChaos(t *testing.T) {
	t.Parallel()
	exportsEachEventOnceUnderChaos(t, newMoneyChaosWorld)
}
