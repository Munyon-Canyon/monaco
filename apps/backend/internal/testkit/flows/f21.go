package flows

import (
	"encoding/json"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	feedPath    = "/v1/feed"
	missingItem = "01890a5d-ac96-774b-bcce-000000000021"
)

func commentsPathOf(item string) string { return feedPath + "/" + item + "/comments" }

type commentWorld struct{ proposal, trade string }

func (w *commentWorld) seed() scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		for _, name := range []string{"kai", "lee", "mia"} {
			scenario.SeededUser(name, "active")(s)
		}
		kai, errKai := ids.ParseUserID(s.Recall("kai"))
		lee, errLee := ids.ParseUserID(s.Recall("lee"))
		if errKai != nil || errLee != nil {
			s.Fatalf("flows: parse the seeded commenters: %v %v", errKai, errLee)
		}
		cabal := testkit.NewCabal(s, s.DB(), testkit.WithCreator(kai), testkit.WithJoiner(lee))
		w.proposal = feedItemOf(s, "proposal", "proposals", cabal.ID)
		w.trade = feedItemOf(s, "trade", "swaps", cabal.ID)
	}
}

func feedItemOf(s *scenario.Scenario, kind, refType string, cabal ids.CabalID) string {
	s.Helper()
	id := ids.Real{}.NewV7()
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO feed_objects
		(id, kind, ref_type, ref_id, cabal_id, title, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'seeded', '{}', now(), now())`,
		id, kind, refType, ids.Real{}.NewV7(), cabal.UUID()); err != nil {
		s.Fatalf("flows: seed the %s feed item: %v", kind, err)
	}
	return id.String()
}

func commentOn(item *string, body, parent string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		payload := `{"body":"` + body + `"`
		if parent != "" {
			payload += `,"parent_comment_id":"` + s.Recall(parent) + `"`
		}
		scenario.Post(commentsPathOf(*item), payload+`}`)(s)
	}
}

func commentsHold(item *string, stored, live int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var rows, alive, counted int
		if err := s.DB().QueryRow(s.Context(), `SELECT
			(SELECT count(*) FROM feed_comments WHERE feed_object_id = $1::uuid),
			(SELECT count(*) FROM feed_comments WHERE feed_object_id = $1::uuid AND deleted_at IS NULL),
			(SELECT comment_count FROM feed_objects WHERE id = $1::uuid)`, *item).Scan(&rows, &alive, &counted); err != nil {
			s.Fatalf("flows: count the comments of %s: %v", *item, err)
		}
		if rows != stored || alive != live || counted != live {
			s.Fatalf("flows: item %s holds %d comments, %d live, comment_count %d, want %d, %d and %d",
				*item, rows, alive, counted, stored, live, live)
		}
	}
}

func threadsHold(want ...string) scenario.Step {
	return scenario.ExpectField("items", func(s *scenario.Scenario, raw json.RawMessage) {
		var threads []struct {
			Comment struct {
				BodyDisplay string `json:"body_display"`
			} `json:"comment"`
			Replies []struct {
				BodyDisplay string `json:"body_display"`
			} `json:"replies"`
		}
		if err := json.Unmarshal(raw, &threads); err != nil {
			s.Fatalf("flows: decode the comment threads %s: %v", raw, err)
		}
		var got []string
		for _, thread := range threads {
			got = append(got, thread.Comment.BodyDisplay)
			for _, reply := range thread.Replies {
				got = append(got, reply.BodyDisplay)
			}
		}
		if len(got) != len(want) {
			s.Fatalf("flows: threads read %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				s.Fatalf("flows: threads read %v, want %v", got, want)
			}
		}
	})
}

func F21CreateCommentOK(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee")).
		When(
			commentOn(&w.proposal, "nice", ""),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("body", "nice"),
			scenario.ExpectJSON("is_mine", true),
			scenario.Remember("id", "top"),
			scenario.Replay(),
			scenario.ExpectRemembered("id", "top"),
			scenario.AsUser("mia"),
			commentOn(&w.trade, "gm", ""),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.AsUser("kai"),
			commentOn(&w.proposal, "agreed", "top"),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectRemembered("parent_comment_id", "top"),
			scenario.Remember("id", "reply"),
			scenario.AsUser("lee"),
			commentOn(&w.proposal, "thanks", "reply"),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectRemembered("parent_comment_id", "top"),
			scenario.Delete(feedPath+"/comments/{top}"),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Delete(feedPath+"/comments/{top}"),
			scenario.ExpectStatus(http.StatusNoContent),
			func(s *scenario.Scenario) { scenario.Get(commentsPathOf(w.proposal))(s) },
			scenario.ExpectStatus(http.StatusOK),
			threadsHold("Comment deleted", "agreed", "thanks"),
		).
		Then(
			commentsHold(&w.proposal, 3, 2),
			commentsHold(&w.trade, 1, 1),
			scenario.ExpectEvents(events.TypeCommentCreated, 4),
			scenario.ExpectEvents(events.TypeCommentDeleted, 1),
			scenario.EventuallyPublished(events.TypeCommentCreated, 4),
		)
	s.Then(
		scenario.EventuallyCapturedBy(events.TypeCommentCreated, "comment_created", "feed_object_id", w.proposal),
		scenario.EventuallyCapturedBy(events.TypeCommentCreated, "comment_created", "feed_object_id", w.trade),
	)
}

func F21CreateCommentInvalidInput(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee")).
		When(commentOn(&w.proposal, "   ", "")).
		Then(
			scenario.ExpectProblem(errs.CodeInvalidInput),
			commentsHold(&w.proposal, 0, 0),
			scenario.ExpectEvents(events.TypeCommentCreated, 0),
		)
}

func F21CreateCommentFeedItemNotFound(s *scenario.Scenario) {
	missing := missingItem
	s.Given(scenario.SeededUser("kai", "active"), scenario.AsUser("kai")).
		When(commentOn(&missing, "hi", "")).
		Then(scenario.ExpectProblem(errs.CodeFeedItemNotFound), scenario.ExpectEvents(events.TypeCommentCreated, 0))
}

func F21CreateCommentCommentParentMismatch(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee"), func(s *scenario.Scenario) {
		commentOn(&w.proposal, "top", "")(s)
	}, scenario.Remember("id", "top")).
		When(commentOn(&w.trade, "elsewhere", "top")).
		Then(
			scenario.ExpectProblem(errs.CodeCommentParentMismatch),
			commentsHold(&w.trade, 0, 0),
			scenario.ExpectEvents(events.TypeCommentCreated, 1),
		)
}

func F21CreateCommentCommentMembersOnly(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("mia")).
		When(commentOn(&w.proposal, "let me in", "")).
		Then(
			scenario.ExpectProblem(errs.CodeCommentMembersOnly),
			commentsHold(&w.proposal, 0, 0),
			scenario.ExpectEvents(events.TypeCommentCreated, 0),
		)
}

func F21CreateCommentUnauthorized(s *scenario.Scenario) {
	missing := missingItem
	s.Given(scenario.Anonymous()).
		When(commentOn(&missing, "hi", "")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F21CreateCommentRateLimited(s *scenario.Scenario) {
	w := &commentWorld{}
	steps := make([]scenario.Step, 0, 10)
	for range 5 {
		steps = append(steps, commentOn(&w.trade, "again", ""), scenario.ExpectStatus(http.StatusCreated))
	}
	s.Given(w.seed(), scenario.AsUser("mia")).
		When(steps...).
		Then(commentOn(&w.trade, "too fast", ""), scenario.ExpectProblem(errs.CodeRateLimited))
}

func F21CreateCommentCrashBeforeCommit(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee")).
		When(
			commentOn(&w.proposal, "nice", ""),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("body", "nice"),
		).
		Then(
			commentsHold(&w.proposal, 1, 1),
			scenario.ExpectEvents(events.TypeCommentCreated, 1),
			scenario.EventuallyPublished(events.TypeCommentCreated, 1),
		)
}
