package flows

import (
	"crypto/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const callsResolverPoller = "ranking.calls_resolver"

func (defined) WorkerEnvF19a() []string { return []string{"RANKING_CALLS_RESOLVER_INTERVAL=1s"} }

type seededCall struct {
	proposal, cabal, yes, no uuid.UUID
	spec                     callSpec
}

type callSpec struct {
	kind, outcome, side string
	fill, market, end   int64
	endAge              time.Duration
	preIPO              bool
}

type callWant struct {
	state, source                 string
	moveBps                       int
	yesRight, noRight, cabalRight bool
}

func F19aResolveCallsOK(s *scenario.Scenario) {
	now := time.Now().UTC()
	buyUp := seedCall(
		s,
		now,
		callSpec{kind: "buy", outcome: "passed", side: "yes", fill: 716_520_000, end: 724_400_000},
	)
	sellDown := seedCall(
		s,
		now,
		callSpec{kind: "sell", outcome: "failed", side: "no", market: 200_000_000, end: 180_000_000},
	)
	expired := seedCall(
		s,
		now,
		callSpec{kind: "buy", outcome: "expired", side: "no", market: 100_000_000, end: 110_000_000},
	)
	unpriced := seedCall(s, now, callSpec{kind: "buy", outcome: "passed", side: "yes", end: 100_000_000})
	preIPO := seedCall(s, now, callSpec{
		kind: "buy", outcome: "passed", side: "yes", market: 50_000_000, end: 50_000_000,
		endAge: 30 * time.Hour, preIPO: true,
	})
	s.Given().When(
		scenario.AwaitTickPastTimeouts(callsResolverPoller),
	).Then(
		buyUp.await(callWant{state: "resolved", source: "fill", moveBps: 109, yesRight: true, cabalRight: true}),
		sellDown.await(callWant{state: "resolved", source: "market", moveBps: -1000, yesRight: true, noRight: false}),
		expired.await(callWant{state: "resolved", source: "market", moveBps: 1000, yesRight: true}),
		unpriced.await(callWant{state: "void"}),
		preIPO.await(callWant{state: "resolved", source: "market", noRight: true}),
		awaitResolvedEvents(4),
	)
}

func F19aResolveCallsCrashBeforeCommit(s *scenario.Scenario) {
	call := seedCall(s, time.Now().UTC(), callSpec{
		kind: "buy", outcome: "passed", side: "yes", fill: 716_520_000, end: 724_400_000,
	})
	s.Given().When(
		scenario.TickCrashingAt(callsResolverPoller, faultpoint.BeforeCommit),
		scenario.AwaitTickPastTimeouts(callsResolverPoller),
	).Then(
		call.await(callWant{state: "resolved", source: "fill", moveBps: 109, yesRight: true, cabalRight: true}),
		awaitResolvedEvents(1),
	)
}

func seedCall(s *scenario.Scenario, now time.Time, spec callSpec) seededCall {
	s.Helper()
	g := ids.Real{}
	call := seededCall{proposal: g.NewV7(), cabal: g.NewV7(), yes: g.NewV7(), no: g.NewV7(), spec: spec}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	mint, asset, kind := string(chain.AddressOf(key)), g.NewV7(), "equity"
	if spec.preIPO {
		kind = "pre_ipo"
	}
	decided := now.Add(-25 * time.Hour)
	resolveAt := decided.Add(24 * time.Hour)
	symbol := "F19a" + asset.String()[24:]
	call.exec(s, `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable,
		company_key, first_seen_at, updated_at, chain_checked_at)
		VALUES ($1, $2, $3, 8, 'xstocks', $4, $2, true, $2, $5, $5, $5)`, asset, symbol, mint, kind, now)
	call.exec(s, `INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
		mint, resolveAt.Add(-max(spec.endAge, time.Minute)), spec.end)
	call.exec(s, `INSERT INTO calls (proposal_id, cabal_id, kind, symbol, mint, asset_id, opened_at, decided_at,
		outcome, market_ref_micros, fill_price_micros, resolve_at, state, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $8, $9, $10, $11, 'decided', $7)`,
		call.proposal, call.cabal, spec.kind, symbol, mint, asset, decided, spec.outcome,
		nullable(spec.market), nullable(spec.fill), resolveAt)
	call.exec(s, `INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at)
		VALUES ($1, $2, $4, 'yes', $5), ($1, $3, $4, 'no', $5)`, call.proposal, call.yes, call.no, call.cabal, decided)
	call.exec(s, `INSERT INTO cabal_calls (proposal_id, cabal_id, side) VALUES ($1, $2, $3)`,
		call.proposal, call.cabal, spec.side)
	return call
}

func nullable(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func (c seededCall) exec(s *scenario.Scenario, query string, args ...any) {
	s.Helper()
	if _, err := s.DB().Exec(s.Context(), query, args...); err != nil {
		s.Fatalf("flows: seed the call %s: %v", c.proposal, err)
	}
}

func (c seededCall) await(want callWant) scenario.Step {
	return scenario.Eventually(
		"the call "+c.proposal.String()+" to reach "+want.state,
		func(s *scenario.Scenario) bool {
			var got callWant
			var source pgtype.Text
			var bps pgtype.Int4
			var yes, no, cabal pgtype.Bool
			err := s.DB().QueryRow(s.Context(), `SELECT c.state, c.ref_source, c.move_bps,
			(SELECT is_right FROM call_voters WHERE proposal_id = c.proposal_id AND user_id = $2),
			(SELECT is_right FROM call_voters WHERE proposal_id = c.proposal_id AND user_id = $3),
			(SELECT is_right FROM cabal_calls WHERE proposal_id = c.proposal_id)
			FROM calls AS c WHERE c.proposal_id = $1`, c.proposal, c.yes, c.no).
				Scan(&got.state, &source, &bps, &yes, &no, &cabal)
			if err != nil {
				s.Fatalf("flows: read the call %s: %v", c.proposal, err)
			}
			got.source, got.moveBps = source.String, int(bps.Int32)
			got.yesRight, got.noRight, got.cabalRight = yes.Bool, no.Bool, cabal.Bool
			if got.state != want.state {
				return false
			}
			if got != want {
				s.Fatalf("flows: the call %s resolved to %+v, want %+v", c.proposal, got, want)
			}
			return true
		},
	)
}

func awaitResolvedEvents(n int) scenario.Step {
	return scenario.Eventually("call.resolved events", func(s *scenario.Scenario) bool {
		var got int
		if err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM events WHERE type = $1`, string(events.TypeCallResolved)).Scan(&got); err != nil {
			s.Fatalf("flows: count call.resolved events: %v", err)
		}
		if got > n {
			s.Fatalf("flows: %d call.resolved events, want %d", got, n)
		}
		return got == n
	})
}
