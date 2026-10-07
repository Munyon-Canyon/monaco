package flows

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	aaplxMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	yes       = `{"choice":"yes"}`
	no        = `{"choice":"no"}`
)

type openProposal struct {
	id         ids.ProposalID
	cabalID    ids.CabalID
	proposerID ids.UserID
	path       string
	votes      string
	voters     []ids.UserID
}

type seedT struct{ *scenario.Scenario }

func (seedT) Helper() {}

func seedOpenProposal(s *scenario.Scenario, members int) openProposal {
	c := testkit.NewCabal(seedT{s}, s.DB(), testkit.WithMembers(members))
	id, now := ids.Real{}.NewV7(), time.Now().UTC()
	p := openProposal{
		id: ids.ProposalIDFrom(id), cabalID: c.ID, proposerID: c.Creator.ID, path: "/v1/proposals/" + id.String(),
	}
	p.votes = p.path + "/votes"
	params := sqlc.InsertProposalParams{
		ID: id, CabalID: c.ID.UUID(), ProposerID: c.Creator.ID.UUID(), Kind: "buy", Symbol: "AAPLx",
		Mint: aaplxMint, UsdcMicros: pgtype.Int8{Int64: 5_000_000, Valid: true},
		QuoteOutAmount: 21_000_000, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, Threshold: "majority",
	}
	for _, m := range c.Members {
		p.voters = append(p.voters, m.ID)
		params.VoterIds = append(params.VoterIds, m.ID.UUID())
	}
	if _, err := sqlc.New(s.DB()).InsertProposal(s.Context(), params); err != nil {
		s.Fatalf("flows: insert proposal: %v", err)
	}
	seedFeedProposal(s, p)
	return p
}

func seedFeedCabal(s *scenario.Scenario, cabal ids.CabalID) {
	s.Helper()
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO feed_cabals (cabal_id, name, updated_at)
		SELECT id, name, now() FROM cabals WHERE id = $1 ON CONFLICT (cabal_id) DO NOTHING`, cabal.UUID()); err != nil {
		s.Fatalf("flows: seed the feed cabal: %v", err)
	}
}

func seedFeedProposal(s *scenario.Scenario, p openProposal) {
	s.Helper()
	seedFeedCabal(s, p.cabalID)
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO feed_objects
		(id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, symbol, title, payload, status, created_at, updated_at)
		SELECT $1, 'proposal', 'proposals', $2, $3, name, $4, 'AAPLx', 'seeded', '{"status":"open"}', 'open', now(), now()
		FROM cabals WHERE id = $3`,
		ids.Real{}.NewV7(), p.id.UUID(), p.cabalID.UUID(), p.proposerID.UUID()); err != nil {
		s.Fatalf("flows: seed the proposal feed item: %v", err)
	}
}

func tally(yes, no, voters, needed int) map[string]int {
	return map[string]int{"yes": yes, "no": no, "voters": voters, "needed": needed}
}

func passedProposal(p openProposal) events.ProposalPassed {
	return events.ProposalPassed{
		V: 1, ProposalID: p.id.UUID(), CabalID: p.cabalID.UUID(), ProposerID: p.proposerID.UUID(), Kind: "buy",
		Symbol: "AAPLx", Mint: chain.SolanaAddress(aaplxMint), USDCMicros: money.MicrosFromUint64(5_000_000),
		QuoteOutAmount: 21_000_000,
	}
}

func F10CastVoteOK(s *scenario.Scenario) {
	p := seedOpenProposal(s, 3)
	s.Given(scenario.AsUser("mallory"), scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Post(p.votes, no),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "open"),
			scenario.ExpectJSON("my_ballot", "no"),
			scenario.ExpectJSON("tally", tally(0, 1, 3, 2)),
			scenario.Replay(),
			scenario.Post(p.votes, yes),
			scenario.ExpectJSON("tally", tally(1, 0, 3, 2)),
			scenario.AsSeededUser("bob", p.voters[1]),
			scenario.Post(p.votes, yes),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "passed"),
			scenario.ExpectJSON("tally", tally(2, 0, 3, 2)),
		).
		Then(
			scenario.ExpectEvents(events.TypeProposalPassed, 1),
			scenario.ExpectEventPayload(events.TypeProposalPassed, passedProposal(p)),
			scenario.EventuallyPublished(events.TypeProposalPassed, 1),
			scenario.EventuallyCaptured(events.TypeProposalPassed, "proposal_passed", p.id),
			scenario.EventuallyCabalHint(p.cabalID, "proposal_updated"),
			scenario.NoHintFor("mallory", "proposal_updated", 100*time.Millisecond),
		)
}

func F10CastVoteUnauthorized(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.Anonymous()).
		When(scenario.Post(p.votes, yes)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized), scenario.ExpectEvents(events.TypeProposalPassed, 0))
}

func F10CastVoteProposalNotFound(s *scenario.Scenario) {
	s.Given(scenario.AsUser("alice")).
		When(scenario.Post("/v1/proposals/"+ids.Real{}.NewV7().String()+"/votes", yes)).
		Then(scenario.ExpectProblem(errs.CodeProposalNotFound))
}

func F10CastVoteNotAVoter(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.AsUser("mallory")).
		When(scenario.Post(p.votes, yes)).
		Then(scenario.ExpectProblem(errs.CodeNotAVoter), scenario.ExpectEvents(events.TypeProposalPassed, 0))
}

func F10CastVoteProposalClosed(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Post(p.votes, no),
			scenario.ExpectJSON("status", "failed"),
			scenario.Post(p.votes, yes),
		).
		Then(
			scenario.ExpectProblem(errs.CodeProposalClosed),
			scenario.ExpectEvents(events.TypeProposalFailed, 1),
			scenario.ExpectEvents(events.TypeProposalPassed, 0),
		)
}

func F10CastVoteCrashAfterPublish(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.AsSeededUser("alice", p.voters[0]), scenario.HoldRelay()).
		When(
			scenario.Post(p.votes, yes),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "passed"),
			scenario.PublishCrashingAt(faultpoint.AfterPublish),
		).
		Then(
			scenario.ExpectEvents(events.TypeProposalPassed, 1),
			scenario.EventuallyPublished(events.TypeProposalPassed, 1),
		)
}
