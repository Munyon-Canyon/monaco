package analytics_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type socialScene struct{ user, followee, follow, item, comment, parent uuid.UUID }

func newSocialScene(e *env) socialScene {
	return socialScene{
		user: e.ids.NewV7(), followee: e.ids.NewV7(), follow: e.ids.NewV7(),
		item: e.ids.NewV7(), comment: e.ids.NewV7(), parent: e.ids.NewV7(),
	}
}

func (s socialScene) followed(source string) events.Event {
	return events.FollowCreated{V: 1, FollowID: s.follow, FollowerID: s.user, FolloweeID: s.followee, Source: source}
}

func (s socialScene) commented(kind string, parent *uuid.UUID, excerpt string) events.Event {
	return events.CommentCreated{
		V: 1, CommentID: s.comment, FeedObjectID: s.item, FeedKind: kind, RefType: "swaps", RefID: s.follow,
		AuthorID: s.user, ParentCommentID: parent, Excerpt: excerpt,
	}
}

func (s socialScene) commentCapture(kind string, reply bool) fakes.PostHogCapture {
	return fakes.PostHogCapture{
		Event: "comment_created", DistinctID: s.user.String(),
		Properties: map[string]any{"feed_object_id": s.item.String(), "item_kind": kind, "is_reply": reply},
	}
}

func TestAnalyticsExport_Social(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		event func(s socialScene) events.Event
		want  func(s socialScene) fakes.PostHogCapture
	}{
		"follow.created": {
			event: func(s socialScene) events.Event { return s.followed("suggested") },
			want: func(s socialScene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "follow_created", DistinctID: s.user.String(),
					Properties: map[string]any{"followee_id": s.followee.String(), "source": "suggested"},
				}
			},
		},
		"comment.created on a proposal": {
			event: func(s socialScene) events.Event { return s.commented("proposal", nil, "gm team") },
			want:  func(s socialScene) fakes.PostHogCapture { return s.commentCapture("proposal", false) },
		},
		"comment.created reply on a trade": {
			event: func(s socialScene) events.Event { return s.commented("trade", &s.parent, "agreed") },
			want:  func(s socialScene) fakes.PostHogCapture { return s.commentCapture("trade", true) },
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, analytics.RegisterSocialExports)
			s := newSocialScene(e)
			ev := tt.event(s)
			a := e.appendEvent(t, userActor(s.user), ev)
			m := e.messageOf(ev.Type(), a)
			e.deliver(t, m)
			captures := e.fake.Captures()
			if m.outcome != bus.OutcomeAck || len(captures) != 1 {
				t.Fatalf("verdict %q with %d captures, want ack and one: %+v", m.outcome, len(captures), captures)
			}
			want := tt.want(s)
			want.APIKey, want.UUID, want.Timestamp = apiKey, a.id, a.created
			sameCapture(t, captures[0], want)
			leaksNothing(t, captures[0], a.payload)
			if n := e.deliveriesOf(t, "analytics.posthog."+string(ev.Type())); n != 1 {
				t.Errorf("%d delivery rows for %s, want 1", n, ev.Type())
			}
		})
	}
}

func TestAnalyticsExport_Comment_NoBody(t *testing.T) {
	t.Parallel()
	const text = "the vote closes at noon"
	e := newEnv(t, analytics.RegisterSocialExports)
	s := newSocialScene(e)
	ev := s.commented("proposal", &s.parent, text)
	a := e.appendEvent(t, userActor(s.user), ev)
	e.deliver(t, e.messageOf(ev.Type(), a))
	captures := e.fake.Captures()
	if len(captures) != 1 {
		t.Fatalf("%d captures, want 1: %+v", len(captures), captures)
	}
	if rendered := fmt.Sprintf("%+v", captures[0]); strings.Contains(rendered, text) {
		t.Errorf("the capture %s carries the comment text %q", rendered, text)
	}
}
