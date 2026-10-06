package flows

import (
	"crypto/rand"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	valuationPoller    = "ranking.valuation"
	valuationPotMicros = 106_000_000
	assetPriceMicros   = 2_000_000
	assetDecimals      = 9
)

type valuationSeed struct {
	cabal testkit.SeededCabal
}

type rowCounts struct {
	cabals, people, members int
	flag                    string
}

func seedAsset(s *scenario.Scenario) chain.SolanaAddress {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	mint, id, now := chain.AddressOf(key), ids.Real{}.NewV7(), time.Now().UTC()
	insertAsset(s, id, "F19"+id.String()[24:], string(mint), now)
	return mint
}

func insertAsset(s *scenario.Scenario, id uuid.UUID, symbol, mint string, now time.Time) {
	_, err := s.DB().Exec(s.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at, chain_checked_at)
		VALUES ($1, $2, $3, $4, 'tessera', 'pre_ipo', $2, true, $2, $5, $5, $5)
		ON CONFLICT (mint) DO NOTHING`,
		id, symbol, mint, assetDecimals, now)
	if err != nil {
		s.Fatalf("flows: seed the asset %s: %v", symbol, err)
	}
	for _, age := range []time.Duration{4 * time.Minute, 2 * time.Minute, 0} {
		if _, err := s.DB().Exec(s.Context(),
			`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')
			ON CONFLICT (mint, ts) DO NOTHING`,
			mint, now.Add(-age), assetPriceMicros); err != nil {
			s.Fatalf("flows: seed a price: %v", err)
		}
	}
}

func seedCabal(s *scenario.Scenario, mint chain.SolanaAddress) valuationSeed {
	cabal := testkit.NewCabal(seedT{s}, s.DB())
	testkit.NewLedger(seedT{s}, s.DB()).
		WithFundedMember(cabal.Creator.ID, cabal.ID, money.MicrosFromUint64(100_000_000)).
		WithHolding(cabal.ID, mint, money.NewBaseUnits(3*1_000_000_000, assetDecimals))
	return valuationSeed{cabal: cabal}
}

func F19RunValuationOK(s *scenario.Scenario) {
	mint := seedAsset(s)
	seed := seedCabal(s, mint)
	s.Given(scenario.AsSeededUser("viewer", seed.cabal.Creator.ID)).When(
		queueValuation(seed),
		scenario.AwaitTick(valuationPoller),
	).Then(
		seed.awaitRows(rowCounts{cabals: 1, people: 1, members: 1}),
		seed.awaitSnapshot(valuationPotMicros),
		awaitSnapshotEvent(),
		scenario.EventuallyGlobalHint("leaderboards_updated"),
	)
}

func F19RunValuationPricesStale(s *scenario.Scenario) {
	mint := seedAsset(s)
	seed := seedCabal(s, mint)
	s.Given().When(
		queueValuation(seed),
		scenario.AwaitTick(valuationPoller),
		seed.awaitRows(rowCounts{cabals: 1, people: 1, members: 1}),
		makePricesStale(mint),
		queueValuation(seed),
		scenario.AwaitTick(valuationPoller),
	).Then(
		seed.awaitRows(rowCounts{cabals: 1, members: 1, flag: "stale_prices"}),
		seed.awaitCabalRowValue(valuationPotMicros),
	)
}

func F19RunValuationConservationBroken(s *scenario.Scenario) {
	mint := seedAsset(s)
	seed, healthy := seedCabal(s, mint), seedCabal(s, mint)
	s.Given().When(
		queueValuation(seed),
		scenario.AwaitTick(valuationPoller),
		seed.awaitRows(rowCounts{cabals: 1, people: 1, members: 1}),
		reserveMoreThanThePot(seed),
		queueValuation(seed),
		scenario.AwaitTick(valuationPoller),
	).Then(
		seed.awaitRows(rowCounts{}),
		healthy.awaitRows(rowCounts{cabals: 1, people: 1, members: 1}),
	)
}

func F19RunValuationCabalPaused(s *scenario.Scenario) {
	mint := seedAsset(s)
	seed, healthy := seedCabal(s, mint), seedCabal(s, mint)
	s.Given().When(
		queueValuation(seed),
		scenario.AwaitTick(valuationPoller),
		seed.awaitRows(rowCounts{cabals: 1, people: 1, members: 1}),
		pauseCabal(seed),
		queueValuation(seed),
		scenario.AwaitTick(valuationPoller),
	).Then(
		seed.awaitRows(rowCounts{}),
		healthy.awaitRows(rowCounts{cabals: 1, people: 1, members: 1}),
	)
}

func F19RunValuationCrashBeforeCommit(s *scenario.Scenario) {
	mint := seedAsset(s)
	seed := seedCabal(s, mint)
	s.Given().When(
		queueValuation(seed),
		scenario.TickCrashingAt(valuationPoller, faultpoint.BeforeCommit),
		scenario.AwaitTick(valuationPoller),
	).Then(
		seed.awaitRows(rowCounts{cabals: 1, people: 1, members: 1}),
		seed.awaitSnapshot(valuationPotMicros),
		awaitSnapshotEvent(),
	)
}

func (v valuationSeed) members() string { return "cabal_members:" + v.cabal.ID.String() }

func (v valuationSeed) ownRows(s *scenario.Scenario) rowCounts {
	s.Helper()
	var got rowCounts
	var flagged int
	err := s.DB().QueryRow(s.Context(), `SELECT
		count(*) FILTER (WHERE board = 'cabals' AND subject_id = $1),
		count(*) FILTER (WHERE board = 'people' AND subject_id = $2),
		count(*) FILTER (WHERE board = $3),
		count(*) FILTER (WHERE flags <> '{}' AND (board = 'cabals' AND subject_id = $1 OR board = $3)),
		coalesce(max(flags[1]) FILTER (WHERE board = 'cabals' AND subject_id = $1), '')
		FROM leaderboard_entries WHERE range = 'ALL'`,
		v.cabal.ID.UUID(), v.cabal.Creator.ID.UUID(), v.members()).
		Scan(&got.cabals, &got.people, &got.members, &flagged, &got.flag)
	if err != nil {
		s.Fatalf("flows: read the cabal's board rows: %v", err)
	}
	if got.cabals+got.members != flagged && flagged != 0 {
		got.flag = "mixed"
	}
	return got
}

func (v valuationSeed) awaitRows(want rowCounts) scenario.Step {
	return scenario.Eventually("the board rows of cabal "+v.cabal.ID.String(), func(s *scenario.Scenario) bool {
		return v.ownRows(s) == want
	})
}

func (v valuationSeed) awaitSnapshot(micros int64) scenario.Step {
	return scenario.Eventually("a snapshot of the cabal at its pot value", func(s *scenario.Scenario) bool {
		var n int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM cabal_value_snapshots
			WHERE cabal_id = $1 AND value_micros = $2`, v.cabal.ID.UUID(), micros).Scan(&n); err != nil {
			s.Fatalf("flows: read the cabal snapshots: %v", err)
		}
		return n > 0
	})
}

func (v valuationSeed) awaitCabalRowValue(micros int64) scenario.Step {
	return scenario.Eventually("the stale cabal's row at its previous value", func(s *scenario.Scenario) bool {
		var n int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM leaderboard_entries
			WHERE range = 'ALL' AND board = 'cabals' AND subject_id = $1 AND value_micros = $2`,
			v.cabal.ID.UUID(), micros).Scan(&n); err != nil {
			s.Fatalf("flows: read the cabal row: %v", err)
		}
		return n > 0
	})
}

func awaitSnapshotEvent() scenario.Step {
	return scenario.Eventually("a ranking.snapshot_written event", func(s *scenario.Scenario) bool {
		var n int
		if err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM events WHERE type = 'ranking.snapshot_written'`).Scan(&n); err != nil {
			s.Fatalf("flows: count snapshot events: %v", err)
		}
		return n > 0
	})
}

func makePricesStale(mint chain.SolanaAddress) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		if _, err := s.DB().Exec(s.Context(),
			`UPDATE price_points SET ts = ts - interval '10 minutes' WHERE mint = $1`, string(mint)); err != nil {
			s.Fatalf("flows: age the prices: %v", err)
		}
	}
}

func queueValuation(v valuationSeed) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO ranking_triggers (cabal_id, reason, created_at)
			VALUES ($1, 'flow19', now() - interval '2 seconds')`, v.cabal.ID.UUID()); err != nil {
			s.Fatalf("flows: queue a valuation: %v", err)
		}
	}
}

func reserveMoreThanThePot(v valuationSeed) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		_, err := s.DB().
			Exec(s.Context(), `INSERT INTO cash_out_jobs (id, cabal_id, user_id, share_units, payout_micros,
			slice_micros, status, created_at, updated_at)
			VALUES ($1, $2, $3, 1, 500000000, 500000000, 'selling', now(), now())`,
				ids.Real{}.NewV7(), v.cabal.ID.UUID(), v.cabal.Creator.ID.UUID())
		if err != nil {
			s.Fatalf("flows: reserve cash for a cash-out: %v", err)
		}
	}
}

func pauseCabal(v valuationSeed) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at)
			VALUES ($1, $2, 'ops', now())`, ids.Real{}.NewV7(), v.cabal.ID.UUID()); err != nil {
			s.Fatalf("flows: pause the cabal: %v", err)
		}
	}
}
