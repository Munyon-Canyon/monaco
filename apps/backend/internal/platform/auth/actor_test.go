package auth_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func TestActor_roundTripsThroughContextAndSetsTheLogActor(t *testing.T) {
	t.Parallel()
	ctx := auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorAdmin, ID: "a1"})
	a, ok := auth.ActorFrom(ctx)
	if !ok || a != (auth.Actor{Kind: auth.ActorAdmin, ID: "a1"}) || a.Key() != "admin:a1" {
		t.Fatalf("ActorFrom = %+v, %v", a, ok)
	}
	if got := observability.ActorFrom(ctx); got != "admin:a1" {
		t.Fatalf("observability actor = %q, want admin:a1", got)
	}
	if _, ok := auth.ActorFrom(context.Background()); ok {
		t.Fatal("ActorFrom(empty) = ok, want none")
	}
}
