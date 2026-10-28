package events_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestReactionEventsNameTheFeedItemAsTheirAggregate(t *testing.T) {
	t.Parallel()
	item := ids.Real{}.NewV7()
	for _, e := range []events.Event{
		events.ReactionAdded{FeedObjectID: item},
		events.ReactionRemoved{FeedObjectID: item},
	} {
		if e.AggregateType() != "feed_object" || e.AggregateID() != item {
			t.Errorf("%T aggregate = %s %s, want feed_object %s", e, e.AggregateType(), e.AggregateID(), item)
		}
	}
}
