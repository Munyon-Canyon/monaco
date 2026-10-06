package analytics_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const referralActor = "system:referrals"

type referralScene struct{ referral, referrer, referee, cabal uuid.UUID }

func newReferralScene(e *env) referralScene {
	return referralScene{
		referral: e.ids.NewV7(), referrer: e.ids.NewV7(), referee: e.ids.NewV7(), cabal: e.ids.NewV7(),
	}
}

func (s referralScene) attributed(source string) events.Event {
	return events.ReferralAttributed{
		V: 1, ReferralID: s.referral, Referrer: s.referrer, Referee: s.referee, CodeKind: "handle", Source: source,
	}
}

func (s referralScene) qualified() events.Event {
	return events.ReferralQualified{
		V: 1, ReferralID: s.referral, Referrer: s.referrer, Referee: s.referee, CabalID: s.cabal,
		AmountMicros: money.MicrosFromUint64(10_000_000),
	}
}

func TestAnalyticsExport_Referrals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		event func(s referralScene) events.Event
		want  func(s referralScene) fakes.PostHogCapture
	}{
		"referral.attributed": {
			event: func(s referralScene) events.Event { return s.attributed("universal_link") },
			want: func(s referralScene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "referral_attributed", DistinctID: s.referee.String(),
					Properties: map[string]any{"source": "universal_link", "referrer_id": s.referrer.String()},
				}
			},
		},
		"referral.qualified": {
			event: referralScene.qualified,
			want: func(s referralScene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "referral_qualified", DistinctID: s.referee.String(),
					Properties: map[string]any{"referrer_id": s.referrer.String()},
				}
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, analytics.RegisterReferralExports)
			s := newReferralScene(e)
			ev := tt.event(s)
			a := e.appendEvent(t, referralActor, ev)
			m := e.messageOf(ev.Type(), a)
			e.deliver(t, m)
			captures := e.fake.Captures()
			if m.outcome != bus.OutcomeAck || len(captures) != 1 {
				t.Fatalf("verdict %q with %d captures, want ack and one: %+v", m.outcome, len(captures), captures)
			}
			want := tt.want(s)
			want.APIKey, want.UUID, want.Timestamp = apiKey, a.id, a.created
			sameCapture(t, captures[0], want)
			leaksNothing(t, captures[0], a.payload)
			rejectsPlantedEmail(t, analytics.Capture{
				Event: captures[0].Event, DistinctID: captures[0].DistinctID,
				Properties: captures[0].Properties, Set: captures[0].Set,
			})
			if n := e.deliveriesOf(t, "analytics.posthog."+string(ev.Type())); n != 1 {
				t.Errorf("%d delivery rows for %s, want 1", n, ev.Type())
			}
		})
	}
}
