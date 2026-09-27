package observability

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestActorFrom_readsWhatWithActorStoredAndIsEmptyOtherwise(t *testing.T) {
	t.Parallel()
	if got := ActorFrom(t.Context()); got != "" {
		t.Fatalf("ActorFrom(empty ctx) = %q, want \"\"", got)
	}
	ctx := WithActor(WithRequestID(t.Context(), "req-1"), "user:u1")
	if got := ActorFrom(ctx); got != "user:u1" {
		t.Fatalf("ActorFrom = %q, want user:u1", got)
	}
}

func TestEventIDFrom_readsWhatWithEventIDStoredAndIsEmptyOtherwise(t *testing.T) {
	t.Parallel()
	if got := EventIDFrom(t.Context()); got != "" {
		t.Fatalf("EventIDFrom(empty ctx) = %q, want \"\"", got)
	}
	id := ids.EventIDFrom(ids.Real{}.NewV7())
	if got := EventIDFrom(WithEventID(t.Context(), id)); got != id.String() {
		t.Fatalf("EventIDFrom = %q, want %s", got, id)
	}
}
