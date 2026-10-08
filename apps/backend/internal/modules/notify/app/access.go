package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	accessDirectionRequest = "request"
	accessDecisionApproved = "approved"
)

type CabalAccessRequested struct {
	Cabals Cabals
	Users  Users
}

func (CabalAccessRequested) Name() string { return "cabal_access_requested" }

func (k CabalAccessRequested) Recipients(ctx context.Context, e events.CabalAccessRequested) ([]ids.UserID, error) {
	if e.Direction != accessDirectionRequest {
		return nil, nil
	}
	view, err := k.Cabals.Cabal(ctx, ids.CabalIDFrom(e.CabalID))
	if err != nil {
		return nil, err
	}
	return []ids.UserID{view.CreatorID}, nil
}

func (k CabalAccessRequested) Render(
	ctx context.Context, e events.CabalAccessRequested, _ ids.UserID,
) (Message, error) {
	view, err := k.Cabals.Cabal(ctx, ids.CabalIDFrom(e.CabalID))
	if err != nil {
		return Message{}, err
	}
	requester, err := proposerName(ctx, k.Users, ids.UserIDFrom(e.UserID))
	if err != nil {
		return Message{}, err
	}
	return Message{
		Title:      "New request to join",
		Body:       requester + " wants to join " + view.Name,
		Data:       map[string]string{"kind": k.Name(), "cabal_id": e.CabalID.String()},
		CollapseID: "access-" + e.RequestID.String(),
	}, nil
}

type CabalAccessApproved struct{ Cabals Cabals }

func (CabalAccessApproved) Name() string { return "cabal_access_approved" }

func (CabalAccessApproved) Recipients(_ context.Context, e events.CabalAccessDecided) ([]ids.UserID, error) {
	if e.Direction != accessDirectionRequest || e.Decision != accessDecisionApproved {
		return nil, nil
	}
	return []ids.UserID{ids.UserIDFrom(e.UserID)}, nil
}

func (k CabalAccessApproved) Render(ctx context.Context, e events.CabalAccessDecided, _ ids.UserID) (Message, error) {
	view, err := k.Cabals.Cabal(ctx, ids.CabalIDFrom(e.CabalID))
	if err != nil {
		return Message{}, err
	}
	return Message{
		Title:      "You're in " + view.Name,
		Body:       "Your request was approved. Say hi.",
		Data:       map[string]string{"kind": k.Name(), "cabal_id": e.CabalID.String()},
		CollapseID: "access-" + e.RequestID.String(),
	}, nil
}
