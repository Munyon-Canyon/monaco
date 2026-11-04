package social_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	laugh = "😂"
	fire  = "🔥"
)

type chatReactor struct {
	chatFixture
	react   *app.ReactChatMessageHandler
	unreact *app.UnreactChatMessageHandler
}

func newChatReactor(t *testing.T) chatReactor {
	t.Helper()
	f := newChatFixture(t)
	return chatReactor{
		chatFixture: f,
		react:       app.NewReactChatMessageHandler(f.deps),
		unreact:     app.NewUnreactChatMessageHandler(f.deps),
	}
}

func (f chatReactor) cmd(message uuid.UUID, viewer ids.UserID, emoji string) app.ReactChatMessage {
	return app.ReactChatMessage{CabalID: f.cabal.ID, MessageID: message, Viewer: viewer, Emoji: emoji}
}

func (f chatReactor) add(t *testing.T, message uuid.UUID, viewer ids.UserID, emoji string) app.ReactionSummary {
	t.Helper()
	got, err := f.react.Handle(f.as(t, viewer), f.cmd(message, viewer, emoji))
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	return got
}

func (f chatReactor) rows(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM chat_message_reactions WHERE cabal_id = $1`, f.cabal.ID.UUID())
}

func (f chatReactor) eventCount(t *testing.T, typ events.Type) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM events WHERE type = $1`, string(typ))
}

func heldOf(emoji string, count int32, mine bool) app.Reaction {
	return app.Reaction{Emoji: domain.Emoji(emoji), Count: count, Mine: mine}
}

func TestChatReactions_addAndRemoveKeepOneRowPerMemberAndEmojiAndReturnTheSummary(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	m := f.mustSend(t, f.member(0), "gm", nil)
	f.addTwice(t, m.ID)
	f.add(t, m.ID, f.member(1), fire)
	got := f.add(t, m.ID, f.member(1), laugh)
	if got.Total != 3 || len(got.Reactions) != 2 || got.Reactions[0] != heldOf(laugh, 2, true) ||
		got.Reactions[1] != heldOf(fire, 1, true) {
		t.Fatalf("summary = %+v, want the laugh first with 2 and the fire with 1", got)
	}
	f.removeTwice(t, m.ID)
	if rows := f.rows(t); rows != 2 {
		t.Fatalf("%d rows, want 2", rows)
	}
	if added, removed := f.eventCount(t, events.TypeChatReactionAdded),
		f.eventCount(t, events.TypeChatReactionRemoved); added != 3 || removed != 1 {
		t.Fatalf("events added = %d, removed = %d, want 3 and 1", added, removed)
	}
}

func (f chatReactor) addTwice(t *testing.T, message uuid.UUID) {
	t.Helper()
	first := f.add(t, message, f.member(0), laugh)
	again := f.add(t, message, f.member(0), laugh)
	if first.Total != 1 || len(first.Reactions) != 1 || first.Reactions[0] != heldOf(laugh, 1, true) ||
		again.Total != first.Total || again.Reactions[0] != first.Reactions[0] {
		t.Fatalf("first = %+v, again = %+v, want one mine reaction twice", first, again)
	}
}

func (f chatReactor) removeTwice(t *testing.T, message uuid.UUID) {
	t.Helper()
	after, err := f.unreact.Handle(f.as(t, f.member(1)), f.cmd(message, f.member(1), laugh))
	if err != nil || after.Total != 2 || after.Reactions[0] != heldOf(laugh, 1, false) {
		t.Fatalf("unreact = %+v, %v, want only the caller's laugh gone", after, err)
	}
	same, err := f.unreact.Handle(f.as(t, f.member(1)), f.cmd(message, f.member(1), laugh))
	if err != nil || same.Total != after.Total {
		t.Fatalf("second unreact = %+v, %v, want the same summary", same, err)
	}
}

func TestChatReactions_aReplyTakesReactionsAndTheEventNamesTheAuthor(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	reply := f.mustSend(t, f.member(1), "reply", &domain.Reply{Parent: top.ID})
	f.add(t, reply.ID, f.member(2), fire)
	var payload json.RawMessage
	if err := f.pool.QueryRow(t.Context(), `SELECT payload FROM events WHERE type = $1`,
		string(events.TypeChatReactionAdded)).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var got events.ChatReactionAdded
	if err := json.Unmarshal(payload, &got); err != nil || got.AuthorID == nil || *got.AuthorID != f.member(1).UUID() ||
		got.UserID != f.member(2).UUID() || got.MessageID != reply.ID || got.Emoji != fire {
		t.Fatalf("payload = %s, %v, want the reply, its author and the reactor", payload, err)
	}
}

func TestChatReactions_refuseWhatTheyCannotAnchor(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	live := f.mustSend(t, f.member(0), "live", nil)
	gone := f.mustSend(t, f.member(0), "gone", nil)
	if err := f.del.Handle(f.as(t, f.member(0)), app.DeleteChatMessage{
		CabalID: f.cabal.ID, MessageID: gone.ID, Caller: f.member(0),
	}); err != nil {
		t.Fatal(err)
	}
	elsewhere := app.ReactChatMessage{
		CabalID: f.other.ID, MessageID: live.ID, Viewer: f.other.Creator.ID, Emoji: laugh,
	}
	for name, tt := range map[string]struct {
		cmd  app.ReactChatMessage
		want errs.Code
	}{
		"a non-member":          {f.cmd(live.ID, f.outsider, laugh), errs.CodeNotCabalMember},
		"an unknown message":    {f.cmd(uuid.Nil, f.member(0), laugh), errs.CodeChatMessageNotFound},
		"a deleted message":     {f.cmd(gone.ID, f.member(0), laugh), errs.CodeChatMessageNotFound},
		"another cabal":         {elsewhere, errs.CodeChatMessageNotFound},
		"text that is no emoji": {f.cmd(live.ID, f.member(0), "nope"), errs.CodeInvalidInput},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := f.react.Handle(f.as(t, tt.cmd.Viewer), tt.cmd)
			wantCode(t, err, tt.want)
			_, err = f.unreact.Handle(f.as(t, tt.cmd.Viewer), tt.cmd)
			wantCode(t, err, tt.want)
		})
	}
	if rows := f.rows(t); rows != 0 {
		t.Fatalf("%d rows after refused calls, want 0", rows)
	}
}

func TestChatReactions_anEleventhDistinctEmojiIsRefusedAndRolledBack(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	m := f.mustSend(t, f.member(0), "gm", nil)
	for _, e := range []string{"😀", "😃", "😄", "😁", "😆", "😅", "😂", "🤣", "😊", "😇"} {
		f.add(t, m.ID, f.member(1), e)
	}
	_, err := f.react.Handle(f.as(t, f.member(1)), f.cmd(m.ID, f.member(1), "🙂"))
	wantCode(t, err, errs.CodeInvalidInput)
	var typed *errs.Error
	if !errors.As(err, &typed) || len(typed.Attrs) != 1 || typed.Attrs[0].Key != "reason" ||
		typed.Attrs[0].Value.String() != "reaction_limit" {
		t.Fatalf("err = %v, want reason=reaction_limit", err)
	}
	if rows := f.rows(t); rows != 10 {
		t.Fatalf("%d rows, want 10", rows)
	}
	if again := f.add(t, m.ID, f.member(1), "😀"); again.Total != 10 {
		t.Fatalf("repeating a held emoji at the limit = %+v, want it to change nothing", again)
	}
}

func TestChatReactions_aChangePublishesCountsWithoutMineAndANoOpPublishesNothing(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	m := f.mustSend(t, f.member(0), "gm", nil)
	before := len(f.rt.Published())
	f.add(t, m.ID, f.member(1), laugh)
	f.add(t, m.ID, f.member(1), laugh)
	published := f.rt.Published()
	if len(published) != before+1 {
		t.Fatalf("published = %+v, want one reaction.updated for two identical calls", published)
	}
	last := published[len(published)-1]
	want := `{"message_id":"` + m.ID.String() + `","reactions":[{"emoji":"` + laugh + `","count":1}],"total":1}`
	if last.Channel != f.channel() || last.Name != app.EventReactionUpdated || string(last.Data) != want {
		t.Fatalf("published = %+v (%s), want %s on %s", last, last.Data, want, f.channel())
	}
	if _, err := f.unreact.Handle(f.as(t, f.member(1)), f.cmd(m.ID, f.member(1), laugh)); err != nil {
		t.Fatal(err)
	}
	last = f.rt.Published()[before+1]
	if last.Name != app.EventReactionUpdated || !strings.Contains(string(last.Data), `"reactions":[],"total":0`) {
		t.Fatalf("published = %+v (%s), want an empty reaction.updated", last, last.Data)
	}
}

func TestChatReactions_aFailedPublishStillSavesTheReactionAndLogsIt(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	m := f.mustSend(t, f.member(0), "gm", nil)
	f.rt.Fail(errs.New(errs.CodeUpstreamUnavailable, "test.ably"))
	ctx, logs := f.loggedCtx(t, f.member(1))
	got, err := f.react.Handle(ctx, f.cmd(m.ID, f.member(1), laugh))
	if err != nil || got.Total != 1 {
		t.Fatalf("Handle = %+v, %v, want the saved reaction and no error", got, err)
	}
	if out := string(logs.Bytes()); strings.Count(out, publishFailedLog) != 1 ||
		!strings.Contains(out, app.EventReactionUpdated) {
		t.Fatalf("logs = %s, want one %s naming %s", out, publishFailedLog, app.EventReactionUpdated)
	}
}

func (f chatRoutes) reactVia(t *testing.T, viewer ids.UserID, id uuid.UUID, emoji string) {
	t.Helper()
	res, err := f.routes.ReactToChatMessage(asUser(t.Context(), viewer), api.ReactToChatMessageRequestObject{
		Id: f.cabal.ID.UUID(), MessageId: id, Emoji: emoji,
	})
	if _, ok := res.(api.ReactToChatMessage200JSONResponse); err != nil || !ok {
		t.Fatalf("react = %#v, %v", res, err)
	}
}

func (f chatRoutes) wantChannelLaughs(t *testing.T, viewer ids.UserID, mine bool) {
	t.Helper()
	page, err := f.channel(t, viewer, api.GetChatMessagesParams{Limit: ptr(10)})
	if err != nil || len(page) != 1 || page[0].ReactionCount != 2 || len(page[0].Reactions) != 1 ||
		page[0].Reactions[0].Count != 2 || page[0].Reactions[0].Mine != mine {
		t.Fatalf("channel = %+v, %v, want 2 laughs with mine = %v", page, err, mine)
	}
}

func TestChatReactions_routesCarryThemOnEveryMessageOfTheChannelThreadAndPost(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	top := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("top")})
	if top.Reactions == nil || len(top.Reactions) != 0 || top.ReactionCount != 0 {
		t.Fatalf("a new message = %+v, want empty reactions", top)
	}
	reply := f.mustPost(t, f.member(1), api.PostChatMessageRequest{Body: ptr("reply"), ParentId: &top.Id})
	f.reactVia(t, f.member(0), top.Id, laugh)
	f.reactVia(t, f.member(1), top.Id, laugh)
	f.reactVia(t, f.member(1), reply.Id, fire)
	f.wantChannelLaughs(t, f.member(0), true)
	f.wantChannelLaughs(t, f.member(2), false)
	thread, err := f.thread(t, f.member(1), top.Id, api.GetChatThreadParams{Limit: ptr(10)})
	if err != nil || thread.Parent.ReactionCount != 2 || len(thread.Replies) != 1 ||
		thread.Replies[0].ReactionCount != 1 || thread.Replies[0].Reactions[0].Emoji != fire {
		t.Fatalf("thread = %+v, %v, want the parent's 2 and the reply's 1", thread, err)
	}
}

func TestChatReactions_unreactingViaTheRouteAndDeletingAMessageDropTheirReactions(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	top := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("top")})
	f.reactVia(t, f.member(0), top.Id, laugh)
	f.reactVia(t, f.member(1), top.Id, laugh)
	res, err := f.routes.UnreactToChatMessage(asUser(t.Context(), f.member(1)), api.UnreactToChatMessageRequestObject{
		Id: f.cabal.ID.UUID(), MessageId: top.Id, Emoji: laugh,
	})
	if got, ok := res.(api.UnreactToChatMessage200JSONResponse); err != nil || !ok || got.Total != 1 {
		t.Fatalf("unreact = %#v, %v, want a total of 1", res, err)
	}
	f.mustPost(t, f.member(2), api.PostChatMessageRequest{Body: ptr("reply"), ParentId: &top.Id})
	if err := f.remove(t, f.member(0), top.Id); err != nil {
		t.Fatal(err)
	}
	gone, err := f.thread(t, f.member(1), top.Id, api.GetChatThreadParams{Limit: ptr(10)})
	if err != nil || !gone.Parent.Deleted || len(gone.Parent.Reactions) != 0 || gone.Parent.ReactionCount != 0 {
		t.Fatalf("a deleted parent = %+v, %v, want no reactions", gone.Parent, err)
	}
}

func TestChatReactions_routesRefuseAnonymousCallers(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	_, err := f.routes.ReactToChatMessage(t.Context(), api.ReactToChatMessageRequestObject{})
	wantCode(t, err, errs.CodeUnauthorized)
	_, err = f.routes.UnreactToChatMessage(t.Context(), api.UnreactToChatMessageRequestObject{})
	wantCode(t, err, errs.CodeUnauthorized)
}

func TestChatReactions_aMemberLeavingTakesTheirReactionsAndKeepsTheOthers(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	m := f.mustSend(t, f.member(0), "gm", nil)
	f.add(t, m.ID, f.member(1), laugh)
	f.add(t, m.ID, f.member(1), fire)
	f.add(t, m.ID, f.member(2), laugh)
	for range 2 {
		if err := f.memberLeft(t, f.member(1)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := app.ChatReactionSummary(t.Context(), f.pool, f.member(2), m.ID)
	if err != nil || got.Total != 1 || got.Reactions[0] != heldOf(laugh, 1, true) {
		t.Fatalf("summary = %+v, %v, want only the stayer's laugh", got, err)
	}
}

func TestChatReactions_aDeletedAccountTakesItsReactionsAway(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	m := f.mustSend(t, f.member(0), "gm", nil)
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO chat_message_reactions (message_id, cabal_id, user_id, emoji) VALUES ($1, $2, $3, $4)`,
		m.ID, f.cabal.ID.UUID(), f.outsider.UUID(), laugh); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM users WHERE id = $1`, f.outsider.UUID()); err != nil {
		t.Fatal(err)
	}
	if rows := f.rows(t); rows != 0 {
		t.Fatalf("%d rows after the account went, want 0", rows)
	}
}

func TestChatReactions_aDatabaseFailureIsInternal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		query   string
		unreact bool
	}{
		{"LiveChatMessageAuthor", false},
		{"AddChatReaction", false},
		{"LockUserPair", false},
		{"IsBlockedEitherWay", false},
		{"CountUserChatReactions", false},
		{"ChatReactionSummaries", false},
		{"LiveChatMessageAuthor", true},
		{"RemoveChatReaction", true},
		{"ChatReactionSummaries", true},
	} {
		t.Run(tt.query, func(t *testing.T) {
			t.Parallel()
			f := newChatReactor(t)
			m := f.mustSend(t, f.member(0), "gm", nil)
			f.add(t, m.ID, f.member(1), fire)
			cfg := f.pool.Config().Copy()
			cfg.ConnConfig.Tracer = &breakOnQuery{name: tt.query}
			down, err := pgxpool.NewWithConfig(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(down.Close)
			deps := f.deps
			deps.UoW, deps.Reads = db.New(down, f.deps.IDs, f.clock), down
			cmd := f.cmd(m.ID, f.member(1), fire)
			if tt.unreact {
				_, err = app.NewUnreactChatMessageHandler(deps).Handle(f.as(t, f.member(1)), cmd)
			} else {
				cmd.Emoji = laugh
				_, err = app.NewReactChatMessageHandler(deps).Handle(f.as(t, f.member(1)), cmd)
			}
			if err == nil {
				t.Fatal("err = nil, want the failed step")
			}
		})
	}
}

func TestChatReactions_aFailedListQueryFailsTheRead(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("top")})
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE chat_message_reactions`); err != nil {
		t.Fatal(err)
	}
	_, err := f.channel(t, f.member(0), api.GetChatMessagesParams{Limit: ptr(10)})
	wantCode(t, err, errs.CodeInternal)
}

func TestChatReactionCleanup_aDatabaseFailureIsInternal(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE chat_message_reactions`); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.memberLeft(t, f.member(1)), errs.CodeInternal)
}
