package flows

import (
	"net/http"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	cabalsPath          = "/v1/cabals"
	walletCreate        = "/privy/v1/wallets"
	cabalOKCreator      = "did:privy:qa-cabal-ok"
	cabalOKStranger     = "did:privy:qa-cabal-stranger"
	cabalInvalidCreator = "did:privy:qa-cabal-invalid"
	cabalDownCreator    = "did:privy:qa-cabal-down"
	cabalCrash          = "did:privy:qa-cabal-crash"
	cabalBody           = `{"name":"Friends pot","join_mode":"open","voter_mode":"all","threshold":"majority",` +
		`"proposal_expiry_seconds":86400}`
	badCabalBody = `{"name":"no","join_mode":"open","voter_mode":"all","threshold":"majority",` +
		`"proposal_expiry_seconds":86400}`
)

func F02CreateCabalOK(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(
			scenario.SignIn(cabalOKCreator),
			scenario.Post(cabalsPath, cabalBody),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("name", "Friends pot"),
			scenario.Remember("id", "cabal"),
			scenario.Replay(),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalCreated, 1),
			scenario.EventuallyPublished(events.TypeCabalCreated, 1),
			scenario.EventuallyHint("cabals"),
			scenario.Get(cabalsPath+"/{cabal}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("name", "Friends pot"),
			scenario.SignIn(cabalOKStranger),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Get(cabalsPath+"/{cabal}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("invite_code", nil),
			scenario.ExpectJSON("me", nil),
			createdLogged(),
			cabalHolds(),
		)
}

func F02CreateCabalInvalidInput(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.SignIn(cabalInvalidCreator), scenario.Post(cabalsPath, badCabalBody)).
		Then(
			scenario.ExpectProblem(errs.CodeInvalidInput),
			scenario.ExpectEvents(events.TypeCabalCreated, 0),
			expectCabals(0),
		)
}

func F02CreateCabalUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(cabalsPath, cabalBody)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized), scenario.ExpectEvents(events.TypeCabalCreated, 0))
}

func F02CreateCabalPrivyUnavailable(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(
			scenario.SignIn(cabalDownCreator),
			fakeTreasuryUnavailable(),
			scenario.Post(cabalsPath, cabalBody),
		).
		Then(
			scenario.ExpectProblem(errs.CodePrivyUnavailable),
			scenario.ExpectEvents(events.TypeCabalCreated, 0),
			expectCabals(0),
		)
}

func fakeTreasuryUnavailable() scenario.Step {
	return func(s *scenario.Scenario) {
		scenario.FakeUpstream(fakes.Step{
			Route: walletCreate, Method: http.MethodPost,
			Headers: map[string]string{
				"privy-idempotency-key": app.TreasuryKey(s.ActorID(), "scenario-1"),
			},
			Action: fakes.ActionFail, Status: http.StatusServiceUnavailable, Times: privyAttempts,
		})(s)
	}
}

func F02CreateCabalCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(
			scenario.SignIn(cabalCrash),
			scenario.Post(cabalsPath, cabalBody),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("id", "cabal"),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalCreated, 1),
			scenario.EventuallyPublished(events.TypeCabalCreated, 1),
			scenario.EventuallyHint("cabals"),
			createdLogged(),
			cabalHolds(),
		)
}

func expectCabals(n int) scenario.Step {
	return func(s *scenario.Scenario) {
		var got int
		err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM cabals WHERE creator_id = $1`, s.ActorID().UUID(),
		).Scan(&got)
		if err != nil || got != n {
			s.Fatalf("cabals = %d, %v, want %d", got, err, n)
		}
	}
}

func createdLogged() scenario.Step {
	return func(s *scenario.Scenario) {
		scenario.EventuallyLog(observability.CabalCreated, map[string]string{"cabal_id": s.Recall("cabal")})(s)
	}
}

const cabalInvariants = `SELECT array_remove(ARRAY[
	CASE WHEN EXISTS (SELECT 1 FROM cabal_members WHERE cabal_id = $1::uuid)
		AND ((SELECT count(*) FROM treasury_wallets WHERE cabal_id = $1::uuid) <> 1
			OR (SELECT count(*) FROM cabal_members WHERE cabal_id = $1::uuid AND role = 'creator') <> 1)
		THEN 'a cabal with members has one treasury wallet and one creator' END,
	CASE WHEN EXISTS (SELECT 1 FROM cabal_access_requests WHERE cabal_id = $1::uuid AND status = 'pending'
		GROUP BY user_id HAVING count(*) > 1)
		THEN 'a user has at most one pending access row' END,
	CASE WHEN EXISTS (SELECT 1 FROM events j
		WHERE j.type = 'cabal.member_joined' AND j.payload->>'cabal_id' = $1::uuid::text
		AND NOT EXISTS (SELECT 1 FROM cabal_members m
			WHERE m.cabal_id = $1::uuid AND m.user_id::text = j.payload->>'user_id')
		AND NOT EXISTS (SELECT 1 FROM events l
			WHERE l.type = 'cabal.member_left' AND l.payload->>'cabal_id' = $1::uuid::text
			AND l.payload->>'user_id' = j.payload->>'user_id' AND l.id > j.id))
		THEN 'every cabal.member_joined has a member row or a later cabal.member_left' END
], NULL)`

func cabalHolds() scenario.Step {
	return func(s *scenario.Scenario) {
		cabal := s.Recall("cabal")
		if cabal == "" {
			return
		}
		var broken []string
		if err := s.DB().QueryRow(s.Context(), cabalInvariants, cabal).Scan(&broken); err != nil {
			s.Fatalf("flows: check cabal %s invariants: %v", cabal, err)
		}
		if len(broken) > 0 {
			s.Fatalf("flows: cabal %s breaks: %s", cabal, strings.Join(broken, "; "))
		}
	}
}
