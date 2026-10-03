package adapters

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestPositionAndAmountDecodingFailures(t *testing.T) {
	t.Parallel()
	queries := &Queries{usdc: usdcMint}
	assets := map[chain.SolanaAddress]app.Asset{}
	if _, err := queries.position(t.Context(), usdcMint, "bad", "1", assets); err == nil {
		t.Fatal("position with bad units error = nil")
	}
	if _, err := queries.position(t.Context(), usdcMint, "1", "bad", assets); err == nil {
		t.Fatal("position with bad cost error = nil")
	}
	if _, err := micros("bad"); err == nil {
		t.Fatal("micros error = nil")
	}
	if _, err := shares("bad"); err == nil {
		t.Fatal("shares error = nil")
	}
	for _, fields := range [][4]string{
		{"bad", "1", "1", "1"},
		{"1", "bad", "1", "1"},
		{"1", "1", "bad", "1"},
		{"1", "1", "1", "bad"},
	} {
		if _, err := stakeFields(ids.CabalID{}, ids.UserID{}, fields[0], fields[1], fields[2], fields[3]); err == nil {
			t.Fatal("stakeFields error = nil")
		}
	}
	queries.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{}, errs.New(errs.CodeAssetNotFound, "test")
	})
	if _, err := queries.position(
		t.Context(), "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", "1", "1", assets,
	); err == nil {
		t.Fatal("position with unavailable asset error = nil")
	}
	if _, err := power10(20); err == nil {
		t.Fatal("power10 overflow error = nil")
	}
	queries.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{Decimals: 20, ChainChecked: true}, nil
	})
	queries.clock = testkit.NewClock(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	if _, err := queries.positionValue(t.Context(), port.Position{
		Mint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Units: money.NewBaseUnits(1, 20),
	}, map[uuid.UUID]app.Price{uuid.Nil: {Micros: money.MicrosFromUint64(1), ObservedAt: queries.clock.Now()}}, assets); err == nil {
		t.Fatal("positionValue scale error = nil")
	}
}

func TestStakeReadBranches(t *testing.T) {
	t.Parallel()
	cabal, user := ids.CabalID{}, ids.UserID{}
	q := newTestQueries(stakeStore{}, nil)
	if got, err := q.Stake(t.Context(), cabal, user); err != nil || got.CabalID != cabal || got.UserID != user {
		t.Fatalf("empty Stake() = %#v, %v", got, err)
	}
	zero := stakeStore{stake: []sqlc.CabalStakeSnapshotRow{{
		ShareUnits: "0", ContributedMicros: "1", WithdrawnMicros: "1", TotalShares: "1",
	}}}
	q = newTestQueries(zero, nil)
	if got, err := q.Stake(t.Context(), cabal, user); err != nil || !got.ValueMicros.IsZero() {
		t.Fatalf("zero Stake() = %#v, %v", got, err)
	}
	badStake := stakeStore{stake: []sqlc.CabalStakeSnapshotRow{{
		ShareUnits: "bad", ContributedMicros: "1", WithdrawnMicros: "1", TotalShares: "1",
	}}}
	if _, err := newTestQueries(badStake, nil).Stake(t.Context(), cabal, user); err == nil {
		t.Fatal("Stake malformed fields error = nil")
	}
	badAsset := stakeStore{stake: []sqlc.CabalStakeSnapshotRow{{
		ShareUnits: "1", ContributedMicros: "1", WithdrawnMicros: "1", TotalShares: "1",
		Asset: pgtype.Text{String: "not-a-mint", Valid: true}, Units: "1", CostBasisMicros: "1",
	}}}
	if _, err := newTestQueries(badAsset, nil).Stake(t.Context(), cabal, user); err == nil {
		t.Fatal("Stake malformed position error = nil")
	}
	priced := stakeStore{stake: []sqlc.CabalStakeSnapshotRow{{
		ShareUnits: "1", ContributedMicros: "1", WithdrawnMicros: "1", TotalShares: "1",
		Asset: pgtype.Text{String: usdcMint, Valid: true}, Units: "1", CostBasisMicros: "1",
	}}}
	got, err := newTestQueries(priced, nil).Stake(t.Context(), cabal, user)
	if err != nil || got.ValueMicros != money.MicrosFromUint64(1) {
		t.Fatalf("Stake USDC-only = %#v, %v", got, err)
	}
	nonUSDC := priced
	nonUSDC.stake[0].Asset = pgtype.Text{String: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Valid: true}
	noPrices := app.PriceReader(func(context.Context) (map[uuid.UUID]app.Price, error) {
		return nil, errs.New(errs.CodeDBUnavailable, "test")
	})
	pricedQueries := newTestQueries(nonUSDC, noPrices)
	pricedQueries.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{ChainChecked: true}, nil
	})
	if _, err := pricedQueries.Stake(t.Context(), cabal, user); err == nil {
		t.Fatal("Stake unavailable prices error = nil")
	}
	failedStore := stakeStore{stakeErr: errs.New(errs.CodeDBUnavailable, "test")}
	if _, err := newTestQueries(failedStore, nil).Stake(t.Context(), cabal, user); err == nil {
		t.Fatal("Stake query error = nil")
	}
}

func TestMemberReadBranches(t *testing.T) {
	t.Parallel()
	cabal, user := ids.CabalID{}, ids.UserID{}
	positionFailure := stakeStore{positionErr: errs.New(errs.CodeDBUnavailable, "test")}
	if _, err := newTestQueries(positionFailure, nil).ShareUnits(t.Context(), cabal, user); err == nil {
		t.Fatal("ShareUnits query error = nil")
	}
	failedStore := stakeStore{stakesErr: errs.New(errs.CodeDBUnavailable, "test")}
	if _, err := newTestQueries(failedStore, nil).StakesOf(t.Context(), user); err == nil {
		t.Fatal("StakesOf query error = nil")
	}
	childFailure := stakeStore{
		stakes:   []sqlc.UserStakesRow{{CabalID: cabal.UUID(), UserID: user.UUID()}},
		stakeErr: errs.New(errs.CodeDBUnavailable, "test"),
	}
	if _, err := newTestQueries(childFailure, nil).StakesOf(t.Context(), user); err == nil {
		t.Fatal("StakesOf child error = nil")
	}
	store := stakeStore{position: sqlc.CabalUserPositionRow{ShareUnits: "2"}}
	got, err := newTestQueries(store, nil).ShareUnits(t.Context(), cabal, user)
	if err != nil || got != money.SharesUnitsFromUint64(2) {
		t.Fatalf("ShareUnits() = %v, %v", got, err)
	}
	zeroStake := []sqlc.CabalStakeSnapshotRow{{
		ShareUnits: "0", ContributedMicros: "1", WithdrawnMicros: "1", TotalShares: "1",
	}}
	if got, err := newTestQueries(stakeStore{
		stake: zeroStake, stakes: []sqlc.UserStakesRow{{CabalID: cabal.UUID(), UserID: user.UUID()}},
	}, nil).StakesOf(t.Context(), user); err != nil || len(got) != 1 {
		t.Fatalf("StakesOf() = %#v, %v", got, err)
	}
}

func TestQueryValueAndShareFailures(t *testing.T) {
	t.Parallel()
	prices := app.PriceReader(func(context.Context) (map[uuid.UUID]app.Price, error) {
		return map[uuid.UUID]app.Price{}, nil
	})
	q := newTestQueries(stakeStore{}, prices)
	assets := map[chain.SolanaAddress]app.Asset{}
	failingPositions := stakeStore{positionsErr: errs.New(errs.CodeDBUnavailable, "test")}
	if _, err := newTestQueries(failingPositions, prices).PotValue(t.Context(), ids.CabalID{}); err == nil {
		t.Fatal("PotValue query error = nil")
	}
	badPosition := stakeStore{positions: []sqlc.CabalPositionsRow{{Asset: "bad", Units: "1", CostBasisMicros: "0"}}}
	if _, err := newTestQueries(badPosition, prices).PotValue(t.Context(), ids.CabalID{}); err == nil {
		t.Fatal("PotValue bad position error = nil")
	}
	positions := []port.Position{
		{Mint: chain.SolanaAddress(usdcMint), Units: money.NewBaseUnits(math.MaxUint64, usdcDecimals)},
		{Mint: chain.SolanaAddress(usdcMint), Units: money.NewBaseUnits(1, usdcDecimals)},
	}
	if _, err := q.potValuePositions(t.Context(), positions, assets); err == nil {
		t.Fatal("potValuePositions overflow error = nil")
	}
	assetMint := chain.SolanaAddress("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	q.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{}, errs.New(errs.CodeAssetNotFound, "test")
	})
	if _, err := q.positionValue(t.Context(), port.Position{Mint: assetMint}, nil, assets); err == nil {
		t.Fatal("positionValue unavailable asset error = nil")
	}
	q.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{Decimals: 0, ChainChecked: true}, nil
	})
	q.clock = testkit.NewClock(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	priceMap := map[uuid.UUID]app.Price{
		uuid.Nil: {
			Micros:     money.MicrosFromUint64(math.MaxUint64),
			ObservedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC),
		},
	}
	if _, err := q.positionValue(t.Context(), port.Position{
		Mint: assetMint, Units: money.NewBaseUnits(math.MaxUint64, 0),
	}, priceMap, assets); err == nil {
		t.Fatal("positionValue multiplication overflow error = nil")
	}
	q.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{}, nil
	})
	assets = map[chain.SolanaAddress]app.Asset{}
	if _, err := q.position(t.Context(), string(assetMint), "1", "1", assets); err == nil {
		t.Fatal("position unchecked asset error = nil")
	}
	if _, err := q.positionValue(t.Context(), port.Position{Mint: assetMint}, priceMap, assets); err == nil {
		t.Fatal("positionValue unchecked asset error = nil")
	}
	if _, err := newTestQueries(stakeStore{total: "bad"}, prices).TotalShares(t.Context(), ids.CabalID{}); err == nil {
		t.Fatal("TotalShares malformed value error = nil")
	}
}

func TestPositionValueUsesEffectiveUIMultiplier(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	assetMint := chain.SolanaAddress("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	asset := app.Asset{
		ID: uuid.Nil, Decimals: 0, ChainChecked: true, UIMultiplierNum: 5, UIMultiplierDen: 1,
		NextUIMultiplierNum: 2, NextUIMultiplierDen: 1, NextUIMultiplierAt: at,
	}
	q := newTestQueries(stakeStore{}, nil)
	assets := map[chain.SolanaAddress]app.Asset{}
	q.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) { return asset, nil })
	price := map[uuid.UUID]app.Price{uuid.Nil: {Micros: money.MicrosFromUint64(3), ObservedAt: at}}
	position := port.Position{Mint: assetMint, Units: money.NewBaseUnits(4, 0)}
	q.clock = testkit.NewClock(at.Add(-time.Nanosecond))
	got, err := q.positionValue(t.Context(), position, price, assets)
	if err != nil || got != money.MicrosFromUint64(60) {
		t.Fatalf("positionValue before step = %v, %v", got, err)
	}
	q.clock = testkit.NewClock(at)
	assets = map[chain.SolanaAddress]app.Asset{}
	got, err = q.positionValue(t.Context(), position, price, assets)
	if err != nil || got != money.MicrosFromUint64(24) {
		t.Fatalf("positionValue at step = %v, %v", got, err)
	}
	asset.UIMultiplierNum = 0
	asset.NextUIMultiplierDen = 0
	assets = map[chain.SolanaAddress]app.Asset{}
	if _, err := q.positionValue(t.Context(), position, price, assets); err == nil {
		t.Fatal("positionValue invalid multiplier error = nil")
	}
	asset.UIMultiplierNum, asset.UIMultiplierDen = 2, 1
	assets = map[chain.SolanaAddress]app.Asset{}
	overflowPrice := map[uuid.UUID]app.Price{
		uuid.Nil: {Micros: money.MicrosFromUint64(1), ObservedAt: at},
	}
	if _, err := q.positionValue(t.Context(), port.Position{
		Mint: assetMint, Units: money.NewBaseUnits(math.MaxUint64, 0),
	}, overflowPrice, assets); err == nil {
		t.Fatal("positionValue multiplier overflow error = nil")
	}
	fractionalPrice := map[uuid.UUID]app.Price{
		uuid.Nil: {Micros: money.MicrosFromUint64(30_000_000), ObservedAt: at},
	}
	assets = map[chain.SolanaAddress]app.Asset{}
	if got, err := q.positionValue(t.Context(), port.Position{
		Mint: assetMint, Units: money.NewBaseUnits(5, 8),
	}, fractionalPrice, assets); err != nil || got != money.MicrosFromUint64(3) {
		t.Fatalf("positionValue fractional multiplier = %v, %v", got, err)
	}
}

func TestHistoricalReadFailures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeDBUnavailable, "test")
	q := &Queries{history: historicalStore{snapshotsErr: boom}, usdc: usdcMint}
	q.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{}, errs.New(errs.CodeAssetNotFound, "test")
	})
	q.history = historicalStore{shareErr: boom}
	if _, err := q.ShareUnitsAt(t.Context(), ids.CabalID{}, ids.UserID{}, time.Time{}); err == nil {
		t.Fatal("ShareUnitsAt query error = nil")
	}
	q.history = historicalStore{snapshotsErr: boom}
	if _, err := q.CabalPositionsAt(t.Context(), time.Time{}); err == nil {
		t.Fatal("CabalPositionsAt snapshot error = nil")
	}
	q.history = historicalStore{snapshots: []sqlc.CabalPositionSnapshotsAtRow{{
		Asset: usdcMint, Units: "1", CostBasisMicros: "1", ShareUnits: "bad",
	}}}
	if _, err := q.CabalPositionsAt(t.Context(), time.Time{}); err == nil {
		t.Fatal("CabalPositionsAt shares error = nil")
	}
	q.history = historicalStore{snapshots: []sqlc.CabalPositionSnapshotsAtRow{{
		Asset: "bad", Units: "1", CostBasisMicros: "0", ShareUnits: "1",
	}}}
	if _, err := q.CabalPositionsAt(t.Context(), time.Time{}); err == nil {
		t.Fatal("CabalPositionsAt position error = nil")
	}
	q.history = historicalStore{membersErr: boom}
	if _, err := q.MemberStakesAt(t.Context(), time.Time{}); err == nil {
		t.Fatal("MemberStakesAt query error = nil")
	}
	q.history = historicalStore{members: []sqlc.MemberStakesAtRow{{ShareUnits: "bad"}}}
	if _, err := q.MemberStakesAt(t.Context(), time.Time{}); err == nil {
		t.Fatal("MemberStakesAt shares error = nil")
	}
	q.history = historicalStore{members: []sqlc.MemberStakesAtRow{{ShareUnits: "1", NetContributedMicros: "bad"}}}
	if _, err := q.MemberStakesAt(t.Context(), time.Time{}); err == nil {
		t.Fatal("MemberStakesAt net error = nil")
	}
}

func TestHistoricalReadsSuccess(t *testing.T) {
	t.Parallel()
	q := &Queries{usdc: usdcMint}
	q.history = historicalStore{snapshots: []sqlc.CabalPositionSnapshotsAtRow{{
		Asset: usdcMint, Units: "1", CostBasisMicros: "1", ShareUnits: "1",
	}}}
	got, err := q.CabalPositionsAt(t.Context(), time.Time{})
	if err != nil || len(got) != 1 || len(got[0].Holdings) != 1 {
		t.Fatalf("CabalPositionsAt() = %#v, %v", got, err)
	}
	q.history = historicalStore{members: []sqlc.MemberStakesAtRow{{ShareUnits: "1", NetContributedMicros: "2"}}}
	members, err := q.MemberStakesAt(t.Context(), time.Time{})
	if err != nil || len(members) != 1 || members[0].NetContributedMicros.Int64() != 2 {
		t.Fatalf("MemberStakesAt() = %#v, %v", members, err)
	}
}

func newTestQueries(store queryStore, prices app.PriceReader) *Queries {
	return &Queries{
		q:      store,
		prices: prices,
		clock:  testkit.NewClock(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)),
		usdc:   usdcMint,
	}
}

type stakeStore struct {
	positions    []sqlc.CabalPositionsRow
	positionsErr error
	stake        []sqlc.CabalStakeSnapshotRow
	stakes       []sqlc.UserStakesRow
	position     sqlc.CabalUserPositionRow
	positionErr  error
	total        string
	stakeErr     error
	stakesErr    error
}

func (s stakeStore) CabalPositions(context.Context, uuid.UUID) ([]sqlc.CabalPositionsRow, error) {
	return s.positions, s.positionsErr
}

func (s stakeStore) CabalTotalShares(context.Context, uuid.UUID) (string, error) { return s.total, nil }

func (s stakeStore) CabalUserPosition(
	context.Context, sqlc.CabalUserPositionParams,
) (sqlc.CabalUserPositionRow, error) {
	return s.position, s.positionErr
}

func (s stakeStore) CabalStakeSnapshot(
	context.Context, sqlc.CabalStakeSnapshotParams,
) ([]sqlc.CabalStakeSnapshotRow, error) {
	return s.stake, s.stakeErr
}

func (s stakeStore) UserStakes(context.Context, uuid.UUID) ([]sqlc.UserStakesRow, error) {
	return s.stakes, s.stakesErr
}

func (stakeStore) CabalUserShareUnitsAt(context.Context, sqlc.CabalUserShareUnitsAtParams) (string, error) {
	return "0", nil
}

func (stakeStore) CabalPositionSnapshotsAt(
	context.Context, sqlc.CabalPositionSnapshotsAtParams,
) ([]sqlc.CabalPositionSnapshotsAtRow, error) {
	return nil, nil
}

func (stakeStore) MemberStakesAt(context.Context, time.Time) ([]sqlc.MemberStakesAtRow, error) {
	return nil, nil
}

type historicalStore struct {
	snapshots    []sqlc.CabalPositionSnapshotsAtRow
	members      []sqlc.MemberStakesAtRow
	snapshotsErr error
	membersErr   error
	shareErr     error
}

func (h historicalStore) CabalUserShareUnitsAt(context.Context, sqlc.CabalUserShareUnitsAtParams) (string, error) {
	return "0", h.shareErr
}

func (h historicalStore) CabalPositionSnapshotsAt(
	context.Context, sqlc.CabalPositionSnapshotsAtParams,
) ([]sqlc.CabalPositionSnapshotsAtRow, error) {
	return h.snapshots, h.snapshotsErr
}

func (h historicalStore) MemberStakesAt(context.Context, time.Time) ([]sqlc.MemberStakesAtRow, error) {
	return h.members, h.membersErr
}
