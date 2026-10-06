package analytics_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type moneyScene struct{ user, cabal, id uuid.UUID }

func newMoneyScene(e *env) moneyScene {
	return moneyScene{user: e.ids.NewV7(), cabal: e.ids.NewV7(), id: e.ids.NewV7()}
}

type moneyCase struct {
	actor string
	event func(s moneyScene) events.Event
	want  func(s moneyScene) fakes.PostHogCapture
}

func exportsEachCaseOnce(t *testing.T, register func(*analytics.Registry), tests map[string]moneyCase) {
	t.Helper()
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, register)
			s := newMoneyScene(e)
			ev := tt.event(s)
			a := e.appendEvent(t, tt.actor, ev)
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

func leaksNothing(t *testing.T, got fakes.PostHogCapture, payload []byte) {
	t.Helper()
	if err := analytics.CheckNoPII(analytics.Capture{
		Event: got.Event, DistinctID: got.DistinctID, Properties: got.Properties, Set: got.Set,
	}); err != nil {
		t.Errorf("the exported capture leaks personal data: %v", err)
	}
	rendered := fmt.Sprintf("%+v", got)
	for _, secret := range keysIn(payload) {
		if strings.Contains(rendered, secret) {
			t.Errorf("the %s capture %s carries %s from its payload", got.Event, rendered, secret)
		}
	}
}

func keysIn(payload []byte) []string {
	var tree any
	if err := json.Unmarshal(payload, &tree); err != nil {
		return nil
	}
	var found []string
	var walk func(node any)
	walk = func(node any) {
		switch node := node.(type) {
		case map[string]any:
			for _, v := range node {
				walk(v)
			}
		case []any:
			for _, v := range node {
				walk(v)
			}
		case string:
			if raw, ok := chain.DecodeBase58(node); ok && (len(raw) == 32 || len(raw) == 64) {
				found = append(found, node)
			}
		}
	}
	walk(tree)
	return found
}
