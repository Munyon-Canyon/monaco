package social_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/feedtest"
)

type proposalThread struct {
	chatRoutes
	proposal uuid.UUID
	item     uuid.UUID
	card     uuid.UUID
}

func newProposalThread(t *testing.T) proposalThread {
	t.Helper()
	f := proposalThread{chatRoutes: newChatRoutes(t)}
	e := f.proposalEvent("buy")
	f.proposal = e.ProposalID
	f.item = feedtest.Item(t, f.pool, f.clock, f.deps.IDs, func(it *feed.Item) {
		it.Kind, it.RefID, it.CabalID, it.ActorID = feed.KindProposal, e.ProposalID, f.cabal.ID, f.member(0)
		it.Payload = feed.Payload{CabalName: "Alpha Cabal", Symbol: "NVDA", AssetName: "Nvidia", Action: feed.ActionBuy}
	})
	f.writeProposal(t, e, "put it up")
	f.card = f.proposalRows(t)[0].Id
	return f
}

func (f proposalThread) reply(t *testing.T, author ids.UserID, body string, parent *uuid.UUID) api.Comment {
	t.Helper()
	res, err := f.routes.PostProposalComment(asUser(t.Context(), author), api.PostProposalCommentRequestObject{
		Id: f.proposal, Body: &api.CreateCommentRequest{Body: &body, ParentCommentId: parent},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	return api.Comment(res.(api.PostProposalComment201JSONResponse))
}

func (f proposalThread) read(t *testing.T, viewer ids.UserID, p api.GetChatThreadParams) api.ChatThread {
	t.Helper()
	th, err := f.thread(t, viewer, f.card, p)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

func (f proposalThread) published(t *testing.T, since int) []testkit.RealtimePublish {
	t.Helper()
	return f.rt.Published()[since:]
}

func ids3(msgs []api.ChatMessage) []uuid.UUID {
	out := make([]uuid.UUID, len(msgs))
	for i, m := range msgs {
		out[i] = m.Id
	}
	return out
}

type threeReplies struct {
	proposalThread
	first, second, answer api.Comment
}

func newThreeReplies(t *testing.T) threeReplies {
	t.Helper()
	f := threeReplies{proposalThread: newProposalThread(t)}
	f.first = f.reply(t, f.member(0), "first", nil)
	f.second = f.reply(t, f.member(1), "second", nil)
	f.answer = f.reply(t, f.member(2), "answer", &f.first.Id)
	return f
}

func (f threeReplies) replies(t *testing.T, viewer ids.UserID) []api.ChatMessage {
	t.Helper()
	return f.read(t, viewer, api.GetChatThreadParams{Limit: ptr(50)}).Replies
}

func TestProposalThread_readsTheProposalsCommentsOldestFirst(t *testing.T) {
	t.Parallel()
	f := newThreeReplies(t)
	got := f.read(t, f.member(0), api.GetChatThreadParams{Limit: ptr(50)})
	if got.Parent.Id != f.card || got.Parent.ReplyCount != 3 || got.Parent.LastReplyAt == nil {
		t.Fatalf("parent = %+v, want the card with 3 replies", got.Parent)
	}
	want := []uuid.UUID{f.first.Id, f.second.Id, f.answer.Id}
	if have := ids3(got.Replies); !slices.Equal(have, want) {
		t.Fatalf("replies = %v, want %v", have, want)
	}
	for _, r := range got.Replies {
		wantCommentBacked(t, r, f.card)
	}
}

func wantCommentBacked(t *testing.T, r api.ChatMessage, card uuid.UUID) {
	t.Helper()
	if r.CommentId == nil || *r.CommentId != r.Id || r.ParentId == nil || *r.ParentId != card ||
		r.Kind != "user" || r.AlsoInChannel || r.ReplyCount != 0 || r.Deleted {
		t.Fatalf("reply = %+v, want a comment-backed reply under the card", r)
	}
}

func TestProposalThread_aReplyThatAnswersACommentCarriesItsQuote(t *testing.T) {
	t.Parallel()
	f := newThreeReplies(t)
	got := f.replies(t, f.member(0))
	if got[0].ReplyTo != nil || got[2].ReplyTo == nil || got[2].ReplyTo.CommentId != f.first.Id ||
		got[2].ReplyTo.AuthorDisplayName != "Kai" {
		t.Fatalf("quotes = %+v and %+v, want none on the first and the first comment on the answer",
			got[0].ReplyTo, got[2].ReplyTo)
	}
}

func TestProposalThread_repliesCarryTheCommentsReactions(t *testing.T) {
	t.Parallel()
	f := newThreeReplies(t)
	if _, err := f.routes.PutCommentReaction(asUser(t.Context(), f.member(1)), api.PutCommentReactionRequestObject{
		CommentId: f.first.Id, Emoji: laugh,
	}); err != nil {
		t.Fatal(err)
	}
	if r := f.replies(t, f.member(0))[0]; r.ReactionCount != 1 || len(r.Reactions) != 1 ||
		r.Reactions[0].Emoji != laugh || r.Reactions[0].Mine {
		t.Fatalf("reactions = %+v, want one %s not mine", r.Reactions, laugh)
	}
	if mine := f.replies(t, f.member(1))[0]; !mine.Reactions[0].Mine {
		t.Fatalf("reactions = %+v, want mine for the reactor", mine.Reactions)
	}
}

func (f proposalThread) pageIDs(t *testing.T, viewer ids.UserID) []uuid.UUID {
	t.Helper()
	page, err := app.ListComments(t.Context(), f.pool, app.CommentsQuery{
		FeedObjectID: f.item, Viewer: viewer, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	var out []uuid.UUID
	for _, th := range page.Threads {
		if !th.Comment.System && !th.Comment.Deleted {
			out = append(out, th.Comment.ID)
		}
		for _, r := range th.Replies {
			if !r.Deleted {
				out = append(out, r.ID)
			}
		}
	}
	return out
}

func TestProposalThread_matchesTheProposalPage(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	kai, sam := f.member(0), f.member(1)
	top := f.reply(t, kai, "top", nil)
	f.reply(t, sam, "folded", &top.Id)
	gone := f.reply(t, sam, "gone", nil)
	if _, err := f.routes.DeleteFeedComment(asUser(t.Context(), sam), api.DeleteFeedCommentRequestObject{
		CommentId: gone.Id,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO feed_comments
		(id, feed_object_id, author_kind, system_outcome, body, created_at)
		VALUES ($1, $2, 'system', 'closed', 'Closed.', $3)`, f.deps.IDs.NewV7(), f.item, f.clock.Now()); err != nil {
		t.Fatal(err)
	}

	pageIDs := f.pageIDs(t, kai)
	got := ids3(f.read(t, kai, api.GetChatThreadParams{Limit: ptr(50)}).Replies)
	if len(got) != 2 || !slices.Equal(got, pageIDs) {
		t.Fatalf("chat replies = %v, proposal page = %v, want the same two live user comments", got, pageIDs)
	}
}

func TestProposalThread_pagesWithBefore(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	all := make([]uuid.UUID, 0, 5)
	for _, body := range []string{"a", "b", "c", "d", "e"} {
		all = append(all, f.reply(t, f.member(0), body, nil).Id)
	}
	newest := f.read(t, f.member(0), api.GetChatThreadParams{Limit: ptr(2)}).Replies
	if have := ids3(newest); len(have) != 2 || have[0] != all[3] || have[1] != all[4] {
		t.Fatalf("newest page = %v, want %v", have, all[3:])
	}
	older := f.read(t, f.member(0), api.GetChatThreadParams{Limit: ptr(2), Before: &all[3]}).Replies
	if have := ids3(older); len(have) != 2 || have[0] != all[1] || have[1] != all[2] {
		t.Fatalf("older page = %v, want %v", have, all[1:3])
	}
	oldest := f.read(t, f.member(0), api.GetChatThreadParams{Limit: ptr(2), Before: &all[1]}).Replies
	if have := ids3(oldest); len(have) != 1 || have[0] != all[0] {
		t.Fatalf("oldest page = %v, want %v", have, all[:1])
	}
	_, err := f.thread(t, f.member(0), f.card, api.GetChatThreadParams{Limit: ptr(2), Before: ptr(f.card)})
	wantCode(t, err, errs.CodeChatMessageNotFound)
}

func TestProposalThread_leavesOutBlockedAuthors(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	kai, sam := f.member(0), f.member(1)
	f.reply(t, sam, "from the blocked", nil)
	keep := f.reply(t, f.member(2), "kept", nil)
	f.blocks(t, kai, sam)

	got := f.read(t, kai, api.GetChatThreadParams{Limit: ptr(50)})
	if have := ids3(got.Replies); len(have) != 1 || have[0] != keep.Id {
		t.Fatalf("replies = %v, want only %s", have, keep.Id)
	}
	if other := f.read(t, f.member(2), api.GetChatThreadParams{Limit: ptr(50)}); len(other.Replies) != 2 {
		t.Fatalf("replies for another member = %d, want 2", len(other.Replies))
	}
}

func TestProposalThread_aProposalWithoutAFeedItemHasNoReplies(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	e := f.proposalEvent("buy")
	f.writeProposal(t, e, "early")
	card := f.proposalRows(t)[0].Id
	got, err := f.thread(t, f.member(0), card, api.GetChatThreadParams{Limit: ptr(50)})
	if err != nil || got.Parent.Id != card || got.Parent.ReplyCount != 0 || len(got.Replies) != 0 {
		t.Fatalf("thread = %+v, %v, want the card with no replies", got, err)
	}
}

func TestProposalThread_theCardCountsTheProposalsLiveComments(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	kai := f.member(0)
	f.reply(t, kai, "one", nil)
	gone := f.reply(t, kai, "two", nil)
	last := f.reply(t, f.member(2), "three", nil)
	if _, err := f.routes.DeleteFeedComment(asUser(t.Context(), kai), api.DeleteFeedCommentRequestObject{
		CommentId: gone.Id,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.postAs(
		t,
		f.member(0),
		api.PostChatMessageRequest{ParentId: &f.card, Body: ptr("chat reply")},
	); err != nil {
		t.Fatal(err)
	}

	cards := f.proposalRows(t)
	if len(cards) != 1 || cards[0].ReplyCount != 2 || cards[0].LastReplyAt == nil ||
		!cards[0].LastReplyAt.Equal(last.CreatedAt) {
		t.Fatalf("card = %+v, want 2 replies and the newest comment's time %s", cards, last.CreatedAt)
	}
	if got := f.read(t, kai, api.GetChatThreadParams{Limit: ptr(50)}); got.Parent.ReplyCount != 2 ||
		len(got.Replies) != 2 {
		t.Fatalf("thread = %+v, want the same two", got)
	}
}

func TestProposalThread_aPinnedCardKeepsTheProposalsCount(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	f.reply(t, f.member(0), "one", nil)
	res, err := f.routes.PinChatMessage(asUser(t.Context(), f.cabal.Creator.ID), api.PinChatMessageRequestObject{
		Id: f.cabal.ID.UUID(), MessageId: f.card,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pinned := api.ChatMessage(res.(api.PinChatMessage200JSONResponse)); pinned.ReplyCount != 1 {
		t.Fatalf("pinned card reply_count = %d, want 1", pinned.ReplyCount)
	}
	pins, err := f.routes.GetChatPins(asUser(t.Context(), f.member(0)), api.GetChatPinsRequestObject{
		Id: f.cabal.ID.UUID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if list := pins.(api.GetChatPins200JSONResponse).Messages; len(list) != 1 || list[0].ReplyCount != 1 {
		t.Fatalf("pins = %+v, want the card with 1 reply", list)
	}
}

func TestProposalThread_aChatMessageThreadIsUnchanged(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	top := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("hello")})
	reply := f.mustPost(t, f.member(1), api.PostChatMessageRequest{Body: ptr("hi"), ParentId: &top.Id})
	got, err := f.thread(t, f.member(0), top.Id, api.GetChatThreadParams{Limit: ptr(50)})
	if err != nil || got.Parent.CommentId != nil || len(got.Replies) != 1 || got.Replies[0].Id != reply.Id ||
		got.Replies[0].CommentId != nil || got.Replies[0].ReplyTo != nil {
		t.Fatalf("thread = %+v, %v, want the chat reply with no comment fields", got, err)
	}
}

func (f proposalThread) wantEvents(t *testing.T, since int, names ...string) []testkit.RealtimePublish {
	t.Helper()
	got, channel := f.published(t, since), "cabal:"+f.cabal.ID.String()
	have := make([]string, 0, len(got))
	for _, p := range got {
		if p.Channel != channel {
			t.Fatalf("published on %s, want %s", p.Channel, channel)
		}
		have = append(have, p.Name)
	}
	if !slices.Equal(have, names) {
		t.Fatalf("published = %v, want %v", have, names)
	}
	return got
}

func decode(t *testing.T, p testkit.RealtimePublish, into any) {
	t.Helper()
	if err := json.Unmarshal(p.Data, into); err != nil {
		t.Fatal(err)
	}
}

func TestProposalThread_aCommentOnTheProposalReachesTheChatChannel(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	first := f.reply(t, f.member(0), "first", nil)

	since := len(f.rt.Published())
	answer := f.reply(t, f.member(2), "answer", &first.Id)
	got := f.wantEvents(t, since, app.EventMessageCreated, app.EventThreadUpdated)
	var created api.ChatMessage
	decode(t, got[0], &created)
	if created.Id != answer.Id || created.CommentId == nil || *created.CommentId != answer.Id ||
		created.ParentId == nil || *created.ParentId != f.card {
		t.Fatalf("message.created = %+v, want the answer as a reply under the card", created)
	}
	if created.ReplyTo == nil || created.ReplyTo.CommentId != first.Id || created.ReplyTo.AuthorDisplayName != "Kai" {
		t.Fatalf("reply_to = %+v, want the first comment's quote", created.ReplyTo)
	}
	var thread app.ThreadUpdated
	decode(t, got[1], &thread)
	if thread.MessageID != f.card || thread.ReplyCount != 2 || !thread.LastReplyAt.Equal(answer.CreatedAt) {
		t.Fatalf("thread.updated = %+v, want the card with 2 replies at %s", thread, answer.CreatedAt)
	}
}

func TestProposalThread_aDeletedOrRemovedCommentReachesTheChatChannel(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	kai := f.member(0)
	mine := f.reply(t, kai, "mine", nil)
	theirs := f.reply(t, f.member(2), "theirs", nil)

	since := len(f.rt.Published())
	if _, err := f.routes.DeleteFeedComment(asUser(t.Context(), kai), api.DeleteFeedCommentRequestObject{
		CommentId: mine.Id,
	}); err != nil {
		t.Fatal(err)
	}
	got := f.wantEvents(t, since, app.EventMessageDeleted, app.EventThreadUpdated)
	var deleted app.MessageDeleted
	var thread app.ThreadUpdated
	decode(t, got[0], &deleted)
	decode(t, got[1], &thread)
	if deleted.ID != mine.Id || thread.MessageID != f.card || thread.ReplyCount != 1 ||
		!thread.LastReplyAt.Equal(theirs.CreatedAt) {
		t.Fatalf(
			"published = %s and %s, want message.deleted for the comment and 1 reply left",
			got[0].Data,
			got[1].Data,
		)
	}

	since = len(f.rt.Published())
	if _, err := f.routes.RemoveComment(asAdmin(t), api.RemoveCommentRequestObject{
		Id: theirs.Id, Body: &apibase.ReasonBody{Reason: "spam"},
	}); err != nil {
		t.Fatal(err)
	}
	got = f.wantEvents(t, since, app.EventMessageDeleted, app.EventThreadUpdated)
	thread = app.ThreadUpdated{}
	decode(t, got[1], &thread)
	if thread.ReplyCount != 0 || !thread.LastReplyAt.IsZero() {
		t.Fatalf("thread.updated = %s, want no replies and no last_reply_at", got[1].Data)
	}
}

func TestProposalThread_aReactionOnACommentReachesTheChatChannel(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	c := f.reply(t, f.member(0), "react to me", nil)

	since := len(f.rt.Published())
	if _, err := f.routes.PutCommentReaction(asUser(t.Context(), f.member(1)), api.PutCommentReactionRequestObject{
		CommentId: c.Id, Emoji: laugh,
	}); err != nil {
		t.Fatal(err)
	}
	got := f.wantEvents(t, since, app.EventReactionUpdated)
	var updated app.ReactionUpdated
	decode(t, got[0], &updated)
	if updated.MessageID != c.Id || updated.Total != 1 || len(updated.Reactions) != 1 ||
		updated.Reactions[0].Emoji != laugh || updated.Reactions[0].Count != 1 {
		t.Fatalf("reaction.updated = %+v, want one %s on the comment", updated, laugh)
	}

	since = len(f.rt.Published())
	if _, err := f.routes.DeleteCommentReaction(
		asUser(t.Context(), f.member(1)),
		api.DeleteCommentReactionRequestObject{
			CommentId: c.Id, Emoji: laugh,
		},
	); err != nil {
		t.Fatal(err)
	}
	f.wantEvents(t, since, app.EventReactionUpdated)
}

func TestProposalThread_aCommentOnAnItemWithoutACardPublishesNothing(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	proposal := f.deps.IDs.NewV7()
	feedtest.Item(t, f.pool, f.clock, f.deps.IDs, func(it *feed.Item) {
		it.Kind, it.RefID, it.CabalID, it.ActorID = feed.KindProposal, proposal, f.cabal.ID, f.member(0)
		it.Payload = feed.Payload{CabalName: "Alpha Cabal", Symbol: "NVDA", AssetName: "Nvidia", Action: feed.ActionBuy}
	})
	body := "no card yet"
	if _, err := f.routes.PostProposalComment(asUser(t.Context(), f.member(0)), api.PostProposalCommentRequestObject{
		Id: proposal, Body: &api.CreateCommentRequest{Body: &body},
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.rt.Published(); len(got) != 0 {
		t.Fatalf("published = %+v, want nothing", got)
	}
}

func (f proposalThread) post(t *testing.T, author ids.UserID, req *api.CreateCommentRequest) api.Comment {
	t.Helper()
	res, err := f.routes.PostProposalComment(asUser(t.Context(), author), api.PostProposalCommentRequestObject{
		Id: f.proposal, Body: req,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	return api.Comment(res.(api.PostProposalComment201JSONResponse))
}

func TestProposalThread_aGifReadsAsAnAttachment(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	gif := f.post(t, f.member(0), gifRequest(nil, nil))
	got := f.read(t, f.member(1), api.GetChatThreadParams{Limit: ptr(50)}).Replies
	if len(got) != 1 || got[0].Id != gif.Id || got[0].Attachment == nil || got[0].Body == nil ||
		*got[0].Body != "GIF" {
		t.Fatalf("replies = %+v, want the GIF with the GIF body", got)
	}
	if g, err := got[0].Attachment.AsGifAttachment(); err != nil || g.Id != gifID {
		t.Fatalf("gif = %+v, %v, want %s", g, err, gifID)
	}
}

func TestProposalThread_aPollReadsWithTheReadersVote(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	ask := f.post(t, f.member(0), pollRequest("Trim to 40%?", []string{"Yes", "Wait"}, ptr(24)))
	if _, err := f.routes.PutCommentPollVote(asUser(t.Context(), f.member(1)), api.PutCommentPollVoteRequestObject{
		CommentId: ask.Id, Body: &api.PollVoteRequest{Position: 1},
	}); err != nil {
		t.Fatal(err)
	}
	got := f.read(t, f.member(1), api.GetChatThreadParams{Limit: ptr(50)}).Replies
	if len(got) != 1 || got[0].Id != ask.Id || got[0].Attachment == nil {
		t.Fatalf("replies = %+v, want the poll", got)
	}
	poll, err := got[0].Attachment.AsPollAttachment()
	if err != nil || poll.Total != 1 || len(poll.Options) != 2 || poll.Options[1].Count != 1 ||
		poll.MyPosition == nil || *poll.MyPosition != 1 {
		t.Fatalf("poll = %+v, %v, want one vote for the second option by the reader", poll, err)
	}
}

type stockStub struct {
	cards map[uuid.UUID]app.StockCard
	err   error
}

func (s stockStub) StockCards(context.Context, []uuid.UUID) (map[uuid.UUID]app.StockCard, error) {
	return s.cards, s.err
}

func TestLoadCommentViews_resolvesAStockOnACommentBackedReply(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	asset := f.deps.IDs.NewV7()
	stock := &domain.Attachment{Kind: domain.AttachmentStock, Stock: &domain.StockSnapshot{AssetID: asset}}
	msgs := func() []app.ChatMessage {
		return []app.ChatMessage{
			{ID: f.deps.IDs.NewV7()},
			{ID: f.deps.IDs.NewV7(), CommentID: f.deps.IDs.NewV7(), Attachment: stock},
		}
	}

	got := msgs()
	cards := stockStub{cards: map[uuid.UUID]app.StockCard{asset: {ID: asset, Symbol: "NVDA"}}}
	if err := app.LoadCommentViews(t.Context(), f.pool, cards, f.member(0), got, f.now); err != nil {
		t.Fatal(err)
	}
	if got[0].Stock != nil || got[1].Stock == nil || got[1].Stock.Symbol != "NVDA" {
		t.Fatalf("stocks = %+v and %+v, want one for the comment-backed reply only", got[0].Stock, got[1].Stock)
	}

	failing := stockStub{err: errs.New(errs.CodeUpstreamUnavailable, "test")}
	err := app.LoadCommentViews(t.Context(), f.pool, failing, f.member(0), msgs(), f.now)
	wantCode(t, err, errs.CodeUpstreamUnavailable)
}

func TestProposalThread_readsFailWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"counts":      `ALTER TABLE feed_comments RENAME COLUMN deleted_at TO gone_at`,
		"replies":     `ALTER TABLE comment_attachments RENAME TO attachments_gone`,
		"reactions":   `ALTER TABLE comment_reactions RENAME TO reactions_gone`,
		"feed lookup": `ALTER TABLE feed_objects RENAME COLUMN kind TO what`,
	}
	for name, breaker := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newProposalThread(t)
			f.reply(t, f.member(0), "one", nil)
			if _, err := f.pool.Exec(t.Context(), breaker); err != nil {
				t.Fatal(err)
			}
			_, err := f.thread(t, f.member(0), f.card, api.GetChatThreadParams{Limit: ptr(50)})
			wantCode(t, err, errs.CodeInternal)
		})
	}
}

func TestProposalThread_aPublishThatCannotFindTheCardIsLoggedAndTheCommentStands(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	if _, err := f.pool.Exec(
		t.Context(),
		`ALTER TABLE cabal_messages RENAME COLUMN proposal_id TO proposal`,
	); err != nil {
		t.Fatal(err)
	}
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(
		asUser(t.Context(), f.member(0)), observability.NewLogger(config.Config{Env: config.EnvTest}, logs),
	)
	body := "still stored"
	res, err := f.routes.PostProposalComment(ctx, api.PostProposalCommentRequestObject{
		Id: f.proposal, Body: &api.CreateCommentRequest{Body: &body},
	})
	if err != nil {
		t.Fatalf("PostProposalComment = %v, want the comment stored", err)
	}
	if _, ok := res.(api.PostProposalComment201JSONResponse); !ok {
		t.Fatalf("response = %#v", res)
	}
	if out := logs.Bytes(); !strings.Contains(string(out), publishFailedLog) {
		t.Fatalf("logs = %s, want %s", out, publishFailedLog)
	}
}

func TestProposalThread_aBlockedProposerWithNoRepliesLeavesNothingToOpen(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	f.blocks(t, f.member(1), f.member(0))
	_, err := f.thread(t, f.member(1), f.card, api.GetChatThreadParams{Limit: ptr(50)})
	wantCode(t, err, errs.CodeChatMessageNotFound)
}

func TestProposalThread_channelAndPinReadsFailWithInternalWhenTheCountsFail(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	f.reply(t, f.member(0), "one", nil)
	if _, err := f.routes.PinChatMessage(asUser(t.Context(), f.cabal.Creator.ID), api.PinChatMessageRequestObject{
		Id: f.cabal.ID.UUID(), MessageId: f.card,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE feed_comments RENAME COLUMN deleted_at TO gone_at`); err != nil {
		t.Fatal(err)
	}
	_, err := f.channel(t, f.member(0), api.GetChatMessagesParams{Limit: ptr(10)})
	wantCode(t, err, errs.CodeInternal)
	_, err = f.routes.GetChatPins(asUser(t.Context(), f.member(0)), api.GetChatPinsRequestObject{
		Id: f.cabal.ID.UUID(),
	})
	wantCode(t, err, errs.CodeInternal)
}

func TestLoadCommentViews_failsWhenThePollsCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	poll := &domain.Attachment{Kind: domain.AttachmentPoll, Poll: &domain.Poll{Question: "q"}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := app.LoadCommentViews(ctx, f.pool, stockStub{}, f.member(0), []app.ChatMessage{
		{ID: f.deps.IDs.NewV7(), CommentID: f.deps.IDs.NewV7(), Attachment: poll},
	}, f.now)
	wantCode(t, err, errs.CodeInternal)
}

func TestProposalThread_removingMonacosNotePublishesNothing(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	note := f.deps.IDs.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO feed_comments
		(id, feed_object_id, author_kind, system_outcome, body, created_at)
		VALUES ($1, $2, 'system', 'closed', 'Closed.', $3)`, note, f.item, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	since := len(f.rt.Published())
	if _, err := f.routes.RemoveComment(asAdmin(t), api.RemoveCommentRequestObject{
		Id: note, Body: &apibase.ReasonBody{Reason: "spam"},
	}); err != nil {
		t.Fatal(err)
	}
	f.wantEvents(t, since)
}

func TestProposalThread_aReactionSummaryFailureIsLoggedAfterTheCommit(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	c := f.reply(t, f.member(0), "react to me", nil)
	if _, err := f.routes.PutCommentReaction(asUser(t.Context(), f.member(1)), api.PutCommentReactionRequestObject{
		CommentId: c.Id, Emoji: laugh,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE comment_reactions RENAME COLUMN created_at TO at`); err != nil {
		t.Fatal(err)
	}
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(
		asUser(t.Context(), f.member(1)), observability.NewLogger(config.Config{Env: config.EnvTest}, logs),
	)
	_, err := f.routes.DeleteCommentReaction(ctx, api.DeleteCommentReactionRequestObject{CommentId: c.Id, Emoji: laugh})
	wantCode(t, err, errs.CodeInternal)
	if out := string(logs.Bytes()); !strings.Contains(out, publishFailedLog) ||
		!strings.Contains(out, app.EventReactionUpdated) {
		t.Fatalf("logs = %s, want %s naming %s", out, publishFailedLog, app.EventReactionUpdated)
	}
}
