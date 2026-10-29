package flows

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const weeklyLedgerPoller = "ranking.weekly_ledger"

func (defined) SeedsF37() map[string]Seeder {
	return map[string]Seeder{
		"F37BuildLedgersOK":                seedSignedIn,
		"F37BuildLedgersCrashBeforeCommit": seedSignedIn,
	}
}

func (defined) WorkerEnvF37() []string { return []string{"RANKING_LEDGER_POLL_INTERVAL=1s"} }

type ledgerFixture struct{ voter uuid.UUID }

func ledgerDelivery(now time.Time) time.Time {
	at := now.UTC()
	day := time.Date(at.Year(), at.Month(), at.Day(), 17, 0, 0, 0, time.UTC)
	day = day.AddDate(0, 0, -int(day.Weekday()))
	if day.After(at) {
		day = day.AddDate(0, 0, -7)
	}
	return day
}

func seedLedgerVoter(s *scenario.Scenario) ledgerFixture {
	s.Helper()
	delivery := ledgerDelivery(time.Now())
	opened, resolved := delivery.Add(-4*24*time.Hour), delivery.Add(-2*24*time.Hour)
	cabal := testkit.NewCabal(seedT{s}, s.DB(), testkit.WithMembers(2))
	voter := cabal.Members[len(cabal.Members)-1].ID.UUID()
	proposal := ids.Real{}.NewV7()
	_, err := s.DB().Exec(s.Context(), `INSERT INTO calls (proposal_id, cabal_id, kind, symbol, opened_at, decided_at,
		outcome, state, move_bps, resolved_at, updated_at)
		VALUES ($1, $2, 'buy', 'TSLAx', $3, $3, 'passed', 'resolved', 500, $4, $4)`,
		proposal, cabal.ID.UUID(), opened, resolved)
	if err != nil {
		s.Fatalf("flows: seed the ledger call: %v", err)
	}
	_, err = s.DB().Exec(s.Context(), `INSERT INTO call_voters
		(proposal_id, user_id, cabal_id, choice, choice_at, is_right)
		VALUES ($1, $2, $3, 'yes', $4, true)`, proposal, voter, cabal.ID.UUID(), opened)
	if err != nil {
		s.Fatalf("flows: seed the ledger voter: %v", err)
	}
	return ledgerFixture{voter: voter}
}

func (fx ledgerFixture) awaitLedger() scenario.Step {
	const what = "the voter to have a weekly ledger and one ledger_ready event"
	return scenario.Eventually(what, func(s *scenario.Scenario) bool {
		var ledgers, ready int
		if err := s.DB().QueryRow(s.Context(), `SELECT
			(SELECT count(*) FROM weekly_ledgers WHERE user_id = $1),
			(SELECT count(*) FROM events WHERE type = $2 AND aggregate_id = $1)`,
			fx.voter, string(events.TypeRankingLedgerReady)).Scan(&ledgers, &ready); err != nil {
			s.Fatalf("flows: count the voter's ledger and events: %v", err)
		}
		return ledgers == 1 && ready == 1
	})
}

func F37BuildLedgersOK(s *scenario.Scenario) {
	fx := seedLedgerVoter(s)
	s.Given().When(
		scenario.AwaitTickPastTimeouts(weeklyLedgerPoller),
	).Then(fx.awaitLedger())
}

func F37BuildLedgersCrashBeforeCommit(s *scenario.Scenario) {
	fx := seedLedgerVoter(s)
	s.Given().When(
		scenario.TickCrashingAt(weeklyLedgerPoller, faultpoint.BeforeCommit),
		scenario.AwaitTickPastTimeouts(weeklyLedgerPoller),
	).Then(fx.awaitLedger())
}
