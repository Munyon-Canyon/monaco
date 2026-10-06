package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type nudgeKind struct {
	awaiting    identity.AuthState
	title, body string
}

func nudgeKindOf(kind string) (nudgeKind, bool) {
	switch kind {
	case "add_phone":
		return nudgeKind{
			awaiting: identity.AuthAwaitingPhone,
			title:    "Find your friends on Monaco",
			body:     "Add your number to see who you know.",
		}, true
	case "link_x":
		return nudgeKind{
			awaiting: identity.AuthAwaitingSocials,
			title:    "Connect X",
			body:     "Link X to find people you already follow.",
		}, true
	}
	return nudgeKind{}, false
}

type Nudge struct{ Users Users }

func (Nudge) Name() string { return "nudge" }

func (k Nudge) Recipients(ctx context.Context, e events.UserNudgeDue) ([]ids.UserID, error) {
	kind, known := nudgeKindOf(e.Kind)
	if !known {
		return nil, nil
	}
	user := ids.UserIDFrom(e.UserID)
	cards, err := k.Users.UsersByID(ctx, []ids.UserID{user})
	if err != nil {
		return nil, err
	}
	card := cards[user]
	if card.Deleted || card.AccountStatus != identity.AccountActive || card.AuthState != kind.awaiting {
		return nil, nil
	}
	return []ids.UserID{user}, nil
}

func (k Nudge) Render(_ context.Context, e events.UserNudgeDue, to ids.UserID) (Message, error) {
	kind, _ := nudgeKindOf(e.Kind)
	return Message{
		Title:      kind.title,
		Body:       kind.body,
		Data:       map[string]string{"kind": k.Name(), "user_id": to.String()},
		CollapseID: "nudge-" + to.String(),
	}, nil
}
