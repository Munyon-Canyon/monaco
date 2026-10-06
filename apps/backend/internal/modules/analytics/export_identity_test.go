package analytics_test

import (
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const identityActor = "system:identity"

type identityScene struct {
	user    uuid.UUID
	created time.Time
}

func newIdentityScene(e *env) identityScene {
	return identityScene{user: e.ids.NewV7(), created: e.clock.Now().In(time.FixedZone("UTC+5", 5*60*60))}
}

func (s identityScene) createdAt() string { return s.created.UTC().Format(time.RFC3339Nano) }

func (s identityScene) signedUp() events.Event {
	return events.UserCreated{V: 1, UserID: s.user, LoginProvider: "sms", CreatedAt: s.created}
}

func (s identityScene) advanced() events.Event {
	return events.UserAuthStateChanged{
		V: 1, UserID: s.user, From: "CREATED", To: "AWAITING_PHONE", Cause: "onboarding", At: s.created,
	}
}

func TestAnalyticsExport_Identity(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		event func(s identityScene) events.Event
		want  func(s identityScene) fakes.PostHogCapture
	}{
		"user.created": {
			event: identityScene.signedUp,
			want: func(s identityScene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event:      "user_signed_up",
					DistinctID: s.user.String(),
					Properties: map[string]any{"login_provider": "sms"},
					Set: map[string]any{
						"login_provider": "sms",
						"created_at":     s.createdAt(),
						"auth_state":     "CREATED",
					},
				}
			},
		},
		"user.auth_state_changed": {
			event: identityScene.advanced,
			want: func(s identityScene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "auth_state_changed", DistinctID: s.user.String(),
					Properties: map[string]any{"from": "CREATED", "to": "AWAITING_PHONE", "cause": "onboarding"},
					Set:        map[string]any{"auth_state": "AWAITING_PHONE"},
				}
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, analytics.RegisterIdentityExports)
			s := newIdentityScene(e)
			ev := tt.event(s)
			a := e.appendEvent(t, identityActor, ev)
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

func TestAnalyticsExport_Identity_NoPII(t *testing.T) {
	t.Parallel()
	const email = "kai.reyes@example.com"
	e := newEnv(t, analytics.RegisterIdentityExports)
	s := newIdentityScene(e)
	_, err := e.pool.Exec(t.Context(), `INSERT INTO users
		(id, privy_user_id, email, login_provider, auth_state_changed_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'email', $4, $4, $4)`, s.user, "did:privy:"+s.user.String(), email, s.created)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []events.Event{s.signedUp(), s.advanced()} {
		e.deliver(t, e.messageOf(ev.Type(), e.appendEvent(t, identityActor, ev)))
	}
	captures := e.fake.Captures()
	if len(captures) != 2 {
		t.Fatalf("%d captures, want 2: %+v", len(captures), captures)
	}
	for _, c := range captures {
		got := analytics.Capture{Event: c.Event, DistinctID: c.DistinctID, Properties: c.Properties, Set: c.Set}
		if err := analytics.CheckNoPII(got); err != nil {
			t.Errorf("the %s capture leaks personal data: %v", c.Event, err)
		}
		if rendered := fmt.Sprintf("%+v", c); strings.Contains(rendered, email) {
			t.Errorf("the %s capture %s carries the email of the user row", c.Event, rendered)
		}
		rejectsPlantedEmail(t, got)
	}
}

func withEmailKey(m map[string]any) map[string]any {
	out := map[string]any{"email": "someone"}
	maps.Copy(out, m)
	return out
}

func rejectsPlantedEmail(t *testing.T, c analytics.Capture) {
	t.Helper()
	inProperties, inSet := c, c
	inProperties.Properties = withEmailKey(c.Properties)
	inSet.Set = withEmailKey(c.Set)
	for place, planted := range map[string]analytics.Capture{"properties": inProperties, "set": inSet} {
		if code := errs.CodeOf(analytics.CheckNoPII(planted)); code != errs.CodeAnalyticsPII {
			t.Errorf("an email key planted in the %s of the %s capture gave %q, want %q",
				place, c.Event, code, errs.CodeAnalyticsPII)
		}
	}
}
