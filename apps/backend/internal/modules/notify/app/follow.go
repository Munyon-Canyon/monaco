package app

import (
	"context"
	"fmt"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	newFollowerDailyCap = 3
	unnamedFollower     = "Someone"
)

type NewFollower struct{ Users Users }

func (NewFollower) Name() string { return "new_follower" }

func (NewFollower) DailyCap() int { return newFollowerDailyCap }

func (NewFollower) Recipients(_ context.Context, e events.FollowCreated) ([]ids.UserID, error) {
	return []ids.UserID{ids.UserIDFrom(e.FolloweeID)}, nil
}

func (k NewFollower) Render(ctx context.Context, e events.FollowCreated, _ ids.UserID) (Message, error) {
	follower := ids.UserIDFrom(e.FollowerID)
	cards, err := k.Users.UsersByID(ctx, []ids.UserID{follower})
	if err != nil {
		return Message{}, err
	}
	return Message{
		Title:      "New follower",
		Body:       followerLabel(cards[follower]) + " followed you",
		Data:       map[string]string{"kind": k.Name(), "user_id": follower.String()},
		CollapseID: "follow-" + follower.String(),
	}, nil
}

func followerLabel(card identity.UserCard) string {
	switch {
	case card.Deleted || card.DisplayName == "":
		return unnamedFollower
	case card.Handle == "":
		return card.DisplayName
	case card.Handle == card.DisplayName:
		return "@" + card.Handle
	}
	return fmt.Sprintf("%s (@%s)", card.DisplayName, card.Handle)
}
