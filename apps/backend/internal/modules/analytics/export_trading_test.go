package analytics_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const usdcMint = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"

func (s scene) source() events.TradeSource {
	return events.TradeSource{Kind: "proposal", ID: s.proposal}
}

func TestAnalyticsExport_Trading(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		event func(s scene, swap uuid.UUID) events.Event
		want  func(s scene) fakes.PostHogCapture
	}{
		"trade.confirmed buy": {
			event: func(s scene, swap uuid.UUID) events.Event {
				return events.TradeConfirmed{
					V: 1, SwapID: swap, CabalID: s.cabal, Source: s.source(), SourceBatchSize: 1, Action: "buy",
					Symbol: "AAPLx", InMint: usdcMint, InAmount: 25_500_001, OutMint: aaplxMint, OutAmount: 105_000_000,
					USDCMicros: money.MicrosFromUint64(25_500_001),
				}
			},
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "trade_executed", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"action": "buy", "symbol": "AAPLx", "usdc_amount": 25.500001}),
				}
			},
		},
		"trade.confirmed sell": {
			event: func(s scene, swap uuid.UUID) events.Event {
				return events.TradeConfirmed{
					V: 1, SwapID: swap, CabalID: s.cabal, Source: s.source(), SourceBatchSize: 1, Action: "sell",
					Symbol: "AAPLx", InMint: aaplxMint, InAmount: 4_000_000, OutMint: usdcMint, OutAmount: 24_750_000,
					USDCMicros: money.MicrosFromUint64(24_750_000),
				}
			},
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "trade_executed", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"action": "sell", "symbol": "AAPLx", "usdc_amount": 24.75}),
				}
			},
		},
		"trade.blocked": {
			event: func(s scene, _ uuid.UUID) events.Event {
				return events.TradeBlocked{
					V: 1, CabalID: s.cabal, Source: s.source(), SourceBatchSize: 1, Action: "buy", Symbol: "AAPLx",
					Code: errs.CodeInsufficientFunds, Have: 24_999_999, Need: 25_000_000,
				}
			},
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "trade_blocked", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"code": "insufficient_funds"}),
				}
			},
		},
		"trade.failed": {
			event: func(s scene, swap uuid.UUID) events.Event {
				return events.TradeFailed{
					V: 1, SwapID: swap, CabalID: s.cabal, Source: s.source(), SourceBatchSize: 1, Action: "buy",
					Symbol: "AAPLx", InMint: usdcMint, InAmount: 25_000_000, FailureCode: "jupiter_failed",
					JupiterCode: "6001",
				}
			},
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "trade_failed", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"failure_code": "jupiter_failed"}),
				}
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := &governanceFake{}
			e := newEnv(t, func(r *analytics.Registry) { analytics.RegisterProposalExports(r, g) })
			s := newScene(e)
			g.opened(ids.ProposalIDFrom(s.proposal), ids.UserIDFrom(s.proposer))
			ev := tt.event(s, e.ids.NewV7())
			a := e.appendEvent(t, "system:trading.engine", ev)
			m := e.messageOf(ev.Type(), a)
			e.deliver(t, m)
			captures := e.fake.Captures()
			if m.outcome != bus.OutcomeAck || len(captures) != 1 {
				t.Fatalf("verdict %q with %d captures, want ack and one: %+v", m.outcome, len(captures), captures)
			}
			want := tt.want(s)
			want.APIKey, want.UUID, want.Timestamp = apiKey, a.id, a.created
			sameCapture(t, captures[0], want)
			got := captures[0]
			if err := analytics.CheckNoPII(analytics.Capture{
				Event: got.Event, DistinctID: got.DistinctID, Properties: got.Properties,
			}); err != nil {
				t.Errorf("the exported capture leaks personal data: %v", err)
			}
			if n := e.deliveriesOf(t, "analytics.posthog."+string(ev.Type())); n != 1 {
				t.Errorf("%d delivery rows for %s, want 1", n, ev.Type())
			}
		})
	}
}

func TestAnalyticsExport_Trading_CashOutSourceSkipped(t *testing.T) {
	t.Parallel()
	g := &governanceFake{}
	g.Fail("Proposer", errs.New(errs.CodeInternal, "governanceFake.Proposer"))
	e := newEnv(t, func(r *analytics.Registry) { analytics.RegisterProposalExports(r, g) })
	s := newScene(e)
	cashOut := events.TradeSource{Kind: "cashout", ID: s.proposal}
	for _, ev := range []events.Event{
		events.TradeConfirmed{
			V: 1, SwapID: e.ids.NewV7(), CabalID: s.cabal, Source: cashOut, SourceBatchSize: 1, Action: "sell",
			Symbol: "AAPLx", InMint: aaplxMint, InAmount: 4_000_000, OutMint: usdcMint, OutAmount: 24_750_000,
			USDCMicros: money.MicrosFromUint64(24_750_000),
		},
		events.TradeBlocked{
			V: 1, CabalID: s.cabal, Source: cashOut, SourceBatchSize: 1, Action: "sell", Symbol: "AAPLx",
			Code: errs.CodeCabalPaused,
		},
		events.TradeFailed{
			V: 1, SwapID: e.ids.NewV7(), CabalID: s.cabal, Source: cashOut, SourceBatchSize: 1, Action: "sell",
			Symbol: "AAPLx", InMint: aaplxMint, InAmount: 4_000_000, FailureCode: "jupiter_failed",
		},
	} {
		m := e.messageOf(ev.Type(), e.appendEvent(t, "system:trading.engine", ev))
		e.deliver(t, m)
		if m.outcome != bus.OutcomeAck {
			t.Errorf("%s verdict %q, want ack: a cash out sale has its own event and the proposer lookup must not run",
				ev.Type(), m.outcome)
		}
	}
	skipped := logged(t, e, "analytics.capture_skipped")
	if e.fake.Received() != 0 || len(skipped) != 3 || skipped[0]["reason"] != "mapper" {
		t.Fatalf("%d captures received and %d capture_skipped lines %v, want none received and three for the mapper",
			e.fake.Received(), len(skipped), skipped)
	}
}
