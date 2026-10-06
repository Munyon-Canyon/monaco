package flows

import (
	"net/http"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func (t trade) swapFails() []scenario.Step {
	failing := scenario.FakeSwap(fakes.SetSwap{Owner: t.treasury, Status: fakes.SwapFailed, Code: 6001})
	return slices.Concat(t.given, []scenario.Step{failing})
}

func (t trade) afterFailedSwap(then ...scenario.Step) []scenario.Step {
	return slices.Concat(t.pass(), []scenario.Step{
		t.ends(events.TypeTradeFailed),
		scenario.FakeSwap(fakes.SetSwap{Owner: t.treasury, Status: fakes.SwapSuccess}),
	}, then)
}

func (t trade) retry() scenario.Step {
	return func(s *scenario.Scenario) {
		var id string
		if err := s.DB().QueryRow(s.Context(),
			`SELECT id::text FROM swaps WHERE source_id = $1 ORDER BY created_at, id LIMIT 1`, t.id.UUID(),
		).Scan(&id); err != nil {
			s.Fatalf("flows: read the swap to retry: %v", err)
		}
		scenario.Post("/v1/swaps/"+id+"/retry", "")(s)
	}
}

func F12RetryTradeOK(s *scenario.Scenario) {
	t := seedTrade(s, tradeOpts{})
	s.Given(t.swapFails()...).
		When(t.afterFailedSwap(
			t.retry(),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.ExpectJSON("status", "retry_requested"),
		)...).
		Then(
			t.ends(events.TypeTradeConfirmed),
			t.expect(map[events.Type]int{
				events.TypeTradeSubmitted: 2, events.TypeTradeFailed: 1, events.TypeTradeConfirmed: 1,
			}),
		)
}

func F12RetryTradeUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post("/v1/swaps/"+ids.Real{}.NewV7().String()+"/retry", "")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F12RetryTradeSwapNotFound(s *scenario.Scenario) {
	s.Given(scenario.AsUser("alice")).
		When(scenario.Post("/v1/swaps/"+ids.Real{}.NewV7().String()+"/retry", "")).
		Then(scenario.ExpectProblem(errs.CodeSwapNotFound))
}

func F12RetryTradeNotCabalMember(s *scenario.Scenario) {
	t := seedTrade(s, tradeOpts{})
	s.Given(t.swapFails()...).
		When(t.afterFailedSwap(scenario.AsUser("mallory"), t.retry())...).
		Then(scenario.ExpectProblem(errs.CodeNotCabalMember), t.expect(map[events.Type]int{
			events.TypeTradeSubmitted: 1, events.TypeTradeFailed: 1,
		}))
}

func F12RetryTradeSwapNotRetryable(s *scenario.Scenario) {
	t := seedTrade(s, tradeOpts{})
	s.Given(t.given...).
		When(append(t.pass(), t.ends(events.TypeTradeConfirmed), t.retry())...).
		Then(scenario.ExpectProblem(errs.CodeSwapNotRetryable), t.expect(map[events.Type]int{
			events.TypeTradeSubmitted: 1, events.TypeTradeConfirmed: 1,
		}))
}

func F12RetryTradeInsufficientFunds(s *scenario.Scenario) {
	t := seedTrade(s, tradeOpts{})
	s.Given(t.swapFails()...).
		When(t.afterFailedSwap(
			scenario.FakeBalance(fakes.SetBalance{
				Owner: t.treasury, Mint: string(testkit.USDCMint), Amount: tradeMicros - 1, Decimals: 6,
			}),
			t.retry(),
			scenario.ExpectStatus(http.StatusAccepted),
		)...).
		Then(
			t.ends(events.TypeTradeBlocked),
			scenario.ExpectEventPayload(events.TypeTradeBlocked, map[string]any{
				"cabal_id": t.cabalID.String(), "code": string(errs.CodeInsufficientFunds),
			}),
			t.expect(map[events.Type]int{
				events.TypeTradeSubmitted: 1, events.TypeTradeFailed: 1, events.TypeTradeBlocked: 1,
			}),
		)
}
