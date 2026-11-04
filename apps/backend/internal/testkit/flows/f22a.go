package flows

import (
	"net/http"
	"net/url"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func chatReactionPath(base, emoji string) string { return base + "/reactions/" + url.PathEscape(emoji) }

func reactToChat(emoji string) scenario.Step {
	return scenario.Put(chatReactionPath(chatParentPath, emoji), "")
}

func unreactToChat(emoji string) scenario.Step {
	return scenario.Delete(chatReactionPath(chatParentPath, emoji))
}

func chatReactionRows(want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var got int
		err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM chat_message_reactions WHERE cabal_id = $1::uuid`, s.Recall("cabal")).Scan(&got)
		if err != nil {
			s.Fatalf("flows: count the chat reactions: %v", err)
		}
		if got != want {
			s.Fatalf("flows: the cabal holds %d chat reactions, want %d", got, want)
		}
	}
}

func F22aReactChatMessageOK(s *scenario.Scenario) {
	s.Given(chatThread("react-ok")...).
		When(
			reactToChat(party),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(1, heldReaction{party, 1, true}),
			scenario.Replay(),
			reactToChat(party),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(1, heldReaction{party, 1, true}),
			scenario.SignIn(chatMemberOf("react-ok")),
			reactToChat(party),
			summaryHolds(2, heldReaction{party, 2, true}),
			scenario.Get(chatPath),
			scenario.ExpectStatus(http.StatusOK),
			unreactToChat(party),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(1, heldReaction{party, 1, false}),
			unreactToChat(party),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(1, heldReaction{party, 1, false}),
		).
		Then(
			chatReactionRows(1),
			scenario.ExpectEvents(events.TypeChatReactionAdded, 2),
			scenario.ExpectEvents(events.TypeChatReactionRemoved, 1),
			scenario.EventuallyPublished(events.TypeChatReactionAdded, 2),
			scenario.ExpectAblyPublished(app.EventReactionUpdated, "cabal", 3),
		)
}

func F22aReactChatMessageInvalidInput(s *scenario.Scenario) {
	s.Given(chatThread("react-invalid")...).
		When(reactToChat(notEmoji)).
		Then(
			scenario.ExpectProblem(errs.CodeInvalidInput),
			chatReactionRows(0),
			scenario.ExpectEvents(events.TypeChatReactionAdded, 0),
		)
}

func F22aReactChatMessageNotCabalMember(s *scenario.Scenario) {
	s.Given(chatThread("react-outsider")...).
		When(scenario.SignIn(chatOutsiderOf("react-outsider")), reactToChat(party)).
		Then(
			scenario.ExpectProblem(errs.CodeNotCabalMember),
			chatReactionRows(0),
			scenario.ExpectEvents(events.TypeChatReactionAdded, 0),
		)
}

func F22aReactChatMessageChatMessageNotFound(s *scenario.Scenario) {
	s.Given(chatCabal("react-missing")...).
		When(scenario.Put(chatReactionPath(chatPath+"/"+missingMessage, party), "")).
		Then(
			scenario.ExpectProblem(errs.CodeChatMessageNotFound),
			chatReactionRows(0),
			scenario.ExpectEvents(events.TypeChatReactionAdded, 0),
		)
}

func F22aReactChatMessageUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Put(chatReactionPath(missingChat+"/"+missingMessage, party), "")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F22aReactChatMessageCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(chatThread("react-crash")...).
		When(
			reactToChat(party),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(1, heldReaction{party, 1, true}),
		).
		Then(
			chatReactionRows(1),
			scenario.ExpectEvents(events.TypeChatReactionAdded, 1),
			scenario.EventuallyPublished(events.TypeChatReactionAdded, 1),
			scenario.ExpectAblyPublished(app.EventReactionUpdated, "cabal", 1),
		)
}

func F22aUnreactChatMessageOK(s *scenario.Scenario) {
	s.Given(append(chatThread("unreact-ok"), reactToChat(eyes), scenario.ExpectStatus(http.StatusOK))...).
		When(
			unreactToChat(eyes),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(0),
		).
		Then(
			chatReactionRows(0),
			scenario.ExpectEvents(events.TypeChatReactionRemoved, 1),
		)
}

func F22aUnreactChatMessageNotCabalMember(s *scenario.Scenario) {
	s.Given(chatThread("unreact-outsider")...).
		When(scenario.SignIn(chatOutsiderOf("unreact-outsider")), unreactToChat(party)).
		Then(scenario.ExpectProblem(errs.CodeNotCabalMember))
}

func F22aUnreactChatMessageChatMessageNotFound(s *scenario.Scenario) {
	s.Given(chatCabal("unreact-missing")...).
		When(scenario.Delete(chatReactionPath(chatPath+"/"+missingMessage, party))).
		Then(scenario.ExpectProblem(errs.CodeChatMessageNotFound))
}

func F22aUnreactChatMessageInvalidInput(s *scenario.Scenario) {
	s.Given(chatThread("unreact-invalid")...).
		When(unreactToChat(notEmoji)).
		Then(scenario.ExpectProblem(errs.CodeInvalidInput))
}

func F22aUnreactChatMessageUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Delete(chatReactionPath(missingChat+"/"+missingMessage, party))).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}
