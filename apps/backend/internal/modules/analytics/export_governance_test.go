package analytics_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const aaplxMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

type scene struct {
	proposal, cabal, proposer, voter uuid.UUID
}

func newScene(e *env) scene {
	return scene{proposal: e.ids.NewV7(), cabal: e.ids.NewV7(), proposer: e.ids.NewV7(), voter: e.ids.NewV7()}
}

func (s scene) props(extra map[string]any) map[string]any {
	props := map[string]any{"proposal_id": s.proposal.String(), "cabal_id": s.cabal.String()}
	for k, v := range extra {
		props[k] = v
	}
	return props
}

func TestAnalyticsExport_Governance(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		event func(s scene) events.Event
		actor func(s scene) string
		want  func(s scene) fakes.PostHogCapture
	}{
		"proposal.passed": {
			event: func(s scene) events.Event {
				return events.ProposalPassed{
					V: 1, ProposalID: s.proposal, CabalID: s.cabal, ProposerID: s.proposer, Kind: "buy",
					Symbol: "AAPLx", Mint: aaplxMint, USDCMicros: money.MicrosFromUint64(25_500_001),
					QuoteOutAmount: 105_000_000,
				}
			},
			actor: func(s scene) string { return userActor(s.voter) },
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "proposal_passed", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"kind": "buy", "symbol": "AAPLx", "usdc_amount": 25.500001}),
				}
			},
		},
		"proposal.passed sell": {
			event: func(s scene) events.Event {
				return events.ProposalPassed{
					V: 1, ProposalID: s.proposal, CabalID: s.cabal, ProposerID: s.proposer, Kind: "sell",
					Symbol: "AAPLx", Mint: aaplxMint, TokenAmount: 4_000_000, QuoteOutAmount: 25_000_000,
				}
			},
			actor: func(s scene) string { return userActor(s.voter) },
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "proposal_passed", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"kind": "sell", "symbol": "AAPLx", "usdc_amount": 0.0}),
				}
			},
		},
		"proposal.failed": {
			event: func(s scene) events.Event {
				return events.ProposalFailed{V: 1, ProposalID: s.proposal, CabalID: s.cabal}
			},
			actor: func(s scene) string { return userActor(s.voter) },
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event:      "proposal_failed",
					DistinctID: s.proposer.String(),
					Properties: s.props(nil),
				}
			},
		},
		"proposal.expired": {
			event: func(s scene) events.Event {
				return events.ProposalExpired{V: 1, ProposalID: s.proposal, CabalID: s.cabal}
			},
			actor: func(scene) string { return "system:poller.expiry" },
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event:      "proposal_expired",
					DistinctID: s.proposer.String(),
					Properties: s.props(nil),
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
			ev := tt.event(s)
			a := e.appendEvent(t, tt.actor(s), ev)
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

func TestAnalyticsExport_ProposerLookupUnavailable_Naks(t *testing.T) {
	t.Parallel()
	g := &governanceFake{}
	g.FailOnce("Proposer", errs.New(errs.CodeDBUnavailable, "governanceFake.Proposer"))
	e := newEnv(t, func(r *analytics.Registry) { analytics.RegisterProposalExports(r, g) })
	s := newScene(e)
	g.opened(ids.ProposalIDFrom(s.proposal), ids.UserIDFrom(s.proposer))
	ev := events.ProposalFailed{V: 1, ProposalID: s.proposal, CabalID: s.cabal}
	a := e.appendEvent(t, userActor(s.voter), ev)
	m := e.messageOf(ev.Type(), a)
	type attempt struct {
		outcome bus.Outcome
		delay   time.Duration
	}
	got := make([]attempt, 0, 2)
	for range 2 {
		e.deliver(t, m)
		got = append(got, attempt{m.outcome, m.delay})
	}
	want := []attempt{{bus.OutcomeNak, bus.NakSchedule()[0]}, {bus.OutcomeAck, 0}}
	captures := e.fake.Captures()
	if !reflect.DeepEqual(got, want) || len(captures) != 1 || e.fake.Received() != 1 || len(e.deadLetters(t)) != 0 ||
		e.deliveriesOf(t, "analytics.posthog.proposal.failed") != 1 {
		t.Fatalf("attempts %+v with %d captures, %d received, %d dead letters; want %+v, one, one, none",
			got, len(captures), e.fake.Received(), len(e.deadLetters(t)), want)
	}
	if captures[0].UUID != a.id || captures[0].DistinctID != s.proposer.String() {
		t.Errorf("capture = %+v, want uuid %s for proposer %s", captures[0], a.id, s.proposer)
	}
}
