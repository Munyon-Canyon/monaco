package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const stockUnitsPerToken = 100_000_000

type openMarket struct {
	market.Catalog
	market.Prices
	*marketfake.CalendarFake
}

type noPauses struct{}

func (noPauses) PausedCabals(context.Context) (funding.PausedSet, error) {
	return funding.PausedSet{}, nil
}

type pausedEverywhere struct{}

func (pausedEverywhere) PausedCabals(context.Context) (funding.PausedSet, error) {
	return funding.PausedSet{Global: true}, nil
}

type valuationRig struct {
	pool     *pgxpool.Pool
	now      time.Time
	cabal    testkit.SeededCabal
	ledger   *testkit.Ledger
	treasury treasury.Queries
	ports    app.Ports
	writer   app.SnapshotWriter
}

func newValuationRig(t *testing.T) *valuationRig {
	t.Helper()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	d := module.Deps{Pool: pool, Clock: testkit.NewClock(now), Config: testkit.Config()}
	stock := marketfake.AAPLx()
	_, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at, chain_checked_at)
		VALUES ($1, $2, $3, 8, 'xstocks', 'equity', $4, true, $5, $6, $6, $6)`,
		stock.ID.UUID(), stock.Symbol, stock.Mint.String(), stock.DisplayName, stock.CompanyKey, now)
	if err != nil {
		t.Fatal(err)
	}
	calendar := marketfake.NewCalendar()
	calendar.SetState(stock.ID, market.SessionInfo{State: "open"})
	marketModule := market.New(d)
	rig := &valuationRig{
		pool: pool, now: now, cabal: testkit.NewCabal(t, pool), ledger: testkit.NewLedger(t, pool),
		treasury: treasury.New(d).Queries(), writer: newWriter(pool, now),
	}
	rig.ports = app.Ports{
		Market:   openMarket{Catalog: marketModule.Catalog(), Prices: marketModule.Prices(), CalendarFake: calendar},
		Treasury: rig.treasury, Funding: noPauses{}, Cabals: cabal.New(d).Queries(), Users: identity.New(d).Queries(),
		Previous: sqlc.New(pool),
	}
	rig.price(t, 2_000_000)
	rig.ledger.
		WithFundedMember(rig.cabal.Creator.ID, rig.cabal.ID, money.MicrosFromUint64(100_000_000)).
		WithHolding(rig.cabal.ID, stock.Mint.Address(), money.NewBaseUnits(3*stockUnitsPerToken, 8))
	return rig
}

func (r *valuationRig) price(t *testing.T, micros int64) {
	t.Helper()
	for _, age := range []time.Duration{4 * time.Minute, 2 * time.Minute, 0} {
		if _, err := r.pool.Exec(t.Context(),
			`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
			marketfake.AAPLx().Mint.String(), r.now.Add(-age), micros); err != nil {
			t.Fatal(err)
		}
	}
}

func (r *valuationRig) run(t *testing.T, at time.Time) app.Valuation {
	t.Helper()
	got, err := app.NewRunValuation(r.ports, testkit.USDCMint).Run(t.Context(), at)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (r *valuationRig) write(t *testing.T, valuation app.Valuation) {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "system:ranking.valuation")
	if _, err := r.writer.Write(ctx, valuation, valuation.AsOf, valuation.AsOf); err != nil {
		t.Fatal(err)
	}
}

func (r *valuationRig) rows(t *testing.T, board string) int {
	t.Helper()
	return count(t, r.pool, `SELECT count(*) FROM leaderboard_entries WHERE range = 'ALL' AND board = $1`, board)
}

func (r *valuationRig) wantRows(t *testing.T, want map[string]int) {
	t.Helper()
	for board, n := range want {
		if got := r.rows(t, board); got != n {
			t.Fatalf("board %s has %d ALL rows, want %d", board, got, n)
		}
	}
}

func TestRunValuation_aFullRunWritesTheBoardsAndASecondRunReplacesThem(t *testing.T) {
	t.Parallel()
	rig := newValuationRig(t)
	first := rig.run(t, rig.now.Add(time.Minute))
	rig.write(t, first)
	members := app.MembersBoard(rig.cabal.ID.UUID())
	rig.wantRows(t, map[string]int{"cabals": 1, "people": 1, members: 1})
	joiner := testkit.SeedUser(t, rig.pool, testkit.UserOpts{})
	if _, err := rig.pool.Exec(t.Context(), `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		VALUES ($1, $2, 'member', true, $3)`, rig.cabal.ID.UUID(), joiner.ID.UUID(), rig.now); err != nil {
		t.Fatal(err)
	}
	rig.ledger.WithFundedMember(joiner.ID, rig.cabal.ID, money.MicrosFromUint64(53_000_000))
	second := rig.run(t, rig.now.Add(2*time.Minute))
	rig.write(t, second)
	rig.wantRows(t, map[string]int{"cabals": 1, "people": 2, members: 2})
	if n := count(t, rig.pool, `SELECT count(*) FROM leaderboard_entries
		WHERE range = 'ALL' AND board = 'cabals' AND value_micros = 159000000`); n != 1 || len(second.Entries) != 5 {
		t.Fatalf(
			"second run: %d rows, %d cabal rows at 159 USDC, want 5 rows replacing the old ones",
			len(second.Entries),
			n,
		)
	}
	if n := count(t, rig.pool, `SELECT count(*) FROM leaderboard_runs`); n != 2 {
		t.Fatalf("runs = %d, want 2", n)
	}
}

func TestRunValuation_aStaleCabalKeepsItsPreviousRowsWithTheFlag(t *testing.T) {
	t.Parallel()
	rig := newValuationRig(t)
	rig.write(t, rig.run(t, rig.now.Add(time.Minute)))
	stale := rig.run(t, rig.now.Add(30*time.Minute))
	if len(stale.Cabals) != 0 || len(stale.Flagged) != 1 || stale.Excluded != 1 {
		t.Fatalf("stale run = %+v, want the one cabal flagged", stale)
	}
	rig.write(t, stale)
	members := app.MembersBoard(rig.cabal.ID.UUID())
	for _, board := range []string{"cabals", members} {
		if rig.rows(t, board) != 1 || count(t, rig.pool, `SELECT count(*) FROM leaderboard_entries
			WHERE range = 'ALL' AND board = $1 AND flags = ARRAY['stale_prices']`, board) != 1 {
			t.Fatalf("board %s lost its row or the stale flag", board)
		}
	}
	if count(
		t,
		rig.pool,
		`SELECT count(*) FROM leaderboard_entries WHERE board = 'cabals' AND value_micros = 106000000`,
	) != 1 {
		t.Fatal("the stale cabal's row no longer carries its previous value")
	}
}

func TestRunValuation_aGlobalPauseKeepsTheBoardsAndWritesNothing(t *testing.T) {
	t.Parallel()
	rig := newValuationRig(t)
	rig.write(t, rig.run(t, rig.now.Add(time.Minute)))
	rig.ports.Funding = pausedEverywhere{}
	held := rig.run(t, rig.now.Add(2*time.Minute))
	ctx := observability.WithActor(t.Context(), "system:ranking.valuation")
	if _, err := rig.writer.Write(ctx, held, held.AsOf, held.AsOf); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Write() of a run that excluded every cabal = %v, want it refused", err)
	}
	rig.wantRows(t, map[string]int{"cabals": 1, "people": 1, app.MembersBoard(rig.cabal.ID.UUID()): 1})
	if runs := count(t, rig.pool, `SELECT count(*) FROM leaderboard_runs`); runs != 1 ||
		count(t, rig.pool, `SELECT count(*) FROM events WHERE type = 'ranking.snapshot_written'`) != 1 {
		t.Fatalf("a refused write left %d runs, want only the first run and its event", runs)
	}
}

func TestRunValuation_aCabalHoldingAnUncataloguedMintKeepsItsRowsFlaggedWhileTheRestAreWritten(t *testing.T) {
	t.Parallel()
	rig := newValuationRig(t)
	other := testkit.NewCabal(t, rig.pool)
	rig.ledger.WithFundedMember(other.Creator.ID, other.ID, money.MicrosFromUint64(40_000_000))
	rig.write(t, rig.run(t, rig.now.Add(time.Minute)))
	rig.ledger.WithHolding(other.ID, "11111111111111111111111111111111", money.NewBaseUnits(1, 8))
	second := rig.run(t, rig.now.Add(2*time.Minute))
	rig.write(t, second)
	if len(second.Cabals) != 1 || len(second.Flagged) != 1 || second.Flagged[0].CabalID != other.ID {
		t.Fatalf("second run = %d valued, %+v flagged, want the original cabal valued and the other flagged",
			len(second.Cabals), second.Flagged)
	}
	flagged := count(t, rig.pool, `SELECT count(*) FROM leaderboard_entries WHERE range = 'ALL'
		AND flags = ARRAY['unpriced_assets'] AND (subject_id = $1 OR board = $2)`,
		other.ID.UUID(), app.MembersBoard(other.ID.UUID()))
	healthy := count(t, rig.pool, `SELECT count(*) FROM leaderboard_entries WHERE range = 'ALL'
		AND flags = '{}' AND (subject_id = $1 OR board = $2)`,
		rig.cabal.ID.UUID(), app.MembersBoard(rig.cabal.ID.UUID()))
	if flagged != 2 || healthy != 2 {
		t.Fatalf("flagged rows = %d, healthy rows = %d, want 2 and 2", flagged, healthy)
	}
}
