package analytics_test

import (
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type cabalScene struct {
	user, cabal, request uuid.UUID
	members              *fakes.Cabal
}

func newCabalScene(t *testing.T, memberships int) cabalScene {
	t.Helper()
	g := testkit.NewIDs(testkit.RandSeed(t))
	s := cabalScene{user: g.NewV7(), cabal: g.NewV7(), request: g.NewV7()}
	rows := make([]fakes.CabalMember, memberships)
	for i := range rows {
		rows[i] = fakes.CabalMember{
			CabalID: ids.CabalIDFrom(g.NewV7()), Member: cabal.MemberView{UserID: ids.UserIDFrom(s.user)},
		}
	}
	s.members = fakes.NewCabal(nil, rows)
	return s
}

func (s cabalScene) registerExports(r *analytics.Registry) {
	analytics.RegisterCabalExports(r, s.members)
}

func (s cabalScene) capture(event string, count any, extra map[string]any) fakes.PostHogCapture {
	props := map[string]any{"cabal_id": s.cabal.String()}
	maps.Copy(props, extra)
	return fakes.PostHogCapture{
		Event: event, DistinctID: s.user.String(), Properties: props, Set: map[string]any{"cabal_count": count},
	}
}

func TestAnalyticsExport_Cabal(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		memberships int
		event       func(s cabalScene) events.Event
		want        func(s cabalScene) fakes.PostHogCapture
	}{
		"cabal.created": {
			memberships: 1,
			event: func(s cabalScene) events.Event {
				return events.CabalCreated{
					V: 1, CabalID: s.cabal, CreatorID: s.user, Name: "Friends pot", JoinMode: "request",
					VoterMode: "list", Threshold: "unanimous", ProposalExpirySeconds: 3600, SlippageBps: 50,
					TreasuryAddress: chain.SolanaAddress(keyOf(3, 32)),
				}
			},
			want: func(s cabalScene) fakes.PostHogCapture {
				return s.capture("cabal_created", 1.0, map[string]any{"join_mode": "request", "threshold": "unanimous"})
			},
		},
		"cabal.member_joined": {
			memberships: 3,
			event: func(s cabalScene) events.Event {
				return events.CabalMemberJoined{
					V: 1, CabalID: s.cabal, UserID: s.user, Role: "member", Via: "invite", RequestID: s.request,
				}
			},
			want: func(s cabalScene) fakes.PostHogCapture {
				return s.capture("cabal_joined", 3.0, map[string]any{"via": "invite"})
			},
		},
		"cabal.member_joined by the creator": {
			memberships: 1,
			event: func(s cabalScene) events.Event {
				return events.CabalMemberJoined{V: 1, CabalID: s.cabal, UserID: s.user, Role: "creator", Via: "create"}
			},
			want: func(s cabalScene) fakes.PostHogCapture {
				return s.capture("cabal_joined", 1.0, map[string]any{"via": "create"})
			},
		},
		"cabal.member_left": {
			memberships: 0,
			event: func(s cabalScene) events.Event {
				return events.CabalMemberLeft{V: 1, CabalID: s.cabal, UserID: s.user, WasVoter: true}
			},
			want: func(s cabalScene) fakes.PostHogCapture { return s.capture("cabal_left", 0.0, nil) },
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newCabalScene(t, tt.memberships)
			e := newEnv(t, s.registerExports)
			ev := tt.event(s)
			a := e.appendEvent(t, userActor(s.user), ev)
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
			if n := e.deliveriesOf(t, "analytics.posthog."+string(ev.Type())); n != 1 {
				t.Errorf("%d delivery rows for %s, want 1", n, ev.Type())
			}
		})
	}
}

func TestAnalyticsExport_CabalCount_PortUnavailable_Naks(t *testing.T) {
	t.Parallel()
	s := newCabalScene(t, 2)
	s.members.FailOnce("CabalsOf", errs.New(errs.CodeInternal, "fakes.Cabal.CabalsOf"))
	e := newEnv(t, s.registerExports)
	ev := events.CabalMemberLeft{V: 1, CabalID: s.cabal, UserID: s.user}
	a := e.appendEvent(t, userActor(s.user), ev)
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
		e.deliveriesOf(t, "analytics.posthog.cabal.member_left") != 1 {
		t.Fatalf("attempts %+v with %d captures, %d received, %d dead letters; want %+v, one, one, none",
			got, len(captures), e.fake.Received(), len(e.deadLetters(t)), want)
	}
	wantCapture := s.capture("cabal_left", 2.0, nil)
	wantCapture.APIKey, wantCapture.UUID, wantCapture.Timestamp = apiKey, a.id, a.created
	sameCapture(t, captures[0], wantCapture)
}
