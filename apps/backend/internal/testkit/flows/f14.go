package flows

import (
	"net/http"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	cashOutsPath   = cabalsPath + "/{cabal}/cashouts"
	cashOutJobPath = cashOutsPath + "/{job}"
	cashOutPayout  = "treasury.cashout_payout"
	f14Stake       = 2_000_000
	rpcAttempts    = 3
)

func (defined) WorkerEnvF14() []string { return []string{"RELAYER_PRIVATE_KEY=" + CashOutRelayerKey()} }

func (defined) AloneF14() []Script { return []Script{F14CashOutPayoutsRPCUnavailable} }

func CashOutRelayerKey() string { return chain.EncodeBase58(fakes.FixtureKey("f14-relayer")) }

func cashOutOf(micros string) string { return `{"usdc_micros":"` + micros + `"}` }

func f14Founded(script string) []scenario.Step {
	return []scenario.Step{
		scenario.SignIn("did:privy:qa-f14-" + script),
		scenario.ExpectStatus(http.StatusOK),
		scenario.Post(cabalsPath, cabalOf("open", "all")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "cabal"),
	}
}

func f14Staked(script string) []scenario.Step {
	return append(f14Founded(script), funded(f14Stake))
}

func cashedOut(micros string) []scenario.Step {
	return []scenario.Step{
		scenario.Post(cashOutsPath, cashOutOf(micros)),
		scenario.ExpectStatus(http.StatusAccepted),
		scenario.ExpectJSON("status", "started"),
		scenario.Remember("id", "job"),
	}
}

func cashOutRefused(code errs.Code) []scenario.Step {
	return []scenario.Step{
		scenario.ExpectProblem(code),
		scenario.ExpectEvents(events.TypeCashOutStarted, 0),
		cabalHolds(),
	}
}

func jobEnds(status string, code string) scenario.Step {
	return func(s *scenario.Scenario) {
		scenario.Eventually("the cash out job ends "+status, func(s *scenario.Scenario) bool {
			got, _ := jobState(s)
			return got == status
		})(s)
		if _, got := jobState(s); got != code {
			s.Fatalf("flows: cash out job result code %q, want %q", got, code)
		}
		scenario.Get(cashOutJobPath)(s)
		scenario.ExpectStatus(http.StatusOK)(s)
		scenario.ExpectJSON("status", status)(s)
	}
}

func jobState(s *scenario.Scenario) (status, code string) {
	if err := s.DB().QueryRow(s.Context(),
		`SELECT status, coalesce(result_code, '') FROM cash_out_jobs WHERE id = $1`, s.Recall("job"),
	).Scan(&status, &code); err != nil {
		s.Fatalf("flows: read cash out job %s: %v", s.Recall("job"), err)
	}
	return status, code
}

func payoutAttempts(want ...string) scenario.Step {
	return func(s *scenario.Scenario) {
		var got []string
		if err := s.DB().QueryRow(s.Context(),
			`SELECT coalesce(array_agg(status ORDER BY attempt), '{}') FROM cash_out_payouts WHERE job_id = $1`,
			s.Recall("job"),
		).Scan(&got); err != nil {
			s.Fatalf("flows: read payout attempts: %v", err)
		}
		if len(got) != len(want) {
			s.Fatalf("flows: payout attempts %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				s.Fatalf("flows: payout attempts %v, want %v", got, want)
			}
		}
	}
}

func jobEvents(typ events.Type, n int) scenario.Step {
	return func(s *scenario.Scenario) {
		var got int
		if err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM events WHERE type = $1 AND payload->>'job_id' = $2`, string(typ), s.Recall("job"),
		).Scan(&got); err != nil {
			s.Fatalf("flows: count %s events: %v", typ, err)
		}
		if got != n {
			s.Fatalf("flows: %d %s events for the job, want %d", got, typ, n)
		}
	}
}

func sharesHeld(want string) scenario.Step {
	return func(s *scenario.Scenario) {
		if got := heldShares(s); got != want {
			s.Fatalf("flows: member holds %s share units, want %s", got, want)
		}
	}
}

func treasurySigning(action fakes.Action, times int) scenario.Step {
	return func(s *scenario.Scenario) {
		scenario.FakeUpstream(fakes.Step{
			Route: signRoute(s), Method: http.MethodPost, Reset: true,
			Action: action, Status: http.StatusServiceUnavailable, Times: times,
		})(s)
	}
}

func signRoute(s *scenario.Scenario) string {
	var wallet string
	if err := s.DB().QueryRow(s.Context(),
		`SELECT privy_wallet_id FROM treasury_wallets WHERE cabal_id = $1`, s.Recall("cabal"),
	).Scan(&wallet); err != nil {
		s.Fatalf("flows: read treasury wallet: %v", err)
	}
	return "/privy/v1/wallets/" + wallet + "/rpc"
}

func chainSays(fixture string) scenario.Step {
	return scenario.FakeUpstream(fakes.Step{
		Route: "/rpc/getSignatureStatuses", Action: fakes.ActionSucceed,
		Fixture: "/rpc/getSignatureStatuses/" + fixture, Times: 100, Reset: true,
	})
}

func outageRetried(code errs.Code) scenario.Step {
	return func(s *scenario.Scenario) {
		var started string
		if err := s.DB().QueryRow(s.Context(),
			`SELECT id::text FROM events WHERE type = $1 AND payload->>'job_id' = $2`,
			string(events.TypeCashOutStarted), s.Recall("job"),
		).Scan(&started); err != nil {
			s.Fatalf("flows: read the cash out started event: %v", err)
		}
		scenario.EventuallyLog(observability.BusDispatched, map[string]string{
			"handler": cashOutPayout, "event_id": started, "code": string(code),
		})(s)
	}
}

func paid() []scenario.Step {
	return []scenario.Step{
		jobEnds("completed", ""),
		payoutAttempts("confirmed"),
		jobEvents(events.TypeCashOutCompleted, 1),
		jobEvents(events.TypeCashOutFailed, 0),
		sharesHeld("1000000"),
		cabalHolds(),
	}
}

func F14CashOutOK(s *scenario.Scenario) {
	s.Given(append(f14Staked("ok"), chainSays("finalized"))...).
		When(append(cashedOut("1000000"), scenario.Replay(), scenario.ExpectEvents(events.TypeCashOutStarted, 1))...).
		Then(paid()...)
}

func F14CashOutInvalidInput(s *scenario.Scenario) {
	s.Given(f14Staked("invalid")...).
		When(scenario.Post(cashOutsPath, cashOutOf("99999"))).
		Then(append(cashOutRefused(errs.CodeInvalidInput), sharesHeld("2000000"))...)
}

func F14CashOutInsufficientShares(s *scenario.Scenario) {
	s.Given(f14Founded("shares")...).
		When(scenario.Post(cashOutsPath, cashOutOf("1000000"))).
		Then(cashOutRefused(errs.CodeInsufficientShares)...)
}

func F14CashOutCashOutInProgress(s *scenario.Scenario) {
	s.Given(append(f14Staked("progress"), treasurySigning(fakes.ActionFail, 1000), chainSays("finalized"))...).
		When(append(cashedOut("500000"), scenario.Post(cashOutsPath, cashOutOf("500000")))...).
		Then(
			scenario.ExpectProblem(errs.CodeCashOutInProgress),
			scenario.ExpectEvents(events.TypeCashOutStarted, 1),
			sharesHeld("1500000"),
			treasurySigning(fakes.ActionSucceed, 1),
			jobEnds("completed", ""),
			payoutAttempts("confirmed"),
			sharesHeld("1500000"),
			cabalHolds(),
		)
}

func F14CashOutCabalPaused(s *scenario.Scenario) {
	s.Given(append(f14Staked("paused"), func(s *scenario.Scenario) {
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at)
			VALUES (gen_random_uuid(), $1, 'ops', $2)`, s.Recall("cabal"), time.Now().UTC()); err != nil {
			s.Fatalf("flows: pause the cabal: %v", err)
		}
	})...).
		When(scenario.Post(cashOutsPath, cashOutOf("1000000"))).
		Then(append(cashOutRefused(errs.CodeCabalPaused), sharesHeld("2000000"))...)
}

func F14CashOutPriceUnavailable(s *scenario.Scenario) {
	s.Given(append(f14Staked("unpriced"), holding(unpricedMint, 1))...).
		When(scenario.Post(cashOutsPath, cashOutOf("1000000"))).
		Then(append(cashOutRefused(errs.CodePriceUnavailable), sharesHeld("2000000"))...)
}

func F14CashOutPayoutsPrivyUnavailable(s *scenario.Scenario) {
	s.Given(append(f14Staked("privy"),
		treasurySigning(fakes.ActionFail, privyAttempts), chainSays("finalized"))...).
		When(cashedOut("1000000")...).
		Then(append([]scenario.Step{outageRetried(errs.CodePrivyUnavailable)}, paid()...)...)
}

func F14CashOutPayoutsRPCUnavailable(s *scenario.Scenario) {
	s.Given(append(f14Staked("rpc"), scenario.FakeUpstream(fakes.Step{
		Route: "/rpc/getLatestBlockhash", Action: fakes.ActionFail, Status: http.StatusServiceUnavailable,
		Times: rpcAttempts,
	}), chainSays("finalized"))...).
		When(cashedOut("1000000")...).
		Then(append([]scenario.Step{outageRetried(errs.CodeRPCUnavailable)}, paid()...)...)
}

func CashOutRejectedOnChain(s *scenario.Scenario) {
	s.Given(append(f14Staked("failed"), chainSays("rejected"))...).
		When(cashedOut("1000000")...).
		Then(
			jobEnds("failed", string(errs.CodePayoutFailed)),
			payoutAttempts("failed"),
			jobEvents(events.TypeCashOutFailed, 1),
			jobEvents(events.TypeCashOutCompleted, 0),
			sharesHeld("2000000"),
			cabalHolds(),
		)
}
