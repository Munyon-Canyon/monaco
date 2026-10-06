package notify_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const excerptMax = 120

func (r *pushRig) reply(t *testing.T, parent, replier ids.UserID) events.CommentCreated {
	t.Helper()
	e := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	author := parent.UUID()
	e.CommentID, e.AuthorID, e.ParentAuthorID, e.ReplyToUserID = r.ids.NewV7(), replier.UUID(), &author, nil
	return e
}

func (r *pushRig) emitReply(t *testing.T, e events.CommentCreated) bus.Delivery {
	t.Helper()
	return r.emit(t, userActor(ids.UserIDFrom(e.AuthorID)), e)
}

func (r *pushRig) handleReply(t *testing.T, d bus.Delivery, e events.CommentCreated) error {
	t.Helper()
	return handleKinds(t, r, r.sender, d, e, app.CommentReply{Users: r.users})
}

func replyPush(to ids.UserID, e events.CommentCreated, title string) apns.Push {
	return apns.Push{
		UserID: to, Token: token('a'), Environment: apns.Sandbox, CollapseID: "comment-" + e.ParentCommentID.String(),
		Title: title, Body: e.Excerpt, Data: map[string]string{
			"kind": "comment_reply", "feed_item_id": e.FeedObjectID.String(),
			"proposal_id": e.ProposalID.String(), "cabal_id": e.CabalID.String(),
		},
	}
}

func TestNotify_CommentReply_ParentAuthor(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	parent, replier := r.follower(t, "ada", "Ada"), r.follower(t, "bea", "Bea")
	r.device(t, parent, token('a'))
	r.device(t, replier, token('b'))
	e := r.reply(t, parent, replier)
	d := r.emitReply(t, e)

	wantVerdict(t, r.handleReply(t, d, e), "", errs.VerdictAck)

	want := replyPush(parent, e, "Bea replied to your comment")
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{parent: "delivered"})
	r.wantSentEvents(t, d, 1)
	r.wantRecorded(t, d, 1)
}

func TestNotify_CommentReply_Silent(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	parent, replier := r.follower(t, "ada", "Ada"), r.follower(t, "bea", "Bea")
	r.device(t, parent, token('a'))
	r.device(t, replier, token('b'))
	for name, edit := range map[string]func(*events.CommentCreated){
		"a top-level comment": func(e *events.CommentCreated) {
			e.ParentCommentID, e.ParentAuthorID = nil, nil
		},
		"a self-reply":                 func(e *events.CommentCreated) { e.AuthorID = *e.ParentAuthorID },
		"a reply to a deleted comment": func(e *events.CommentCreated) { e.ParentDeleted = true },
	} {
		t.Logf("case: %s", name)
		e := r.reply(t, parent, replier)
		edit(&e)
		d := r.emitReply(t, e)

		wantVerdict(t, r.handleReply(t, d, e), "", errs.VerdictAck)

		if sent := r.sender.Sent(); len(sent) != 0 {
			t.Fatalf("%s: sent %+v, want nothing", name, sent)
		}
		r.wantRows(t, map[string]int{})
		r.wantRecorded(t, d, 1)
	}
}

func TestNotify_CommentReply_Recipients(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	parent := ids.UserIDFrom(*golden.ParentAuthorID)
	for name, tc := range map[string]struct {
		edit func(*events.CommentCreated)
		want []ids.UserID
	}{
		"the author of the comment replied to": {
			func(e *events.CommentCreated) { e.ReplyToUserID = nil }, []ids.UserID{parent},
		},
		"a top-level comment":     {func(e *events.CommentCreated) { e.ParentCommentID = nil }, nil},
		"a deleted comment":       {func(e *events.CommentCreated) { e.ParentDeleted = true }, nil},
		"a parent with no author": {func(e *events.CommentCreated) { e.ParentAuthorID = nil }, nil},
	} {
		e := golden
		tc.edit(&e)

		got, err := app.CommentReply{}.Recipients(t.Context(), e)

		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s: Recipients = %v, %v, want %v", name, got, err, tc.want)
		}
	}
}

type commentRender struct {
	edit  func(*events.CommentCreated)
	cards map[ids.UserID]identity.UserCard
	err   error
	title string
	body  string
	data  map[string]string
}

func commentRenders(golden events.CommentCreated) map[string]commentRender {
	replier := ids.UserIDFrom(golden.AuthorID)
	card := func(name string, deleted bool) map[ids.UserID]identity.UserCard {
		return map[ids.UserID]identity.UserCard{replier: {ID: replier, DisplayName: name, Deleted: deleted}}
	}
	onItem := map[string]string{"kind": "comment_reply", "feed_item_id": golden.FeedObjectID.String()}
	onProposal := map[string]string{
		"kind": "comment_reply", "feed_item_id": golden.FeedObjectID.String(),
		"proposal_id": golden.ProposalID.String(), "cabal_id": golden.CabalID.String(),
	}
	cut, whole := strings.Repeat("é", excerptMax), strings.Repeat("é", excerptMax-1)
	byBea, bySomeone := "Bea replied to your comment", "Someone replied to your comment"
	keep := func(*events.CommentCreated) {}
	return map[string]commentRender{
		"a comment on a proposal": {
			edit: keep, cards: card("Bea", false), title: byBea, body: golden.Excerpt, data: onProposal,
		},
		"a comment on a feed item": {
			edit:  func(e *events.CommentCreated) { e.ProposalID = nil },
			cards: card("Bea", false), title: byBea, body: golden.Excerpt, data: onItem,
		},
		"a proposal with no cabal": {
			edit:  func(e *events.CommentCreated) { e.CabalID = nil },
			cards: card("Bea", false), title: byBea, body: golden.Excerpt, data: onItem,
		},
		"an excerpt of 120 runes": {
			edit:  func(e *events.CommentCreated) { e.Excerpt = cut },
			cards: card("Bea", false), title: byBea, body: cut + "…", data: onProposal,
		},
		"an excerpt of 119 runes": {
			edit:  func(e *events.CommentCreated) { e.Excerpt = whole },
			cards: card("Bea", false), title: byBea, body: whole, data: onProposal,
		},
		"a deleted author who kept a name": {
			edit: keep, cards: card("Bea", true), title: bySomeone, body: golden.Excerpt, data: onProposal,
		},
		"an author with no name": {
			edit: keep, cards: card("", false), title: bySomeone, body: golden.Excerpt, data: onProposal,
		},
		"an author the port does not know": {
			edit: keep, title: bySomeone, body: golden.Excerpt, data: onProposal,
		},
		"a users port failure": {edit: keep, err: errs.New(errs.CodeDBUnavailable, "test.users")},
	}
}

func TestNotify_CommentReply_Render(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	collapse := "comment-" + golden.ParentCommentID.String()
	for name, tc := range commentRenders(golden) {
		e := golden
		tc.edit(&e)
		kind := app.CommentReply{Users: users{cards: tc.cards, err: tc.err}}

		msg, err := kind.Render(t.Context(), e, ids.UserID{})

		want := app.Message{}
		if tc.err == nil {
			want = app.Message{Title: tc.title, Body: tc.body, Data: tc.data, CollapseID: collapse}
		}
		if !errors.Is(err, tc.err) || !reflect.DeepEqual(msg, want) {
			t.Errorf("%s: Render = %+v, %v, want %+v, %v", name, msg, err, want, tc.err)
		}
	}
}

func TestNotify_CommentReply_ReachesTheParentAuthorThroughTheModule(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	parent, replier := r.follower(t, "ada", "Ada"), r.follower(t, "bea", "Bea")
	r.device(t, parent, token('a'))
	r.device(t, replier, token('b'))
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
	e := r.reply(t, parent, replier)

	d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), userActor(replier), e)

	want := replyPush(parent, e, "Bea replied to your comment")
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{parent: "delivered"})
	r.wantRecorded(t, d, 1)
}
