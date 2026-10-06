package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Test struct{}

func (Test) Name() string { return "test" }

func (Test) Recipients(_ context.Context, e events.NotifyTestRequested) ([]ids.UserID, error) {
	return []ids.UserID{ids.UserIDFrom(e.UserID)}, nil
}

func (Test) Render(_ context.Context, _ events.NotifyTestRequested, to ids.UserID) (Message, error) {
	return Message{
		Title:      "Monaco test",
		Body:       "Push is working.",
		Data:       map[string]string{"kind": "test", "user_id": to.String()},
		CollapseID: "test-" + to.String(),
	}, nil
}
