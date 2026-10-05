package trading_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type flow11 struct {
	*engineEnv
	s        *scenario.Scenario
	voter    ids.UserID
	proposal ids.ProposalID
	votes    string
}

type flowSeedT struct{ *scenario.Scenario }

func (flowSeedT) Helper() {}

func newFlow11(t *testing.T, arrange func(e *engineEnv)) *flow11 {
	t.Helper()
	e := newEngineEnvOn(newLayerEnvOn(nil))
	f := &flow11{engineEnv: e}
	ports := e.ports()
	ports.Proposals = nil
	f.s = scenario.New(t, scenario.WithModules(
		func(d module.Deps) module.Module { return governance.New(d) },
		func(d module.Deps) module.Module {
			d.Config.Solana.RPCURL, d.Config.Timeouts.RPC = "http://127.0.0.1:1", time.Second
			return trading.New(d, trading.WithEnginePorts(ports), trading.WithChain(e.venue, e.signer))
		},
	))
	c := testkit.NewCabal(flowSeedT{f.s}, f.s.DB())
	e.cabal, f.voter = c.ID, c.Creator.ID
	e.seedCabal(cabal.StatusActive, 100)
	if arrange != nil {
		arrange(e)
	}
	f.openBuy(t)
	return f
}

func (f *flow11) openBuy(t *testing.T) {
	t.Helper()
	id, now := f.ids.NewV7(), clock.Real{}.Now().UTC()
	f.proposal, f.votes = ids.ProposalIDFrom(id), "/v1/proposals/"+id.String()+"/votes"
	if _, err := f.s.DB().Exec(t.Context(), `WITH p AS (
		INSERT INTO proposals (id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount,
			status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'buy', 'AAPLx', $4, $5, $6, 'open', $7, $8, $8) RETURNING id)
		INSERT INTO proposal_voters (proposal_id, voter_id) SELECT p.id, $3 FROM p`,
		id, f.cabal.UUID(), f.voter.UUID(), aaplxMint, buyMicros, quotedOut, now.Add(24*time.Hour), now,
	); err != nil {
		t.Fatalf("flow 11: insert the proposal: %v", err)
	}
}

func (f *flow11) pass(then ...scenario.Step) {
	f.s.Given(scenario.AsSeededUser("alice", f.voter)).
		When(
			scenario.Post(f.votes, `{"choice":"yes"}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "passed"),
			scenario.EventuallyEvent(busevents.TypeProposalPassed),
		).
		Then(then...)
}

func (f *flow11) blocks(code errs.Code, have, need string) {
	f.pass(
		scenario.ExpectEvents(busevents.TypeTradeBlocked, 1),
		scenario.ExpectEventPayload(busevents.TypeTradeBlocked, map[string]any{
			"code": string(code), "have": have, "need": need, "action": "buy", "symbol": "AAPLx",
			"cabal_id": f.cabal.String(), "source_batch_size": 1,
		}),
		scenario.ExpectEvents(busevents.TypeTradeSubmitted, 0),
	)
}

func TestFlow11_ExecuteTrade_OK(t *testing.T) {
	t.Parallel()
	f := newFlow11(t, nil)
	f.pass(
		scenario.ExpectEvents(busevents.TypeTradeSubmitted, 1),
		scenario.ExpectEvents(busevents.TypeTradeConfirmed, 1),
		scenario.ExpectEventPayload(busevents.TypeTradeConfirmed, map[string]any{
			"action": "buy", "symbol": "AAPLx", "in_amount": "25000000", "usdc_micros": "25000000",
		}),
		scenario.ExpectEvents(busevents.TypeTradeBlocked, 0),
	)
}

func TestFlow11_ExecuteTrade_AssetUntradable(t *testing.T) {
	t.Parallel()
	newFlow11(t, func(e *engineEnv) { e.catalog.Fail("AssetBySymbol", errs.New(errs.CodeAssetNotFound, "market")) }).
		blocks(errs.CodeAssetUntradable, "0", "0")
}

func TestFlow11_ExecuteTrade_InsufficientFunds(t *testing.T) {
	t.Parallel()
	newFlow11(t, func(e *engineEnv) { e.ledger.SetTokens(e.wallet.Address, usdcToken(), 24_999_999) }).
		blocks(errs.CodeInsufficientFunds, "24999999", "25000000")
}

func TestFlow11_ExecuteTrade_SlippageExceeded(t *testing.T) {
	t.Parallel()
	newFlow11(t, func(e *engineEnv) { e.quote(usdcToken(), aaplxToken(), 20_000_000, true) }).
		blocks(errs.CodeSlippageExceeded, "20000000", "20790000")
}

func TestFlow11_ExecuteTrade_NoRoute(t *testing.T) {
	t.Parallel()
	newFlow11(t, func(e *engineEnv) { e.quote(usdcToken(), aaplxToken(), 0, false) }).
		blocks(errs.CodeNoRoute, "0", "0")
}

func TestFlow11_ExecuteTrade_CabalPaused(t *testing.T) {
	t.Parallel()
	newFlow11(t, func(e *engineEnv) { e.pauses.Pause(e.cabal, funding.PauseReasonExternalDeposit) }).
		blocks(errs.CodeCabalPaused, "0", "0")
}

func TestFlow11_ExecuteTrade_JupiterUnavailable(t *testing.T) {
	t.Parallel()
	f := newFlow11(t, func(e *engineEnv) {
		e.jup.FailOnce("Quote", errs.New(errs.CodeJupiterUnavailable, "jupiter.Quote"))
	})
	f.pass(
		scenario.ExpectEvents(busevents.TypeTradeBlocked, 0),
		scenario.ExpectEvents(busevents.TypeTradeConfirmed, 1),
	)
}

func TestFlow11_ExecuteTrade_SwapFailed(t *testing.T) {
	t.Parallel()
	f := newFlow11(t, func(e *engineEnv) {
		e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: 6001})
	})
	f.pass(
		scenario.ExpectEvents(busevents.TypeTradeFailed, 1),
		scenario.ExpectEventPayload(busevents.TypeTradeFailed, map[string]any{
			"failure_code": "jupiter_failed", "jupiter_code": "6001",
		}),
		scenario.ExpectEvents(busevents.TypeTradeConfirmed, 0),
		scenario.ExpectEvents(busevents.TypeTradeBlocked, 0),
	)
}
