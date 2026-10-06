package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Cabals interface {
	Cabal(ctx context.Context, id ids.CabalID) (cabal.View, error)
	Members(ctx context.Context, id ids.CabalID) ([]cabal.MemberView, error)
}

type CabalPaused struct{ Cabals Cabals }

func (CabalPaused) Name() string { return "cabal_paused" }

func (k CabalPaused) Recipients(ctx context.Context, e events.CabalPaused) ([]ids.UserID, error) {
	return memberIDs(ctx, k.Cabals, e.CabalID)
}

func (k CabalPaused) Render(ctx context.Context, e events.CabalPaused, _ ids.UserID) (Message, error) {
	return pauseNotice(ctx, k.Cabals, k.Name(), *e.CabalID, "%s is paused",
		"Trading is on hold while we check something. Your money is safe; we'll tell you when it resumes.")
}

type CabalResumed struct{ Cabals Cabals }

func (CabalResumed) Name() string { return "cabal_resumed" }

func (k CabalResumed) Recipients(ctx context.Context, e events.CabalResumed) ([]ids.UserID, error) {
	return memberIDs(ctx, k.Cabals, e.CabalID)
}

func (k CabalResumed) Render(ctx context.Context, e events.CabalResumed, _ ids.UserID) (Message, error) {
	return pauseNotice(ctx, k.Cabals, k.Name(), *e.CabalID, "%s is back", "Trading has resumed.")
}

func memberIDs(ctx context.Context, cabals Cabals, cabalID *uuid.UUID) ([]ids.UserID, error) {
	if cabalID == nil {
		return nil, nil
	}
	views, err := cabals.Members(ctx, ids.CabalIDFrom(*cabalID))
	if err != nil {
		return nil, err
	}
	users := make([]ids.UserID, len(views))
	for i, v := range views {
		users[i] = v.UserID
	}
	return users, nil
}

func pauseNotice(
	ctx context.Context, cabals Cabals, kind string, cabalID uuid.UUID, titleFormat, body string,
) (Message, error) {
	view, err := cabals.Cabal(ctx, ids.CabalIDFrom(cabalID))
	if err != nil {
		return Message{}, err
	}
	return Message{
		Title:      fmt.Sprintf(titleFormat, view.Name),
		Body:       body,
		Data:       map[string]string{"kind": kind, "cabal_id": cabalID.String()},
		CollapseID: "pause-" + cabalID.String(),
	}, nil
}
