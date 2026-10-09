package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func withGovernanceAndAdmin() scenario.Option {
	return scenario.WithModules(
		func(d module.Deps) module.Module {
			return governance.New(d, governance.WithPorts(governance.Ports{Cabals: cabal.New(d).Queries()}))
		},
		func(d module.Deps) module.Module { return admin.New(d) },
	)
}

type lookupProposal struct {
	id       uuid.UUID
	cabal    ids.CabalID
	proposer ids.UserID
	voters   int
	voter    ids.UserID
}

func seedLookupProposal(t *testing.T, s *scenario.Scenario, c testkit.SeededCabal, status string) lookupProposal {
	t.Helper()
	id, now := ids.Real{}.NewV7(), clock.Real{}.Now().UTC()
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
	if status != "open" {
		if _, err := s.DB().
			Exec(t.Context(), `UPDATE proposals SET status = $2 WHERE id = $1`, id, status); err != nil {
			t.Fatal(err)
		}
	}
	return lookupProposal{
		id: id, cabal: c.ID, proposer: c.Creator.ID, voters: len(c.Members), voter: c.Members[0].ID,
	}
}

func seedHandledCabal(t *testing.T, s *scenario.Scenario) testkit.SeededCabal {
	t.Helper()
	creator := testkit.SeedUser(t, s.DB(), testkit.UserOpts{Handle: "proposer1"})
	return testkit.NewCabal(t, s.DB(), testkit.WithCreator(creator.ID), testkit.WithMembers(2))
}

func appendAs(t *testing.T, s *scenario.Scenario, gen ids.Generator, actor string, ev events.Event) {
	t.Helper()
	uow := db.New(s.DB(), gen, clock.Real{})
	ctx := observability.WithActor(context.WithoutCancel(t.Context()), actor)
	err := uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ev) })
	if err != nil {
		t.Fatal(err)
	}
}

func decode(t *testing.T, raw json.RawMessage, out any) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
}

func seedHistory(t *testing.T, s *scenario.Scenario, p lookupProposal) {
	t.Helper()
	gen := testkit.NewIDs(51)
	appendAs(t, s, gen, "user:"+p.proposer.String(), events.ProposalCreated{
		V: 1, ProposalID: p.id, CabalID: p.cabal.UUID(), ProposerID: p.proposer.UUID(), Kind: "buy", Symbol: "AAPLx",
		Mint: aaplxMint, USDCMicros: money.MicrosFromUint64(5_000_000), QuoteOutAmount: 21_000_000,
		ExpiresAt: clock.Real{}.Now().Add(time.Hour), VoterCount: p.voters,
	})
	appendAs(t, s, gen, "system:poller.test", events.ProposalPassed{
		V: 1, ProposalID: p.id, CabalID: p.cabal.UUID(), Kind: "buy", Symbol: "AAPLx", Mint: aaplxMint,
		USDCMicros: money.MicrosFromUint64(5_000_000), QuoteOutAmount: 21_000_000, ProposerID: p.proposer.UUID(),
	})
}

func expectProposerOnly(t *testing.T, p lookupProposal) scenario.Step {
	t.Helper()
	return scenario.ExpectField("proposer", func(s *scenario.Scenario, raw json.RawMessage) {
		var got map[string]any
		decode(t, raw, &got)
		if len(got) != 2 || got["id"] != p.proposer.String() || got["handle"] != "proposer1" {
			s.Fatalf("proposer = %s, want only the id and the handle", raw)
		}
	})
}

func expectHistory(t *testing.T, want ...[2]string) scenario.Step {
	t.Helper()
	return scenario.ExpectField("status_history", func(s *scenario.Scenario, raw json.RawMessage) {
		var got []struct {
			Status    string `json:"status"`
			ActorType string `json:"actor_type"`
		}
		decode(t, raw, &got)
		if len(got) != len(want) {
			s.Fatalf("status_history = %s, want %v", raw, want)
		}
		for i, w := range want {
			if got[i].Status != w[0] || got[i].ActorType != w[1] {
				s.Fatalf("status_history[%d] = %+v, want %v", i, got[i], w)
			}
		}
	})
}

func expectOneAction(t *testing.T) scenario.Step {
	t.Helper()
	return scenario.ExpectField("recent_admin_actions", func(s *scenario.Scenario, raw json.RawMessage) {
		var got []struct {
			Action string `json:"action"`
			Reason string `json:"reason"`
		}
		decode(t, raw, &got)
		if len(got) != 1 || got[0].Action != "proposal_void" || got[0].Reason != "spam proposal" {
			s.Fatalf("recent_admin_actions = %s, want the one proposal_void", raw)
		}
	})
}

func auditRowOf(t *testing.T, id uuid.UUID) scenario.Step {
	t.Helper()
	return scenario.Eventually("the audit row of the void", func(s *scenario.Scenario) bool {
		var n int
		err := s.DB().QueryRow(t.Context(), `SELECT count(*) FROM admin_actions WHERE target_id = $1`, id.String()).
			Scan(&n)
		return err == nil && n == 1
	})
}

func TestAdminProposalLookup_StatusHistory(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernanceAndAdmin())
	p := seedLookupProposal(t, s, seedHandledCabal(t, s), "open")
	seedHistory(t, s, p)
	path := "/v1/admin/proposals/" + p.id.String()
	s.Given(scenario.SeededAdmin("viewer", "viewer"), scenario.SeededAdmin("mod", "moderator"), scenario.AsUser("mod")).
		When(
			scenario.Post(path+"/void", `{"reason":"spam proposal"}`),
			scenario.ExpectStatus(http.StatusOK),
			auditRowOf(t, p.id),
			scenario.AsUser("viewer"),
			scenario.Get(path),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("id", p.id.String()),
			scenario.ExpectJSON("status", "voided"),
			scenario.ExpectJSON("swap", nil),
			expectProposerOnly(t, p),
			expectHistory(t, [2]string{"open", "user"}, [2]string{"passed", "system"}, [2]string{"voided", "admin"}),
			expectOneAction(t),
		).
		Then(scenario.ExpectStatus(http.StatusOK))
}

func seedFailedSwap(t *testing.T, s *scenario.Scenario, p lookupProposal) uuid.UUID {
	t.Helper()
	id := ids.Real{}.NewV7()
	_, err := s.DB().Exec(t.Context(), `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address,
		action, symbol, in_mint, out_mint, out_decimals, in_amount, slippage_bps, source_batch_size, status,
		failure_code, tx_signature, created_at, updated_at)
		VALUES ($1, 'proposal', $2, $3, 'treasury', 'buy', 'AAPLx', 'in', 'out', 8, 5000000, 100, 1, 'failed',
		'jupiter_failed', 'sig1', now(), now())`, id, p.id, p.cabal.UUID())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func expectBallot(t *testing.T, p lookupProposal) scenario.Step {
	t.Helper()
	return scenario.ExpectField("voters", func(s *scenario.Scenario, raw json.RawMessage) {
		var voters []map[string]any
		decode(t, raw, &voters)
		voted := 0
		for _, v := range voters {
			if v["user_id"] == p.voter.String() && v["choice"] == "yes" && v["cast_at"] != nil {
				voted++
			}
		}
		if len(voters) != p.voters || voted != 1 {
			s.Fatalf("voters = %s, want %d with the ballot of %s", raw, p.voters, p.voter)
		}
	})
}

func TestAdminProposalLookup_ShowsTheBallotsAndTheLinkedSwap(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernanceAndAdmin())
	p := seedLookupProposal(t, s, seedHandledCabal(t, s), "passed")
	if _, err := s.DB().Exec(t.Context(),
		`INSERT INTO votes (proposal_id, voter_id, choice, cast_at) VALUES ($1, $2, 'yes', now())`,
		p.id, p.voter.UUID()); err != nil {
		t.Fatal(err)
	}
	swap := seedFailedSwap(t, s, p)
	s.Given(scenario.SeededAdmin("viewer", "viewer"), scenario.AsUser("viewer")).
		When(
			scenario.Get("/v1/admin/proposals/"+p.id.String()),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "passed"),
			scenario.ExpectJSON("status_history", []string{}),
			scenario.ExpectJSON("recent_admin_actions", []string{}),
			expectBallot(t, p),
			scenario.ExpectField("swap", func(s *scenario.Scenario, raw json.RawMessage) {
				var got map[string]any
				decode(t, raw, &got)
				if got["swap_id"] != swap.String() || got["status"] != "failed" ||
					got["failure_code"] != "jupiter_failed" || got["tx_signature"] != "sig1" {
					s.Fatalf("swap = %s, want the failed swap %s", raw, swap)
				}
			}),
		).
		Then(scenario.ExpectStatus(http.StatusOK))
}

func TestAdminProposalLookup_RefusesWhatItMustNotShow(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernanceAndAdmin())
	p := seedLookupProposal(t, s, seedHandledCabal(t, s), "open")
	path := "/v1/admin/proposals/" + p.id.String()
	s.Given(scenario.AsSeededUser("member", p.proposer)).
		When(
			scenario.Get(path),
			scenario.ExpectProblem(errs.CodeAdminForbidden),
			scenario.Anonymous(),
			scenario.Get(path),
			scenario.ExpectProblem(errs.CodeUnauthorized),
			scenario.SeededAdmin("viewer", "viewer"),
			scenario.AsUser("viewer"),
			scenario.Get("/v1/admin/proposals/"+uuid.NewString()),
			scenario.ExpectProblem(errs.CodeProposalNotFound),
		).
		Then(scenario.ExpectStatus(http.StatusNotFound))
}

func expectListed(t *testing.T, want ...uuid.UUID) scenario.Step {
	t.Helper()
	wanted := make([]string, len(want))
	for i, w := range want {
		wanted[i] = w.String()
	}
	slices.Sort(wanted)
	return scenario.ExpectField("proposals", func(s *scenario.Scenario, raw json.RawMessage) {
		var items []struct {
			ID       string  `json:"id"`
			MyBallot *string `json:"my_ballot"`
			CanVote  bool    `json:"can_vote"`
		}
		decode(t, raw, &items)
		got := make([]string, len(items))
		for i, item := range items {
			got[i] = item.ID
			if item.MyBallot != nil || item.CanVote {
				s.Fatalf("proposal %s has a caller's ballot or vote right", item.ID)
			}
		}
		slices.Sort(got)
		if !slices.Equal(got, wanted) {
			s.Fatalf("proposals = %v, want %v", got, wanted)
		}
	})
}

func nextPage(t *testing.T, list string) scenario.Step {
	t.Helper()
	return func(s *scenario.Scenario) {
		var cursor string
		scenario.ExpectField("next_cursor", func(_ *scenario.Scenario, raw json.RawMessage) {
			decode(t, raw, &cursor)
		})(s)
		scenario.Get(list + "?limit=1&cursor=" + cursor)(s)
	}
}

func TestAdminCabalProposals_FilterByStatusAndPage(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernanceAndAdmin())
	c := seedHandledCabal(t, s)
	open, passed := seedLookupProposal(t, s, c, "open"), seedLookupProposal(t, s, c, "passed")
	list := "/v1/admin/cabals/" + c.ID.String() + "/proposals"
	s.Given(scenario.SeededAdmin("viewer", "viewer"), scenario.AsUser("viewer")).
		When(
			scenario.Get(list),
			scenario.ExpectStatus(http.StatusOK),
			expectListed(t, open.id, passed.id),
			scenario.ExpectJSON("next_cursor", nil),
			scenario.Get(list+"?status=open"),
			expectListed(t, open.id),
			scenario.Get(list+"?status=passed"),
			expectListed(t, passed.id),
			scenario.Get(list+"?limit=1"),
			scenario.ExpectStatus(http.StatusOK),
			nextPage(t, list),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("next_cursor", nil),
			scenario.Get(list+"?status=nonsense"),
			scenario.ExpectStatus(http.StatusBadRequest),
			scenario.Get("/v1/admin/cabals/"+uuid.NewString()+"/proposals"),
			scenario.ExpectProblem(errs.CodeCabalNotFound),
		).
		Then(scenario.ExpectStatus(http.StatusNotFound))
}
