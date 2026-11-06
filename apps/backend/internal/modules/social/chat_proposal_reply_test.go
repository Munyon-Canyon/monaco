package social_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f proposalThread) replyInChat(
	t *testing.T, author ids.UserID, body string, alsoInChannel bool,
) api.ChatMessage {
	t.Helper()
	return f.mustPost(t, author, api.PostChatMessageRequest{
		Body: &body, ParentId: &f.card, AlsoInChannel: &alsoInChannel,
	})
}

func (f proposalThread) rowsUnderCard(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM cabal_messages WHERE parent_id = $1`, f.card)
}

func (f proposalThread) copies(t *testing.T) []api.ChatMessage {
	t.Helper()
	var out []api.ChatMessage
	for _, m := range f.mustChannel(t) {
		if m.ParentId != nil && *m.ParentId == f.card {
			out = append(out, m)
		}
	}
	return out
}

func (f proposalThread) pageComments(t *testing.T) []uuid.UUID {
	t.Helper()
	res, err := f.routes.GetProposalComments(asUser(t.Context(), f.member(2)), api.GetProposalCommentsRequestObject{
		Id: f.proposal,
	})
	if err != nil {
		t.Fatal(err)
	}
	items := res.(api.GetProposalComments200JSONResponse).Items
	out := make([]uuid.UUID, len(items))
	for i, item := range items {
		out[i] = item.Comment.Id
	}
	return out
}

func reasonOf(err error) string {
	for _, attr := range errs.Detail(err) {
		if attr.Key == "reason" {
			return attr.Value.String()
		}
	}
	return ""
}

func wantCommentReply(t *testing.T, got api.ChatMessage, card uuid.UUID, also bool) {
	t.Helper()
	if got.CommentId == nil || *got.CommentId != got.Id || got.ParentId == nil || *got.ParentId != card ||
		got.AlsoInChannel != also || got.Body == nil || *got.Body != "agree" {
		t.Fatalf("reply = %+v, want a comment-backed reply under the card", got)
	}
}

func (f proposalThread) wantOneReplyCounted(t *testing.T, comment api.ChatMessage) {
	t.Helper()
	if page := f.pageComments(t); len(page) != 1 || page[0] != comment.Id {
		t.Fatalf("proposal comments = %v, want the reply", page)
	}
	th := f.read(t, f.member(2), api.GetChatThreadParams{Limit: ptr(50)})
	if th.Parent.ReplyCount != 1 || len(th.Replies) != 1 || th.Replies[0].Id != comment.Id {
		t.Fatalf("thread = %+v, want the reply counted once", th)
	}
}

func TestChatReplyToProposal_isTheProposalsComment(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	got := f.replyInChat(t, f.member(1), "agree", false)
	wantCommentReply(t, got, f.card, false)
	if rows := f.rowsUnderCard(t); rows != 0 {
		t.Fatalf("%d chat rows under the card, want none: the reply is a comment", rows)
	}
	f.wantOneReplyCounted(t, got)
	if copies := f.copies(t); len(copies) != 0 {
		t.Fatalf("channel copies = %+v, want none while the switch is off", copies)
	}
}

func (f proposalThread) wantOneCopyOf(t *testing.T, comment, copyID api.ChatMessage) {
	t.Helper()
	copies := f.copies(t)
	if len(copies) != 1 || copies[0].Id != copyID.Id || copies[0].CommentId == nil ||
		*copies[0].CommentId != comment.Id || !copies[0].AlsoInChannel || copies[0].Body == nil ||
		*copies[0].Body != "agree" {
		t.Fatalf("channel copies = %+v, want the one copy of the comment", copies)
	}
	if rows := f.rowsUnderCard(t); rows != 1 {
		t.Fatalf("%d chat rows under the card, want just the copy", rows)
	}
	if cards := f.proposalRows(t); cards[0].ReplyCount != 1 {
		t.Fatalf("card reply_count = %d, want the comment counted once", cards[0].ReplyCount)
	}
}

func TestChatReplyToProposal_alsoInChannelWritesOneCopy(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	since := len(f.rt.Published())
	got := f.replyInChat(t, f.member(1), "agree", true)
	wantCommentReply(t, got, f.card, false)

	published := f.wantEvents(t, since, app.EventMessageCreated, app.EventThreadUpdated, app.EventMessageCreated)
	var created api.ChatMessage
	decode(t, published[2], &created)
	if created.Id == got.Id || created.CommentId == nil || *created.CommentId != got.Id ||
		created.ParentId == nil || *created.ParentId != f.card || !created.AlsoInChannel {
		t.Fatalf("message.created for the copy = %+v, want its own id and the comment's", created)
	}
	f.wantOneCopyOf(t, got, created)
	f.wantOneReplyCounted(t, got)
}

func TestChatReplyToProposal_theCopySharesTheCommentsReactions(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	got := f.replyInChat(t, f.member(1), "agree", true)
	if _, err := f.routes.PutCommentReaction(asUser(t.Context(), f.member(2)), api.PutCommentReactionRequestObject{
		CommentId: got.Id, Emoji: laugh,
	}); err != nil {
		t.Fatal(err)
	}
	copies := f.copies(t)
	if len(copies) != 1 || copies[0].ReactionCount != 1 || len(copies[0].Reactions) != 1 ||
		copies[0].Reactions[0].Emoji != laugh || !copies[0].Reactions[0].Mine {
		t.Fatalf("copy = %+v, want the comment's one reaction", copies)
	}
	th := f.read(t, f.member(2), api.GetChatThreadParams{Limit: ptr(50)})
	if th.Replies[0].ReactionCount != 1 {
		t.Fatalf("comment reply = %+v, want the same one reaction", th.Replies[0])
	}
}

func TestChatReplyToProposal_theCopyGoesWithItsComment(t *testing.T) {
	t.Parallel()
	for name, remove := range map[string]func(*testing.T, proposalThread, api.ChatMessage){
		"deleted by its author": func(t *testing.T, f proposalThread, c api.ChatMessage) {
			t.Helper()
			_, err := f.routes.DeleteFeedComment(asUser(t.Context(), f.member(1)), api.DeleteFeedCommentRequestObject{
				CommentId: c.Id,
			})
			if err != nil {
				t.Fatal(err)
			}
		},
		"removed by a moderator": func(t *testing.T, f proposalThread, c api.ChatMessage) {
			t.Helper()
			_, err := f.routes.RemoveComment(asAdmin(t), api.RemoveCommentRequestObject{
				Id: c.Id, Body: &apibase.ReasonBody{Reason: "spam"},
			})
			if err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newProposalThread(t)
			got := f.replyInChat(t, f.member(1), "agree", true)
			copyID := f.copies(t)[0].Id

			since := len(f.rt.Published())
			remove(t, f, got)

			published := f.wantEvents(
				t, since, app.EventMessageDeleted, app.EventMessageDeleted, app.EventThreadUpdated,
			)
			var gone app.MessageDeleted
			decode(t, published[0], &gone)
			if gone.ID != copyID {
				t.Fatalf("message.deleted = %s, want the copy %s", published[0].Data, copyID)
			}
			if copies := f.copies(t); len(copies) != 0 {
				t.Fatalf("channel copies = %+v, want none after the comment went", copies)
			}
			if n := f.count(
				t, `SELECT count(*) FROM cabal_messages WHERE id = $1 AND deleted_at IS NOT NULL`, copyID,
			); n != 1 {
				t.Fatalf("copy soft-deleted rows = %d, want 1", n)
			}
		})
	}
}

func TestChatReplyToProposal_oneNotificationPerRecipient(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	f.reply(t, f.member(2), "earlier", nil)
	before := f.posted(t)

	f.replyInChat(t, f.member(1), "quiet", false)
	if got := f.posted(t); got != before {
		t.Fatalf("chat.message_posted = %d, want %d: a comment is announced by comment.created alone", got, before)
	}
	if got := f.count(t, `SELECT count(*) FROM events WHERE type = $1`, string(events.TypeCommentCreated)); got != 2 {
		t.Fatalf("comment.created = %d, want 2", got)
	}

	f.replyInChat(t, f.member(1), "loud", true)
	if got := f.posted(t); got != before+1 {
		t.Fatalf("chat.message_posted = %d, want exactly one for the copy", got)
	}
	ev := f.lastPosted(t)
	want := []uuid.UUID{f.member(2).UUID()}
	if len(ev.MentionedUserIDs) != 0 || ev.MentionedUserIDs == nil || len(ev.ThreadParticipantIDs) != 1 ||
		ev.ThreadParticipantIDs[0] != want[0] || ev.ParentID == nil || *ev.ParentID != f.card || !ev.AlsoInChannel {
		t.Fatalf(
			"event = %+v, want only the earlier commenter: the proposer is told by the comment, the replier by nobody",
			ev,
		)
	}
}

func TestChatReplyToProposal_aBodyOverTheCommentLimitIsRefused(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	for _, body := range []string{strings.Repeat("a", 1001), strings.Repeat("é", 1001)} {
		_, err := f.postAs(t, f.member(1), api.PostChatMessageRequest{Body: &body, ParentId: &f.card})
		if errs.CodeOf(err) != errs.CodeInvalidInput || reasonOf(err) != "comment_too_long" {
			t.Fatalf("err = %v, want invalid_input with reason comment_too_long", err)
		}
	}
	edge := strings.Repeat("é", 1000)
	if _, err := f.postAs(t, f.member(1), api.PostChatMessageRequest{Body: &edge, ParentId: &f.card}); err != nil {
		t.Fatalf("1000 scalars: %v, want the comment limit to include them", err)
	}
	if page := f.pageComments(t); len(page) != 1 {
		t.Fatalf("proposal comments = %v, want only the 1000-scalar reply", page)
	}
}

func TestChatReplyToProposal_aProposalWithoutAFeedItemIsPending(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	e := f.proposalEvent("buy")
	f.writeProposal(t, e, "thesis")
	card := f.proposalRows(t)[0].Id
	_, err := f.postAs(t, f.member(1), api.PostChatMessageRequest{Body: ptr("hi"), ParentId: &card})
	if errs.CodeOf(err) != errs.CodeFeedItemPending {
		t.Fatalf("err = %v, want feed_item_pending", err)
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE parent_id = $1`, card); n != 0 {
		t.Fatalf("%d chat rows under the card, want none", n)
	}
}

func TestChatReplyToProposal_aDeletedCardIsNotFound(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	_, err := f.pool.Exec(t.Context(), `UPDATE cabal_messages SET deleted_at = now() WHERE id = $1`, f.card)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.postAs(t, f.member(1), api.PostChatMessageRequest{Body: ptr("hi"), ParentId: &f.card})
	if errs.CodeOf(err) != errs.CodeChatParentNotFound {
		t.Fatalf("err = %v, want chat_parent_not_found", err)
	}
}

func TestChatReplyToProposal_onlyAMemberMayReply(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	outsider := f.deps.IDs.NewV7()
	_, err := f.postAs(t, ids.UserIDFrom(outsider), api.PostChatMessageRequest{Body: ptr("hi"), ParentId: &f.card})
	if errs.CodeOf(err) != errs.CodeNotCabalMember {
		t.Fatalf("err = %v, want not_cabal_member", err)
	}
}

func TestChatReplyToProposal_attachmentsFollowTheCommentRules(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	gif, err := f.postAs(t, f.member(1), api.PostChatMessageRequest{
		ParentId: &f.card, Attachment: chatGif(), AlsoInChannel: ptr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gif.CommentId == nil || gif.Attachment == nil || gif.Body == nil || *gif.Body != "GIF" {
		t.Fatalf("gif reply = %+v, want a comment with its attachment", gif)
	}
	copies := f.copies(t)
	if len(copies) != 1 || copies[0].Body == nil || *copies[0].Body != "GIF" || copies[0].Attachment != nil {
		t.Fatalf("copy = %+v, want the attachment's display text only", copies)
	}

	_, err = f.postAs(t, f.member(1), api.PostChatMessageRequest{
		ParentId: &f.card, Attachment: chatThesis(f.proposal, ptr(0), ptr(3)),
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input: the proposal page refuses a quote of no thesis", err)
	}
	if page := f.pageComments(t); len(page) != 1 {
		t.Fatalf("proposal comments = %v, want only the gif", page)
	}
}

func TestChatReplyToProposal_repliesToOtherMessagesStayChatReplies(t *testing.T) {
	t.Parallel()
	f := newProposalThread(t)
	top := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("gm")})
	reply := f.mustPost(t, f.member(1), api.PostChatMessageRequest{
		Body: ptr("gn"), ParentId: &top.Id, AlsoInChannel: ptr(true),
	})
	if reply.CommentId != nil || reply.ParentId == nil || *reply.ParentId != top.Id || !reply.AlsoInChannel {
		t.Fatalf("reply = %+v, want an ordinary chat reply", reply)
	}
}

func TestChatReplyToProposal_failWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		breaker string
		run     func(*testing.T, proposalThread) error
	}{
		"the card lookup": {
			`DROP TABLE cabal_messages CASCADE`,
			func(t *testing.T, f proposalThread) error {
				t.Helper()
				_, err := f.postAs(t, f.member(1), api.PostChatMessageRequest{Body: ptr("hi"), ParentId: &f.card})
				return err
			},
		},
		"the copy insert": {
			`ALTER TABLE cabal_messages ADD CONSTRAINT no_copies CHECK (comment_id IS NULL) NOT VALID`,
			func(t *testing.T, f proposalThread) error {
				t.Helper()
				_, err := f.postAs(t, f.member(1), api.PostChatMessageRequest{
					Body: ptr("hi"), ParentId: &f.card, AlsoInChannel: ptr(true),
				})
				return err
			},
		},
		"the participants read": {
			`CREATE FUNCTION arm_reads() RETURNS trigger LANGUAGE plpgsql AS $$
			   BEGIN PERFORM set_config('chat.reads', 'broken', true); RETURN NULL; END $$;
			 CREATE TRIGGER arm_reads AFTER INSERT ON cabal_messages FOR EACH ROW EXECUTE FUNCTION arm_reads();
			 ALTER TABLE cabal_messages ENABLE ROW LEVEL SECURITY;
			 ALTER TABLE cabal_messages FORCE ROW LEVEL SECURITY;
			 CREATE POLICY open ON cabal_messages USING (true) WITH CHECK (true);
			 CREATE POLICY broken_reads ON cabal_messages AS RESTRICTIVE FOR SELECT
			   USING (1 / (CASE WHEN current_setting('chat.reads', true) = 'broken' THEN 0 ELSE 1 END) = 1)`,
			func(t *testing.T, f proposalThread) error {
				t.Helper()
				_, err := f.postAs(t, f.member(1), api.PostChatMessageRequest{
					Body: ptr("hi"), ParentId: &f.card, AlsoInChannel: ptr(true),
				})
				return err
			},
		},
		"the copy's event": {
			`ALTER TABLE events ADD CONSTRAINT no_chat_events CHECK (type <> 'chat.message_posted') NOT VALID`,
			func(t *testing.T, f proposalThread) error {
				t.Helper()
				_, err := f.postAs(t, f.member(1), api.PostChatMessageRequest{
					Body: ptr("hi"), ParentId: &f.card, AlsoInChannel: ptr(true),
				})
				return err
			},
		},
		"the copy's removal": {
			`ALTER TABLE cabal_messages ADD CONSTRAINT keep_copies CHECK (deleted_at IS NULL) NOT VALID`,
			func(t *testing.T, f proposalThread) error {
				t.Helper()
				c := f.replyInChat(t, f.member(1), "hi", true)
				_, err := f.routes.DeleteFeedComment(
					asUser(t.Context(), f.member(1)),
					api.DeleteFeedCommentRequestObject{
						CommentId: c.Id,
					},
				)
				return err
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newProposalThread(t)
			if _, err := f.pool.Exec(t.Context(), tt.breaker); err != nil {
				t.Fatal(err)
			}
			wantCode(t, tt.run(t, f), errs.CodeInternal)
		})
	}
}
