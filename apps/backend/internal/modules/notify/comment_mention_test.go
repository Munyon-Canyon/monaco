package notify_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestNotify_CommentMention_Recipients(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	replier := ids.UserIDFrom(golden.AuthorID)
	one, two := ids.UserIDFrom(fixtureUUID(t, "b11")), ids.UserIDFrom(fixtureUUID(t, "b12"))
	down := errs.New(errs.CodeDBUnavailable, "test.blocks")
	for name, tc := range map[string]struct {
		mentioned []ids.UserID
		blocks    blocks
		want      []ids.UserID
		err       error
	}{
		"nobody mentioned": {nil, blocks{}, []ids.UserID{}, nil},
		"two mentioned":    {[]ids.UserID{one, two}, blocks{}, []ids.UserID{one, two}, nil},
		"one blocked the replier": {
			[]ids.UserID{one, two}, blocks{pairs: map[[2]ids.UserID]bool{{one, replier}: true}}, []ids.UserID{two}, nil,
		},
		"the replier blocked one": {
			[]ids.UserID{one, two}, blocks{pairs: map[[2]ids.UserID]bool{{replier, two}: true}}, []ids.UserID{one}, nil,
		},
		"the block store is down": {[]ids.UserID{one}, blocks{err: down}, nil, down},
	} {
		e := golden
		e.MentionedUserIDs = uuidsOf(tc.mentioned...)

		got, err := app.CommentMention{Blocks: tc.blocks}.Recipients(t.Context(), e)

		if !errors.Is(err, tc.err) || (err == nil && !slices.Equal(got, tc.want)) {
			t.Errorf("%s: Recipients = %v, %v, want %v, %v", name, got, err, tc.want, tc.err)
		}
	}
}

func TestNotify_CommentMention_TheMentionedAreLeftOutOfTheReplyKinds(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	e := golden
	e.MentionedUserIDs = []uuid.UUID{*golden.ItemActorID, *golden.ReplyToAuthorID}

	post, err := app.PostReply{Blocks: blocks{}}.Recipients(t.Context(), e)
	if err != nil || post != nil {
		t.Errorf("PostReply Recipients = %v, %v, want none", post, err)
	}
	reply, err := app.CommentReply{Blocks: blocks{}}.Recipients(t.Context(), e)
	if err != nil || reply != nil {
		t.Errorf("CommentReply Recipients = %v, %v, want none", reply, err)
	}
}

func TestNotify_CommentMention_Render(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	replier := ids.UserIDFrom(golden.AuthorID)
	card := identity.UserCard{ID: replier, DisplayName: "Bea"}
	cards := map[ids.UserID]identity.UserCard{replier: card}
	for name, tc := range map[string]struct {
		edit func(*events.CommentCreated)
		body string
		pres func(events.CommentCreated) app.Presentation
	}{
		"a cabal thread": {
			func(e *events.CommentCreated) { e.Excerpt = "Still out: @Andres, Jordan?" },
			"“Still out: @Andres, Jordan?” On Buy NVDA.", cabalPres,
		},
		"another thread": {
			func(e *events.CommentCreated) { e.ReplyRule = "anyone" }, "“Buy it before earnings” On Buy NVDA.",
			personPres(card),
		},
		"a GIF with no text": {
			func(e *events.CommentCreated) { e.Excerpt, e.AttachmentKind = "", "gif" }, "Sent a GIF On Buy NVDA.", cabalPres,
		},
		"nothing to say": {func(e *events.CommentCreated) { e.Excerpt = "" }, "On Buy NVDA.", cabalPres},
	} {
		e := golden
		tc.edit(&e)
		kind := app.CommentMention{Users: users{cards: cards}, Cabals: cabalsOf(e), Blocks: blocks{}}

		msg, err := kind.Render(t.Context(), e, ids.UserID{})

		want := app.Message{
			Title: "Bea mentioned you", Body: tc.body, Data: wantReplyData("comment_mention", e, false),
			CollapseID: "comment-" + e.ReplyToRootID.String(), Presentation: tc.pres(e),
		}
		if err != nil || !reflect.DeepEqual(msg, want) {
			t.Errorf("%s: Render = %+v, %v, want %+v", name, msg, err, want)
		}
	}
}

func TestNotify_CommentMention_RenderFailures(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	down := errs.New(errs.CodeDBUnavailable, "test.down")
	for name, kind := range map[string]app.CommentMention{
		"a users port failure":  {Users: users{err: down}, Cabals: cabalsOf(golden)},
		"a cabals port failure": {Users: users{}, Cabals: downCabals{err: down}},
	} {
		if _, err := kind.Render(t.Context(), golden, ids.UserID{}); !errors.Is(err, down) {
			t.Errorf("%s: Render error = %v, want %v", name, err, down)
		}
	}
}

func TestNotify_CommentMention_Identity(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	kind := app.CommentMention{}
	if kind.Name() != "comment_mention" || kind.ItemKey(golden) != "comment:"+golden.CommentID.String() {
		t.Errorf("Name = %q, ItemKey = %q", kind.Name(), kind.ItemKey(golden))
	}
}

func TestNotify_CommentMention_AMentionedParentAuthorGetsOnePush(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	owner, replier := r.follower(t, "ada", "Ada"), r.follower(t, "nora", "Nora")
	answered := r.follower(t, "cleo", "Cleo")
	r.device(t, owner, token('a'))
	r.device(t, replier, token('b'))
	r.device(t, answered, token('c'))
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
	e := r.reply(t, answered, replier)
	ownerID := owner.UUID()
	e.ItemActorID, e.ItemKind, e.ItemNoun, e.ReplyRule = &ownerID, "result", "sale", "anyone"
	e.Excerpt = "Still out: @ada, @cleo?"
	e.MentionedUserIDs = uuidsOf(owner, answered)

	d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), userActor(replier), e)

	sent := r.sender.Sent()
	if len(sent) != 2 {
		t.Fatalf("sent %+v, want one push per mentioned person", sent)
	}
	for _, p := range sent {
		if p.Title != "Nora mentioned you" || p.Data["kind"] != "comment_mention" ||
			p.ThreadID != "user-"+replier.String() {
			t.Errorf("push %+v, want a comment_mention from Nora on the replier's thread", p)
		}
	}
	r.wantStates(t, d, map[ids.UserID]string{owner: "delivered", answered: "delivered"})
	r.wantCount(t, "notification rows", 2, `SELECT count(*) FROM notifications WHERE kind = 'comment_mention'`)
	r.wantCount(t, "reply rows", 0, `SELECT count(*) FROM notifications WHERE kind IN ('post_reply', 'comment_reply')`)
}

func TestNotify_CommentMention_ARedeliveredEventPushesOnce(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	mentioned, replier := r.follower(t, "ada", "Ada"), r.follower(t, "bea", "Bea")
	r.device(t, mentioned, token('a'))
	r.device(t, replier, token('b'))
	e := r.reply(t, replier, replier)
	e.ReplyToAuthorID, e.ParentAuthorID = nil, nil
	e.MentionedUserIDs = uuidsOf(mentioned)
	d := r.emitReply(t, e)
	kind := app.CommentMention{Users: r.users, Cabals: cabalsOf(e), Blocks: blocks{}}

	first := handleKinds(t, r, r.sender, d, e, kind)
	second := handleKinds(t, r, r.sender, d, e, kind)

	wantVerdict(t, first, "", errs.VerdictAck)
	wantVerdict(t, second, "", errs.VerdictAck)
	if sent := r.sender.Sent(); len(sent) != 1 || sent[0].UserID != mentioned || sent[0].Title != "Bea mentioned you" {
		t.Fatalf("sent %+v after a redelivery, want one mention", sent)
	}
	r.wantCount(t, "rows after a redelivery", 1, `SELECT count(*) FROM notifications`)
}

func TestNotify_CommentMention_FixtureMatchesTheRenderedPush(t *testing.T) {
	t.Parallel()
	const avatars = "https://example.com/storage/v1/object/public/avatars/"
	titans := cabal.View{
		ID: ids.CabalIDFrom(fixtureUUID(t, "c01")), Name: "Tech Titans", PictureURL: avatars + "tech-titans.png",
	}
	cabalID, titansName := fixtureUUID(t, "c01"), "Tech Titans"
	root, logan, proposalID := fixtureUUID(t, "d02"), fixtureUUID(t, "b03"), fixtureUUID(t, "a01")
	e := events.CommentCreated{
		CommentID: fixtureUUID(
			t,
			"d01",
		),
		FeedObjectID:       fixtureUUID(t, "e01"),
		ThreadFeedObjectID: fixtureUUID(t, "e01"),
		CabalID:            &cabalID,
		CabalName:          &titansName,
		AuthorID:           fixtureUUID(t, "b01"),
		ParentCommentID:    &root,
		ItemActorID:        &logan,
		ItemActorName:      "Logan",
		ItemKind:           "result",
		ItemLabel:          "Sell TSLA",
		ItemNoun:           "proposal",
		ReplyRule:          "members",
		Excerpt:            "Still out: @Andres, Jordan?",
		ProposalID:         &proposalID,
		MentionedUserIDs:   []uuid.UUID{fixtureUUID(t, "b02")},
	}
	author := ids.UserIDFrom(e.AuthorID)
	cards := map[ids.UserID]identity.UserCard{
		author: {ID: author, DisplayName: "Mia", PhotoURL: avatars + "mia.png"},
	}
	kind := app.CommentMention{
		Users: users{cards: cards}, Cabals: fakes.NewCabal([]fakes.CabalSeed{{View: titans}}, nil), Blocks: blocks{},
	}

	msg, err := kind.Render(t.Context(), e, ids.UserIDFrom(fixtureUUID(t, "b02")))
	if err != nil {
		t.Fatalf("Render = %v", err)
	}

	wantFixture(t, "comment_mention.apns", msg)
}
