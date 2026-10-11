package events_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestCommentEventsNameTheCommentAsTheirAggregate(t *testing.T) {
	t.Parallel()
	comment := ids.Real{}.NewV7()
	for _, e := range []events.Event{
		events.CommentCreated{CommentID: comment, FeedObjectID: ids.Real{}.NewV7()},
		events.CommentDeleted{CommentID: comment, FeedObjectID: ids.Real{}.NewV7()},
	} {
		if e.AggregateType() != "feed_comment" || e.AggregateID() != comment {
			t.Errorf("%T aggregate = %s %s, want feed_comment %s", e, e.AggregateType(), e.AggregateID(), comment)
		}
	}
}
