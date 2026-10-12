package flows

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func F30PinnedBuyChainTradingOff(s *scenario.Scenario) {
	cabal := testkit.NewCabal(seedT{s}, s.DB())
	testkit.NewLedger(seedT{s}, s.DB()).
		WithFundedMember(cabal.Creator.ID, cabal.ID, money.MicrosFromUint64(f30FundedMicros))
	proposal := f30PinnedOpen(s, cabal.ID, cabal.Creator.ID)
	s.Given(scenario.AsSeededUser("member", cabal.Creator.ID)).
		When(
			scenario.Post("/v1/proposals/"+proposal.String()+"/votes", yes),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "passed"),
			scenario.Eventually("the pinned proposal is blocked because Base trading is off",
				f30ProposalIs(proposal, "execution_blocked", errs.CodeChainTradingOff)),
		).
		Then(
			scenario.ExpectEventPayload(events.TypeTradeBlocked, map[string]any{
				"cabal_id": cabal.ID.String(), "code": string(errs.CodeChainTradingOff),
			}),
			f30NoSwap(proposal),
		)
}

func f30PinnedOpen(s *scenario.Scenario, cabal ids.CabalID, voter ids.UserID) uuid.UUID {
	s.Helper()
	id, now := ids.Real{}.NewV7(), time.Now().UTC()
	if _, err := sqlc.New(s.DB()).InsertProposal(s.Context(), sqlc.InsertProposalParams{
		ID: id, CabalID: cabal.UUID(), ProposerID: voter.UUID(), Kind: "buy", Symbol: "AAPLx", Mint: aaplxMint,
		UsdcMicros: pgtype.Int8{Int64: RoutedBuyMicros, Valid: true}, QuoteOutAmount: RoutedRequotedUnits,
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, Threshold: "majority",
		VoterIds: []uuid.UUID{voter.UUID()},
	}); err != nil {
		s.Fatalf("flows: insert the pinned proposal: %v", err)
	}
	comparison, err := json.Marshal(f30Comparison(multichain.SideBuy, RoutedBuyMicros))
	if err != nil {
		s.Fatalf("flows: encode the proposal's comparison: %v", err)
	}
	if _, err := s.DB().Exec(s.Context(), `UPDATE proposals SET network = $2, price_comparison = $3 WHERE id = $1`,
		id, string(RoutedListing().Network), comparison); err != nil {
		s.Fatalf("flows: pin the proposal to Base: %v", err)
	}
	return id
}

func f30ProposalIs(proposal uuid.UUID, status string, reason errs.Code) func(*scenario.Scenario) bool {
	return func(s *scenario.Scenario) bool {
		var got, why string
		err := s.DB().QueryRow(s.Context(),
			`SELECT status, coalesce(status_reason, '') FROM proposals WHERE id = $1`, proposal).Scan(&got, &why)
		return err == nil && got == status && why == string(reason)
	}
}

func f30NoSwap(proposal uuid.UUID) scenario.Step {
	return func(s *scenario.Scenario) {
		var n int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM swaps WHERE source_id = $1`,
			proposal).Scan(&n); err != nil {
			s.Fatalf("flows: count the pinned proposal's swaps: %v", err)
		}
		if n != 0 {
			s.Fatalf("flows: the pinned proposal has %d swaps, want none", n)
		}
	}
}
