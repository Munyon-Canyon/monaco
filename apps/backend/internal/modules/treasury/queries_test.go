package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestPotValue_USDCOnly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	q := newQueries(f)
	got, err := q.PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(100_000_000) {
		t.Fatalf("PotValue() = %v, %v", got, err)
	}
}

func TestPotValue_SubtractsLiveCashOutReservations(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO cash_out_jobs
  (id, cabal_id, user_id, share_units, payout_micros, status, created_at, updated_at)
VALUES ($1, $2, $3, 10, 30000000, $4, $5, $5)`
	for _, status := range []string{"started", "selling", "paying", "completed", "partial", "failed"} {
		if _, err := f.pool.Exec(
			t.Context(), insert, f.ids.NewV7(), cabal.UUID(), f.user(t).UUID(), status, f.clock.Now(),
		); err != nil {
			t.Fatal(err)
		}
	}
	got, err := newQueries(f).PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(10_000_000) {
		t.Fatalf("PotValue() = %v, %v", got, err)
	}
}

func TestPotValue_WithHoldings(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	swap, err := f.swap(cabal, 40_000_000, 200_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), f.clock.Now())
	q := adapterQueries(f, prices)
	got, err := q.PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(120_000_000) {
		t.Fatalf("PotValue() = %v, %v", got, err)
	}
	positions, err := q.Positions(t.Context(), cabal)
	if err != nil || len(positions) != 2 || string(positions[0].Mint) != usdcMint ||
		positions[1].Units.Decimals() != 8 {
		t.Fatalf("Positions() = %#v, %v", positions, err)
	}
}

func TestPotValue_StalePriceUnavailable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	prices := &marketfake.PricesFake{}
	prices.Set(
		marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), f.clock.Now().Add(-5*time.Minute-time.Nanosecond),
	)
	q := adapterQueries(f, prices)
	_, err := q.PotValue(t.Context(), cabal)
	wantCode(t, err, errs.CodePriceUnavailable)
}

func TestPotValue_ExactPriceBoundaryFresh(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), f.clock.Now().Add(-5*time.Minute))
	got, err := adapterQueries(f, prices).PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(120_000_000) {
		t.Fatalf("PotValue() = %v, %v", got, err)
	}
}

func TestPotValue_MissingPriceUnavailable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	q := adapterQueries(f, &marketfake.PricesFake{})
	_, err := q.PotValue(t.Context(), cabal)
	wantCode(t, err, errs.CodePriceUnavailable)
}

func TestPotValue_EmptyCabalZero(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	got, err := newQueries(f).PotValue(t.Context(), f.cabal(t))
	if err != nil || !got.IsZero() {
		t.Fatalf("PotValue() = %v, %v", got, err)
	}
}

func TestPotValue_ChecksHoldingsBeforePrices(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	reader := app.PriceReader(func(context.Context) (map[uuid.UUID]app.Price, error) {
		return nil, errs.New(errs.CodeDBUnavailable, "test")
	})
	q := adapterQueriesWithReader(f, reader)
	if got, err := q.PotValue(t.Context(), f.cabal(t)); err != nil || !got.IsZero() {
		t.Fatalf("empty PotValue() = %v, %v", got, err)
	}
	usdcOnly := seedUSDCOnly(t, f)
	got, err := q.PotValue(t.Context(), usdcOnly)
	if err != nil || got != money.MicrosFromUint64(100_000_000) {
		t.Fatalf("USDC-only PotValue() = %v, %v", got, err)
	}
}

func TestPotValue_QueryCount(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	prices := &marketfake.PricesFake{}
	for _, asset := range marketfake.Fixtures() {
		prices.Set(asset.ID, money.MicrosFromUint64(30_000_000), f.clock.Now())
	}
	reads := 0
	reader := app.PriceReader(func(ctx context.Context) (map[uuid.UUID]app.Price, error) {
		reads++
		latest, err := prices.LatestPrices(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[uuid.UUID]app.Price, len(latest))
		for id, price := range latest {
			out[id.UUID()] = app.Price{Micros: price.Micros, ObservedAt: price.ObservedAt}
		}
		return out, nil
	})
	q := adapterQueriesWithReader(f, reader)
	assertPotValueQueries(t, q, cabal, &reads)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, $2, 1, 1, $3), ($1, $4, 1, 1, $3)`
	tsla, jpst := marketfake.TSLAx().Mint.Address(), marketfake.JPSTx().Mint.Address()
	_, err := f.pool.Exec(
		t.Context(), insert, cabal.UUID(), tsla, f.clock.Now(), jpst,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertPotValueQueries(t, q, cabal, &reads)
}

func assertPotValueQueries(t *testing.T, q *adapters.Queries, cabal ids.CabalID, reads *int) {
	t.Helper()
	*reads = 0
	testkit.AssertQueries(t, "PotValue one and three holdings", func() {
		if _, err := q.PotValue(t.Context(), cabal); err != nil {
			t.Fatal(err)
		}
	})
	if *reads != 1 {
		t.Fatalf("price reads = %d, want 1", *reads)
	}
}

func TestStakeAndStakesOf(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, other, redeemed, cabal := f.user(t), f.user(t), f.user(t), f.cabal(t)
	mustFundHistory(t, f, user, cabal, 100_000_000, 100)
	mustFundHistory(t, f, other, cabal, 50_000_000, 50)
	mustFundHistory(t, f, redeemed, cabal, 25_000_000, 25)
	mustCashOutHistory(t, f, redeemed, cabal, 25_000_000, 25)
	q := newQueries(f)
	stake, err := q.Stake(t.Context(), cabal, user)
	if err != nil || stake.ValueMicros != money.MicrosFromUint64(100_000_000) || stake.TotalShares.Uint64() != 150 {
		t.Fatalf("Stake() = %#v, %v", stake, err)
	}
	stakes, err := q.StakesOf(t.Context(), user)
	if err != nil || len(stakes) != 1 || stakes[0] != stake {
		t.Fatalf("StakesOf(user) = %#v, %v", stakes, err)
	}
	otherStakes, err := q.StakesOf(t.Context(), other)
	if err != nil || len(otherStakes) != 1 || otherStakes[0].ShareUnits.Uint64() != 50 {
		t.Fatalf("StakesOf(other) = %#v, %v", otherStakes, err)
	}
	redeemedStakes, err := q.StakesOf(t.Context(), redeemed)
	if err != nil || len(redeemedStakes) != 0 {
		t.Fatalf("StakesOf(redeemed) = %#v, %v", redeemedStakes, err)
	}
}

func TestStake_NoHoldingsReturnsZeroValue(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	mustFundHistory(t, f, user, cabal, 100_000_000, 100)
	if _, err := f.pool.Exec(t.Context(), "DELETE FROM cabal_positions WHERE cabal_id = $1", cabal.UUID()); err != nil {
		t.Fatal(err)
	}
	stake, err := newQueries(f).Stake(t.Context(), cabal, user)
	if err != nil || !stake.ValueMicros.IsZero() || stake.ShareUnits.Uint64() != 100 {
		t.Fatalf("Stake() = %#v, %v", stake, err)
	}
}

func TestQueries_EmptyMemberReadsZero(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	q := newQueries(f)
	cabal, user := f.cabal(t), f.user(t)
	units, err := q.ShareUnits(t.Context(), cabal, user)
	if err != nil || !units.IsZero() {
		t.Fatalf("ShareUnits() = %v, %v", units, err)
	}
	stake, err := q.Stake(t.Context(), cabal, user)
	if err != nil || stake.CabalID != cabal || stake.UserID != user || !stake.ShareUnits.IsZero() {
		t.Fatalf("Stake() = %#v, %v", stake, err)
	}
	stakes, err := q.StakesOf(t.Context(), user)
	if err != nil || len(stakes) != 0 {
		t.Fatalf("StakesOf() = %#v, %v", stakes, err)
	}
}

func TestPositions_InvalidStoredMintFailsDecode(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, 'not-a-mint', 1, 0, $2)`
	_, err := f.pool.Exec(t.Context(), insert, cabal.UUID(), f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = newQueries(f).Positions(t.Context(), cabal)
	wantCode(t, err, errs.CodeDecodeFailed)
}

func TestQueries_CanceledReadsFail(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	q := newQueries(f)
	cabal, user := f.cabal(t), f.user(t)
	for _, call := range []func() error{
		func() error { _, err := q.Positions(ctx, cabal); return err },
		func() error { _, err := q.PotValue(ctx, cabal); return err },
		func() error { _, err := q.TotalShares(ctx, cabal); return err },
		func() error { _, err := q.ShareUnits(ctx, cabal, user); return err },
		func() error { _, err := q.Stake(ctx, cabal, user); return err },
		func() error { _, err := q.StakesOf(ctx, user); return err },
	} {
		if call() == nil {
			t.Fatal("error = nil")
		}
	}
}

func TestPositionsAndStakesAt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, other, cabal := f.user(t), f.user(t), f.cabal(t)
	q := adapterQueriesWithReader(f, app.PriceReader(func(context.Context) (map[uuid.UUID]app.Price, error) {
		return map[uuid.UUID]app.Price{}, nil
	}))
	before := f.clock.Now().Add(-time.Nanosecond)
	if got, err := q.CabalPositionsAt(t.Context(), before); err != nil || len(got) != 0 {
		t.Fatalf("CabalPositionsAt(before) = %#v, %v", got, err)
	}
	mustFundHistory(t, f, user, cabal, 100_000_000, 100)
	snapshots := make([]historySnapshot, 0, 9)
	snapshots = append(snapshots, captureHistory(t, q, cabal, user, f.clock.Now(), []treasury.MemberStake{
		wantMemberStake(user, cabal, 100, 100_000_000),
	}))
	f.clock.Advance(time.Second)
	mustCashOutHistory(t, f, user, cabal, 100_000_000, 100)
	snapshots = append(snapshots, captureHistory(t, q, cabal, user, f.clock.Now(), []treasury.MemberStake{
		wantMemberStake(user, cabal, 0, 0),
	}))
	f.clock.Advance(time.Second)
	mustFundHistory(t, f, user, cabal, 100_000_000, 100)
	memberStakes := make([]treasury.MemberStake, 0, 2)
	memberStakes = append(memberStakes, wantMemberStake(user, cabal, 100, 100_000_000))
	snapshots = append(snapshots, captureHistory(t, q, cabal, user, f.clock.Now(), memberStakes))
	f.clock.Advance(time.Second)
	mustPostHistorySwap(t, f, cabal, 40_000_000, 200_000_000)
	snapshots = append(snapshots, captureHistory(t, q, cabal, user, f.clock.Now(), memberStakes))
	f.clock.Advance(time.Second)
	mustPostHistorySwap(t, f, cabal, -10_000_000, -50_000_000)
	snapshots = append(snapshots, captureHistory(t, q, cabal, user, f.clock.Now(), memberStakes))
	f.clock.Advance(time.Second)
	mustPostHistoryAssetSwap(t, f, cabal, domain.Asset(marketfake.TSLAx().Mint.Address()), 20_000_000, 25_000_000)
	snapshots = append(snapshots, captureHistory(t, q, cabal, user, f.clock.Now(), memberStakes))
	f.clock.Advance(time.Second)
	mustPostHistorySwap(t, f, cabal, -5_000_000, -25_000_000)
	snapshots = append(snapshots, captureHistory(t, q, cabal, user, f.clock.Now(), memberStakes))
	f.clock.Advance(time.Second)
	postNoCabalDeposit(t, f, f.user(t))
	snapshots = append(snapshots, captureHistory(t, q, cabal, user, f.clock.Now(), memberStakes))
	f.clock.Advance(time.Second)
	mustFundHistory(t, f, other, cabal, 50_000_000, 50)
	memberStakes = append(memberStakes, wantMemberStake(other, cabal, 50, 50_000_000))
	snapshots = append(snapshots, captureHistory(t, q, cabal, user, f.clock.Now(), memberStakes))
	for _, snapshot := range snapshots {
		assertHistoricalSnapshot(t, q, snapshot)
	}
}

func postNoCabalDeposit(t *testing.T, f fixture, user ids.UserID) {
	t.Helper()
	txn, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: f.ids.NewV7(), UserID: user, Kind: domain.UserDeposit, Status: domain.TxnSettled,
	}, []domain.UserEntry{
		{Account: domain.UserWallet, Asset: usdc, Amount: amount(1)},
		{Account: domain.UserExternal, Asset: usdc, Amount: amount(-1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		return f.ledger.PostUserTxn(ctx, tx, txn)
	}); err != nil {
		t.Fatal(err)
	}
}

func mustPostHistorySwap(t *testing.T, f fixture, cabal ids.CabalID, micros, units int64) {
	t.Helper()
	txn, err := f.swap(cabal, micros, units)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(txn); err != nil {
		t.Fatal(err)
	}
}

func mustPostHistoryAssetSwap(
	t *testing.T, f fixture, cabal ids.CabalID, asset domain.Asset, micros, units int64,
) {
	t.Helper()
	txn, err := swapForAsset(f, cabal, asset, micros, units)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(txn); err != nil {
		t.Fatal(err)
	}
}

func mustFundHistory(t *testing.T, f fixture, user ids.UserID, cabal ids.CabalID, micros, shares int64) {
	t.Helper()
	u, c, err := f.fund(user, cabal, micros, shares, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
}

func mustCashOutHistory(t *testing.T, f fixture, user ids.UserID, cabal ids.CabalID, micros, shares int64) {
	t.Helper()
	u, c, err := f.cashOut(user, cabal, micros, shares, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
}

type historySnapshot struct {
	at        time.Time
	cabal     ids.CabalID
	user      ids.UserID
	positions []treasury.CabalPositions
	units     money.SharesUnits
	members   []treasury.MemberStake
}

func captureHistory(
	t *testing.T,
	q *adapters.Queries,
	cabal ids.CabalID,
	user ids.UserID,
	at time.Time,
	wantMembers []treasury.MemberStake,
) historySnapshot {
	t.Helper()
	livePositions, err := q.Positions(t.Context(), cabal)
	if err != nil {
		t.Fatal(err)
	}
	liveShares, err := q.TotalShares(t.Context(), cabal)
	if err != nil {
		t.Fatal(err)
	}
	wantPositions := cabalPositionsFromLive(cabal, livePositions, liveShares)
	positions, err := q.CabalPositionsAt(t.Context(), at)
	if err != nil {
		t.Fatal(err)
	}
	if !sameCabalPositions(positions, wantPositions) {
		t.Fatalf("CabalPositionsAt(%v) = %#v, want live %#v", at, positions, wantPositions)
	}
	units, err := q.ShareUnitsAt(t.Context(), cabal, user, at)
	if err != nil {
		t.Fatal(err)
	}
	wantUnits := wantMembers[0].ShareUnits
	if units != wantUnits {
		t.Fatalf("ShareUnitsAt(%v) = %v, want %v", at, units, wantUnits)
	}
	members, err := q.MemberStakesAt(t.Context(), at)
	if err != nil {
		t.Fatal(err)
	}
	if !sameMemberStakes(members, wantMembers) {
		t.Fatalf("MemberStakesAt(%v) = %#v, want %#v", at, members, wantMembers)
	}
	return historySnapshot{
		at: at, cabal: cabal, user: user, positions: wantPositions, units: wantUnits, members: wantMembers,
	}
}

func assertHistoricalSnapshot(t *testing.T, q *adapters.Queries, want historySnapshot) {
	t.Helper()
	positions, err := q.CabalPositionsAt(t.Context(), want.at)
	if err != nil {
		t.Fatal(err)
	}
	units, err := q.ShareUnitsAt(t.Context(), want.cabal, want.user, want.at)
	if err != nil {
		t.Fatal(err)
	}
	members, err := q.MemberStakesAt(t.Context(), want.at)
	if err != nil {
		t.Fatal(err)
	}
	matches := sameCabalPositions(positions, want.positions) && units == want.units &&
		sameMemberStakes(members, want.members)
	if !matches {
		t.Fatalf("history at %v does not match expected snapshot", want.at)
	}
}

func cabalPositionsFromLive(
	cabal ids.CabalID, holdings []treasury.Position, totalShares money.SharesUnits,
) []treasury.CabalPositions {
	if len(holdings) == 0 {
		return nil
	}
	return []treasury.CabalPositions{{CabalID: cabal, Holdings: holdings, TotalShares: totalShares}}
}

func wantMemberStake(user ids.UserID, cabal ids.CabalID, shares uint64, net int64) treasury.MemberStake {
	return treasury.MemberStake{
		UserID: user, CabalID: cabal, ShareUnits: money.SharesUnitsFromUint64(shares),
		NetContributedMicros: money.SignedMicrosFromInt64(net),
	}
}

func sameCabalPositions(got, want []treasury.CabalPositions) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].CabalID != want[i].CabalID || got[i].TotalShares != want[i].TotalShares ||
			!samePositionSlices(got[i].Holdings, want[i].Holdings) {
			return false
		}
	}
	return true
}

func samePositionSlices(got, want []treasury.Position) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameMemberStakes(got, want []treasury.MemberStake) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func swapForAsset(
	f fixture, cabal ids.CabalID, asset domain.Asset, micros, units int64,
) (domain.CabalTxn, error) {
	return domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: f.ids.NewV7(), CabalID: cabal, Kind: domain.CabalSwap, Status: domain.TxnSettled, SwapID: f.ids.NewV7(),
		TxSignature: "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZrRFMV6UjKdiSZkQUW",
	}, []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: usdc, Amount: amount(-micros)},
		{Account: domain.CabalVenue, Asset: usdc, Amount: amount(micros)},
		{Account: domain.CabalTreasury, Asset: asset, Amount: amount(units)},
		{Account: domain.CabalVenue, Asset: asset, Amount: amount(-units)},
	})
}

func seedHolding(t *testing.T, f fixture) ids.CabalID {
	t.Helper()
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	swap, err := f.swap(cabal, 40_000_000, 200_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
	return cabal
}

func seedUSDCOnly(t *testing.T, f fixture) ids.CabalID {
	t.Helper()
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	return cabal
}

func newQueries(f fixture) *adapters.Queries {
	return adapterQueries(f, &marketfake.PricesFake{})
}

func adapterQueries(f fixture, prices *marketfake.PricesFake) *adapters.Queries {
	return adapterQueriesWithReader(f, priceReader(prices))
}

func adapterQueriesWithReader(f fixture, reader app.PriceReader) *adapters.Queries {
	resolver := app.MintResolver(func(_ context.Context, mint chain.SolanaAddress) (app.Asset, error) {
		for _, asset := range marketfake.Fixtures() {
			if string(mint) == string(asset.Mint.Address()) {
				return app.Asset{
					ID: asset.ID.UUID(), Symbol: asset.Symbol, DisplayName: asset.DisplayName,
					Decimals: asset.Decimals, ChainChecked: asset.ChainChecked,
				}, nil
			}
		}
		return app.Asset{}, errs.New(errs.CodeAssetNotFound, "test.AssetByMint")
	})
	return adapters.NewQueries(f.pool, resolver, reader, f.clock, usdcMint)
}

func priceReader(prices *marketfake.PricesFake) app.PriceReader {
	return app.PriceReader(func(ctx context.Context) (map[uuid.UUID]app.Price, error) {
		latest, err := prices.LatestPrices(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[uuid.UUID]app.Price, len(latest))
		for id, price := range latest {
			out[id.UUID()] = app.Price{Micros: price.Micros, ObservedAt: price.ObservedAt}
		}
		return out, nil
	})
}
