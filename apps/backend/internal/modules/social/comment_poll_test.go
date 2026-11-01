package social_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type pollThread struct {
	quoteThread
	hours *int
}

func pollRequest(question string, options []string, hours *int) *api.CreateCommentRequest {
	req := &api.CreateCommentRequest{Attachment: &api.CommentAttachmentInput{}}
	err := req.Attachment.FromPollAttachmentInput(api.PollAttachmentInput{
		Kind: api.PollAttachmentInputKindPoll, Question: question, Options: options, ClosesInHours: hours,
	})
	if err != nil {
		panic(err)
	}
	return req
}

func (f pollThread) ask(t *testing.T, who ids.UserID, item uuid.UUID) api.Comment {
	t.Helper()
	c, err := f.post(t, item, who, pollRequest("Trim to 40% before earnings?", []string{"Yes", "Wait"}, f.hours))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func pollOf(t *testing.T, c api.Comment) api.PollAttachment {
	t.Helper()
	if c.Attachment == nil {
		t.Fatalf("comment %+v has no attachment", c)
	}
	poll, err := c.Attachment.AsPollAttachment()
	if err != nil {
		t.Fatal(err)
	}
	return poll
}

func (f pollThread) vote(t *testing.T, who ids.UserID, comment uuid.UUID, position int) (api.PollState, error) {
	t.Helper()
	res, err := f.routes.PutCommentPollVote(asUser(t.Context(), who), api.PutCommentPollVoteRequestObject{
		CommentId: comment, Body: &api.PollVoteRequest{Position: position},
	})
	if err != nil {
		return api.PollState{}, err
	}
	return api.PollState(res.(api.PutCommentPollVote200JSONResponse)), nil
}

func (f pollThread) mustVote(t *testing.T, who ids.UserID, comment uuid.UUID, position int) api.PollState {
	t.Helper()
	got, err := f.vote(t, who, comment, position)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f pollThread) mustWithdraw(t *testing.T, who ids.UserID, comment uuid.UUID) api.PollState {
	t.Helper()
	got, err := f.withdraw(t, who, comment)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f pollThread) withdraw(t *testing.T, who ids.UserID, comment uuid.UUID) (api.PollState, error) {
	t.Helper()
	res, err := f.routes.DeleteCommentPollVote(asUser(t.Context(), who), api.DeleteCommentPollVoteRequestObject{
		CommentId: comment,
	})
	if err != nil {
		return api.PollState{}, err
	}
	return api.PollState(res.(api.DeleteCommentPollVote200JSONResponse)), nil
}

func newPollThread(t *testing.T) pollThread {
	t.Helper()
	return pollThread{quoteThread: newQuoteThread(t)}
}

func wantPollCounts(t *testing.T, got api.PollState, mine *int, counts ...int) {
	t.Helper()
	total := 0
	for i, want := range counts {
		total += want
		if got.Options[i].Count != want || got.Options[i].Position != i {
			t.Errorf("option %d = %+v, want count %d", i, got.Options[i], want)
		}
	}
	if got.Total != total || len(got.Options) != len(counts) {
		t.Errorf("poll = %+v, want total %d over %d options", got, total, len(counts))
	}
	if (got.MyPosition == nil) != (mine == nil) || mine != nil && *got.MyPosition != *mine {
		t.Errorf("my_position = %v, want %v", got.MyPosition, mine)
	}
}

func TestCommentPoll_storesThePollAndReturnsItWithZeroCounts(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	created := f.ask(t, f.nora, f.item)
	poll := pollOf(t, created)
	if created.Body != nil || created.BodyDisplay != poll.Question || poll.Closed || poll.Total != 0 ||
		!poll.ClosesAt.Equal(f.clock.Now().Add(24*time.Hour)) || poll.MyPosition != nil ||
		poll.Options[0].Label != "Yes" || poll.Options[1].Label != "Wait" {
		t.Fatalf("created = %+v poll = %+v, want a fresh 24 hour poll", created, poll)
	}
}

func TestCommentPoll_isListedAndAnnouncedWithItsQuestion(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	poll := pollOf(t, f.ask(t, f.nora, f.item))
	listed := pollOf(t, f.page(t, f.author).Items[0].Comment)
	if listed.Question != poll.Question || !listed.ClosesAt.Equal(poll.ClosesAt) || len(listed.Options) != 2 {
		t.Fatalf("listed poll = %+v, want the created one", listed)
	}
	created1 := f.created(t)
	if len(created1) != 1 || created1[0].PollQuestion != poll.Question || created1[0].AttachmentKind != "poll" ||
		created1[0].Excerpt != poll.Question {
		t.Fatalf("comment.created = %+v, want the poll question and kind", created1)
	}
}

func TestCommentPoll_keepsTheTextBesideThePollAndHonoursCloseHours(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	f.hours = ptr(2)
	req := pollRequest("Sell?", []string{"A", "B", "C", "D"}, f.hours)
	req.Body = ptr("my take")
	c, err := f.post(t, f.item, f.nora, req)
	if err != nil {
		t.Fatal(err)
	}
	poll := pollOf(t, c)
	if c.BodyDisplay != "my take" || !poll.ClosesAt.Equal(f.clock.Now().Add(2*time.Hour)) || len(poll.Options) != 4 {
		t.Fatalf("comment = %+v poll = %+v, want the text and a 2 hour poll", c, poll)
	}
}

func TestCommentPoll_refusesBadPollsAndWritesNothing(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	cases := map[string]*api.CreateCommentRequest{
		"one option":      pollRequest("Q?", []string{"A"}, nil),
		"five options":    pollRequest("Q?", []string{"A", "B", "C", "D", "E"}, nil),
		"duplicates":      pollRequest("Q?", []string{"Yes", " yes "}, nil),
		"a long label":    pollRequest("Q?", []string{"A", strings.Repeat("x", 41)}, nil),
		"no question":     pollRequest(" ", []string{"A", "B"}, nil),
		"closes too soon": pollRequest("Q?", []string{"A", "B"}, ptr(0)),
		"closes too late": pollRequest("Q?", []string{"A", "B"}, ptr(169)),
	}
	for name, req := range cases {
		if _, err := f.post(t, f.item, f.nora, req); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: error = %v, want invalid_input", name, err)
		}
	}
	if comments, attachments := f.rows(t); comments != 0 || attachments != 0 {
		t.Fatalf("rows = %d comments, %d attachments, want none", comments, attachments)
	}
}

func TestCommentPoll_refusesAnAttachmentTheServerCannotRead(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	for name, raw := range map[string]string{
		"a string":          `"x"`,
		"a poll of strings": `{"kind":"poll","options":"x"}`,
		"a gif of a string": `{"kind":"gif","gif":"x"}`,
	} {
		req := &api.CreateCommentRequest{Attachment: &api.CommentAttachmentInput{}}
		if err := req.Attachment.UnmarshalJSON([]byte(raw)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.post(t, f.item, f.nora, req); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: error = %v, want invalid_input", name, err)
		}
	}
}

func TestCommentPoll_aNonMemberCannotPostOneOnAMembersOnlyProposal(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	proposal := f.commentRoutes.item(t, feed.KindProposal)
	_, err := f.post(t, proposal, f.nora, pollRequest("Q?", []string{"A", "B"}, nil))
	wantCode(t, err, errs.CodeCommentMembersOnly)
}

func TestCommentPoll_oneVoteEachMovesAndWithdraws(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	proposal := f.commentRoutes.item(t, feed.KindTrade)
	poll := f.ask(t, f.author, proposal)
	zero, one := 0, 1
	wantPollCounts(t, f.mustVote(t, f.nora, poll.Id, 1), &one, 0, 1)
	wantPollCounts(t, f.mustVote(t, f.nora, poll.Id, 1), &one, 0, 1)
	wantPollCounts(t, f.mustVote(t, f.author, poll.Id, 0), &zero, 1, 1)
	wantPollCounts(t, f.mustVote(t, f.nora, poll.Id, 0), &zero, 2, 0)
	read := pollOf(t, f.page2(t, proposal, f.nora).Items[0].Comment)
	if read.Total != 2 || read.MyPosition == nil || *read.MyPosition != 0 || read.Options[0].Count != 2 {
		t.Fatalf("read poll = %+v, want 2 votes and my vote on 0", read)
	}
	wantPollCounts(t, f.mustWithdraw(t, f.nora, poll.Id), nil, 1, 0)
	wantPollCounts(t, f.mustWithdraw(t, f.nora, poll.Id), nil, 1, 0)
	var voted int
	err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`,
		string(events.TypeCommentPollVoted)).Scan(&voted)
	if err != nil || voted != 4 {
		t.Fatalf("%d comment.poll_voted events, err %v, want 4", voted, err)
	}
}

func (f pollThread) page2(t *testing.T, item uuid.UUID, viewer ids.UserID) api.CommentPage {
	t.Helper()
	return f.read(t, viewer, item, api.GetFeedCommentsParams{})
}

func TestCommentPoll_refusesBadVotes(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	poll := f.ask(t, f.nora, f.item)
	_, err := f.vote(t, f.author, poll.Id, 2)
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.vote(t, f.author, poll.Id, -1)
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.routes.PutCommentPollVote(asUser(t.Context(), f.author), api.PutCommentPollVoteRequestObject{
		CommentId: poll.Id,
	})
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.vote(t, f.author, f.gen.NewV7(), 0)
	wantCode(t, err, errs.CodeCommentNotFound)
	_, err = f.withdraw(t, f.author, f.gen.NewV7())
	wantCode(t, err, errs.CodeCommentNotFound)
	plain := f.say(t, f.nora, "plain", uuid.Nil)
	_, err = f.vote(t, f.author, plain.Id, 0)
	wantCode(t, err, errs.CodeCommentNotFound)
	_, err = f.routes.PutCommentPollVote(t.Context(), api.PutCommentPollVoteRequestObject{
		CommentId: poll.Id, Body: &api.PollVoteRequest{},
	})
	wantCode(t, err, errs.CodeUnauthorized)
	_, err = f.routes.DeleteCommentPollVote(t.Context(), api.DeleteCommentPollVoteRequestObject{CommentId: poll.Id})
	wantCode(t, err, errs.CodeUnauthorized)
}

func TestCommentPoll_aDeletedCommentTakesNoVotes(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	poll := f.ask(t, f.nora, f.item)
	if _, err := f.routes.DeleteFeedComment(asUser(t.Context(), f.nora), api.DeleteFeedCommentRequestObject{
		CommentId: poll.Id,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.vote(t, f.author, poll.Id, 0)
	wantCode(t, err, errs.CodeCommentNotFound)
	if got := f.page(t, f.author).Items[0].Comment; got.Attachment != nil {
		t.Fatalf("deleted poll reply = %+v, want no attachment", got)
	}
}

func TestCommentPoll_aClosedPollTakesNoVoteAndReadsClosed(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	poll := f.ask(t, f.nora, f.item)
	if _, err := f.vote(t, f.author, poll.Id, 0); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(24 * time.Hour)
	_, err := f.vote(t, f.author, poll.Id, 1)
	wantCode(t, err, errs.CodePollClosed)
	_, err = f.withdraw(t, f.author, poll.Id)
	wantCode(t, err, errs.CodePollClosed)
	read := pollOf(t, f.page(t, f.author).Items[0].Comment)
	if !read.Closed || read.Total != 1 || read.MyPosition == nil || *read.MyPosition != 0 {
		t.Fatalf("closed poll = %+v, want closed with my vote kept", read)
	}
}

func TestCommentPoll_anItemTheViewerCannotSeeTakesNoVote(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	poll := f.ask(t, f.nora, f.item)
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO feed_mutes (user_id, target_type, target_id, created_at)
		VALUES ($1, 'item', $2, $3)`, f.author.UUID(), f.item.String(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := f.vote(t, f.author, poll.Id, 0)
	wantCode(t, err, errs.CodeFeedItemNotFound)
}

func TestCommentPoll_aFailedWriteIsInternal(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	poll := f.ask(t, f.nora, f.item)
	if _, err := f.pool.Exec(
		t.Context(),
		`ALTER TABLE comment_poll_votes RENAME TO comment_poll_votes_gone`,
	); err != nil {
		t.Fatal(err)
	}
	_, err := f.vote(t, f.author, poll.Id, 0)
	wantCode(t, err, errs.CodeInternal)
	_, err = f.withdraw(t, f.author, poll.Id)
	wantCode(t, err, errs.CodeInternal)
	_, err = f.routes.GetFeedComments(asUser(t.Context(), f.author), api.GetFeedCommentsRequestObject{Id: f.item})
	wantCode(t, err, errs.CodeInternal)
}

func TestCommentPoll_aFailedOptionsWriteLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	if _, err := f.pool.Exec(
		t.Context(),
		`ALTER TABLE comment_poll_options RENAME TO comment_poll_options_gone`,
	); err != nil {
		t.Fatal(err)
	}
	_, err := f.post(t, f.item, f.nora, pollRequest("Q?", []string{"A", "B"}, nil))
	wantCode(t, err, errs.CodeInternal)
	if comments, attachments := f.rows(t); comments != 0 || attachments != 0 {
		t.Fatalf("rows = %d comments, %d attachments, want none", comments, attachments)
	}
}

func TestCommentPoll_aFailedPollReadIsInternal(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	poll := f.ask(t, f.nora, f.item)
	if _, err := f.pool.Exec(
		t.Context(),
		`ALTER TABLE comment_attachments RENAME TO comment_attachments_gone`,
	); err != nil {
		t.Fatal(err)
	}
	_, err := f.vote(t, f.author, poll.Id, 0)
	wantCode(t, err, errs.CodeInternal)
}

func TestCommentPoll_aFailedCountReadAfterTheVoteIsInternal(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	poll := f.ask(t, f.nora, f.item)
	_, err := f.pool.Exec(t.Context(), `
		CREATE FUNCTION drop_options() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN ALTER TABLE comment_poll_options RENAME TO comment_poll_options_gone; RETURN NEW; END $$;
		CREATE TRIGGER drop_options AFTER INSERT ON comment_poll_votes FOR EACH ROW EXECUTE FUNCTION drop_options()`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.vote(t, f.author, poll.Id, 0)
	wantCode(t, err, errs.CodeInternal)
}
