package events_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestNotifyEvents_keyTheRequestByItsUserAndTheSendByItsNotification(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(1)
	user, notification := g.NewV7(), g.NewV7()
	for _, tt := range []struct {
		event     events.Event
		want      events.Type
		aggregate string
		id        uuid.UUID
	}{
		{events.NotifyTestRequested{UserID: user}, events.TypeNotifyTestRequested, "user", user},
		{
			events.NotificationSent{NotificationID: notification, UserID: user},
			events.TypeNotificationSent,
			"notification", notification,
		},
	} {
		if tt.event.Type() != tt.want || tt.event.AggregateType() != tt.aggregate || tt.event.AggregateID() != tt.id {
			t.Errorf("%T = %s/%s/%s, want %s/%s/%s", tt.event, tt.event.Type(), tt.event.AggregateType(),
				tt.event.AggregateID(), tt.want, tt.aggregate, tt.id)
		}
	}
}
