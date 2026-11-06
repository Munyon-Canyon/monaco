package flows

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
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
		err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM cabal_messages WHERE cabal_id = $1 AND kind <> 'system'`,
			s.Recall("cabal")).Scan(&got)
		if err != nil {
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

func seedProposalCard(withFeedItem bool) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var proposer string
		err := s.DB().QueryRow(s.Context(), `SELECT user_id::text FROM cabal_members WHERE cabal_id = $1::uuid LIMIT 1`,
			s.Recall("cabal")).Scan(&proposer)
		if err != nil {
			s.Fatalf("seed the proposal card: read the proposer: %v", err)
		}
		card, ref := ids.Real{}.NewV7().String(), ids.Real{}.NewV7().String()
		_, err = s.DB().Exec(s.Context(), `INSERT INTO cabal_messages
			(id, cabal_id, author_id, kind, proposal_id, dedupe_key, created_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'proposal', $4::uuid, $5::text, now())`,
			card, s.Recall("cabal"), proposer, ref, "proposal:"+ref)
		if err != nil {
			s.Fatalf("seed the proposal card: %v", err)
		}
		if !withFeedItem {
			return
		}
		_, err = s.DB().Exec(s.Context(), `INSERT INTO feed_objects
			(id, kind, ref_type, ref_id, cabal_id, actor_id, title, payload, created_at, updated_at)
			VALUES ($1::uuid, 'proposal', 'proposals', $2::uuid, $3::uuid, $4::uuid, 'seeded', '{}', now(), now())`,
			ids.Real{}.NewV7().String(), ref, s.Recall("cabal"), proposer)
		if err != nil {
			s.Fatalf("seed the proposal's feed item: %v", err)
		}
	}
}

func proposalCard(s *scenario.Scenario) (card, ref string) {
	s.Helper()
	err := s.DB().QueryRow(s.Context(), `SELECT id::text, proposal_id::text FROM cabal_messages
		WHERE cabal_id = $1::uuid AND kind = 'proposal'`, s.Recall("cabal")).Scan(&card, &ref)
	if err != nil {
		s.Fatalf("read the seeded proposal card: %v", err)
	}
	return card, ref
}

func replyToCard(body string, alsoInChannel bool) scenario.Step {
	return func(s *scenario.Scenario) {
		card, _ := proposalCard(s)
		scenario.Post(chatPath, `{"body":"`+body+`","parent_id":"`+card+
			`","also_in_channel":`+strconv.FormatBool(alsoInChannel)+`}`)(s)
	}
}

func repliedToCard(s *scenario.Scenario) {
	card, _ := proposalCard(s)
	scenario.ExpectJSON("parent_id", card)(s)
}

func getCardComments(s *scenario.Scenario) {
	_, ref := proposalCard(s)
	scenario.Get("/v1/proposals/" + ref + "/comments")(s)
}

func proposalReplyHolds(comments, copies int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		card, ref := proposalCard(s)
		var gotComments, gotCopies, gotChatReplies int
		err := s.DB().QueryRow(s.Context(), `SELECT
			(SELECT count(*) FROM feed_comments c JOIN feed_objects f ON f.id = c.feed_object_id
				WHERE f.ref_type = 'proposals' AND f.ref_id = $1::uuid),
			(SELECT count(*) FROM cabal_messages WHERE parent_id = $2::uuid AND comment_id IS NOT NULL),
			(SELECT count(*) FROM cabal_messages WHERE parent_id = $2::uuid AND comment_id IS NULL)`,
			ref, card).Scan(&gotComments, &gotCopies, &gotChatReplies)
		if err != nil {
			s.Fatalf("count the proposal replies: %v", err)
		}
		if gotComments != comments || gotCopies != copies || gotChatReplies != 0 {
			s.Fatalf("%d comments, %d copies and %d chat replies under the card, want %d, %d and 0",
				gotComments, gotCopies, gotChatReplies, comments, copies)
		}
	}
}

func parentLastRepliers(want int) scenario.Step {
	return scenario.ExpectField("parent", func(s *scenario.Scenario, raw json.RawMessage) {
		s.Helper()
		var parent struct {
			LastRepliers []struct {
				ID string `json:"id"`
			} `json:"last_repliers"`
		}
		if err := json.Unmarshal(raw, &parent); err != nil {
			s.Fatalf("read the parent's last_repliers: %v", err)
		}
		rows, err := s.DB().Query(s.Context(), `SELECT author_id::text FROM (
				SELECT DISTINCT ON (author_id) author_id, created_at, id FROM cabal_messages
				WHERE parent_id = $1::uuid AND deleted_at IS NULL ORDER BY author_id, created_at DESC, id DESC
			) newest ORDER BY created_at DESC, id DESC LIMIT 3`, s.Recall("parent"))
		if err != nil {
			s.Fatalf("read the repliers: %v", err)
		}
		defer rows.Close()
		var wantIDs []string
		gotIDs := make([]string, 0, len(parent.LastRepliers))
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				s.Fatalf("scan a replier: %v", err)
			}
			wantIDs = append(wantIDs, id)
		}
		for _, r := range parent.LastRepliers {
			gotIDs = append(gotIDs, r.ID)
		}
		if len(wantIDs) != want || !slices.Equal(gotIDs, wantIDs) {
			s.Fatalf("parent last_repliers = %v, want the %d newest distinct repliers %v", gotIDs, want, wantIDs)
		}
	})
}

func F22PostChatMessageOK(s *scenario.Scenario) {
	s.Given(append(chatThread("post-ok"), seedProposalCard(true))...).
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
			replyToCard("quiet", false),
			scenario.ExpectStatus(http.StatusCreated),
			repliedToCard,
			scenario.ExpectJSON("also_in_channel", false),
			scenario.Remember("comment_id", "quiet"),
			scenario.ExpectRemembered("id", "quiet"),
			replyToCard("loud", true),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("comment_id", "loud"),
			scenario.ExpectRemembered("id", "loud"),
			getCardComments,
			scenario.ExpectStatus(http.StatusOK),
			scenario.SignIn(chatCreatorOf("post-ok")),
			replyTo("parent", "second", false),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Get(chatParentPath+"/thread"),
			scenario.ExpectStatus(http.StatusOK),
			parentLastRepliers(2),
		).
		Then(
			chatRows(5),
			replyCount("parent", 2),
			proposalReplyHolds(2, 1),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 4),
			scenario.ExpectEventPayload(events.TypeChatMessagePosted, map[string]any{"also_in_channel": true}),
			scenario.EventuallyPublished(events.TypeChatMessagePosted, 4),
			scenario.ExpectEvents(events.TypeCommentCreated, 2),
			scenario.ExpectAblyPublished(app.EventMessageCreated, "cabal", 6),
			scenario.ExpectAblyPublished(app.EventThreadUpdated, "cabal", 4),
		)
}

func F22PostChatMessageFeedItemPending(s *scenario.Scenario) {
	s.Given(append(chatCabal("post-feed-pending"), seedProposalCard(false),
		scenario.SignIn(chatMemberOf("post-feed-pending")))...).
		When(replyToCard("too early", true)).
		Then(scenario.ExpectProblem(errs.CodeFeedItemPending), proposalReplyHolds(0, 0), chatRows(1),
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0))
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
			Route: fakes.AblyMessagesRun, Contains: `"name":"` + app.EventMessageCreated + `"`,
			Action: fakes.ActionFail, Status: http.StatusInternalServerError, Times: 1,
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
