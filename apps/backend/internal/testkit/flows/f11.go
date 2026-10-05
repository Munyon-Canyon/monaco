package flows

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	tradeMicros     = 30_000_000
	fundedMicros    = 100_000_000
	quotedOut       = 11_000_000
	noRouteMicros   = 31_000_000
	downMicros      = 32_000_000
	jupiterOrder    = "/jupiter/swap/v2/order"
	jupiterAttempts = 3
)

type trade struct {
	openProposal
	wallet   string
	treasury string
	given    []scenario.Step
}

type tradeOpts struct {
	symbol   string
	micros   int64
	quoteOut int64
	usdc     uint64
}

func seedTrade(s *scenario.Scenario, opts tradeOpts) trade {
	if opts.symbol == "" {
		opts.symbol = "AAPLx"
	}
	if opts.micros == 0 {
		opts.micros = tradeMicros
	}
	if opts.quoteOut == 0 {
		opts.quoteOut = quotedOut
	}
	if opts.usdc == 0 {
		opts.usdc = fundedMicros
	}
	c := testkit.NewCabal(seedT{s}, s.DB())
	t := trade{wallet: c.PrivyWalletID, treasury: string(fakes.PrivyWalletAddress(c.PrivyWalletID))}
	if _, err := s.DB().Exec(s.Context(), `UPDATE treasury_wallets SET address = $1 WHERE cabal_id = $2`,
		t.treasury, c.ID.UUID()); err != nil {
		s.Fatalf("flows: point the treasury at its fake Privy wallet: %v", err)
	}
	id, now := ids.Real{}.NewV7(), time.Now().UTC()
	t.openProposal = openProposal{
		id: ids.ProposalIDFrom(id), cabalID: c.ID, proposerID: c.Creator.ID, path: "/v1/proposals/" + id.String(),
		voters: []ids.UserID{c.Creator.ID},
	}
	t.votes = t.path + "/votes"
	if _, err := sqlc.New(s.DB()).InsertProposal(s.Context(), sqlc.InsertProposalParams{
		ID: id, CabalID: c.ID.UUID(), ProposerID: c.Creator.ID.UUID(), Kind: "buy", Symbol: opts.symbol,
		Mint: aaplxMint, UsdcMicros: pgtype.Int8{Int64: opts.micros, Valid: true},
		QuoteOutAmount: opts.quoteOut, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
		VoterIds: []uuid.UUID{c.Creator.ID.UUID()},
	}); err != nil {
		s.Fatalf("flows: insert proposal: %v", err)
	}
	testkit.NewLedger(seedT{s}, s.DB()).WithFundedMember(c.Creator.ID, c.ID, money.MicrosFromUint64(fundedMicros))
	ensureTradableAAPLx(s)
	t.given = []scenario.Step{
		scenario.FakeWallet(t.wallet),
		scenario.FakeBalance(fakes.SetBalance{
			Owner: t.treasury, Mint: string(testkit.USDCMint), Amount: opts.usdc, Decimals: 6,
		}),
		scenario.AsSeededUser("alice", t.voters[0]),
	}
	return t
}

func ensureTradableAAPLx(s *scenario.Scenario) {
	a, now := marketfake.AAPLx(), time.Now().UTC()
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO assets (
		id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable, company_key,
		first_seen_at, updated_at, chain_checked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8, $9, $9, $9)
		ON CONFLICT (mint) DO UPDATE SET chain_checked_at = coalesce(assets.chain_checked_at, $9)`,
		a.ID.UUID(), a.Symbol, a.Mint.String(), int16(a.Decimals), string(a.Issuer), string(a.Kind),
		a.DisplayName, a.CompanyKey, now,
	); err != nil {
		s.Fatalf("flows: seed a tradable AAPLx: %v", err)
	}
}

func (t trade) pass() []scenario.Step {
	return []scenario.Step{
		scenario.Post(t.votes, yes),
		scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectJSON("status", "passed"),
	}
}

func (t trade) ends(want events.Type) scenario.Step {
	return scenario.Eventually(string(want)+" for cabal "+t.cabalID.String(), func(s *scenario.Scenario) bool {
		return t.count(s, want) > 0
	})
}

func (t trade) count(s *scenario.Scenario, typ events.Type) int {
	var n int
	if err := s.DB().QueryRow(s.Context(),
		`SELECT count(*) FROM events WHERE type = $1 AND payload->>'cabal_id' = $2`,
		string(typ), t.cabalID.String()).Scan(&n); err != nil {
		s.Fatalf("flows: count %s events: %v", typ, err)
	}
	return n
}

func (t trade) expect(want map[events.Type]int) scenario.Step {
	return func(s *scenario.Scenario) {
		for _, typ := range []events.Type{
			events.TypeTradeBlocked, events.TypeTradeSubmitted, events.TypeTradeConfirmed, events.TypeTradeFailed,
		} {
			if got := t.count(s, typ); got != want[typ] {
				s.Fatalf("flows: %d %s events for cabal %s, want %d", got, typ, t.cabalID, want[typ])
			}
		}
	}
}

func (t trade) run(s *scenario.Scenario, given []scenario.Step, end events.Type, want map[events.Type]int) {
	s.Given(append(t.given, given...)...).
		When(t.pass()...).
		Then(t.ends(end), t.expect(want))
}

func (t trade) blocks(s *scenario.Scenario, code errs.Code, given ...scenario.Step) {
	t.run(s, given, events.TypeTradeBlocked, map[events.Type]int{events.TypeTradeBlocked: 1})
	scenario.ExpectEventPayload(events.TypeTradeBlocked, map[string]any{
		"cabal_id": t.cabalID.String(), "code": string(code),
	})(s)
}

func quoteFails(micros int64, step fakes.Step) scenario.Step {
	step.Route, step.Method = jupiterOrder, http.MethodGet
	step.Query = map[string]string{"amount": strconv.FormatInt(micros, 10)}
	return scenario.FakeUpstream(step)
}

func F11ExecuteTradeOK(s *scenario.Scenario) {
	seedTrade(s, tradeOpts{}).run(s, nil, events.TypeTradeConfirmed,
		map[events.Type]int{events.TypeTradeSubmitted: 1, events.TypeTradeConfirmed: 1})
}

func F11ExecuteTradeAssetUntradable(s *scenario.Scenario) {
	seedTrade(s, tradeOpts{symbol: "DELISTEDx"}).blocks(s, errs.CodeAssetUntradable)
}

func F11ExecuteTradeInsufficientFunds(s *scenario.Scenario) {
	seedTrade(s, tradeOpts{usdc: tradeMicros - 1}).blocks(s, errs.CodeInsufficientFunds)
}

func F11ExecuteTradeSlippageExceeded(s *scenario.Scenario) {
	seedTrade(s, tradeOpts{quoteOut: 2 * quotedOut}).blocks(s, errs.CodeSlippageExceeded)
}

func F11ExecuteTradeNoRoute(s *scenario.Scenario) {
	seedTrade(s, tradeOpts{micros: noRouteMicros}).blocks(s, errs.CodeNoRoute, quoteFails(noRouteMicros, fakes.Step{
		Action: fakes.ActionSucceed, Fixture: jupiterOrder + "/no-route",
	}))
}

func F11ExecuteTradeCabalPaused(s *scenario.Scenario) {
	t := seedTrade(s, tradeOpts{})
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at)
		VALUES ($1, $2, 'external_deposit', now())`, ids.Real{}.NewV7(), t.cabalID.UUID()); err != nil {
		s.Fatalf("flows: pause the cabal: %v", err)
	}
	t.blocks(s, errs.CodeCabalPaused)
}

func F11ExecuteTradeJupiterUnavailable(s *scenario.Scenario) {
	t := seedTrade(s, tradeOpts{micros: downMicros})
	down := fakes.Step{Action: fakes.ActionFail, Status: http.StatusServiceUnavailable, Times: jupiterAttempts}
	t.run(s, []scenario.Step{quoteFails(downMicros, down)},
		events.TypeTradeConfirmed, map[events.Type]int{events.TypeTradeSubmitted: 1, events.TypeTradeConfirmed: 1})
}

func F11ExecuteTradeSwapFailed(s *scenario.Scenario) {
	t := seedTrade(s, tradeOpts{})
	t.run(s, []scenario.Step{scenario.FakeSwap(fakes.SetSwap{Owner: t.treasury, Status: fakes.SwapFailed, Code: 6001})},
		events.TypeTradeFailed, map[events.Type]int{events.TypeTradeSubmitted: 1, events.TypeTradeFailed: 1})
	scenario.ExpectEventPayload(events.TypeTradeFailed, map[string]any{
		"cabal_id": t.cabalID.String(), "failure_code": "jupiter_failed", "jupiter_code": "6001",
	})(s)
}
