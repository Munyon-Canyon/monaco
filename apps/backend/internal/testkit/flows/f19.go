package flows

import (
	"crypto/rand"
	"encoding/json"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
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
	seedRankedAsset(s)
	s.Given(rankedUsers()...).When(
		scenario.AwaitTick(valuationPoller),
	).Then(
		awaitRankedRows(),
		readBoard("/v1/leaderboards/cabals?range=ALL", rankedCabals, ""),
		readBoard("/v1/leaderboards/people?range=1W", rankedPeople, ""),
		awaitSnapshotEvent(),
		scenario.EventuallyGlobalHint("leaderboards_updated"),
		scenario.EventuallyLogged("ranking.run.completed"),
	)
}

func F19RunValuationPricesStale(s *scenario.Scenario) {
	seedRankedAsset(s)
	s.Given(rankedUsers()...).When(
		scenario.AwaitTick(valuationPoller),
		awaitRankedRows(),
		makePricesStale(rankedMint),
		queueRankedValuation(),
		scenario.AwaitTick(valuationPoller),
	).Then(
		awaitRankedFlag("stale_prices"),
		readBoard("/v1/leaderboards/cabals?range=ALL", rankedCabals, "stale_prices"),
		scenario.EventuallyLogged("ranking.cabal.excluded"),
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
			`UPDATE price_points SET ts = ts - interval '6 minutes' WHERE mint = $1`, string(mint)); err != nil {
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

const rankedMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

func seedRankedAsset(s *scenario.Scenario) {
	insertAsset(s, ids.Real{}.NewV7(), "AAPLx", rankedMint, time.Now().UTC())
	var kind domain.Kind
	err := s.DB().QueryRow(s.Context(), `SELECT kind FROM assets WHERE mint = $1`, rankedMint).Scan(&kind)
	if err != nil {
		s.Fatalf("flows: read the kind of AAPLx: %v", err)
	}
	session, err := domain.Session(kind, time.Now().UTC())
	if err != nil {
		s.Fatalf("flows: the session of AAPLx: %v", err)
	}
	if session.Continuous || session.State == domain.StateOpen {
		return
	}
	if _, err := s.DB().Exec(s.Context(),
		`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')
		ON CONFLICT (mint, ts) DO NOTHING`,
		rankedMint, session.LastClose.Add(-time.Minute), assetPriceMicros); err != nil {
		s.Fatalf("flows: seed the close price: %v", err)
	}
}

func rankedUsers() []scenario.Step {
	return []scenario.Step{
		scenario.SeededUser("alice", "active"),
		scenario.SeededUser("bob", "active"),
		scenario.SeededOnto("two-cabals-ranked", "alice", "bob"),
		scenario.AsUser("alice"),
	}
}

func rankedPeople(s *scenario.Scenario) []string {
	s.Helper()
	return []string{s.Recall("alice"), s.Recall("bob")}
}

func rankedCabals(s *scenario.Scenario) []string {
	s.Helper()
	rows, err := s.DB().Query(s.Context(),
		`SELECT id::text FROM cabals WHERE creator_id = ANY($1::uuid[]) ORDER BY name`, rankedPeople(s))
	if err != nil {
		s.Fatalf("flows: read the seeded cabals: %v", err)
	}
	cabals, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(cabals) != 2 {
		s.Fatalf("flows: the seeded cabals = %v, want 2: %v", cabals, err)
	}
	return cabals
}

func awaitRankedRows() scenario.Step {
	return scenario.Eventually("the two seeded cabals on the cabals board", func(s *scenario.Scenario) bool {
		var n int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM leaderboard_entries
			WHERE board = 'cabals' AND range = 'ALL' AND subject_id = ANY($1::uuid[])`,
			rankedCabals(s)).Scan(&n); err != nil {
			s.Fatalf("flows: count the cabals board rows: %v", err)
		}
		return n == 2
	})
}

func awaitRankedFlag(flag string) scenario.Step {
	return scenario.Eventually("the "+flag+" flag on the holding cabal", func(s *scenario.Scenario) bool {
		var n int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM leaderboard_entries
			WHERE board = 'cabals' AND range = 'ALL' AND subject_id = ANY($1::uuid[]) AND $2 = ANY(flags)`,
			rankedCabals(s), flag).Scan(&n); err != nil {
			s.Fatalf("flows: read the flags of the seeded cabals: %v", err)
		}
		return n == 1
	})
}

func queueRankedValuation() scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO ranking_triggers (cabal_id, reason, created_at)
			SELECT id, 'flow19', now() - interval '2 seconds' FROM cabals WHERE id = ANY($1::uuid[])`,
			rankedCabals(s)); err != nil {
			s.Fatalf("flows: queue a valuation of the seeded cabals: %v", err)
		}
	}
}

type boardRow struct {
	Subject struct {
		ID string `json:"id"`
	} `json:"subject"`
	Flags []string `json:"flags"`
}

func boardRows(s *scenario.Scenario, raw json.RawMessage) []boardRow {
	s.Helper()
	var rows []boardRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		s.Fatalf("flows: decode the board rows %s: %v", raw, err)
	}
	return rows
}

func readBoard(path string, want func(*scenario.Scenario) []string, flag string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		listed := boardPages(s, path)
		flagged := false
		for _, id := range want(s) {
			flags, ok := listed[id]
			if !ok && flag == "" {
				s.Fatalf("flows: %s does not list %s among %d rows", path, id, len(listed))
			}
			flagged = flagged || slices.Contains(flags, flag)
		}
		if flag != "" && !flagged {
			s.Fatalf("flows: no seeded subject on %s carries %s among %d rows", path, flag, len(listed))
		}
	}
}

func boardPages(s *scenario.Scenario, path string) map[string][]string {
	s.Helper()
	listed, cursor := map[string][]string{}, ""
	for {
		page := path + "&limit=50"
		if cursor != "" {
			page += "&cursor=" + url.QueryEscape(cursor)
		}
		scenario.Get(page)(s)
		scenario.ExpectStatus(200)(s)
		scenario.ExpectField("rows", func(s *scenario.Scenario, raw json.RawMessage) {
			for _, row := range boardRows(s, raw) {
				listed[row.Subject.ID] = row.Flags
			}
		})(s)
		cursor = ""
		scenario.ExpectField("next_cursor", func(s *scenario.Scenario, raw json.RawMessage) {
			s.Helper()
			if err := json.Unmarshal(raw, &cursor); err != nil {
				s.Fatalf("flows: decode next_cursor %s: %v", raw, err)
			}
		})(s)
		if cursor == "" {
			return listed
		}
	}
}
