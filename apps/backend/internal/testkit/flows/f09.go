package flows

import (
	"net/http"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	f09AAPLxMint      = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	f09USDCMint       = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	f09Buy            = `{"kind":"buy","symbol":"AAPLx","usdc_micros":25000000}`
	f09NoRouteBuy     = `{"kind":"buy","symbol":"AAPLx","usdc_micros":24000000}`
	f09UnavailableBuy = `{"kind":"buy","symbol":"AAPLx","usdc_micros":23000000}`
	f09CrashBuy       = `{"kind":"buy","symbol":"AAPLx","usdc_micros":22000000}`
)

type f09Cabal struct {
	id      ids.CabalID
	members []ids.UserID
}

func seedF09(s *scenario.Scenario, holdings ...string) f09Cabal {
	cabal := testkit.NewCabal(seedT{s}, s.DB(), testkit.WithMembers(3))
	ensureSamplerCatalog()(s)
	if _, err := s.DB().Exec(s.Context(), `UPDATE assets SET chain_checked_at = $1,
		tradable_override = CASE symbol WHEN 'AAPLx' THEN true ELSE false END
		WHERE symbol IN ('AAPLx', 'JPSTx')`, time.Now().UTC()); err != nil {
		s.Fatalf("flows: set flow 09 tradability: %v", err)
	}
	for _, holding := range holdings {
		asset, units := f09USDCMint, holding
		if holding == "aapl" {
			asset, units = f09AAPLxMint, "11000000"
		}
		seedF09Holding(s, cabal.ID, asset, units)
	}
	users := make([]ids.UserID, len(cabal.Members))
	for i, member := range cabal.Members {
		users[i] = member.ID
	}
	return f09Cabal{id: cabal.ID, members: users}
}

func seedF09Holding(s *scenario.Scenario, cabal ids.CabalID, asset, units string) {
	id, now := ids.Real{}.NewV7(), time.Now().UTC()
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO cabal_txns
		(id, cabal_id, seq, kind, status, swap_id, transfer_id, tx_signature, created_at)
		VALUES ($1, $2, 1, 'fund', 'settled', NULL, NULL, '', $3)`, id, cabal.UUID(), now); err != nil {
		s.Fatalf("flows: seed flow 09 header: %v", err)
	}
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO cabal_txn_entries
		(txn_id, seq, account, asset, amount) VALUES ($1, 0, 'treasury', $2, $3), ($1, 1, 'venue', $2, -$3)`,
		id, asset, units); err != nil {
		s.Fatalf("flows: seed flow 09 entries: %v", err)
	}
	cost := "0"
	if asset == f09USDCMint {
		cost = units
	}
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO cabal_positions
		(cabal_id, asset, units, cost_basis_micros, updated_at) VALUES ($1, $2, $3, $4, $5)`,
		cabal.UUID(), asset, units, cost, now); err != nil {
		s.Fatalf("flows: seed flow 09 position: %v", err)
	}
}

func f09Path(c f09Cabal) string { return "/v1/cabals/" + c.id.String() + "/proposals" }

func f09Route(s *scenario.Scenario, amount string, action fakes.Action, fixture string) {
	scenario.FakeUpstream(fakes.Step{
		Route: "/jupiter/swap/v2/order", Query: map[string]string{"amount": amount}, Action: action, Fixture: fixture,
		Status: http.StatusServiceUnavailable, Times: 100,
	})(s)
}

func f09Member(c f09Cabal) scenario.Step { return scenario.AsSeededUser("alice", c.members[0]) }

func F09ProposeTradeOK(s *scenario.Scenario) {
	c := seedF09(s, "100000000")
	s.Given(f09Member(c), func(s *scenario.Scenario) { f09Route(s, "25000000", fakes.ActionSucceed, "/jupiter/swap/v2/order") }).
		When(scenario.Post(f09Path(c), f09Buy), scenario.ExpectStatus(http.StatusCreated), scenario.Replay()).
		Then(scenario.ExpectEvents(events.TypeProposalCreated, 1), scenario.EventuallyPublished(events.TypeProposalCreated, 1))
}

func F09ProposeTradeInvalidInput(s *scenario.Scenario) {
	c := seedF09(s)
	s.Given(f09Member(c)).When(scenario.Post(f09Path(c), `{"kind":"buy","symbol":"AAPLx"}`)).
		Then(scenario.ExpectProblem(errs.CodeInvalidInput), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradeUnauthorized(s *scenario.Scenario) {
	c := seedF09(s)
	s.Given(scenario.Anonymous()).When(scenario.Post(f09Path(c), f09Buy)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradeNotCabalMember(s *scenario.Scenario) {
	c := seedF09(s)
	s.Given(scenario.AsUser("mallory")).When(scenario.Post(f09Path(c), f09Buy)).
		Then(scenario.ExpectProblem(errs.CodeNotCabalMember), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradeAssetNotFound(s *scenario.Scenario) {
	c := seedF09(s)
	s.Given(f09Member(c)).When(scenario.Post(f09Path(c), `{"kind":"buy","symbol":"NOPE","usdc_micros":1}`)).
		Then(scenario.ExpectProblem(errs.CodeAssetNotFound), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradeAssetUntradable(s *scenario.Scenario) {
	c := seedF09(s, "1")
	s.Given(f09Member(c)).When(scenario.Post(f09Path(c), `{"kind":"buy","symbol":"JPSTx","usdc_micros":1}`)).
		Then(scenario.ExpectProblem(errs.CodeAssetUntradable), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradeNoRoute(s *scenario.Scenario) {
	c := seedF09(s, "100000000")
	s.Given(f09Member(c), func(s *scenario.Scenario) {
		f09Route(s, "24000000", fakes.ActionSucceed, "/jupiter/swap/v2/order/no-route")
	}).
		When(scenario.Post(f09Path(c), f09NoRouteBuy)).
		Then(scenario.ExpectProblem(errs.CodeNoRoute), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradePotExceeded(s *scenario.Scenario) {
	c := seedF09(s, "100000000")
	s.Given(f09Member(c)).When(scenario.Post(f09Path(c), `{"kind":"buy","symbol":"AAPLx","usdc_micros":100000001}`)).
		Then(scenario.ExpectProblem(errs.CodePotExceeded), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradeInsufficientFunds(s *scenario.Scenario) {
	c := seedF09(s, "aapl")
	s.Given(f09Member(c)).When(scenario.Post(f09Path(c), `{"kind":"sell","symbol":"AAPLx","token_amount":11000001}`)).
		Then(scenario.ExpectProblem(errs.CodeInsufficientFunds), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradeJupiterUnavailable(s *scenario.Scenario) {
	c := seedF09(s, "100000000")
	s.Given(f09Member(c), func(s *scenario.Scenario) { f09Route(s, "23000000", fakes.ActionFail, "") }).
		When(scenario.Post(f09Path(c), f09UnavailableBuy)).
		Then(scenario.ExpectProblem(errs.CodeJupiterUnavailable), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradePriceUnavailable(s *scenario.Scenario) {
	c := seedF09(s, "aapl")
	s.Given(f09Member(c)).When(scenario.Post(f09Path(c), f09Buy)).
		Then(scenario.ExpectProblem(errs.CodePriceUnavailable), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F09ProposeTradeCrashAfterPublish(s *scenario.Scenario) {
	c := seedF09(s, "100000000")
	s.Given(f09Member(c), scenario.HoldRelay(), func(s *scenario.Scenario) {
		f09Route(s, "22000000", fakes.ActionSucceed, "/jupiter/swap/v2/order")
	}).When(
		scenario.Post(f09Path(c), f09CrashBuy), scenario.ExpectStatus(http.StatusCreated),
		scenario.PublishCrashingAt(faultpoint.AfterPublish),
	).Then(scenario.ExpectEvents(events.TypeProposalCreated, 1), scenario.EventuallyPublished(events.TypeProposalCreated, 1))
}
