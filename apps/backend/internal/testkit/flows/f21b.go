package flows

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	party    = "🎉"
	eyes     = "👀"
	thumbs   = "👍🏽"
	notEmoji = "nope"
)

func reactionsPathOf(item string) string { return feedPath + "/" + item + "/reactions" }

func reactWith(item *string, emoji string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		scenario.Put(reactionsPathOf(*item)+"/"+url.PathEscape(emoji), "")(s)
	}
}

func unreactWith(item *string, emoji string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		scenario.Delete(reactionsPathOf(*item) + "/" + url.PathEscape(emoji))(s)
	}
}

type heldReaction struct {
	emoji string
	count int
	mine  bool
}

func summaryHolds(total int, want ...heldReaction) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		scenario.ExpectJSON("total", total)(s)
		scenario.ExpectField("reactions", func(s *scenario.Scenario, raw json.RawMessage) {
			var got []struct {
				Emoji string `json:"emoji"`
				Count int    `json:"count"`
				Mine  bool   `json:"mine"`
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				s.Fatalf("flows: decode the reactions %s: %v", raw, err)
			}
			if len(got) != len(want) {
				s.Fatalf("flows: reactions read %s, want %v", raw, want)
			}
			for i, w := range want {
				if got[i].Emoji != w.emoji || got[i].Count != w.count || got[i].Mine != w.mine {
					s.Fatalf("flows: reactions read %s, want %v", raw, want)
				}
			}
		})(s)
	}
}

func reactionRowsHold(item *string, want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var got int
		if err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM feed_reactions WHERE feed_object_id = $1::uuid`, *item).Scan(&got); err != nil {
			s.Fatalf("flows: count the reactions of %s: %v", *item, err)
		}
		if got != want {
			s.Fatalf("flows: item %s holds %d reactions, want %d", *item, got, want)
		}
	}
}

func F21bReactFeedItemOK(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee")).
		When(
			reactWith(&w.proposal, party),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(1, heldReaction{party, 1, true}),
			scenario.Replay(),
			reactWith(&w.proposal, party),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(1, heldReaction{party, 1, true}),
			reactWith(&w.proposal, eyes),
			summaryHolds(2, heldReaction{party, 1, true}, heldReaction{eyes, 1, true}),
			scenario.AsUser("kai"),
			reactWith(&w.proposal, party),
			summaryHolds(3, heldReaction{party, 2, true}, heldReaction{eyes, 1, false}),
			unreactWith(&w.proposal, party),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(2, heldReaction{party, 1, false}, heldReaction{eyes, 1, false}),
			unreactWith(&w.proposal, party),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(2, heldReaction{party, 1, false}, heldReaction{eyes, 1, false}),
			scenario.AsUser("lee"),
			reactWith(&w.trade, thumbs),
			summaryHolds(1, heldReaction{thumbs, 1, true}),
		).
		Then(
			reactionRowsHold(&w.proposal, 2),
			reactionRowsHold(&w.trade, 1),
			scenario.ExpectEvents(events.TypeReactionAdded, 4),
			scenario.ExpectEvents(events.TypeReactionRemoved, 1),
			scenario.EventuallyPublished(events.TypeReactionAdded, 4),
		)
}

func F21bReactFeedItemInvalidInput(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee")).
		When(reactWith(&w.proposal, notEmoji)).
		Then(
			scenario.ExpectProblem(errs.CodeInvalidInput),
			reactionRowsHold(&w.proposal, 0),
			scenario.ExpectEvents(events.TypeReactionAdded, 0),
		)
}

func F21bReactFeedItemFeedItemNotFound(s *scenario.Scenario) {
	missing := missingItem
	s.Given(scenario.SeededUser("kai", "active"), scenario.AsUser("kai")).
		When(reactWith(&missing, party)).
		Then(scenario.ExpectProblem(errs.CodeFeedItemNotFound), scenario.ExpectEvents(events.TypeReactionAdded, 0))
}

func F21bReactFeedItemUnauthorized(s *scenario.Scenario) {
	missing := missingItem
	s.Given(scenario.Anonymous()).
		When(reactWith(&missing, party)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F21bReactFeedItemRateLimited(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("mia")).
		When(scenario.UntilRateLimited(40, reactWith(&w.trade, party))).
		Then(scenario.ExpectProblem(errs.CodeRateLimited), reactionRowsHold(&w.trade, 1))
}

func F21bReactFeedItemCrashBeforeCommit(s *scenario.Scenario) {
	w := &commentWorld{}
	s.Given(w.seed(), scenario.AsUser("lee")).
		When(
			reactWith(&w.proposal, party),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusOK),
			summaryHolds(1, heldReaction{party, 1, true}),
		).
		Then(
			reactionRowsHold(&w.proposal, 1),
			scenario.ExpectEvents(events.TypeReactionAdded, 1),
			scenario.EventuallyPublished(events.TypeReactionAdded, 1),
		)
}
