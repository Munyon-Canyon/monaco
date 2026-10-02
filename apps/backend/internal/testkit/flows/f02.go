package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	cabalsPath    = "/v1/cabals"
	walletCreate  = "/privy/v1/wallets"
	cabalCreator  = "did:privy:qa-1"
	cabalStranger = "did:privy:qa-2"
	cabalCrash    = "did:privy:qa-crash"
	cabalBody     = `{"name":"Friends pot","join_mode":"open","voter_mode":"all","threshold":"majority",` +
		`"proposal_expiry_seconds":86400}`
	badCabalBody = `{"name":"no","join_mode":"open","voter_mode":"all","threshold":"majority",` +
		`"proposal_expiry_seconds":86400}`
)

func F02CreateCabalOK(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(
			scenario.SignIn(cabalCreator),
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
			scenario.SignIn(cabalStranger),
			scenario.Get(cabalsPath+"/{cabal}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("invite_code", nil),
			scenario.ExpectJSON("me", nil),
		)
}

func F02CreateCabalInvalidInput(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.SignIn(cabalCreator), scenario.Post(cabalsPath, badCabalBody)).
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
			scenario.SignIn(cabalCreator),
			scenario.FakeUpstream(fakes.Step{
				Route:  walletCreate,
				Action: fakes.ActionFail,
				Status: http.StatusServiceUnavailable,
				Times:  privyAttempts,
			}),
			scenario.Post(cabalsPath, cabalBody),
		).
		Then(
			scenario.ExpectProblem(errs.CodePrivyUnavailable),
			scenario.ExpectEvents(events.TypeCabalCreated, 0),
			expectCabals(0),
		)
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
			expectOneTreasury(),
		)
}

func expectCabals(n int) scenario.Step {
	return func(s *scenario.Scenario) {
		var got int
		err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM cabals`).Scan(&got)
		if err != nil || got != n {
			s.Fatalf("cabals = %d, %v, want %d", got, err, n)
		}
	}
}

func expectOneTreasury() scenario.Step {
	return func(s *scenario.Scenario) {
		var cabals, wallets int
		err := s.DB().QueryRow(s.Context(),
			`SELECT (SELECT count(*) FROM cabals), (SELECT count(*) FROM treasury_wallets)`,
		).Scan(&cabals, &wallets)
		if err != nil || cabals != 1 || wallets != 1 {
			s.Fatalf("cabals %d wallets %d err %v, want one of each", cabals, wallets, err)
		}
	}
}
