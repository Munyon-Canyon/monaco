package flows

import (
	"encoding/json"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const catchUpItems = "/v1/me/catch-up/items/"

func (defined) SeedsF35() map[string]Seeder {
	return map[string]Seeder{
		"F35ClearItemOK":                seedSignedIn,
		"F35ClearItemNotFound":          seedSignedIn,
		"F35ClearItemUnauthorized":      seedAnonymous,
		"F35ClearItemCrashBeforeCommit": seedSignedIn,
	}
}

type pendingInvite struct {
	member ids.UserID
	key    string
}

func seedPendingInvite(s *scenario.Scenario) pendingInvite {
	member := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{})
	pot := testkit.NewCabal(seedT{s}, s.DB())
	id := ids.Real{}.NewV7()
	_, err := s.DB().Exec(s.Context(), `INSERT INTO cabal_access_requests
		(id, cabal_id, user_id, direction, invited_by, expires_at, created_at)
		VALUES ($1, $2, $3, 'invite', $4, now() + interval '7 days', now() - interval '1 minute')`,
		id, pot.ID.UUID(), member.ID.UUID(), pot.Creator.ID.UUID())
	if err != nil {
		s.Fatalf("flows: seed a pending invite: %v", err)
	}
	return pendingInvite{member: member.ID, key: "invite:" + id.String()}
}

func F35ClearItemOK(s *scenario.Scenario) {
	invite := seedPendingInvite(s)
	s.Given(scenario.AsSeededUser("member", invite.member)).
		When(
			scenario.Get(catchUp),
			scenario.ExpectJSON("count", 1),
			scenario.Post(catchUpItems+invite.key+"/clear", ""),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusNoContent),
		).
		Then(
			scenario.Get(catchUp),
			scenario.ExpectJSON("count", 0),
			scenario.ExpectField("cleared", func(s *scenario.Scenario, raw json.RawMessage) {
				var cleared []struct {
					Key    string `json:"key"`
					Reason string `json:"cleared_reason"`
				}
				if err := json.Unmarshal(raw, &cleared); err != nil || len(cleared) != 1 ||
					cleared[0].Key != invite.key || cleared[0].Reason != "cleared" {
					s.Fatalf("flows: cleared = %s, want the one cleared invite", raw)
				}
			}),
		)
}

func F35ClearItemNotFound(s *scenario.Scenario) {
	invite := seedPendingInvite(s)
	s.Given(scenario.AsSeededUser("member", invite.member)).
		When(
			scenario.Post(catchUpItems+"invite:"+ids.Real{}.NewV7().String()+"/clear", ""),
			scenario.ExpectProblem(errs.CodeNotFound),
		).
		Then(
			scenario.Get(catchUp),
			scenario.ExpectJSON("count", 1),
		)
}

func F35ClearItemUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(catchUpItems+"invite:"+ids.Real{}.NewV7().String()+"/clear", "")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F35ClearItemCrashBeforeCommit(s *scenario.Scenario) {
	invite := seedPendingInvite(s)
	s.Given(scenario.AsSeededUser("member", invite.member)).
		When(
			scenario.Post(catchUpItems+invite.key+"/clear", ""),
			catchUpStateEmpty(invite.member),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusNoContent),
		).
		Then(
			scenario.Get(catchUp),
			scenario.ExpectJSON("count", 0),
		)
}

func catchUpStateEmpty(user ids.UserID) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var rows int
		err := s.DB().QueryRow(
			s.Context(),
			`SELECT count(*) FROM catch_up_state WHERE user_id = $1`,
			user.UUID(),
		).Scan(&rows)
		if err != nil {
			s.Fatalf("flows: count catch up state: %v", err)
		}
		if rows != 0 {
			s.Fatalf("flows: a crashed clear left %d state rows, want none", rows)
		}
	}
}
