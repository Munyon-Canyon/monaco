package admin_test

import (
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type liveLetter struct {
	id          string
	status      string
	occurrences int
	redriven    bool
	rows        int
}

func readLiveLetter(s *scenario.Scenario) liveLetter {
	var l liveLetter
	err := s.DB().QueryRow(s.Context(), `SELECT count(*), coalesce(max(id::text), ''), coalesce(max(status), ''),
		coalesce(max(occurrences), 0), coalesce(bool_or(redriven_at IS NOT NULL), false) FROM dead_letters`).
		Scan(&l.rows, &l.id, &l.status, &l.occurrences, &l.redriven)
	if err != nil {
		s.Fatalf("admin_test: read the dead letters: %v", err)
	}
	return l
}

func awaitLetter(what string, ok func(liveLetter) bool) scenario.Step {
	return scenario.Eventually(what, func(s *scenario.Scenario) bool {
		scenario.AwaitTick("admin.deadletters")(s)
		return ok(readLiveLetter(s))
	})
}

func TestDeadLetters_RedriveFailsAgain(t *testing.T) {
	t.Parallel()
	s := flow27Scenario(t)
	var letter string
	s.Given(
		scenario.SeededUser("alice", "active"), scenario.SeededUser("bob", "active"),
		scenario.FakeUpstream(fakes.Step{
			Route: "/posthog/batch/", Action: fakes.ActionFail, Status: http.StatusBadRequest, Times: 2,
		}),
		scenario.AsUser("alice"),
	).When(
		scenario.Post("/v1/users/{bob}/follow", `{}`), scenario.ExpectStatus(http.StatusOK),
		awaitLetter("the first failure", func(l liveLetter) bool { return l.rows == 1 && l.status == "open" }),
		func(s *scenario.Scenario) { letter = readLiveLetter(s).id },
		scenario.SeededAdmin("op", "operator"), scenario.AsUser("op"),
		func(s *scenario.Scenario) {
			scenario.Post("/v1/admin/dead-letters/"+letter+"/redrive", `{"reason":"posthog fixed"}`)(s)
		},
		scenario.ExpectStatus(http.StatusNoContent),
	).Then(
		awaitLetter("the failed redrive to reopen the row", func(l liveLetter) bool {
			return l.status == "open" && l.occurrences == 2
		}),
		func(s *scenario.Scenario) {
			if l := readLiveLetter(s); l.rows != 1 || l.id != letter || !l.redriven {
				s.Fatalf(
					"admin_test: dead letters after the failed redrive = %+v, want the same row with redriven_at kept",
					l,
				)
			}
		},
	)
}
