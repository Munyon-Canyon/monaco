package auth

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type ActorKind string

const (
	ActorUser    ActorKind = "user"
	ActorAgent   ActorKind = "agent"
	ActorAdmin   ActorKind = "admin"
	ActorSystem  ActorKind = "system"
	ActorService ActorKind = "service"
)

type Standing string

const (
	StandingActive    Standing = "active"
	StandingSuspended Standing = "suspended"
	StandingBanned    Standing = "banned"
	StandingDeleted   Standing = "deleted"
)

type Actor struct {
	Kind     ActorKind
	ID       string
	Role     string
	Standing Standing
}

func (a Actor) Key() string { return string(a.Kind) + ":" + a.ID }

type actorKey struct{}

func WithActor(ctx context.Context, a Actor) context.Context {
	ctx = observability.WithActor(ctx, a.Key())
	return context.WithValue(ctx, actorKey{}, a)
}

func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}
