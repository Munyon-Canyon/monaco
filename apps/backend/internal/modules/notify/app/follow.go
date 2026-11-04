package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	newFollowerDailyCap = 3
	unnamedFollower     = "Someone"
	noHandleBody        = "View their profile."
	referralFollow      = "referral"
)

type NewFollower struct{ Users Users }

func (NewFollower) Name() string { return "new_follower" }

func (NewFollower) DailyCap() int { return newFollowerDailyCap }

func (NewFollower) Recipients(_ context.Context, e events.FollowCreated) ([]ids.UserID, error) {
	if e.Source == referralFollow {
		return nil, nil
	}
	return []ids.UserID{ids.UserIDFrom(e.FolloweeID)}, nil
}

func (k NewFollower) Render(ctx context.Context, e events.FollowCreated, _ ids.UserID) (Message, error) {
	follower := ids.UserIDFrom(e.FollowerID)
	cards, err := k.Users.UsersByID(ctx, []ids.UserID{follower})
	if err != nil {
		return Message{}, err
	}
	card := cards[follower]
	card.ID = follower
	name, body := unnamedFollower, noHandleBody
	if !card.Deleted && card.DisplayName != "" {
		name = card.DisplayName
	}
	if !card.Deleted && card.Handle != "" {
		body = "@" + card.Handle
	}
	return Message{
		Title:        name + " follows you now.",
		Body:         body,
		Data:         map[string]string{"kind": k.Name(), "user_id": follower.String()},
		CollapseID:   "follow-" + follower.String(),
		Presentation: ForPerson(card),
	}, nil
}
