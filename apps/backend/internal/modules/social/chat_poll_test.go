package social_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type chatPoller struct {
	chatFixture
	vote   *app.VoteChatPollHandler
	unvote *app.UnvoteChatPollHandler
}

func newChatPoller(t *testing.T) chatPoller {
	t.Helper()
	f := newChatFixture(t)
	return chatPoller{
		chatFixture: f,
		vote:        app.NewVoteChatPollHandler(f.deps),
		unvote:      app.NewUnvoteChatPollHandler(f.deps),
	}
}

func (f chatPoller) post(t *testing.T, deps app.ChatDeps, author ids.UserID) (app.ChatMessage, error) {
	t.Helper()
	in, err := domain.ParseChatInput(nil, &domain.AttachmentInput{
		Kind: "poll", Poll: &domain.PollInput{Question: "Trim?", Options: []string{"Yes", "Wait"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return app.NewPostChatMessageHandler(deps).Handle(f.as(t, author), app.PostChatMessage{
		CabalID: f.cabal.ID, Author: author, Input: in,
	})
}

func (f chatPoller) poll(t *testing.T) app.ChatMessage {
	t.Helper()
	m, err := f.post(t, f.deps, f.member(0))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f chatPoller) cast(t *testing.T, id uuid.UUID, user ids.UserID, position int) app.PollView {
	t.Helper()
	got, err := f.vote.Handle(f.as(t, user), app.VoteChatPoll{
		CabalID: f.cabal.ID, MessageID: id, Viewer: user, Position: position,
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f chatPoller) votes(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM chat_poll_votes`)
}

func TestChatPoll_aPostedPollStoresItsOptionsAndAVoteMovesWithoutDuplicating(t *testing.T) {
	t.Parallel()
	f := newChatPoller(t)
	m := f.poll(t)
	if m.Poll == nil || len(m.Poll.Options) != 2 || m.Attachment.Poll.ClosesAt.IsZero() {
		t.Fatalf("posted poll = %+v, want two options and a closing time", m.Poll)
	}
	first := f.cast(t, m.ID, f.member(1), 1)
	moved := f.cast(t, m.ID, f.member(1), 0)
	if first.Total != 1 || moved.Total != 1 || moved.MyPosition == nil || *moved.MyPosition != 0 ||
		moved.Options[0].Count != 1 || f.votes(t) != 1 {
		t.Fatalf("first %+v, moved %+v, rows %d, want one vote that moved to option 0", first, moved, f.votes(t))
	}
}

func TestChatPoll_unvotingEmptiesThePoll(t *testing.T) {
	t.Parallel()
	f := newChatPoller(t)
	m := f.poll(t)
	f.cast(t, m.ID, f.member(1), 1)
	got, err := f.unvote.Handle(f.as(t, f.member(1)), app.UnvoteChatPoll{
		CabalID: f.cabal.ID, MessageID: m.ID, Viewer: f.member(1),
	})
	if err != nil || got.Total != 0 || got.MyPosition != nil {
		t.Fatalf("unvote = %+v, %v, want an empty poll", got, err)
	}
}

func TestChatPoll_aChangePublishesCountsWithoutMineAndANoOpPublishesNothing(t *testing.T) {
	t.Parallel()
	f := newChatPoller(t)
	m := f.poll(t)
	before := len(f.rt.Published())
	f.cast(t, m.ID, f.member(1), 1)
	cmd := app.UnvoteChatPoll{CabalID: f.cabal.ID, MessageID: m.ID, Viewer: f.member(2)}
	if _, err := f.unvote.Handle(f.as(t, f.member(2)), cmd); err != nil {
		t.Fatal(err)
	}
	published := f.rt.Published()
	if len(published) != before+1 {
		t.Fatalf("published = %+v, want one poll.updated for a vote and a no-op unvote", published)
	}
	want := `{"message_id":"` + m.ID.String() +
		`","options":[{"position":0,"votes":0},{"position":1,"votes":1}],"total_votes":1}`
	last := published[len(published)-1]
	if last.Channel != f.channel() || last.Name != app.EventPollUpdated || string(last.Data) != want {
		t.Fatalf("published = %+v (%s), want %s on %s", last, last.Data, want, f.channel())
	}
}

func TestChatPoll_aFailedPublishStillSavesTheVoteAndLogsIt(t *testing.T) {
	t.Parallel()
	f := newChatPoller(t)
	m := f.poll(t)
	f.rt.Fail(errs.New(errs.CodeUpstreamUnavailable, "test.ably"))
	ctx, logs := f.loggedCtx(t, f.member(1))
	got, err := f.vote.Handle(ctx, app.VoteChatPoll{
		CabalID: f.cabal.ID, MessageID: m.ID, Viewer: f.member(1), Position: 0,
	})
	if err != nil || got.Total != 1 {
		t.Fatalf("Handle = %+v, %v, want the saved vote and no error", got, err)
	}
	out := string(logs.Bytes())
	if !strings.Contains(out, publishFailedLog) || !strings.Contains(out, app.EventPollUpdated) {
		t.Fatalf("logs = %s, want %s naming %s", out, publishFailedLog, app.EventPollUpdated)
	}
}

func TestChatPoll_refusesWhatItCannotAnchor(t *testing.T) {
	t.Parallel()
	f := newChatPoller(t)
	m := f.poll(t)
	plain := f.mustSend(t, f.member(0), "gm", nil)
	for name, c := range map[string]struct {
		message  uuid.UUID
		viewer   ids.UserID
		position int
		code     errs.Code
	}{
		"a plain message":  {plain.ID, f.member(1), 0, errs.CodeChatMessageNotFound},
		"an unknown id":    {f.deps.IDs.NewV7(), f.member(1), 0, errs.CodeChatMessageNotFound},
		"an outsider":      {m.ID, f.outsider, 0, errs.CodeNotCabalMember},
		"a bad position":   {m.ID, f.member(1), 7, errs.CodeInvalidInput},
		"a missing option": {m.ID, f.member(1), 3, errs.CodeInvalidInput},
	} {
		_, err := f.vote.Handle(f.as(t, c.viewer), app.VoteChatPoll{
			CabalID: f.cabal.ID, MessageID: c.message, Viewer: c.viewer, Position: c.position,
		})
		if errs.CodeOf(err) != c.code {
			t.Errorf("%s: error = %v, want %s", name, err, c.code)
		}
	}
	f.clock.Advance(25 * time.Hour)
	_, err := f.vote.Handle(f.as(t, f.member(1)), app.VoteChatPoll{
		CabalID: f.cabal.ID, MessageID: m.ID, Viewer: f.member(1), Position: 0,
	})
	if errs.CodeOf(err) != errs.CodePollClosed || f.votes(t) != 0 {
		t.Fatalf("vote after close = %v with %d rows, want poll_closed and none", err, f.votes(t))
	}
}

func TestChatPoll_aMemberLeavingTakesTheirVotesAndAnAccountDeletionToo(t *testing.T) {
	t.Parallel()
	f := newChatPoller(t)
	m := f.poll(t)
	f.cast(t, m.ID, f.member(1), 0)
	f.cast(t, m.ID, f.member(2), 1)
	if err := f.memberLeft(t, f.member(1)); err != nil || f.votes(t) != 1 {
		t.Fatalf("after leaving: %v with %d votes, want the stayer's one", err, f.votes(t))
	}
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO chat_poll_votes (message_id, user_id, position) VALUES ($1, $2, 0)`,
		m.ID, f.outsider.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM users WHERE id = $1`, f.outsider.UUID()); err != nil {
		t.Fatal(err)
	}
	if f.votes(t) != 1 {
		t.Fatalf("%d votes after the outsider's account went, want the stayer's one", f.votes(t))
	}
}

func TestChatPoll_aDatabaseFailureIsInternal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		query string
		kind  string
	}{
		{"GetChatPoll", "vote"},
		{"UpsertChatPollVote", "vote"},
		{"ListChatPolls", "vote"},
		{"GetChatPoll", "unvote"},
		{"DeleteChatPollVote", "unvote"},
		{"ListChatPolls", "unvote"},
		{"InsertChatPollOptions", "post"},
	} {
		t.Run(tt.kind+tt.query, func(t *testing.T) {
			t.Parallel()
			f := newChatPoller(t)
			m := f.poll(t)
			cfg := f.pool.Config().Copy()
			cfg.ConnConfig.Tracer = &breakOnQuery{name: tt.query}
			down, err := pgxpool.NewWithConfig(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(down.Close)
			deps := f.deps
			deps.UoW, deps.Reads = db.New(down, f.deps.IDs, f.clock), down
			switch tt.kind {
			case "vote":
				_, err = app.NewVoteChatPollHandler(deps).Handle(f.as(t, f.member(1)), app.VoteChatPoll{
					CabalID: f.cabal.ID, MessageID: m.ID, Viewer: f.member(1), Position: 0,
				})
			case "unvote":
				_, err = app.NewUnvoteChatPollHandler(deps).Handle(f.as(t, f.member(1)), app.UnvoteChatPoll{
					CabalID: f.cabal.ID, MessageID: m.ID, Viewer: f.member(1),
				})
			default:
				_, err = f.post(t, deps, f.member(0))
			}
			if err == nil {
				t.Fatal("err = nil, want the failed step")
			}
		})
	}
}

func TestChatPoll_aDroppedVotesTableFailsTheLeaveCleanup(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE chat_poll_votes`); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.memberLeft(t, f.member(1)), errs.CodeInternal)
}

func TestChatPoll_routesCarryTheViewersVoteOnEveryReadAndRefuseABodylessVote(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	var poll api.ChatAttachmentInput
	if err := poll.UnmarshalJSON([]byte(`{"kind":"poll","question":"Trim?","options":["Yes","Wait"]}`)); err != nil {
		t.Fatal(err)
	}
	m := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Attachment: &poll})
	ctx := asUser(t.Context(), f.member(1))
	voted, err := f.routes.VoteChatPoll(ctx, api.VoteChatPollRequestObject{
		Id: f.cabal.ID.UUID(), MessageId: m.Id, Body: &api.PollVoteRequest{Position: 1},
	})
	if err != nil || voted.(api.VoteChatPoll200JSONResponse).Total != 1 {
		t.Fatalf("vote = %+v, %v, want one vote", voted, err)
	}
	_, err = f.routes.VoteChatPoll(ctx, api.VoteChatPollRequestObject{Id: f.cabal.ID.UUID(), MessageId: m.Id})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("bodyless vote error = %v, want invalid_input", err)
	}
	page, err := f.channel(t, f.member(1), api.GetChatMessagesParams{})
	if err != nil || len(page) != 1 || page[0].Attachment == nil {
		t.Fatalf("channel = %+v, %v, want the poll", page, err)
	}
	unvoted, err := f.routes.UnvoteChatPoll(ctx, api.UnvoteChatPollRequestObject{
		Id: f.cabal.ID.UUID(), MessageId: m.Id,
	})
	if err != nil || unvoted.(api.UnvoteChatPoll200JSONResponse).Total != 0 {
		t.Fatalf("unvote = %+v, %v, want an empty poll", unvoted, err)
	}
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE chat_poll_options CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err = f.channel(t, f.member(1), api.GetChatMessagesParams{})
	wantCode(t, err, errs.CodeInternal)
}

func TestChatPoll_routesRefuseAnAnonymousCaller(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	_, err := f.routes.VoteChatPoll(t.Context(), api.VoteChatPollRequestObject{})
	wantCode(t, err, errs.CodeUnauthorized)
	_, err = f.routes.UnvoteChatPoll(t.Context(), api.UnvoteChatPollRequestObject{})
	wantCode(t, err, errs.CodeUnauthorized)
}
