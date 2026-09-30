package analytics_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters/testdata"
)

type widgetBumped struct {
	V  int       `json:"v"`
	ID uuid.UUID `json:"id"`
}

func (widgetBumped) Type() events.Type { return "widget.bumped" }

func (widgetBumped) AggregateType() string { return "widget" }

func (e widgetBumped) AggregateID() uuid.UUID { return e.ID }

func widget(context.Context, widgetBumped) (analytics.Capture, bool, error) {
	return analytics.Capture{Event: "widget_bumped"}, true, nil
}

func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		if got := recover(); got != want {
			t.Fatalf("panic = %v, want %q", got, want)
		}
	}()
	fn()
}

func TestExport_panicsWhenTheSubjectIsNotTheEventsOwn(t *testing.T) {
	t.Parallel()
	mustPanic(t, "analytics: cabal.funded exported for widget.bumped", func() {
		analytics.Export(analytics.NewRegistry(), "cabal.funded", widget)
	})
}

func TestExport_panicsOnASecondExportOfOneSubject(t *testing.T) {
	t.Parallel()
	r := analytics.NewRegistry()
	analytics.Export(r, "widget.bumped", widget)
	mustPanic(t, "analytics: widget.bumped exported twice", func() {
		analytics.Export(r, "widget.bumped", widget)
	})
}

func TestRegistry_Consumer_isNotBuiltWithoutExports(t *testing.T) {
	t.Parallel()
	if c, ok := analytics.NewRegistry().Consumer(nil); ok {
		t.Fatalf("empty registry built consumer %+v, want none: a consumer with no handlers has no subject filter", c)
	}
}

func TestRegistry_Consumer_holdsOneDurableWithOneHandlerPerExportedSubject(t *testing.T) {
	t.Parallel()
	r := analytics.NewRegistry()
	testdata.Register(r)
	analytics.Export(r, "widget.bumped", widget)
	c, ok := r.Consumer(nil)
	if !ok || c.Durable != "analytics" || len(c.Handlers) != 2 {
		t.Fatalf("Consumer = %+v, %v, want durable analytics with two handlers", c, ok)
	}
	want := []struct {
		name string
		typ  events.Type
	}{
		{"analytics.posthog.system.pinged", events.TypeSystemPinged},
		{"analytics.posthog.widget.bumped", "widget.bumped"},
	}
	for i, h := range c.Handlers {
		if h.Name != want[i].name || h.Type() != want[i].typ {
			t.Errorf("handler %d = %s on %s, want %s on %s", i, h.Name, h.Type(), want[i].name, want[i].typ)
		}
	}
}
