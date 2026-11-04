package flows

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	chatPinPath   = chatPath + "/{msg}/pin"
	chatPinsPath  = cabalsPath + "/{cabal}/chat/pins"
	missingPinned = missingChat + "/" + missingMessage + "/pin"
)

func chatPinned(script string, names ...string) []scenario.Step {
	steps := chatCabal(script)
	for _, name := range names {
		steps = append(steps,
			scenario.SignIn(chatCreatorOf(script)),
			scenario.Post(chatPath, `{"body":"`+name+`"}`),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("id", name),
		)
	}
	return steps
}

func pinOf(name string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		scenario.Put(chatPath+"/"+s.Recall(name)+"/pin", "")(s)
	}
}

func unpinOf(name string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		scenario.Delete(chatPath + "/" + s.Recall(name) + "/pin")(s)
	}
}

func pinsListed(names ...string) scenario.Step {
	return scenario.ExpectField("messages", func(s *scenario.Scenario, raw json.RawMessage) {
		s.Helper()
		var got []struct {
			ID       string          `json:"id"`
			PinnedAt *string         `json:"pinned_at"`
			PinnedBy json.RawMessage `json:"pinned_by"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			s.Fatalf("flows: decode the pins %s: %v", raw, err)
		}
		want := make([]string, len(names))
		for i, name := range names {
			want[i] = s.Recall(name)
		}
		have := make([]string, len(got))
		for i, m := range got {
			have[i] = m.ID
			if m.PinnedAt == nil || len(m.PinnedBy) < 3 || string(m.PinnedBy) == "null" {
				s.Fatalf("flows: pin %s carries pinned_at %v and pinned_by %s, want both set",
					m.ID, m.PinnedAt, m.PinnedBy)
			}
		}
		if !slices.Equal(have, want) {
			s.Fatalf("flows: the pins read %v, want %v", have, want)
		}
	})
}

func chatPinsHold(want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var got int
		err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM cabal_messages WHERE cabal_id = $1::uuid AND pinned_at IS NOT NULL`,
			s.Recall("cabal")).Scan(&got)
		if err != nil {
			s.Fatalf("flows: count the chat pins: %v", err)
		}
		if got != want {
			s.Fatalf("flows: the cabal holds %d pins, want %d", got, want)
		}
	}
}

func deleteChatMessageOf(name string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		scenario.Delete(chatPath + "/" + s.Recall(name))(s)
	}
}

func F22fPinChatMessageOK(s *scenario.Scenario) {
	s.Given(append(chatPinned("pin-ok", "rules"),
		scenario.SignIn(chatMemberOf("pin-ok")),
		scenario.Post(chatPath, `{"body":"my take"}`),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "take"),
	)...).
		When(
			scenario.SignIn(chatCreatorOf("pin-ok")),
			pinOf("rules"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectRemembered("id", "rules"),
			pinOf("rules"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.SignIn(chatMemberOf("pin-ok")),
			pinOf("take"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Get(chatPinsPath),
			scenario.ExpectStatus(http.StatusOK),
			pinsListed("take", "rules"),
			scenario.Get(chatPath),
			scenario.ExpectStatus(http.StatusOK),
			scenario.SignIn(chatCreatorOf("pin-ok")),
			deleteChatMessageOf("rules"),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Get(chatPinsPath),
			pinsListed("take"),
		).
		Then(
			chatPinsHold(1),
			scenario.ExpectEvents(events.TypeChatMessagePinned, 2),
			scenario.EventuallyPublished(events.TypeChatMessagePinned, 2),
			scenario.ExpectAblyPublished(app.EventPinUpdated, "cabal", 2),
		)
}

func F22fPinChatMessageNotCabalMember(s *scenario.Scenario) {
	s.Given(chatPinned("pin-outsider", "rules")...).
		When(scenario.SignIn(chatOutsiderOf("pin-outsider")), pinOf("rules")).
		Then(
			scenario.ExpectProblem(errs.CodeNotCabalMember),
			chatPinsHold(0),
			scenario.ExpectEvents(events.TypeChatMessagePinned, 0),
		)
}

func F22fPinChatMessageChatMessageNotFound(s *scenario.Scenario) {
	s.Given(chatPinned("pin-missing", "gone")...).
		When(
			scenario.Put(chatPath+"/"+missingMessage+"/pin", ""),
			scenario.ExpectProblem(errs.CodeChatMessageNotFound),
			deleteChatMessageOf("gone"),
			scenario.ExpectStatus(http.StatusNoContent),
			pinOf("gone"),
		).
		Then(
			scenario.ExpectProblem(errs.CodeChatMessageNotFound),
			chatPinsHold(0),
			scenario.ExpectEvents(events.TypeChatMessagePinned, 0),
		)
}

func F22fPinChatMessageChatPinForbidden(s *scenario.Scenario) {
	s.Given(chatPinned("pin-forbidden", "rules")...).
		When(scenario.SignIn(chatMemberOf("pin-forbidden")), pinOf("rules")).
		Then(
			scenario.ExpectProblem(errs.CodeChatPinForbidden),
			chatPinsHold(0),
			scenario.ExpectEvents(events.TypeChatMessagePinned, 0),
		)
}

func F22fPinChatMessageChatPinLimit(s *scenario.Scenario) {
	s.Given(chatPinned("pin-limit", "one", "two", "three", "four")...).
		When(
			pinOf("one"), scenario.ExpectStatus(http.StatusOK),
			pinOf("two"), scenario.ExpectStatus(http.StatusOK),
			pinOf("three"), scenario.ExpectStatus(http.StatusOK),
			pinOf("four"),
		).
		Then(
			scenario.ExpectProblem(errs.CodeChatPinLimit),
			chatPinsHold(3),
			scenario.ExpectEvents(events.TypeChatMessagePinned, 3),
		)
}

func F22fPinChatMessageUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Put(missingPinned, "")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F22fPinChatMessageCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(chatPinned("pin-crash", "rules")...).
		When(
			pinOf("rules"),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusOK),
		).
		Then(
			chatPinsHold(1),
			scenario.ExpectEvents(events.TypeChatMessagePinned, 1),
			scenario.EventuallyPublished(events.TypeChatMessagePinned, 1),
			scenario.ExpectAblyPublished(app.EventPinUpdated, "cabal", 1),
		)
}

func F22fUnpinChatMessageOK(s *scenario.Scenario) {
	s.Given(append(chatPinned("unpin-ok", "rules", "other"),
		scenario.SignIn(chatMemberOf("unpin-ok")),
		scenario.Post(chatPath, `{"body":"my take"}`),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "take"),
		pinOf("take"),
		scenario.ExpectStatus(http.StatusOK),
		scenario.SignIn(chatCreatorOf("unpin-ok")),
		pinOf("rules"),
		scenario.ExpectStatus(http.StatusOK),
	)...).
		When(
			scenario.SignIn(chatMemberOf("unpin-ok")),
			unpinOf("take"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("pinned_at", nil),
			scenario.ExpectJSON("pinned_by", nil),
			unpinOf("take"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.SignIn(chatCreatorOf("unpin-ok")),
			unpinOf("rules"),
			scenario.ExpectStatus(http.StatusOK),
			unpinOf("other"),
			scenario.ExpectStatus(http.StatusOK),
		).
		Then(
			chatPinsHold(0),
			scenario.ExpectEvents(events.TypeChatMessageUnpinned, 2),
			scenario.EventuallyPublished(events.TypeChatMessageUnpinned, 2),
			scenario.ExpectAblyPublished(app.EventPinUpdated, "cabal", 4),
		)
}

func F22fUnpinChatMessageNotCabalMember(s *scenario.Scenario) {
	s.Given(append(chatPinned("unpin-outsider", "rules"), pinOf("rules"), scenario.ExpectStatus(http.StatusOK))...).
		When(scenario.SignIn(chatOutsiderOf("unpin-outsider")), unpinOf("rules")).
		Then(scenario.ExpectProblem(errs.CodeNotCabalMember), chatPinsHold(1))
}

func F22fUnpinChatMessageChatMessageNotFound(s *scenario.Scenario) {
	s.Given(chatCabal("unpin-missing")...).
		When(scenario.Delete(chatPath + "/" + missingMessage + "/pin")).
		Then(scenario.ExpectProblem(errs.CodeChatMessageNotFound))
}

func F22fUnpinChatMessageChatPinForbidden(s *scenario.Scenario) {
	s.Given(append(chatPinned("unpin-forbidden", "rules"), pinOf("rules"), scenario.ExpectStatus(http.StatusOK))...).
		When(scenario.SignIn(chatMemberOf("unpin-forbidden")), unpinOf("rules")).
		Then(
			scenario.ExpectProblem(errs.CodeChatPinForbidden),
			chatPinsHold(1),
			scenario.ExpectEvents(events.TypeChatMessageUnpinned, 0),
		)
}
