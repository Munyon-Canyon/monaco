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
	e.ReplyToAuthorID = &author
	return e
}

func (r *pushRig) emitReply(t *testing.T, e events.CommentCreated) bus.Delivery {
	t.Helper()
	return r.emit(t, userActor(ids.UserIDFrom(e.AuthorID)), e)
}

func (r *pushRig) handleReply(t *testing.T, d bus.Delivery, e events.CommentCreated) error {
	t.Helper()
	return handleKinds(t, r, r.sender, d, e, app.CommentReply{Users: r.users, Cabals: cabalsOf(e), Blocks: blocks{}})
}

func wantReplyPush(t *testing.T, sent []apns.Push, to ids.UserID, e events.CommentCreated, title, body, thread string) {
	t.Helper()
	if len(sent) != 1 {
		t.Fatalf("sent %+v, want exactly one push", sent)
	}
	p := sent[0]
	if p.UserID != to || p.Title != title || p.Body != body || p.ThreadID != thread ||
		p.CollapseID != "comment-"+e.ReplyToRootID.String() || p.Data["kind"] != "comment_reply" ||
		p.Data["comment_id"] != e.CommentID.String() {
		t.Fatalf("sent %+v, want %s %q %q thread %q", p, to, title, body, thread)
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

	wantReplyPush(t, r.sender.Sent(), parent, e, "Bea replied to you",
		"“Buy it before earnings” On Ada's Buy NVDA.", "cabal-"+e.CabalID.String())
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
		"a comment that answers none": func(e *events.CommentCreated) {
			e.ParentCommentID, e.ParentAuthorID = nil, nil
			e.ReplyToCommentID, e.ReplyToAuthorID, e.ReplyToRootID, e.ReplyToExcerpt = nil, nil, nil, nil
		},
		"a self-reply":                 func(e *events.CommentCreated) { e.AuthorID = *e.ReplyToAuthorID },
		"a reply to a deleted comment": func(e *events.CommentCreated) { e.ReplyToDeleted = true },
		"a reply to a deleted comment in an old event": func(e *events.CommentCreated) {
			e.ReplyToCommentID, e.ReplyToAuthorID, e.ReplyToRootID = nil, nil, nil
			e.ParentDeleted = true
		},
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
	answered := ids.UserIDFrom(*golden.ReplyToAuthorID)
	other := ids.Real{}.NewV7()
	for name, tc := range map[string]struct {
		edit func(*events.CommentCreated)
		want []ids.UserID
	}{
		"the author of the comment replied to": {
			func(e *events.CommentCreated) { e.ReplyToUserID = nil }, []ids.UserID{answered},
		},
		"a top-level reply that quotes a comment": {
			func(e *events.CommentCreated) {
				e.ParentCommentID, e.ParentAuthorID, e.ReplyToAuthorID = nil, nil, &other
			},
			[]ids.UserID{ids.UserIDFrom(other)},
		},
		"a deleted comment": {func(e *events.CommentCreated) { e.ReplyToDeleted = true }, nil},
		"an event from before the answered comment was stored": {
			func(e *events.CommentCreated) {
				e.ReplyToCommentID, e.ReplyToAuthorID, e.ReplyToRootID, e.ReplyToExcerpt = nil, nil, nil, nil
			},
			[]ids.UserID{parent},
		},
		"no answered comment": {
			func(e *events.CommentCreated) {
				e.ReplyToCommentID, e.ReplyToAuthorID, e.ReplyToRootID, e.ParentAuthorID = nil, nil, nil, nil
			},
			nil,
		},
	} {
		e := golden
		tc.edit(&e)

		got, err := app.CommentReply{Cabals: cabalsOf(e), Blocks: blocks{}}.Recipients(t.Context(), e)

		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s: Recipients = %v, %v, want %v", name, got, err, tc.want)
		}
	}
}

func TestNotify_CommentReply_Render(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	replier, itemActor := ids.UserIDFrom(golden.AuthorID), ids.UserIDFrom(*golden.ItemActorID)
	card := identity.UserCard{ID: replier, DisplayName: "Bea"}
	cut, whole := strings.Repeat("é", excerptMax), strings.Repeat("é", excerptMax-1)
	onAda := " On Ada's Buy NVDA."
	for name, tc := range map[string]struct {
		edit  func(*events.CommentCreated)
		to    ids.UserID
		cards map[ids.UserID]identity.UserCard
		title string
		body  string
		pres  func(events.CommentCreated) app.Presentation
	}{
		"a cabal discussion": {
			edit: func(*events.CommentCreated) {}, cards: map[ids.UserID]identity.UserCard{replier: card},
			title: "Bea replied to you", body: "“Buy it before earnings”" + onAda, pres: cabalPres,
		},
		"a reply on the recipient's own post": {
			edit: func(*events.CommentCreated) {}, to: itemActor, cards: map[ids.UserID]identity.UserCard{replier: card},
			title: "Bea replied to you", body: "“Buy it before earnings” On your Buy NVDA.", pres: cabalPres,
		},
		"a post outside a cabal's discussion": {
			edit:  func(e *events.CommentCreated) { e.ReplyRule = "anyone" },
			cards: map[ids.UserID]identity.UserCard{replier: card},
			title: "Bea replied to you", body: "“Buy it before earnings”" + onAda, pres: personPres(card),
		},
		"a members thread with no cabal": {
			edit:  func(e *events.CommentCreated) { e.CabalID = nil },
			cards: map[ids.UserID]identity.UserCard{replier: card},
			title: "Bea replied to you", body: "“Buy it before earnings”" + onAda, pres: personPres(card),
		},
		"an excerpt of 120 runes": {
			edit:  func(e *events.CommentCreated) { e.Excerpt = cut },
			cards: map[ids.UserID]identity.UserCard{replier: card},
			title: "Bea replied to you", body: "“" + cut + "…”" + onAda, pres: cabalPres,
		},
		"an excerpt of 119 runes": {
			edit:  func(e *events.CommentCreated) { e.Excerpt = whole },
			cards: map[ids.UserID]identity.UserCard{replier: card},
			title: "Bea replied to you", body: "“" + whole + "”" + onAda, pres: cabalPres,
		},
		"a GIF with no text": {
			edit:  func(e *events.CommentCreated) { e.Excerpt, e.AttachmentKind = "", "gif" },
			cards: map[ids.UserID]identity.UserCard{replier: card},
			title: "Bea replied to you", body: "Sent a GIF" + onAda, pres: cabalPres,
		},
		"a poll with no text": {
			edit:  func(e *events.CommentCreated) { e.Excerpt, e.PollQuestion = "", "Buy or hold?" },
			cards: map[ids.UserID]identity.UserCard{replier: card},
			title: "Bea replied to you", body: "Asked: Buy or hold?" + onAda, pres: cabalPres,
		},
		"nothing to say": {
			edit:  func(e *events.CommentCreated) { e.Excerpt = "" },
			cards: map[ids.UserID]identity.UserCard{replier: card},
			title: "Bea replied to you", body: "On Ada's Buy NVDA.", pres: cabalPres,
		},
		"a deleted author who kept a name": {
			edit: func(*events.CommentCreated) {}, cards: map[ids.UserID]identity.UserCard{replier: {ID: replier, DisplayName: "Bea", Deleted: true}},
			title: "Someone replied to you", body: "“Buy it before earnings”" + onAda,
			pres: func(e events.CommentCreated) app.Presentation {
				return app.ForCabal(cabalView(e), app.PersonSender(identity.UserCard{ID: replier, Deleted: true}))
			},
		},
		"an author the port does not know": {
			edit:  func(*events.CommentCreated) {},
			title: "Someone replied to you", body: "“Buy it before earnings”" + onAda,
			pres: func(e events.CommentCreated) app.Presentation {
				return app.ForCabal(cabalView(e), app.PersonSender(identity.UserCard{ID: replier}))
			},
		},
	} {
		e := golden
		tc.edit(&e)
		kind := app.CommentReply{Users: users{cards: tc.cards}, Cabals: cabalsOf(e), Blocks: blocks{}}

		msg, err := kind.Render(t.Context(), e, tc.to)

		want := app.Message{
			Title: tc.title, Body: tc.body, Data: wantReplyData("comment_reply", e, true),
			CollapseID: "comment-" + golden.ReplyToRootID.String(), Presentation: tc.pres(e),
		}
		if err != nil || !reflect.DeepEqual(msg, want) {
			t.Errorf("%s: Render = %+v, %v, want %+v", name, msg, err, want)
		}
	}
}

func TestNotify_CommentReply_RenderFailures(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	down := errs.New(errs.CodeDBUnavailable, "test.down")
	for name, kind := range map[string]app.CommentReply{
		"a users port failure":  {Users: users{err: down}, Cabals: cabalsOf(golden), Blocks: blocks{}},
		"a cabals port failure": {Users: users{}, Cabals: downCabals{err: down}, Blocks: blocks{}},
	} {
		if _, err := kind.Render(t.Context(), golden, ids.UserID{}); !errors.Is(err, down) {
			t.Errorf("%s: Render error = %v, want %v", name, err, down)
		}
	}
}

func TestNotify_CommentReply_BlocksDropTheRecipientEitherWay(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	answered, replier := ids.UserIDFrom(*golden.ReplyToAuthorID), ids.UserIDFrom(golden.AuthorID)
	down := errs.New(errs.CodeDBUnavailable, "test.blocks")
	for name, tc := range map[string]struct {
		blocks blocks
		want   []ids.UserID
		err    error
	}{
		"nobody blocked":          {blocks{}, []ids.UserID{answered}, nil},
		"the recipient blocked":   {blocks{pairs: map[[2]ids.UserID]bool{{answered, replier}: true}}, nil, nil},
		"the replier blocked":     {blocks{pairs: map[[2]ids.UserID]bool{{replier, answered}: true}}, nil, nil},
		"the block store is down": {blocks{err: down}, nil, down},
	} {
		got, err := app.CommentReply{Cabals: cabalsOf(golden), Blocks: tc.blocks}.Recipients(t.Context(), golden)

		if !errors.Is(err, tc.err) || !slices.Equal(got, tc.want) {
			t.Errorf("%s: Recipients = %v, %v, want %v, %v", name, got, err, tc.want, tc.err)
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
	e.ReplyRule = "anyone"

	d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), userActor(replier), e)

	wantReplyPush(t, r.sender.Sent(), parent, e, "Bea replied to you",
		"“Buy it before earnings” On Ada's Buy NVDA.", "user-"+replier.String())
	r.wantStates(t, d, map[ids.UserID]string{parent: "delivered"})
	r.wantRecorded(t, d, 1)
}

func TestNotify_CommentReply_CollapsesOnTheAnsweredRoot(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	old := golden
	old.ReplyToRootID = nil
	topLevel := golden
	topLevel.ParentCommentID = nil
	for name, tc := range map[string]struct {
		e    events.CommentCreated
		want string
	}{
		"the answered comment's root": {golden, "comment-" + golden.ReplyToRootID.String()},
		"an old event's parent":       {old, "comment-" + golden.ParentCommentID.String()},
		"a top-level reply":           {topLevel, "comment-" + golden.ReplyToRootID.String()},
	} {
		kind := app.CommentReply{Users: users{}, Cabals: cabalsOf(tc.e), Blocks: blocks{}}

		msg, err := kind.Render(t.Context(), tc.e, ids.UserID{})

		if err != nil || msg.CollapseID != tc.want {
			t.Errorf("%s: collapse id = %q, %v, want %q", name, msg.CollapseID, err, tc.want)
		}
	}
}
