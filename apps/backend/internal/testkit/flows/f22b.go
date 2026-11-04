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
	gifInput = `{"kind":"gif","gif":{"id":"xT9IgDEI1iZyb2wqo8","title":"happy dance",` +
		`"still":{"url":"https://media.giphy.com/media/xT9IgDEI1iZyb2wqo8/200_s.gif","width":200,"height":150},` +
		`"preview":{"url":"https://media.giphy.com/media/xT9IgDEI1iZyb2wqo8/200w.mp4","width":200,"height":150},` +
		`"original":{"url":"https://media.giphy.com/media/xT9IgDEI1iZyb2wqo8/giphy.gif","width":200,"height":150}}}`
	gifOnly = `{"attachment":` + gifInput + `}`
	gifText = `{"body":"lol","attachment":` + gifInput + `}`
)

func attachmentKind(want string) scenario.Step {
	return scenario.ExpectField("attachment", func(s *scenario.Scenario, raw json.RawMessage) {
		var got struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(raw, &got); err != nil || got.Kind != want {
			s.Fatalf("attachment = %s, want kind %s", raw, want)
		}
	})
}

func newestAttachmentKind(want string) scenario.Step {
	return scenario.ExpectField("messages", func(s *scenario.Scenario, raw json.RawMessage) {
		var got []struct {
			Attachment struct {
				Kind string `json:"kind"`
			} `json:"attachment"`
		}
		if err := json.Unmarshal(raw, &got); err != nil || len(got) == 0 || got[0].Attachment.Kind != want {
			s.Fatalf("messages = %s, want the newest to carry a %s attachment", raw, want)
		}
	})
}

func chatAttachmentRows(n int) scenario.Step {
	return func(s *scenario.Scenario) {
		var got int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM chat_message_attachments WHERE cabal_id = $1`,
			s.Recall("cabal")).Scan(&got); err != nil {
			s.Fatalf("count the chat attachments: %v", err)
		}
		if got != n {
			s.Fatalf("%d chat attachments stored, want %d", got, n)
		}
	}
}

func F22bPostChatMessageOK(s *scenario.Scenario) {
	s.Given(chatCabal("post-ok")...).
		When(
			scenario.SignIn(chatCreatorOf("post-ok")),
			scenario.Post(chatPath, gifOnly),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("body", "GIF"),
			attachmentKind("gif"),
			scenario.Replay(),
			scenario.Post(chatPath, gifText),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("body", "lol"),
			attachmentKind("gif"),
			scenario.Get(chatPath),
			scenario.ExpectStatus(http.StatusOK),
			newestAttachmentKind("gif"),
		).
		Then(
			chatRows(2),
			chatAttachmentRows(2),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 2),
			scenario.EventuallyPublished(events.TypeChatMessagePosted, 2),
			scenario.ExpectAblyPublished(app.EventMessageCreated, "cabal", 2),
		)
}

func F22bPostChatMessageInvalidInput(s *scenario.Scenario) {
	s.Given(chatCabal("post-blank")...).
		When(
			scenario.SignIn(chatCreatorOf("post-blank")),
			scenario.Post(chatPath, `{"attachment":{"kind":"photo"}}`),
		).
		Then(
			scenario.ExpectProblem(errs.CodeInvalidInput),
			chatRows(0),
			chatAttachmentRows(0),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0),
		)
}

func F22bPostChatMessageNotCabalMember(s *scenario.Scenario) {
	s.Given(chatCabal("post-outsider")...).
		When(scenario.SignIn(chatOutsiderOf("post-outsider")), scenario.Post(chatPath, gifOnly)).
		Then(
			scenario.ExpectProblem(errs.CodeNotCabalMember),
			chatRows(0),
			chatAttachmentRows(0),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0),
		)
}

func F22bPostChatMessageChatBodyInvalid(s *scenario.Scenario) {
	s.Given(chatCabal("post-blank")...).
		When(scenario.SignIn(chatCreatorOf("post-blank")), scenario.Post(chatPath, `{}`)).
		Then(
			scenario.ExpectProblem(errs.CodeChatBodyInvalid),
			chatRows(0),
			chatAttachmentRows(0),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0),
		)
}

func F22bPostChatMessageUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(missingChat, gifOnly)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F22bPostChatMessageCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(chatCabal("post-crash")...).
		When(
			scenario.SignIn(chatCreatorOf("post-crash")),
			scenario.Post(chatPath, gifOnly),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("body", "GIF"),
		).
		Then(
			chatRows(1),
			chatAttachmentRows(1),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 1),
			scenario.EventuallyPublished(events.TypeChatMessagePosted, 1),
			scenario.ExpectAblyPublished(app.EventMessageCreated, "cabal", 1),
		)
}
