package trading_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type flow12 struct {
	*flow11
	swap uuid.UUID
}

func newFlow12(t *testing.T, fail bool) *flow12 {
	t.Helper()
	f := &flow12{flow11: newFlow11(t, func(e *engineEnv) {
		if fail {
			e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: 6001})
		}
	})}
	seed := fakes.CabalSeed{
		View:  cabal.View{ID: f.cabal, Status: cabal.StatusActive},
		Rules: cabal.Rules{SlippageBps: 100}, Wallet: f.wallet,
	}
	member := fakes.CabalMember{CabalID: f.cabal, Member: cabal.MemberView{UserID: f.voter}}
	f.cabals = fakes.NewCabal([]fakes.CabalSeed{seed}, []fakes.CabalMember{member})
	want := busevents.TypeTradeConfirmed
	if fail {
		want = busevents.TypeTradeFailed
	}
	f.pass(scenario.ExpectEvents(want, 1))
	if err := f.s.DB().QueryRow(t.Context(), `SELECT id FROM swaps WHERE source_id = $1`, f.proposal.UUID()).
		Scan(&f.swap); err != nil {
		t.Fatal(err)
	}
	next := swapTx()
	next[len(next)-2] = 1
	f.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(aaplxToken()),
		jupiter.Order{RequestID: "req-2", Transaction: next})
	return f
}

func (f *flow12) retryPath() string { return "/v1/swaps/" + f.swap.String() + "/retry" }

func (f *flow12) refused(given scenario.Step, path string, status int, code errs.Code) {
	f.s.Given(given).
		When(scenario.Post(path, ""), scenario.ExpectStatus(status), scenario.ExpectProblem(code)).
		Then(scenario.ExpectAllEvents(busevents.TypeTradeRetryRequested, 0))
}

func (f *flow12) proposalStatus(t *testing.T) string {
	t.Helper()
	var status string
	if err := f.s.DB().QueryRow(t.Context(), `SELECT status FROM proposals WHERE id = $1`, f.proposal.UUID()).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (f *flow12) swapStatuses(t *testing.T) []string {
	t.Helper()
	rows, err := f.s.DB().Query(t.Context(), `SELECT status FROM swaps WHERE source_id = $1 ORDER BY created_at, id`,
		f.proposal.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			t.Fatal(err)
		}
		out = append(out, status)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFlow12_RetryTrade_OK(t *testing.T) {
	t.Parallel()
	t.Run("voted quote", func(t *testing.T) {
		t.Parallel()
		retryAtTheVotedQuote(t)
	})
	t.Run("at current price", func(t *testing.T) {
		t.Parallel()
		retryAtTheCurrentPrice(t)
	})
}

func (f *flow12) latestSwap(t *testing.T) (uuid.UUID, int64) {
	t.Helper()
	var id uuid.UUID
	var quote int64
	err := f.s.DB().QueryRow(t.Context(), `SELECT id, quote_out_amount FROM swaps WHERE source_id = $1
		ORDER BY created_at DESC, id DESC LIMIT 1`, f.proposal.UUID()).Scan(&id, &quote)
	if err != nil {
		t.Fatal(err)
	}
	return id, quote
}

func (f *flow12) retryLatest(t *testing.T, body string, want busevents.Type, count int) {
	t.Helper()
	id, _ := f.latestSwap(t)
	f.s.Given(scenario.AsSeededUser("alice", f.voter)).
		When(
			scenario.Post("/v1/swaps/"+id.String()+"/retry", body),
			scenario.ExpectStatus(http.StatusAccepted),
		).
		Then(scenario.Eventually(string(want), func(*scenario.Scenario) bool {
			var n int
			err := f.s.DB().QueryRow(t.Context(),
				`SELECT count(*) FROM events WHERE type = $1 AND payload->'source'->>'id' = $2`,
				string(want), f.proposal.UUID().String()).Scan(&n)
			return err == nil && n >= count
		}))
}

func retryAtTheCurrentPrice(t *testing.T) {
	t.Helper()
	const fresh = 20_000_000
	order := func(e *engineEnv, out uint64) {
		e.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(aaplxToken()), jupiter.Order{
			RequestID: "req-1", Transaction: swapTx(), OutAmount: money.NewBaseUnits(out, aaplxToken().Decimals),
		})
	}
	f := &flow12{flow11: newFlow11(t, func(e *engineEnv) {
		e.quote(usdcToken(), aaplxToken(), fresh, true)
		order(e, fresh*95/100)
	})}
	f.cabals = fakes.NewCabal([]fakes.CabalSeed{{
		View:  cabal.View{ID: f.cabal, Status: cabal.StatusActive},
		Rules: cabal.Rules{SlippageBps: 100}, Wallet: f.wallet,
	}}, []fakes.CabalMember{{CabalID: f.cabal, Member: cabal.MemberView{UserID: f.voter}}})
	f.pass(scenario.ExpectEvents(busevents.TypeTradeFailed, 1))
	id, _ := f.latestSwap(t)
	f.s.Given(scenario.AsSeededUser("alice", f.voter)).
		When(
			scenario.Post("/v1/swaps/"+id.String()+"/retry", `{"at_current_price":true,"accepted_quote_out_amount":1}`),
			scenario.ExpectStatus(http.StatusBadRequest),
			scenario.ExpectProblem(errs.CodeInvalidInput),
		).
		Then(scenario.ExpectAllEvents(busevents.TypeTradeRetryRequested, 0))
	f.retryLatest(t, `{"at_current_price":true}`, busevents.TypeTradeFailed, 2)
	order(f.engineEnv, fresh)
	f.retryLatest(t, "", busevents.TypeTradeConfirmed, 1)
	if got := f.swapStatuses(t); len(got) != 3 || got[0] != "failed" || got[1] != "failed" || got[2] != "confirmed" {
		t.Fatalf("swaps %v, want two price_moved swaps then a confirmed one", got)
	}
	if _, quote := f.latestSwap(t); quote != fresh {
		t.Fatalf("held quote %d, want the execution-time Jupiter quote %d and never a client number", quote, fresh)
	}
}

func retryAtTheVotedQuote(t *testing.T) {
	t.Helper()
	f := newFlow12(t, true)
	f.s.Given(scenario.AsSeededUser("alice", f.voter)).
		When(
			scenario.Post(f.retryPath(), ""),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.ExpectJSON("swap_id", f.swap.String()),
			scenario.ExpectJSON("status", "retry_requested"),
			scenario.Replay(),
			scenario.EventuallyEvent(busevents.TypeTradeRetryRequested),
		).
		Then(
			scenario.ExpectAllEvents(busevents.TypeTradeRetryRequested, 1),
			scenario.ExpectAllEvents(busevents.TypeTradeConfirmed, 1),
			scenario.EventuallyEvent(busevents.TypeTradeConfirmed),
		)
	if got := f.swapStatuses(t); len(got) != 2 || got[0] != "failed" || got[1] != "confirmed" {
		t.Fatalf("swaps %v, want the failed swap then a confirmed one", got)
	}
	if got := f.proposalStatus(t); got != "executed" {
		t.Fatalf("proposal status %q, want executed", got)
	}
}

func TestFlow12_RetryTrade_Unauthorized(t *testing.T) {
	t.Parallel()
	f := newFlow12(t, true)
	f.refused(scenario.Anonymous(), f.retryPath(), http.StatusUnauthorized, errs.CodeUnauthorized)
}

func TestFlow12_RetryTrade_SwapNotFound(t *testing.T) {
	t.Parallel()
	f := newFlow12(t, true)
	f.refused(scenario.AsSeededUser("alice", f.voter), "/v1/swaps/"+f.ids.NewV7().String()+"/retry",
		http.StatusNotFound, errs.CodeSwapNotFound)
}

func TestFlow12_RetryTrade_NotCabalMember(t *testing.T) {
	t.Parallel()
	f := newFlow12(t, true)
	f.refused(scenario.AsUser("mallory"), f.retryPath(), http.StatusForbidden, errs.CodeNotCabalMember)
}

func TestFlow12_RetryTrade_SwapNotRetryable(t *testing.T) {
	t.Parallel()
	f := newFlow12(t, false)
	f.refused(scenario.AsSeededUser("alice", f.voter), f.retryPath(), http.StatusUnprocessableEntity,
		errs.CodeSwapNotRetryable)
}

func TestFlow12_RetryTrade_InsufficientFunds(t *testing.T) {
	t.Parallel()
	f := newFlow12(t, true)
	f.ledger.SetTokens(f.wallet.Address, usdcToken(), 24_999_999)
	f.s.Given(scenario.AsSeededUser("alice", f.voter)).
		When(
			scenario.Post(f.retryPath(), ""),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.EventuallyEvent(busevents.TypeTradeRetryRequested),
		).
		Then(
			scenario.ExpectAllEvents(busevents.TypeTradeBlocked, 1),
			scenario.ExpectEventPayload(busevents.TypeTradeBlocked, map[string]any{
				"code": string(errs.CodeInsufficientFunds), "have": "24999999", "need": "25000000",
			}),
			scenario.EventuallyEvent(busevents.TypeTradeBlocked),
		)
	if got := f.proposalStatus(t); got != "execution_blocked" {
		t.Fatalf("proposal status %q, want execution_blocked", got)
	}
}
