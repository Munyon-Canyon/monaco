package flows

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	cashOutsPath   = cabalsPath + "/{cabal}/cashouts"
	cashOutJobPath = cashOutsPath + "/{job}"
	potPath        = cabalsPath + "/{cabal}/pot"
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

func payoutDispatched(code errs.Code) scenario.Step {
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

func potHolds(cash int64, stocks int) []scenario.Step {
	return []scenario.Step{
		scenario.Get(potPath),
		scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectJSON("cash_micros", cash),
		scenario.ExpectField("holdings", func(s *scenario.Scenario, raw json.RawMessage) {
			var holdings []json.RawMessage
			if err := json.Unmarshal(raw, &holdings); err != nil || len(holdings) != stocks {
				s.Fatalf("flows: pot holdings %s, want %d stocks", raw, stocks)
			}
		}),
	}
}

func F14CashOutOK(s *scenario.Scenario) {
	s.Given(append(f14Staked("ok"), chainSays("finalized"))...).
		When(slices.Concat(potHolds(f14Stake, 0), cashedOut("1000000"),
			[]scenario.Step{scenario.Replay(), scenario.ExpectEvents(events.TypeCashOutStarted, 1)})...).
		Then(append(paid(), potHolds(f14Stake-1_000_000, 0)...)...)
	s.Then(scenario.EventuallyCapturedBy(events.TypeCashOutCompleted, "cash_out_completed", "job_id", s.Recall("job")))
}

func F14CashOutInvalidInput(s *scenario.Scenario) {
	s.Given(f14Staked("invalid")...).
		When(scenario.Post(cashOutsPath, `{"all":true,"usdc_micros":"1000000"}`)).
		Then(append(cashOutRefused(errs.CodeInvalidInput), sharesHeld("2000000"))...)
}

func F14CashOutPotValueChanged(s *scenario.Scenario) {
	s.Given(append(f14Staked("potmoved"), func(s *scenario.Scenario) {
		now := time.Now().UTC()
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO cash_out_jobs
			(id, cabal_id, user_id, share_units, payout_micros, slice_micros, status, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, gen_random_uuid(), 1, $2, $2, 'started', $3, $3)`,
			s.Recall("cabal"), f14Stake+1, now); err != nil {
			s.Fatalf("flows: reserve another member's cash out: %v", err)
		}
	})...).
		When(scenario.Post(cashOutsPath, cashOutOf("1000000"))).
		Then(append(cashOutRefused(errs.CodePotValueChanged), sharesHeld("2000000"))...)
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
		Then(append([]scenario.Step{payoutDispatched(errs.CodePrivyUnavailable)}, paid()...)...)
}

func F14CashOutPayoutsRPCUnavailable(s *scenario.Scenario) {
	s.Given(append(f14Staked("rpc"), scenario.FakeUpstream(fakes.Step{
		Route: "/rpc/getLatestBlockhash", Action: fakes.ActionFail, Status: http.StatusServiceUnavailable,
		Times: rpcAttempts,
	}), chainSays("finalized"))...).
		When(cashedOut("1000000")...).
		Then(append([]scenario.Step{payoutDispatched(errs.CodeRPCUnavailable)}, paid()...)...)
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
	s.Then(scenario.EventuallyCapturedBy(events.TypeCashOutFailed, "cash_out_failed", "job_id", s.Recall("job")))
}

const (
	f14StockUnits = 1_000_000
	f14StockQuote = 1_000_000
	f14SaleRaised = 990_000
)

func f14Stock(script string) chain.SolanaAddress {
	return chain.AddressOf(fakes.FixtureKey("f14-stock-" + script).Public().(ed25519.PublicKey))
}

func stockHeld(script string) scenario.Step {
	return func(s *scenario.Scenario) {
		mint, now := f14Stock(script), time.Now().UTC()
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO assets (
			id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable, company_key, first_seen_at, updated_at, chain_checked_at)
			VALUES ($4, $1, $2, 8, 'xstocks', 'equity', $1, true, $1, $3, $3, $3)`,
			"F14"+script+"x", string(mint), now, uuid.Must(uuid.NewV7())); err != nil {
			s.Fatalf("flows: seed the held stock: %v", err)
		}
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
			VALUES ($1, $2, 100000000, 'jupiter')`, string(mint), now); err != nil {
			s.Fatalf("flows: price the held stock: %v", err)
		}
		holding(mint, f14StockUnits)(s)
	}
}

func jupiterSells(script string, raised string) scenario.Step {
	return func(s *scenario.Scenario) {
		mint := string(f14Stock(script))
		payer := chain.AddressOf(fakes.FixtureKey("f14-relayer").Public().(ed25519.PublicKey))
		unsigned := chainfake.Unsigned(payer, chainfake.WalletAddress(treasuryWalletID(s)))
		order, _ := json.Marshal(map[string]any{
			"requestId": "req-f14-" + script, "inputMint": mint, "outputMint": testkit.USDCMint,
			"inAmount": strconv.Itoa(f14StockUnits), "outAmount": strconv.Itoa(f14StockQuote), "router": "iris",
			"priceImpactPct": "0.01", "routePlan": []any{map[string]any{"percent": 100}},
			"transaction": base64.StdEncoding.EncodeToString(unsigned),
		})
		scenario.FakeUpstream(fakes.Step{
			Route: "/jupiter/swap/v2/order", Method: http.MethodGet, Action: fakes.ActionSucceed,
			Query: map[string]string{"inputMint": mint}, Status: http.StatusOK, Body: order, Times: 2,
		})(s)
		executed, _ := json.Marshal(map[string]any{
			"status": "Success", "signature": "", "code": 0,
			"inputAmountResult": strconv.Itoa(f14StockUnits), "outputAmountResult": raised,
		})
		scenario.FakeUpstream(fakes.Step{
			Route: "/jupiter/swap/v2/execute", Method: http.MethodPost, Action: fakes.ActionSucceed,
			Status: http.StatusOK, Body: executed,
		})(s)
	}
}

func treasuryWalletID(s *scenario.Scenario) string {
	route := signRoute(s)
	return route[len("/privy/v1/wallets/") : len(route)-len("/rpc")]
}

func f14Selling(script string, raised int) []scenario.Step {
	return append(f14Staked(script), stockHeld(script), jupiterSells(script, strconv.Itoa(raised)),
		chainSays("finalized"))
}

func cashedOutAll() []scenario.Step {
	return []scenario.Step{
		scenario.Post(cashOutsPath, `{"all":true}`),
		scenario.ExpectStatus(http.StatusAccepted),
		scenario.ExpectJSON("status", "started"),
		scenario.Remember("id", "job"),
	}
}

func paidWhatTheSaleRaised(s *scenario.Scenario) {
	var payout, returned string
	if err := s.DB().QueryRow(s.Context(),
		`SELECT payout_micros::text, returned_units::text FROM cash_out_jobs WHERE id = $1`, s.Recall("job"),
	).Scan(&payout, &returned); err != nil {
		s.Fatalf("flows: read the cash out job: %v", err)
	}
	if want := strconv.Itoa(f14Stake + f14SaleRaised); payout != want || returned == "0" {
		s.Fatalf("flows: paid %s and returned %s share units, want %s paid and some units returned",
			payout, returned, want)
	}
	sharesHeld(returned)(s)
}

func F14CashOutPayoutsSaleShort(s *scenario.Scenario) {
	s.Given(f14Selling("short", f14SaleRaised)...).
		When(cashedOutAll()...).
		Then(
			jobEnds("partial", string(errs.CodeSaleShort)),
			payoutAttempts("confirmed"),
			jobEvents(events.TypeCashOutPartial, 1),
			jobEvents(events.TypeCashOutCompleted, 0),
			paidWhatTheSaleRaised,
			payoutDispatched(errs.CodeSaleShort),
			cabalHolds(),
		)
	s.Then(scenario.EventuallyCapturedBy(events.TypeCashOutPartial, "cash_out_partial", "job_id", s.Recall("job")))
}

func soldOnce(s *scenario.Scenario) {
	var swaps int
	if err := s.DB().QueryRow(s.Context(),
		`SELECT count(*) FROM swaps WHERE source_kind = 'cashout' AND source_id = $1`, s.Recall("job"),
	).Scan(&swaps); err != nil {
		s.Fatalf("flows: count the cash out swaps: %v", err)
	}
	if swaps != 1 {
		s.Fatalf("flows: the cash out sold in %d swaps, want 1", swaps)
	}
}

func sellCrashed(script string, point faultpoint.Name) func(*scenario.Scenario) {
	return func(s *scenario.Scenario) {
		s.Given(f14Selling(script, f14StockQuote)...).
			When(append(cashedOutAll(), scenario.PublishCrashingAt(point))...).
			Then(
				jobEnds("completed", ""),
				payoutAttempts("confirmed"),
				jobEvents(events.TypeCashOutCompleted, 1),
				jobEvents(events.TypeCashOutPartial, 0),
				soldOnce,
				sharesHeld("0"),
				cabalHolds(),
			)
	}
}

func F14CashOutPayoutsCrashAfterSellRequest(s *scenario.Scenario) {
	sellCrashed("sellcrash", faultpoint.AfterSellRequest)(s)
}

func F14CashOutPayoutsCrashAfterSellConfirm(s *scenario.Scenario) {
	sellCrashed("confirmcrash", faultpoint.AfterSellConfirm)(s)
}

func payoutCrashed(script string, point faultpoint.Name) func(*scenario.Scenario) {
	return func(s *scenario.Scenario) {
		s.Given(append(f14Staked(script), chainSays("finalized"))...).
			When(append(cashedOut("1000000"), scenario.PublishCrashingAt(point))...).
			Then(
				jobEnds("completed", ""),
				jobEvents(events.TypeCashOutCompleted, 1),
				jobEvents(events.TypeCashOutFailed, 0),
				sharesHeld("1000000"),
				cabalHolds(),
			)
	}
}

func F14CashOutPayoutsCrashAfterSign(s *scenario.Scenario) {
	payoutCrashed("signcrash", faultpoint.AfterSign)(s)
}

func F14CashOutPayoutsCrashAfterBroadcast(s *scenario.Scenario) {
	payoutCrashed("broadcastcrash", faultpoint.AfterBroadcast)(s)
}

func F14CashOutPayoutsCrashBeforeCommit(s *scenario.Scenario) {
	payoutCrashed("commitcrash", faultpoint.BeforeCommit)(s)
}
