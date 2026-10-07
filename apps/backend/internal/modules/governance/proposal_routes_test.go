package governance_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestProposalRoutes_aMemberAndAStrangerReadTheSameProposal(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernance())
	c := testkit.NewCabal(t, s.DB(), testkit.WithMembers(2))
	id, now := testkit.NewIDs(11).NewV7(), clock.Real{}.Now().UTC()
	params := sqlc.InsertProposalParams{
		ID: id, CabalID: c.ID.UUID(), ProposerID: c.Creator.ID.UUID(), Kind: "buy", Symbol: "AAPLx",
		Mint: aaplxMint, UsdcMicros: pgtype.Int8{Int64: 5_000_000, Valid: true}, QuoteOutAmount: 21_000_000,
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, Threshold: "majority",
	}
	for _, m := range c.Members {
		params.VoterIds = append(params.VoterIds, m.ID.UUID())
	}
	if _, err := sqlc.New(s.DB()).InsertProposal(t.Context(), params); err != nil {
		t.Fatal(err)
	}
	stranger, err := ids.ParseUserID(testkit.NewIDs(12).NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	detail := "/v1/proposals/" + id.String()
	s.Given(scenario.AsSeededUser("creator", c.Creator.ID)).
		When(
			scenario.Get("/v1/cabals/"+c.ID.String()+"/proposals?filter=open"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("next_cursor", nil),
			expectListedCanVote(t, true),
			scenario.Get(detail),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("can_vote", true),
			scenario.ExpectJSON("can_withdraw", true),
			scenario.ExpectJSON("swap", nil),
			scenario.ExpectJSON("tally", map[string]int{"yes": 0, "no": 0, "voters": 2, "needed": 2}),
			scenario.Get("/v1/me/pending-votes"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.AsSeededUser("stranger", stranger),
			scenario.Get("/v1/cabals/"+c.ID.String()+"/proposals?filter=open"),
			scenario.ExpectStatus(http.StatusOK),
			expectListedCanVote(t, false),
			scenario.Get(detail),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("can_vote", false),
			scenario.ExpectJSON("can_withdraw", false),
			scenario.ExpectJSON("my_ballot", nil),
		).
		Then(scenario.ExpectStatus(http.StatusOK))
}

func expectListedCanVote(t *testing.T, want bool) scenario.Step {
	t.Helper()
	return scenario.ExpectField("proposals", func(_ *scenario.Scenario, raw json.RawMessage) {
		var items []struct {
			CanVote bool `json:"can_vote"`
		}
		if err := json.Unmarshal(raw, &items); err != nil || len(items) != 1 || items[0].CanVote != want {
			t.Fatalf("listed proposals %s: want one with can_vote %v (err %v)", raw, want, err)
		}
	})
}
