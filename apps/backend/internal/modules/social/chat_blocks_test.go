package social_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type blockedChat struct {
	chatRoutes
	top, blockedTop, thirdTop, blockedReply, thirdReply, blockedAlso, underBlocked, lone api.ChatMessage
}

func (f chatRoutes) blocks(t *testing.T, blocker, blocked ids.UserID) {
	t.Helper()
	insertBlock(t, f.pool, f.deps.IDs.NewV7(), blocker, blocked)
}

func insertBlock(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, blocker, blocked ids.UserID) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO user_blocks (id, blocker_id, blocked_id, created_at) VALUES ($1, $2, $3, now())`,
		id, blocker.UUID(), blocked.UUID()); err != nil {
		t.Fatal(err)
	}
}

func newBlockedChat(t *testing.T) blockedChat {
	t.Helper()
	f := blockedChat{chatRoutes: newChatRoutes(t)}
	post := func(author ids.UserID, body string, parent *api.ChatMessage, inChannel bool) api.ChatMessage {
		req := api.PostChatMessageRequest{Body: body}
		if parent != nil {
			req.ParentId, req.AlsoInChannel = &parent.Id, &inChannel
		}
		return f.mustPost(t, author, req)
	}
	f.top = post(f.member(0), "gm", nil, false)
	f.blockedTop = post(f.member(1), "from the blocked", nil, false)
	f.thirdTop = post(f.member(2), "from the third", nil, false)
	f.blockedReply = post(f.member(1), "blocked reply", &f.top, false)
	f.thirdReply = post(f.member(2), "third reply", &f.top, false)
	f.blockedAlso = post(f.member(1), "blocked also in channel", &f.top, true)
	f.underBlocked = post(f.member(2), "reply under the blocked", &f.blockedTop, false)
	f.lone = post(f.member(1), "nobody replied", nil, false)
	f.blocks(t, f.member(0), f.member(1))
	return f
}

func (f blockedChat) channelAs(t *testing.T, viewer ids.UserID, p api.GetChatMessagesParams) []api.ChatMessage {
	t.Helper()
	messages, err := f.channel(t, viewer, p)
	if err != nil {
		t.Fatal(err)
	}
	return messages
}

func TestChatMessages_HidesBlockedAuthor(t *testing.T) {
	t.Parallel()
	f := newBlockedChat(t)
	blocker, third := f.member(0), f.member(2)

	wantIDs(t, f.channelAs(t, blocker, api.GetChatMessagesParams{}), f.thirdTop, f.top)
	wantIDs(t, f.channelAs(t, third, api.GetChatMessagesParams{}),
		f.lone, f.blockedAlso, f.thirdTop, f.blockedTop, f.top)

	after := api.GetChatMessagesParams{After: &f.top.Id}
	wantIDs(t, f.channelAs(t, blocker, after), f.thirdTop)
	wantIDs(t, f.channelAs(t, third, after), f.blockedTop, f.thirdTop, f.blockedAlso, f.lone)
	paged := api.GetChatMessagesParams{After: &f.top.Id, Limit: ptr(1)}
	wantIDs(t, f.channelAs(t, blocker, paged), f.thirdTop)
	wantIDs(t, f.channelAs(t, third, paged), f.blockedTop)

	hidden, err := f.thread(t, blocker, f.top.Id, api.GetChatThreadParams{})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, hidden.Replies, f.thirdReply)
	if hidden.Parent.ReplyCount != 3 {
		t.Fatalf("reply_count = %d, want the hidden replies still counted", hidden.Parent.ReplyCount)
	}
	shown, err := f.thread(t, third, f.top.Id, api.GetChatThreadParams{})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, shown.Replies, f.blockedReply, f.thirdReply, f.blockedAlso)
}

func TestChatThread_ShowsABlockedParentAsDeleted(t *testing.T) {
	t.Parallel()
	f := newBlockedChat(t)
	masked, err := f.thread(t, f.member(0), f.blockedTop.Id, api.GetChatThreadParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !masked.Parent.Deleted || masked.Parent.Body != nil || masked.Parent.ReplyCount != 1 {
		t.Fatalf("parent = %+v, want deleted, with no body and its reply still counted", masked.Parent)
	}
	wantIDs(t, masked.Replies, f.underBlocked)
	_, err = f.thread(t, f.member(0), f.lone.Id, api.GetChatThreadParams{})
	wantCode(t, err, errs.CodeChatMessageNotFound)
	open, err := f.thread(t, f.member(2), f.lone.Id, api.GetChatThreadParams{})
	if err != nil || open.Parent.Deleted {
		t.Fatalf("a third member's read = %+v, %v; want the message", open.Parent, err)
	}
}

func TestChatThread_FailsWhenTheBlockCheckFails(t *testing.T) {
	t.Parallel()
	f := newBlockedChat(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE user_blocks CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err := f.thread(t, f.member(0), f.top.Id, api.GetChatThreadParams{})
	wantCode(t, err, errs.CodeInternal)
}
