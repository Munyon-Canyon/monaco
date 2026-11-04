package flows

import (
	"encoding/json"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	chatPollVotePath = chatPath + "/{poll}/poll/vote"
	chatPollInput    = `{"kind":"poll","question":"Trim to 40%?","options":["Yes","Wait"],"closes_in_hours":24}`
)

func chatPollPosted(script string) []scenario.Step {
	return append(chatCabal(script),
		scenario.SignIn(chatCreatorOf(script)),
		scenario.Post(chatPath, `{"attachment":`+chatPollInput+`}`),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.ExpectJSON("body", "Trim to 40%?"),
		pollAttachmentHolds(0, nil),
		scenario.Remember("id", "poll"),
	)
}

type pollAttachmentView struct {
	Kind       string `json:"kind"`
	Total      int    `json:"total"`
	MyPosition *int   `json:"my_position"`
	ClosesAt   string `json:"closes_at"`
	Options    []struct {
		Position int `json:"position"`
		Count    int `json:"count"`
	} `json:"options"`
}

func checkPollAttachment(s *scenario.Scenario, raw json.RawMessage, total int, mine *int) {
	s.Helper()
	var got pollAttachmentView
	if err := json.Unmarshal(raw, &got); err != nil {
		s.Fatalf("flows: decode the poll %s: %v", raw, err)
	}
	same := got.MyPosition == nil && mine == nil || got.MyPosition != nil && mine != nil && *got.MyPosition == *mine
	if got.Kind != "poll" || got.Total != total || !same || got.ClosesAt == "" || len(got.Options) != 2 {
		s.Fatalf("flows: the poll reads %s, want %d votes and my_position %v", raw, total, mine)
	}
}

func pollAttachmentHolds(total int, mine *int) scenario.Step {
	return scenario.ExpectField("attachment", func(s *scenario.Scenario, raw json.RawMessage) {
		checkPollAttachment(s, raw, total, mine)
	})
}

func newestPollHolds(total int, mine *int) scenario.Step {
	return scenario.ExpectField("messages", func(s *scenario.Scenario, raw json.RawMessage) {
		var got []struct {
			Attachment json.RawMessage `json:"attachment"`
		}
		if err := json.Unmarshal(raw, &got); err != nil || len(got) == 0 {
			s.Fatalf("flows: decode the messages %s: %v", raw, err)
		}
		checkPollAttachment(s, got[0].Attachment, total, mine)
	})
}

func positionOf(n int) *int { return &n }

func voteChatPollAt(position string) scenario.Step {
	return scenario.Put(chatPollVotePath, `{"position":`+position+`}`)
}

func chatPollVotesHold(want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var got int
		err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM chat_poll_votes WHERE message_id = $1::uuid`, s.Recall("poll")).Scan(&got)
		if err != nil {
			s.Fatalf("flows: count the chat poll votes: %v", err)
		}
		if got != want {
			s.Fatalf("flows: the chat poll holds %d votes, want %d", got, want)
		}
	}
}

func closeChatPoll(s *scenario.Scenario) {
	s.Helper()
	if _, err := s.DB().Exec(s.Context(), `UPDATE chat_message_attachments
		SET payload = jsonb_set(payload, '{closes_at}', to_jsonb(now() - interval '1 hour'))
		WHERE message_id = $1::uuid`, s.Recall("poll")); err != nil {
		s.Fatalf("flows: close the chat poll: %v", err)
	}
}

func F22cVoteChatPollOK(s *scenario.Scenario) {
	s.Given(chatPollPosted("poll-ok")...).
		When(
			scenario.SignIn(chatMemberOf("poll-ok")),
			voteChatPollAt("1"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("total", 1),
			scenario.ExpectJSON("my_position", 1),
			scenario.Replay(),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("total", 1),
			scenario.SignIn(chatCreatorOf("poll-ok")),
			voteChatPollAt("0"),
			scenario.ExpectJSON("total", 2),
			scenario.SignIn(chatMemberOf("poll-ok")),
			voteChatPollAt("0"),
			scenario.ExpectJSON("total", 2),
			scenario.ExpectJSON("my_position", 0),
			scenario.Get(chatPath),
			scenario.ExpectStatus(http.StatusOK),
			newestPollHolds(2, positionOf(0)),
		).
		Then(
			chatPollVotesHold(2),
			scenario.ExpectEvents(events.TypeChatPollVoted, 3),
			scenario.EventuallyPublished(events.TypeChatPollVoted, 3),
			scenario.ExpectAblyPublished(app.EventPollUpdated, "cabal", 3),
		)
}

func F22cVoteChatPollInvalidInput(s *scenario.Scenario) {
	s.Given(chatPollPosted("poll-invalid")...).
		When(
			voteChatPollAt("2"),
			scenario.ExpectProblem(errs.CodeInvalidInput),
			voteChatPollAt("9"),
		).
		Then(
			scenario.ExpectProblem(errs.CodeInvalidInput),
			chatPollVotesHold(0),
			scenario.ExpectEvents(events.TypeChatPollVoted, 0),
		)
}

func F22cVoteChatPollNotCabalMember(s *scenario.Scenario) {
	s.Given(chatPollPosted("poll-outsider")...).
		When(scenario.SignIn(chatOutsiderOf("poll-outsider")), voteChatPollAt("0")).
		Then(
			scenario.ExpectProblem(errs.CodeNotCabalMember),
			chatPollVotesHold(0),
			scenario.ExpectEvents(events.TypeChatPollVoted, 0),
		)
}

func F22cVoteChatPollChatMessageNotFound(s *scenario.Scenario) {
	s.Given(chatThread("poll-missing")...).
		When(
			scenario.Put(chatPath+"/"+missingMessage+"/poll/vote", `{"position":0}`),
			scenario.ExpectProblem(errs.CodeChatMessageNotFound),
			scenario.Put(chatParentPath+"/poll/vote", `{"position":0}`),
		).
		Then(
			scenario.ExpectProblem(errs.CodeChatMessageNotFound),
			scenario.ExpectEvents(events.TypeChatPollVoted, 0),
		)
}

func F22cVoteChatPollPollClosed(s *scenario.Scenario) {
	s.Given(append(chatPollPosted("poll-closed"), closeChatPoll)...).
		When(voteChatPollAt("0")).
		Then(
			scenario.ExpectProblem(errs.CodePollClosed),
			chatPollVotesHold(0),
			scenario.ExpectEvents(events.TypeChatPollVoted, 0),
		)
}

func F22cVoteChatPollUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Put(missingChat+"/"+missingMessage+"/poll/vote", `{"position":0}`)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F22cVoteChatPollCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(chatPollPosted("poll-crash")...).
		When(
			scenario.SignIn(chatMemberOf("poll-crash")),
			voteChatPollAt("1"),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("total", 1),
		).
		Then(
			chatPollVotesHold(1),
			scenario.ExpectEvents(events.TypeChatPollVoted, 1),
			scenario.EventuallyPublished(events.TypeChatPollVoted, 1),
			scenario.ExpectAblyPublished(app.EventPollUpdated, "cabal", 1),
		)
}

func F22cUnvoteChatPollOK(s *scenario.Scenario) {
	s.Given(append(chatPollPosted("unvote-ok"),
		scenario.SignIn(chatMemberOf("unvote-ok")), voteChatPollAt("1"), scenario.ExpectStatus(http.StatusOK))...).
		When(
			scenario.Delete(chatPollVotePath),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("total", 0),
			scenario.Delete(chatPollVotePath),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("total", 0),
		).
		Then(
			chatPollVotesHold(0),
			scenario.ExpectAblyPublished(app.EventPollUpdated, "cabal", 2),
		)
}

func F22cUnvoteChatPollNotCabalMember(s *scenario.Scenario) {
	s.Given(chatPollPosted("unvote-outsider")...).
		When(scenario.SignIn(chatOutsiderOf("unvote-outsider")), scenario.Delete(chatPollVotePath)).
		Then(scenario.ExpectProblem(errs.CodeNotCabalMember))
}

func F22cUnvoteChatPollChatMessageNotFound(s *scenario.Scenario) {
	s.Given(chatCabal("unvote-missing")...).
		When(scenario.Delete(chatPath + "/" + missingMessage + "/poll/vote")).
		Then(scenario.ExpectProblem(errs.CodeChatMessageNotFound))
}

func F22cUnvoteChatPollPollClosed(s *scenario.Scenario) {
	s.Given(append(chatPollPosted("unvote-closed"), closeChatPoll)...).
		When(scenario.Delete(chatPollVotePath)).
		Then(scenario.ExpectProblem(errs.CodePollClosed))
}
