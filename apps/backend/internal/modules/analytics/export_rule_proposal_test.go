package analytics_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestAnalyticsExport_RuleProposal(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		event func(s scene) events.Event
		want  func(s scene) fakes.PostHogCapture
	}{
		"created": {
			event: func(s scene) events.Event {
				return events.ProposalCreated{
					V:          1,
					ProposalID: s.proposal,
					CabalID:    s.cabal,
					ProposerID: s.proposer,
					Kind:       "rule",
				}
			},
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "proposal_created", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"kind": "rule"}),
				}
			},
		},
		"passed": {
			event: func(s scene) events.Event {
				return events.ProposalPassed{
					V:          1,
					ProposalID: s.proposal,
					CabalID:    s.cabal,
					ProposerID: s.proposer,
					Kind:       "rule",
				}
			},
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "proposal_passed", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"kind": "rule"}),
				}
			},
		},
		"failed": {
			event: func(s scene) events.Event {
				return events.ProposalFailed{V: 1, ProposalID: s.proposal, CabalID: s.cabal, Kind: "rule"}
			},
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "proposal_failed", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"kind": "rule"}),
				}
			},
		},
		"expired": {
			event: func(s scene) events.Event {
				return events.ProposalExpired{V: 1, ProposalID: s.proposal, CabalID: s.cabal, Kind: "rule"}
			},
			want: func(s scene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "proposal_expired", DistinctID: s.proposer.String(),
					Properties: s.props(map[string]any{"kind": "rule"}),
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
			a := e.appendEvent(t, userActor(s.proposer), ev)
			m := e.messageOf(ev.Type(), a)
			e.deliver(t, m)
			captures := e.fake.Captures()
			if m.outcome != bus.OutcomeAck || len(captures) != 1 {
				t.Fatalf("verdict %q with %d captures, want ack and one: %+v", m.outcome, len(captures), captures)
			}
			want := tt.want(s)
			want.APIKey, want.UUID, want.Timestamp = apiKey, a.id, a.created
			sameCapture(t, captures[0], want)
		})
	}
}
