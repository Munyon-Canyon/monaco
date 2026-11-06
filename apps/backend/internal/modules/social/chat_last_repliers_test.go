package social_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func authorIDs(authors []api.ChatAuthor) []uuid.UUID {
	out := make([]uuid.UUID, len(authors))
	for i, a := range authors {
		out[i] = a.Id
	}
	return out
}

func userUUIDs(users ...ids.UserID) []uuid.UUID {
	out := make([]uuid.UUID, len(users))
	for i, u := range users {
		out[i] = u.UUID()
	}
	return out
}

func (f chatRoutes) replyTo(t *testing.T, author ids.UserID, parent api.ChatMessage, body string) api.ChatMessage {
	t.Helper()
	return f.mustPost(t, author, api.PostChatMessageRequest{Body: ptr(body), ParentId: &parent.Id})
}

func (f chatRoutes) channelMessage(t *testing.T, viewer ids.UserID, id uuid.UUID) api.ChatMessage {
	t.Helper()
	page, err := f.channel(t, viewer, api.GetChatMessagesParams{Limit: ptr(100)})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range page {
		if m.Id == id {
			return m
		}
	}
	t.Fatalf("message %s is not in the channel page", id)
	return api.ChatMessage{}
}

func TestLastRepliers_nameTheNewestThreeOncePerPersonNewestFirst(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	creator, logan, sam, kai := f.cabal.Creator.ID, f.member(0), f.member(1), f.member(2)
	top := f.mustPost(t, logan, api.PostChatMessageRequest{Body: ptr("top")})
	lone := f.mustPost(t, logan, api.PostChatMessageRequest{Body: ptr("nobody answers")})
	for _, author := range []ids.UserID{creator, logan, sam, kai, sam} {
		f.replyTo(t, author, top, "reply")
	}
	want := userUUIDs(sam, kai, logan)

	if got := authorIDs(f.channelMessage(t, kai, top.Id).LastRepliers); !slices.Equal(got, want) {
		t.Fatalf("channel last_repliers = %v, want %v", got, want)
	}
	th, err := f.thread(t, kai, top.Id, api.GetChatThreadParams{Limit: ptr(50)})
	if err != nil {
		t.Fatal(err)
	}
	if got := authorIDs(th.Parent.LastRepliers); !slices.Equal(got, want) {
		t.Fatalf("thread parent last_repliers = %v, want %v", got, want)
	}
	for _, r := range th.Replies {
		if r.LastRepliers == nil || len(r.LastRepliers) != 0 {
			t.Fatalf("reply last_repliers = %#v, want an empty array", r.LastRepliers)
		}
	}
	if got := f.channelMessage(t, kai, lone.Id).LastRepliers; got == nil || len(got) != 0 {
		t.Fatalf("last_repliers with no replies = %#v, want an empty array", got)
	}
}

func TestLastRepliers_leaveOutDeletedRepliesAndDeletedMessages(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	logan, sam, kai := f.member(0), f.member(1), f.member(2)
	top := f.mustPost(t, logan, api.PostChatMessageRequest{Body: ptr("top")})
	f.replyTo(t, sam, top, "first")
	gone := f.replyTo(t, kai, top, "second")
	if got := authorIDs(f.channelMessage(t, logan, top.Id).LastRepliers); !slices.Equal(got, userUUIDs(kai, sam)) {
		t.Fatalf("last_repliers = %v, want kai then sam", got)
	}
	if err := f.remove(t, kai, gone.Id); err != nil {
		t.Fatal(err)
	}
	if got := authorIDs(f.channelMessage(t, logan, top.Id).LastRepliers); !slices.Equal(got, userUUIDs(sam)) {
		t.Fatalf("last_repliers after the delete = %v, want only sam", got)
	}
	if err := f.remove(t, logan, top.Id); err != nil {
		t.Fatal(err)
	}
	placeholder := f.channelMessage(t, kai, top.Id)
	if !placeholder.Deleted || placeholder.ReplyCount != 1 || len(placeholder.LastRepliers) != 0 {
		t.Fatalf("deleted parent = %+v, want a deleted placeholder with an empty last_repliers", placeholder)
	}
}

func TestLastRepliers_leaveOutAuthorsTheCallerBlocked(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	logan, sam, kai := f.member(0), f.member(1), f.member(2)
	top := f.mustPost(t, logan, api.PostChatMessageRequest{Body: ptr("top")})
	f.replyTo(t, sam, top, "first")
	f.replyTo(t, kai, top, "second")
	f.blocks(t, logan, sam)
	if got := authorIDs(f.channelMessage(t, logan, top.Id).LastRepliers); !slices.Equal(got, userUUIDs(kai)) {
		t.Fatalf("last_repliers for the blocker = %v, want only kai", got)
	}
	if got := authorIDs(f.channelMessage(t, kai, top.Id).LastRepliers); !slices.Equal(got, userUUIDs(kai, sam)) {
		t.Fatalf("last_repliers for someone else = %v, want kai then sam", got)
	}
}

func TestLastRepliers_aFormerMemberKeepsItsPlaceWithNoNameOrHandle(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	logan, gone := f.member(0), f.member(1)
	top := f.mustPost(t, logan, api.PostChatMessageRequest{Body: ptr("top")})
	f.replyTo(t, gone, top, "bye")
	got := f.channelMessage(t, logan, top.Id).LastRepliers
	if len(got) != 1 || got[0].Id != gone.UUID() || got[0].Handle != nil || got[0].DisplayName != "" {
		t.Fatalf("last_repliers = %+v, want the former member without a name or handle", got)
	}
}

func TestLastRepliers_aProposalCardTakesThemFromTheProposalsComments(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	logan, sam, kai := f.member(0), f.member(1), f.member(2)
	f.reply(t, logan, "one", nil)
	f.reply(t, sam, "two", nil)
	f.reply(t, kai, "three", nil)
	f.reply(t, sam, "four", nil)
	want := userUUIDs(sam, kai, logan)

	card := f.proposalRows(t)[0]
	if got := authorIDs(card.LastRepliers); !slices.Equal(got, want) {
		t.Fatalf("card last_repliers = %v, want %v", got, want)
	}
	res, err := f.routes.GetProposalComments(asUser(t.Context(), kai), api.GetProposalCommentsRequestObject{
		Id: f.proposal,
	})
	if err != nil {
		t.Fatal(err)
	}
	var fromComments []uuid.UUID
	items := res.(api.GetProposalComments200JSONResponse).Items
	slices.SortStableFunc(items, func(a, b api.CommentThread) int {
		return b.Comment.CreatedAt.Compare(a.Comment.CreatedAt)
	})
	for _, item := range items {
		if id := item.Comment.Author.Id; !slices.Contains(fromComments, id) {
			fromComments = append(fromComments, id)
		}
	}
	if !slices.Equal(fromComments[:3], want) {
		t.Fatalf("authors of the proposal's comments, newest first = %v, want %v", fromComments, want)
	}
	if got := authorIDs(
		f.read(t, kai, api.GetChatThreadParams{Limit: ptr(50)}).Parent.LastRepliers,
	); !slices.Equal(
		got,
		want,
	) {
		t.Fatalf("thread parent last_repliers = %v, want %v", got, want)
	}
}

func TestLastRepliers_aProposalCardLeavesOutBlockedAuthors(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	logan, sam := f.member(0), f.member(1)
	f.reply(t, logan, "one", nil)
	f.reply(t, sam, "two", nil)
	f.blocks(t, logan, sam)
	card := f.channelMessage(t, logan, f.card)
	if got := authorIDs(card.LastRepliers); !slices.Equal(got, userUUIDs(logan)) {
		t.Fatalf("card last_repliers for the blocker = %v, want only logan", got)
	}
}

func lastRepliersOf(t *testing.T, p testkit.RealtimePublish) []uuid.UUID {
	t.Helper()
	var thread app.ThreadUpdated
	decode(t, p, &thread)
	out := make([]uuid.UUID, len(thread.LastRepliers))
	for i, r := range thread.LastRepliers {
		out[i] = r.ID
	}
	return out
}

func TestLastRepliers_theThreadUpdateCarriesThemAfterAReplyAndADelete(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	f.mustSend(t, f.member(1), "one", &domain.Reply{Parent: top.ID})
	f.clock.Advance(time.Second)
	second := f.mustSend(t, f.member(2), "two", &domain.Reply{Parent: top.ID})
	threads := f.threadUpdates(t)
	if len(threads) != 2 || !slices.Equal(lastRepliersOf(t, threads[1]), userUUIDs(f.member(2), f.member(1))) {
		t.Fatalf("thread.updated after two replies = %+v, want kai then the first replier", threads)
	}
	if err := f.del.Handle(f.as(t, f.member(2)), app.DeleteChatMessage{
		CabalID: f.cabal.ID, MessageID: second.ID, Caller: f.member(2),
	}); err != nil {
		t.Fatal(err)
	}
	threads = f.threadUpdates(t)
	if len(threads) != 3 || !slices.Equal(lastRepliersOf(t, threads[2]), userUUIDs(f.member(1))) {
		t.Fatalf("thread.updated after the delete = %+v, want only the first replier", threads)
	}
	var thread app.ThreadUpdated
	decode(t, threads[2], &thread)
	if thread.LastRepliers[0].Handle == nil || *thread.LastRepliers[0].Handle != "member1" {
		t.Fatalf("replier = %+v, want the card of member1", thread.LastRepliers[0])
	}
}

func (f chatFixture) threadUpdates(t *testing.T) []testkit.RealtimePublish {
	t.Helper()
	var out []testkit.RealtimePublish
	for _, p := range f.rt.Published() {
		if p.Name == app.EventThreadUpdated {
			out = append(out, p)
		}
	}
	return out
}

func TestLastRepliers_theThreadUpdateCarriesThemAfterAProposalComment(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	logan, sam := f.member(0), f.member(1)
	f.reply(t, logan, "one", nil)
	since := len(f.rt.Published())
	second := f.reply(t, sam, "two", nil)
	got := f.wantEvents(t, since, app.EventMessageCreated, app.EventThreadUpdated)
	if have := lastRepliersOf(t, got[1]); !slices.Equal(have, userUUIDs(sam, logan)) {
		t.Fatalf("thread.updated after a comment = %v, want sam then logan", have)
	}
	since = len(f.rt.Published())
	if _, err := f.routes.DeleteFeedComment(asUser(t.Context(), sam), api.DeleteFeedCommentRequestObject{
		CommentId: second.Id,
	}); err != nil {
		t.Fatal(err)
	}
	got = f.wantEvents(t, since, app.EventMessageDeleted, app.EventThreadUpdated)
	if have := lastRepliersOf(t, got[1]); !slices.Equal(have, userUUIDs(logan)) {
		t.Fatalf("thread.updated after the delete = %v, want only logan", have)
	}
}

func TestLastRepliers_aChannelPageReadsThemInOneQuery(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	counter := &queryCounter{}
	counting := derivedPool(t, f.pool, func(c *pgxpool.Config) { c.ConnConfig.Tracer = counter })
	page := make([]app.ChatMessage, 0, 50)
	for range 50 {
		top := f.mustSend(t, f.member(0), "top", nil)
		f.mustSend(t, f.member(1), "reply", &domain.Reply{Parent: top.ID})
		page = append(page, top)
	}
	for i := range page {
		page[i].ReplyCount = 1
	}
	if err := app.WithLastRepliers(t.Context(), counting, f.member(2), page); err != nil {
		t.Fatal(err)
	}
	if n := counter.n.Load(); n != 1 {
		t.Fatalf("queries for 50 messages with replies = %d, want 1", n)
	}
	for _, m := range page {
		if !slices.Equal(m.LastRepliers, []ids.UserID{f.member(1)}) {
			t.Fatalf("last repliers = %v, want [%v]", m.LastRepliers, f.member(1))
		}
	}
	quiet := []app.ChatMessage{{ID: f.deps.IDs.NewV7()}}
	if err := app.WithLastRepliers(t.Context(), counting, f.member(2), quiet); err != nil || counter.n.Load() != 1 {
		t.Fatalf("a page without replies queried: %v, %d", err, counter.n.Load())
	}
}
