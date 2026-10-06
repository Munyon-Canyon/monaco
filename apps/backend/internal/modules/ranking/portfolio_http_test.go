package ranking_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	latestSnapshot = `INSERT INTO cabal_value_snapshots (cabal_id, at, value_micros, nav_per_share_micros, total_shares)
VALUES ($1, $2, $3, 1, $4)`
	membersTotal = `SELECT coalesce(sum(value_micros), 0)::bigint FROM leaderboard_entries
WHERE board LIKE 'cabal_members:%' AND range = 'ALL' AND subject_id = $1`
)

func backdateFunds(t *testing.T, s server, user ids.UserID, at time.Time) {
	t.Helper()
	exec(t, s.pool, `UPDATE user_txns SET created_at = $1 WHERE user_id = $2 AND kind = 'fund'`, at, user.UUID())
}

func portfolioOf(t *testing.T, s server, user ids.UserID) api.MyPortfolio {
	t.Helper()
	rec := s.get(t, "/v1/me/portfolio", user)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET portfolio = %d %s", rec.Code, rec.Body)
	}
	var out api.MyPortfolio
	decode(t, rec, &out)
	return out
}

func TestPortfolio_TotalEqualsTheSumOfTheCallersRowsOnTheMembersBoards(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	big := testkit.NewCabal(t, s.pool, testkit.WithName("Big"))
	small := testkit.NewCabal(t, s.pool, testkit.WithName("Small"))
	user := big.Creator.ID
	now := s.clock.Now().UTC()
	ledger := testkit.NewLedger(t, s.pool)
	ledger.WithFundedMember(user, big.ID, money.MicrosFromUint64(100_000_000))
	ledger.WithFundedMember(user, small.ID, money.MicrosFromUint64(50_000_000))
	backdateFunds(t, s, user, now.Add(-3*time.Hour))
	exec(t, s.pool, latestSnapshot, big.ID.UUID(), now.Add(-time.Hour), 90_000_000, sharesOf(t, s, big.ID))
	exec(t, s.pool, latestSnapshot, big.ID.UUID(), now, 120_000_000, sharesOf(t, s, big.ID))
	exec(t, s.pool, latestSnapshot, small.ID.UUID(), now, 40_000_000, sharesOf(t, s, small.ID))
	seedBoard(t, s.pool, now, []boardRow{
		{board: domain.MembersBoard(big.ID.UUID()), rank: 1, subject: user.UUID(), value: 120_000_000},
		{board: domain.MembersBoard(small.ID.UUID()), rank: 1, subject: user.UUID(), value: 40_000_000},
	})
	got := portfolioOf(t, s, user)
	var boards int64
	if err := s.pool.QueryRow(t.Context(), membersTotal, user.UUID()).Scan(&boards); err != nil {
		t.Fatal(err)
	}
	if got.TotalValueMicros != boards || got.TotalValueMicros != 160_000_000 {
		t.Fatalf("total = %d, boards = %d, want both 160000000", got.TotalValueMicros, boards)
	}
	assertPortfolioTotals(t, got)
	assertPortfolioRows(t, got.Cabals, big.ID, sharesOf(t, s, big.ID))
}

func assertPortfolioTotals(t *testing.T, got api.MyPortfolio) {
	t.Helper()
	if got.PnlMicros != 10_000_000 || got.ReturnBps == nil || *got.ReturnBps != 666 || got.ComputedAt == nil ||
		got.PricesAsOf == nil || len(got.Cabals) != 2 {
		t.Fatalf("portfolio = %+v, want P&L 10M and a 666 bps return over two cabals", got)
	}
}

func assertPortfolioRows(t *testing.T, rows []api.PortfolioCabal, big ids.CabalID, shares int64) {
	t.Helper()
	assertFirstPortfolioRow(t, rows[0], big, shares)
	second := rows[1]
	if second.Cabal.Name != "Small" || second.PnlMicros != -10_000_000 || *second.ReturnBps != -2000 ||
		second.SliceBps != 2500 || rows[0].SliceBps+second.SliceBps != 10_000 {
		t.Fatalf("second row = %+v", second)
	}
}

func assertFirstPortfolioRow(t *testing.T, first api.PortfolioCabal, big ids.CabalID, shares int64) {
	t.Helper()
	if first.Cabal.Id != big.UUID() || first.Cabal.Name != "Big" || first.ValueMicros != 120_000_000 ||
		first.ShareUnits != shares || first.NetContributedMicros != 100_000_000 || first.PnlMicros != 20_000_000 ||
		first.ReturnBps == nil || *first.ReturnBps != 2000 || first.SliceBps != 7500 {
		t.Fatalf("first row = %+v", first)
	}
}

func TestPortfolio_IsEmptyBeforeTheFirstRunAndForAMemberWithNoStakeAndNeedsAToken(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	got := portfolioOf(t, s, user)
	if got.Cabals == nil || len(got.Cabals) != 0 || got.TotalValueMicros != 0 || got.ReturnBps != nil ||
		got.ComputedAt != nil || got.PricesAsOf != nil {
		t.Fatalf("before the first run = %+v, want an empty portfolio with null times", got)
	}
	seedRun(t, s)
	if got = portfolioOf(t, s, user); got.Cabals == nil || len(got.Cabals) != 0 || got.ComputedAt == nil {
		t.Fatalf("with no stake = %+v, want an empty portfolio with the run's times", got)
	}
	if rec := s.get(t, "/v1/me/portfolio", ids.UserID{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a token = %d, want 401", rec.Code)
	}
}

func TestPortfolio_IssuesOneQueryPerSource(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := testkit.NewCabal(t, s.pool)
	now := s.clock.Now().UTC()
	testkit.NewLedger(t, s.pool).WithFundedMember(cabal.Creator.ID, cabal.ID, money.MicrosFromUint64(10_000_000))
	backdateFunds(t, s, cabal.Creator.ID, now.Add(-time.Hour))
	exec(t, s.pool, latestSnapshot, cabal.ID.UUID(), now, 12_000_000, sharesOf(t, s, cabal.ID))
	seedRun(t, s)
	testkit.AssertQueries(t, "portfolio", func() { portfolioOf(t, s, cabal.Creator.ID) })
}

func TestPortfolio_LatestValuesFailures(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	boards := adapters.Boards{DB: s.pool}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := boards.LatestValuesOf(ctx, []ids.CabalID{cabal}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("canceled read: err = %v, want internal", err)
	}
	exec(t, s.pool, `INSERT INTO cabal_value_snapshots VALUES ($1, $2, -1, 1, 1)`, cabal.UUID(), s.clock.Now())
	if _, err := boards.LatestValuesOf(t.Context(), []ids.CabalID{cabal}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("negative value: err = %v, want internal", err)
	}
}
