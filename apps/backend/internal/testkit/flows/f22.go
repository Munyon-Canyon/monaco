package flows

import (
	"net/http"
	"slices"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	chatPath       = cabalsPath + "/{cabal}/messages"
	chatParentPath = chatPath + "/{parent}"
	missingChat    = missingCabal + "/messages"
	missingMessage = "01890a5d-ac96-774b-bcce-000000000005"
)

func chatCreatorOf(script string) string { return "did:privy:qa-f22-" + script + "-creator" }

func chatMemberOf(script string) string { return "did:privy:qa-f22-" + script + "-member" }

func chatOutsiderOf(script string) string { return "did:privy:qa-f22-" + script + "-outsider" }

func chatCabal(script string) []scenario.Step {
	return []scenario.Step{
		scenario.SignIn(chatCreatorOf(script)),
		scenario.Post(cabalsPath, cabalOf("open", "all")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "cabal"),
		scenario.SignIn(chatMemberOf(script)),
		scenario.Post(membersPath, ""),
		scenario.ExpectStatus(http.StatusOK),
	}
}

func chatThread(script string) []scenario.Step {
	return append(chatCabal(script),
		scenario.SignIn(chatCreatorOf(script)),
		scenario.Post(chatPath, `{"body":"buy AAPLx?"}`),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "parent"),
	)
}

func replyTo(parent, body string, alsoInChannel bool) scenario.Step {
	return func(s *scenario.Scenario) {
		scenario.Post(chatPath, `{"body":"`+body+`","parent_id":"`+s.Recall(parent)+
			`","also_in_channel":`+strconv.FormatBool(alsoInChannel)+`}`)(s)
	}
}

func chatRows(n int) scenario.Step {
	return func(s *scenario.Scenario) {
		var got int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM cabal_messages WHERE cabal_id = $1`,
			s.Recall("cabal")).Scan(&got); err != nil {
			s.Fatalf("count the chat messages: %v", err)
		}
		if got != n {
			s.Fatalf("%d chat messages stored, want %d", got, n)
		}
	}
}

func replyCount(parent string, n int) scenario.Step {
	return func(s *scenario.Scenario) {
		var got int
		if err := s.DB().QueryRow(s.Context(), `SELECT reply_count FROM cabal_messages WHERE id = $1`,
			s.Recall(parent)).Scan(&got); err != nil {
			s.Fatalf("read the reply count: %v", err)
		}
		if got != n {
			s.Fatalf("reply_count = %d, want %d", got, n)
		}
	}
}

func F22PostChatMessageOK(s *scenario.Scenario) {
	s.Given(chatThread("post-ok")...).
		When(
			scenario.Replay(),
			scenario.ExpectJSON("body", "buy AAPLx?"),
			scenario.SignIn(chatMemberOf("post-ok")),
			replyTo("parent", "wagmi", true),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectRemembered("parent_id", "parent"),
			scenario.ExpectJSON("also_in_channel", true),
			scenario.Get(chatParentPath+"/thread"),
			scenario.ExpectStatus(http.StatusOK),
		).
		Then(
			chatRows(2),
			replyCount("parent", 1),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 2),
			scenario.ExpectEventPayload(events.TypeChatMessagePosted, map[string]any{"also_in_channel": true}),
			scenario.EventuallyPublished(events.TypeChatMessagePosted, 2),
			scenario.ExpectAblyPublished(app.EventMessageCreated, "cabal", 2),
			scenario.ExpectAblyPublished(app.EventThreadUpdated, "cabal", 1),
		)
}

func setHandle(did, handle string) []scenario.Step {
	return []scenario.Step{
		scenario.SignIn(did),
		scenario.Put(setHandlePath, `{"handle":"`+handle+`"}`),
		scenario.ExpectStatus(http.StatusOK),
	}
}

func mentionedHandles(want ...string) scenario.Step {
	return func(s *scenario.Scenario) {
		var got []string
		rows, err := s.DB().Query(s.Context(), `
			SELECT u.handle FROM events e, jsonb_array_elements_text(e.payload->'mentioned_user_ids') m
			JOIN users u ON u.id = m::uuid
			WHERE e.type = $1 ORDER BY u.handle`, string(events.TypeChatMessagePosted))
		if err != nil {
			s.Fatalf("read the mentioned handles: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var h string
			if err := rows.Scan(&h); err != nil {
				s.Fatalf("scan a mentioned handle: %v", err)
			}
			got = append(got, h)
		}
		if !slices.Equal(got, want) {
			s.Fatalf("mentioned handles = %v, want %v", got, want)
		}
	}
}

func ChatPostMentionsResolved(s *scenario.Scenario) {
	const script = "post-mentions"
	given := slices.Concat(chatCabal(script),
		setHandle(chatCreatorOf(script), "qa22_author"),
		setHandle(chatMemberOf(script), "qa22_member"),
		setHandle(chatOutsiderOf(script), "qa22_outsider"),
		[]scenario.Step{scenario.SignIn(chatCreatorOf(script))},
	)
	s.Given(given...).
		When(
			scenario.Post(chatPath, `{"body":"@qa22_member @QA22_outsider @qa22_author @qa22_nobody hi"}`),
			scenario.ExpectStatus(http.StatusCreated),
		).
		Then(
			scenario.ExpectEvents(events.TypeChatMessagePosted, 1),
			mentionedHandles("qa22_member"),
		)
}

func F22PostChatMessageNotCabalMember(s *scenario.Scenario) {
	s.Given(chatCabal("post-outsider")...).
		When(scenario.SignIn(chatOutsiderOf("post-outsider")), scenario.Post(chatPath, `{"body":"gm"}`)).
		Then(scenario.ExpectProblem(errs.CodeNotCabalMember), chatRows(0),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0))
}

func F22PostChatMessageChatParentNotFound(s *scenario.Scenario) {
	s.Given(chatCabal("post-no-parent")...).
		When(scenario.Post(chatPath, `{"body":"gm","parent_id":"`+missingMessage+`"}`)).
		Then(scenario.ExpectProblem(errs.CodeChatParentNotFound), chatRows(0),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0))
}

func F22PostChatMessageChatParentIsReply(s *scenario.Scenario) {
	s.Given(append(chatThread("post-nested"),
		replyTo("parent", "yes", false),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "reply"),
	)...).
		When(replyTo("reply", "no", false)).
		Then(scenario.ExpectProblem(errs.CodeChatParentIsReply), chatRows(2),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 2))
}

func F22PostChatMessageChatBodyInvalid(s *scenario.Scenario) {
	s.Given(chatCabal("post-blank")...).
		When(scenario.Post(chatPath, `{"body":"   "}`)).
		Then(scenario.ExpectProblem(errs.CodeChatBodyInvalid), chatRows(0),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0))
}

func F22PostChatMessageUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(missingChat, `{"body":"gm"}`)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F22PostChatMessageCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(chatCabal("post-crash")...).
		When(
			scenario.Post(chatPath, `{"body":"gm"}`),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("body", "gm"),
		).
		Then(
			chatRows(1),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 1),
			scenario.EventuallyPublished(events.TypeChatMessagePosted, 1),
			scenario.ExpectAblyPublished(app.EventMessageCreated, "cabal", 1),
		)
}

func ChatPostSurvivesAblyDown(s *scenario.Scenario) {
	s.Given(append(chatCabal("post-ably-down"),
		scenario.FakeUpstream(fakes.Step{
			Route: fakes.AblyMessagesRun, Action: fakes.ActionFail, Status: http.StatusInternalServerError, Times: 1,
		}),
		scenario.SignIn(chatCreatorOf("post-ably-down")),
	)...).
		When(
			scenario.Post(chatPath, `{"body":"gm"}`),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("body", "gm"),
		).
		Then(
			chatRows(1),
			scenario.EventuallyLog(observability.SocialChatPublishFailed, map[string]string{"event": app.EventMessageCreated}),
			scenario.ExpectAblyPublished(app.EventMessageCreated, "cabal", 0),
		)
}

func F22DeleteChatMessageOK(s *scenario.Scenario) {
	s.Given(chatThread("delete-ok")...).
		When(
			scenario.Delete(chatParentPath),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Delete(chatParentPath),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Get(chatPath),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("messages", []any{}),
		).
		Then(
			chatRows(1),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 1),
			scenario.ExpectAblyPublished(app.EventMessageDeleted, "cabal", 1),
		)
}

func F22DeleteChatMessageChatMessageNotOwned(s *scenario.Scenario) {
	s.Given(chatThread("delete-not-owner")...).
		When(scenario.SignIn(chatMemberOf("delete-not-owner")), scenario.Delete(chatParentPath)).
		Then(scenario.ExpectProblem(errs.CodeChatMessageNotOwned), chatRows(1))
}

func F22DeleteChatMessageUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Delete(missingChat + "/" + missingMessage)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}
