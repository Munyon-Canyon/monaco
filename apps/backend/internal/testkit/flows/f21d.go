package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const pollVotePath = feedPath + "/comments/{poll}/poll/vote"

func pollOn(item *string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		scenario.Post(commentsPathOf(*item), `{"attachment":{"kind":"poll","question":"Trim to 40%?",`+
			`"options":["Yes","Wait"],"closes_in_hours":24}}`)(s)
		scenario.ExpectStatus(http.StatusCreated)(s)
		scenario.Remember("id", "poll")(s)
	}
}

func votePosition(position string) scenario.Step {
	return scenario.Put(pollVotePath, `{"position":`+position+`}`)
}

func pollCounts(total int) scenario.Step {
	return scenario.ExpectJSON("total", total)
}

func closePoll(s *scenario.Scenario) {
	s.Helper()
	if _, err := s.DB().Exec(s.Context(), `UPDATE comment_attachments
		SET payload = jsonb_set(payload, '{closes_at}', to_jsonb(now() - interval '1 hour'))
		WHERE comment_id = $1::uuid`, s.Recall("poll")); err != nil {
		s.Fatalf("flows: close the poll: %v", err)
	}
}

func pollVotesHold(want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var got int
		if err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM comment_poll_votes WHERE comment_id = $1::uuid`, s.Recall("poll")).
			Scan(&got); err != nil {
			s.Fatalf("flows: count the poll votes: %v", err)
		}
		if got != want {
			s.Fatalf("flows: the poll holds %d votes, want %d", got, want)
		}
	}
}

func F21dVoteCommentPollOK(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee"), pollOn(&w.proposal)).
		When(
			scenario.AsUser("kai"),
			votePosition("1"),
			scenario.ExpectStatus(http.StatusOK),
			pollCounts(1),
			scenario.ExpectJSON("my_position", 1),
			scenario.Replay(),
			scenario.ExpectStatus(http.StatusOK),
			pollCounts(1),
			scenario.AsUser("mia"),
			votePosition("0"),
			scenario.ExpectProblem(errs.CodeCommentMembersOnly),
			scenario.AsUser("lee"),
			votePosition("0"),
			scenario.ExpectStatus(http.StatusOK),
			pollCounts(2),
			scenario.AsUser("kai"),
			votePosition("0"),
			pollCounts(2),
			scenario.ExpectJSON("my_position", 0),
			scenario.Delete(pollVotePath),
			scenario.ExpectStatus(http.StatusOK),
			pollCounts(1),
			scenario.Delete(pollVotePath),
			pollCounts(1),
		).
		Then(
			pollVotesHold(1),
			scenario.ExpectEvents(events.TypeCommentPollVoted, 3),
			scenario.EventuallyPublished(events.TypeCommentPollVoted, 3),
		)
}

func F21dVoteCommentPollInvalidInput(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee"), pollOn(&w.proposal)).
		When(votePosition("3")).
		Then(
			scenario.ExpectProblem(errs.CodeInvalidInput),
			pollVotesHold(0),
			scenario.ExpectEvents(events.TypeCommentPollVoted, 0),
		)
}

func F21dVoteCommentPollPollClosed(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee"), pollOn(&w.proposal), closePoll).
		When(votePosition("0")).
		Then(
			scenario.ExpectProblem(errs.CodePollClosed),
			pollVotesHold(0),
			scenario.ExpectEvents(events.TypeCommentPollVoted, 0),
		)
}

func F21dVoteCommentPollCommentNotFound(s *scenario.Scenario) {
	missing := missingItem
	s.Given(scenario.SeededUser("kai", "active"), scenario.AsUser("kai")).
		When(scenario.Put(feedPath+"/comments/"+missing+"/poll/vote", `{"position":0}`)).
		Then(scenario.ExpectProblem(errs.CodeCommentNotFound), scenario.ExpectEvents(events.TypeCommentPollVoted, 0))
}

func F21dVoteCommentPollUnauthorized(s *scenario.Scenario) {
	missing := missingItem
	s.Given(scenario.Anonymous()).
		When(scenario.Put(feedPath+"/comments/"+missing+"/poll/vote", `{"position":0}`)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F21dVoteCommentPollRateLimited(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee"), pollOn(&w.trade), scenario.AsUser("mia")).
		When(scenario.UntilRateLimited(40, votePosition("0"))).
		Then(scenario.ExpectProblem(errs.CodeRateLimited), pollVotesHold(1))
}

func F21dVoteCommentPollCrashBeforeCommit(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee"), pollOn(&w.proposal)).
		When(
			votePosition("1"),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusOK),
			pollCounts(1),
		).
		Then(
			pollVotesHold(1),
			scenario.ExpectEvents(events.TypeCommentPollVoted, 1),
			scenario.EventuallyPublished(events.TypeCommentPollVoted, 1),
		)
}

func (defined) SeedsF21d() map[string]Seeder {
	return map[string]Seeder{
		"F21dVoteCommentPollOK":                seedSignedIn,
		"F21dVoteCommentPollInvalidInput":      seedSignedIn,
		"F21dVoteCommentPollPollClosed":        seedSignedIn,
		"F21dVoteCommentPollCommentNotFound":   seedSignedIn,
		"F21dVoteCommentPollUnauthorized":      seedAnonymous,
		"F21dVoteCommentPollRateLimited":       seedSignedIn,
		"F21dVoteCommentPollCrashBeforeCommit": seedSignedIn,
	}
}
