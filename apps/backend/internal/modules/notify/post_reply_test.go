package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type blocks struct {
	pairs map[[2]ids.UserID]bool
	err   error
}

func (b blocks) BlockedByMe(_ context.Context, viewer, user ids.UserID) (bool, error) {
	return b.pairs[[2]ids.UserID{viewer, user}], b.err
}

type downCabals struct {
	app.Cabals
	err error
}

func (d downCabals) Cabal(context.Context, ids.CabalID) (cabal.View, error) {
	return cabal.View{}, d.err
}

func cabalView(e events.CommentCreated) cabal.View {
	return cabal.View{ID: ids.CabalIDFrom(*e.CabalID), Name: cabalName}
}

func cabalsOf(e events.CommentCreated) *fakes.Cabal {
	if e.CabalID == nil {
		return fakes.NewCabal(nil, nil)
	}
	return fakes.NewCabal([]fakes.CabalSeed{{View: cabalView(e)}}, nil)
}

func cabalPres(e events.CommentCreated) app.Presentation {
	replier := identity.UserCard{ID: ids.UserIDFrom(e.AuthorID), DisplayName: "Bea"}
	return app.ForCabal(cabalView(e), app.PersonSender(replier))
}

func personPres(card identity.UserCard) func(events.CommentCreated) app.Presentation {
	return func(events.CommentCreated) app.Presentation { return app.ForPerson(card) }
}

func wantReplyData(kind string, e events.CommentCreated, quoted bool) map[string]string {
	root := e.CommentID
	if e.ParentCommentID != nil {
		root = *e.ParentCommentID
	}
	data := map[string]string{
		"kind": kind, "feed_item_id": e.ThreadFeedObjectID.String(), "comment_id": e.CommentID.String(),
		"root_comment_id": root.String(), "item_kind": e.ItemKind, "item_label": e.ItemLabel,
		"item_actor_name": e.ItemActorName,
	}
	if e.ProposalID != nil {
		data["proposal_id"] = e.ProposalID.String()
	}
	if e.CabalID != nil {
		data["cabal_id"] = e.CabalID.String()
	}
	if e.CabalName != nil {
		data["cabal_name"] = *e.CabalName
	}
	if quoted && e.ReplyToExcerpt != nil {
		data["quoted_excerpt"] = *e.ReplyToExcerpt
	}
	return data
}

func TestNotify_PostReply_Recipients(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	owner := ids.UserIDFrom(*golden.ItemActorID)
	replier, answered := ids.UserIDFrom(golden.AuthorID), ids.UserIDFrom(*golden.ReplyToAuthorID)
	ownerID := *golden.ItemActorID
	down := errs.New(errs.CodeDBUnavailable, "test.blocks")
	for name, tc := range map[string]struct {
		edit   func(*events.CommentCreated)
		blocks blocks
		want   []ids.UserID
		err    error
	}{
		"a reply on a cabal-shaped result": {func(*events.CommentCreated) {}, blocks{}, []ids.UserID{owner}, nil},
		"a reply on a proposal":            {func(e *events.CommentCreated) { e.ItemKind = "proposal" }, blocks{}, []ids.UserID{owner}, nil},
		"a reply on a trade":               {func(e *events.CommentCreated) { e.ItemKind = "trade" }, blocks{}, []ids.UserID{owner}, nil},
		"a reply on a join":                {func(e *events.CommentCreated) { e.ItemKind = "member_joined" }, blocks{}, []ids.UserID{owner}, nil},
		"a reply on a price move":          {func(e *events.CommentCreated) { e.ItemKind = "price_move" }, blocks{}, nil, nil},
		"a reply on a cabal creation":      {func(e *events.CommentCreated) { e.ItemKind = "cabal_created" }, blocks{}, nil, nil},
		"an item with no actor":            {func(e *events.CommentCreated) { e.ItemActorID = nil }, blocks{}, nil, nil},
		"the owner replying":               {func(e *events.CommentCreated) { e.AuthorID = ownerID }, blocks{}, nil, nil},
		"the owner answered by comment_reply": {
			func(e *events.CommentCreated) { e.ReplyToAuthorID = &ownerID }, blocks{}, nil, nil,
		},
		"the owner is the parent author of an old event": {
			func(e *events.CommentCreated) {
				e.ReplyToCommentID, e.ReplyToAuthorID, e.ReplyToRootID, e.ParentAuthorID = nil, nil, nil, &ownerID
			},
			blocks{},
			nil, nil,
		},
		"the owner's comment was deleted": {
			func(e *events.CommentCreated) { e.ReplyToAuthorID, e.ReplyToDeleted = &ownerID, true }, blocks{}, []ids.UserID{owner}, nil,
		},
		"the owner blocked the replier": {
			func(*events.CommentCreated) {}, blocks{pairs: map[[2]ids.UserID]bool{{owner, replier}: true}}, nil, nil,
		},
		"the replier blocked the owner": {
			func(*events.CommentCreated) {}, blocks{pairs: map[[2]ids.UserID]bool{{replier, owner}: true}}, nil, nil,
		},
		"the block store is down": {func(*events.CommentCreated) {}, blocks{err: down}, nil, down},
	} {
		e := golden
		tc.edit(&e)
		_ = answered

		got, err := app.PostReply{Blocks: tc.blocks}.Recipients(t.Context(), e)

		if !errors.Is(err, tc.err) || !slices.Equal(got, tc.want) {
			t.Errorf("%s: Recipients = %v, %v, want %v, %v", name, got, err, tc.want, tc.err)
		}
	}
}

func TestNotify_PostReply_Render(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	replier := ids.UserIDFrom(golden.AuthorID)
	card := identity.UserCard{ID: replier, DisplayName: "Bea"}
	cards := map[ids.UserID]identity.UserCard{replier: card}
	for name, tc := range map[string]struct {
		edit  func(*events.CommentCreated)
		title string
		body  string
		pres  func(events.CommentCreated) app.Presentation
	}{
		"a reply on a cabal proposal's thread": {
			func(*events.CommentCreated) {}, "Bea replied to your proposal", "“Buy it before earnings”", cabalPres,
		},
		"a reply on a member's sale": {
			func(e *events.CommentCreated) { e.ReplyRule, e.ItemNoun = "anyone", "sale" },
			"Bea replied to your sale", "“Buy it before earnings”", personPres(card),
		},
		"a GIF with no text": {
			func(e *events.CommentCreated) { e.Excerpt, e.AttachmentKind = "", "gif" },
			"Bea replied to your proposal", "Sent a GIF", cabalPres,
		},
		"a poll with no text": {
			func(e *events.CommentCreated) { e.Excerpt, e.PollQuestion = "", "Buy or hold?" },
			"Bea replied to your proposal", "Asked: Buy or hold?", cabalPres,
		},
		"a long excerpt": {
			func(e *events.CommentCreated) { e.Excerpt = strings.Repeat("é", excerptMax) },
			"Bea replied to your proposal", "“" + strings.Repeat("é", excerptMax) + "…”", cabalPres,
		},
		"nothing to say": {
			func(e *events.CommentCreated) { e.Excerpt = "" }, "Bea replied to your proposal", "", cabalPres,
		},
	} {
		e := golden
		tc.edit(&e)
		kind := app.PostReply{Users: users{cards: cards}, Cabals: cabalsOf(e), Blocks: blocks{}}

		msg, err := kind.Render(t.Context(), e, ids.UserID{})

		want := app.Message{
			Title: tc.title, Body: tc.body, Data: wantReplyData("post_reply", e, false),
			CollapseID: "post-" + e.ThreadFeedObjectID.String(), Presentation: tc.pres(e),
		}
		if err != nil || !reflect.DeepEqual(msg, want) {
			t.Errorf("%s: Render = %+v, %v, want %+v", name, msg, err, want)
		}
	}
}

func TestNotify_PostReply_RenderFailures(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	down := errs.New(errs.CodeDBUnavailable, "test.down")
	for name, kind := range map[string]app.PostReply{
		"a users port failure":  {Users: users{err: down}, Cabals: cabalsOf(golden)},
		"a cabals port failure": {Users: users{}, Cabals: downCabals{err: down}},
	} {
		if _, err := kind.Render(t.Context(), golden, ids.UserID{}); !errors.Is(err, down) {
			t.Errorf("%s: Render error = %v, want %v", name, err, down)
		}
	}
}

func TestNotify_ReplyKeys(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	want := "comment:" + golden.CommentID.String()
	if got := (app.PostReply{}).ItemKey(golden); got != want {
		t.Errorf("PostReply ItemKey = %q, want %q", got, want)
	}
	if got := (app.PostReply{}).Name(); got != "post_reply" {
		t.Errorf("Name = %q", got)
	}
}

func TestNotify_PostReply_AReplyOnYourSalePushesYouOnce(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	owner, replier := r.follower(t, "ada", "Ada"), r.follower(t, "nora", "Nora")
	r.device(t, owner, token('a'))
	r.device(t, replier, token('b'))
	e := r.reply(t, owner, replier)
	ownerID := owner.UUID()
	other := r.ids.NewV7()
	e.ItemActorID, e.ItemKind, e.ItemNoun, e.ReplyRule = &ownerID, "result", "sale", "anyone"
	e.ReplyToAuthorID, e.ParentAuthorID = &other, &other
	d := r.emitReply(t, e)

	err := handleKinds(t, r, r.sender, d, e,
		app.CommentReply{Users: r.users, Cabals: cabalsOf(e), Blocks: blocks{}},
		app.PostReply{Users: r.users, Cabals: cabalsOf(e), Blocks: blocks{}})

	wantVerdict(t, err, "", errs.VerdictAck)
	sent := r.sender.Sent()
	if len(sent) != 1 || sent[0].UserID != owner || sent[0].Title != "Nora replied to your sale" ||
		sent[0].ThreadID != "user-"+replier.String() || sent[0].CollapseID != "post-"+e.ThreadFeedObjectID.String() ||
		sent[0].Data["sender_name"] != "Nora" {
		t.Fatalf("sent %+v, want one post_reply from Nora on a user thread", sent)
	}
}

func fixtureUUID(t *testing.T, n string) uuid.UUID {
	t.Helper()
	id, err := ids.ParseUserID("01920000-0000-7000-8000-000000000" + n)
	if err != nil {
		t.Fatal(err)
	}
	return id.UUID()
}

func presentationData(p app.Presentation) map[string]string {
	data := map[string]string{
		"sender_kind": p.Sender.Kind, "sender_id": p.Sender.ID, "sender_name": p.Sender.Name,
		"sender_image_url": p.Sender.ImageURL,
	}
	if p.Group != nil {
		data["group_id"], data["group_name"], data["group_image_url"] = p.Group.ID, p.Group.Name, p.Group.ImageURL
	}
	return data
}

func TestNotify_ReplyFixturesMatchTheRenderedPush(t *testing.T) {
	t.Parallel()
	const avatars = "https://example.com/storage/v1/object/public/avatars/"
	titans := cabal.View{
		ID:         ids.CabalIDFrom(fixtureUUID(t, "c01")),
		Name:       "Tech Titans",
		PictureURL: avatars + "tech-titans.png",
	}
	andres := ids.UserIDFrom(fixtureUUID(t, "b02"))
	logan := fixtureUUID(t, "b03")
	cabalID, titansName := fixtureUUID(t, "c01"), "Tech Titans"
	reply := func(author, label, noun, rule string) events.CommentCreated {
		root, answered := fixtureUUID(t, "d02"), fixtureUUID(t, "b02")
		excerpt := "Priced in already."
		quoted := "Deliveries miss is priced in."
		return events.CommentCreated{
			CommentID:          fixtureUUID(t, "d01"),
			FeedObjectID:       fixtureUUID(t, "e01"),
			ThreadFeedObjectID: fixtureUUID(t, "e01"),
			CabalID:            &cabalID,
			CabalName:          &titansName,
			AuthorID:           fixtureUUID(t, author),
			ParentCommentID:    &root,
			ReplyToAuthorID:    &answered,
			ReplyToRootID:      &root,
			ReplyToExcerpt:     &quoted,
			ItemActorID:        &logan,
			ItemActorName:      "Logan",
			ItemKind:           "result",
			ItemLabel:          label,
			ItemNoun:           noun,
			ReplyRule:          rule,
			Excerpt:            excerpt,
		}
	}
	proposal := reply("b01", "Sell TSLA", "proposal", "members")
	proposalID := fixtureUUID(t, "a01")
	proposal.ProposalID = &proposalID
	sale := reply("b04", "AAPL sale", "sale", "anyone")
	sale.Excerpt = "He called the top again."
	for file, tc := range map[string]struct {
		e    events.CommentCreated
		name string
	}{
		"comment_reply_proposal.apns": {proposal, "Mia"},
		"post_reply.apns":             {sale, "Sam"},
	} {
		author := ids.UserIDFrom(tc.e.AuthorID)
		cards := map[ids.UserID]identity.UserCard{
			author: {ID: author, DisplayName: tc.name, PhotoURL: avatars + strings.ToLower(tc.name) + ".png"},
		}
		kind := app.CommentReply{
			Users:  users{cards: cards},
			Cabals: fakes.NewCabal([]fakes.CabalSeed{{View: titans}}, nil),
			Blocks: blocks{},
		}

		msg, err := kind.Render(t.Context(), tc.e, andres)
		if err != nil {
			t.Fatalf("%s: Render = %v", file, err)
		}
		wantFixture(t, file, msg)
	}
}

func wantFixture(t *testing.T, file string, msg app.Message) {
	t.Helper()
	raw, err := fs.ReadFile(os.DirFS("../../../../mobile/PushFixtures"), file)
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	aps := fixture["aps"].(map[string]any)
	alert := aps["alert"].(map[string]any)
	delete(fixture, "aps")
	delete(fixture, "Simulator Target Bundle")
	data := map[string]any{}
	for k, v := range msg.Data {
		data[k] = v
	}
	for k, v := range presentationData(msg.Presentation) {
		if v != "" {
			data[k] = v
		}
	}
	if alert["title"] != msg.Title || alert["body"] != msg.Body || aps["thread-id"] != msg.Presentation.ThreadID ||
		!reflect.DeepEqual(fixture, data) {
		t.Errorf("%s: fixture %v %v, rendered %q %q %q %v", file, aps, fixture, msg.Title, msg.Body,
			msg.Presentation.ThreadID, data)
	}
}
